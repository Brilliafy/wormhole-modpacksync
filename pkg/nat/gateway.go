package nat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PortMappingResult holds the lease info returned by NAT-PMP, PCP, or UPnP.
type PortMappingResult struct {
	Protocol     string // "nat-pmp", "pcp", "upnp"
	ExternalIP   net.IP
	ExternalPort int
	InternalPort int
	Lifetime     time.Duration
}

// ProbeGatewayPortMapping attempts in parallel: NAT-PMP (RFC 6886), PCP (RFC 6887), and UPnP IGD.
// If any succeeds, it returns the external port mapping.
func ProbeGatewayPortMapping(ctx context.Context, localPort int) (*PortMappingResult, error) {
	gateways := discoverDefaultGateways()
	if len(gateways) == 0 {
		return nil, fmt.Errorf("no default gateway discovered")
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resChan := make(chan *PortMappingResult, len(gateways)*3)
	var wg sync.WaitGroup

	for _, gw := range gateways {
		// 1. NAT-PMP Probe
		wg.Add(1)
		go func(g net.IP) {
			defer wg.Done()
			if res, err := probeNATPMP(ctx, g, localPort); err == nil && res != nil {
				select {
				case resChan <- res:
				default:
				}
			}
		}(gw)

		// 2. PCP Probe
		wg.Add(1)
		go func(g net.IP) {
			defer wg.Done()
			if res, err := probePCP(ctx, g, localPort); err == nil && res != nil {
				select {
				case resChan <- res:
				default:
				}
			}
		}(gw)
	}

	// 3. UPnP IGD Probe
	wg.Add(1)
	go func() {
		defer wg.Done()
		if res, err := probeUPnP(ctx, localPort); err == nil && res != nil {
			select {
			case resChan <- res:
			default:
			}
		}
	}()

	// Wait in background and close resChan
	go func() {
		wg.Wait()
		close(resChan)
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res, ok := <-resChan:
		if ok && res != nil {
			return res, nil
		}
	}

	return nil, fmt.Errorf("no port mapping protocol supported by gateway")
}

// discoverDefaultGateways locates gateway IPs from /proc/net/route or common local subnets.
func discoverDefaultGateways() []net.IP {
	var results []net.IP
	seen := make(map[string]bool)

	addGW := func(ip net.IP) {
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return
		}
		ip4 := ip.To4()
		if ip4 == nil {
			return
		}
		s := ip4.String()
		if !seen[s] {
			seen[s] = true
			results = append(results, ip4)
		}
	}

	// Read /proc/net/route on Linux
	if f, err := os.Open("/proc/net/route"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 3 && fields[1] == "00000000" { // default route
				gwHex := fields[2]
				if len(gwHex) == 8 {
					if b, err := hex.DecodeString(gwHex); err == nil && len(b) == 4 {
						// /proc/net/route is in little-endian order
						ip := net.IPv4(b[3], b[2], b[1], b[0])
						addGW(ip)
					}
				}
			}
		}
	}

	// Fallback: derive .1 from active local IPv4 interfaces
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok {
					if ip4 := ipnet.IP.To4(); ip4 != nil && ip4.IsPrivate() {
						addGW(net.IPv4(ip4[0], ip4[1], ip4[2], 1))
						addGW(net.IPv4(ip4[0], ip4[1], ip4[2], 254))
					}
				}
			}
		}
	}

	return results
}

// probeNATPMP implements RFC 6886 NAT-PMP port mapping request.
func probeNATPMP(ctx context.Context, gwIP net.IP, localPort int) (*PortMappingResult, error) {
	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:5351", gwIP), 600*time.Millisecond)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	_ = conn.SetDeadline(time.Now().Add(600 * time.Millisecond))

	// Step 1: Request External IP (Version 0, Opcode 0)
	if _, err := conn.Write([]byte{0, 0}); err != nil {
		return nil, err
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil || n < 12 || buf[0] != 0 || buf[1] != 128 || binary.BigEndian.Uint16(buf[2:4]) != 0 {
		return nil, fmt.Errorf("nat-pmp external ip failed")
	}
	extIP := net.IPv4(buf[8], buf[9], buf[10], buf[11])

	// Step 2: Request UDP Mapping (Version 0, Opcode 1)
	req := make([]byte, 12)
	req[0] = 0 // Vers
	req[1] = 1 // Opcode: UDP mapping
	// bytes 2-3 reserved
	binary.BigEndian.PutUint16(req[4:6], uint16(localPort)) // Internal Port
	binary.BigEndian.PutUint16(req[6:8], uint16(localPort)) // Suggested External Port
	binary.BigEndian.PutUint32(req[8:12], 3600)             // 1 hour lifetime

	_ = conn.SetDeadline(time.Now().Add(600 * time.Millisecond))
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}

	n, err = conn.Read(buf)
	if err != nil || n < 16 || buf[0] != 0 || buf[1] != 129 {
		return nil, fmt.Errorf("nat-pmp port map failed")
	}
	resCode := binary.BigEndian.Uint16(buf[2:4])
	if resCode != 0 {
		return nil, fmt.Errorf("nat-pmp returned error code: %d", resCode)
	}

	mappedPort := int(binary.BigEndian.Uint16(buf[10:12]))
	lifetime := time.Duration(binary.BigEndian.Uint32(buf[12:16])) * time.Second

	return &PortMappingResult{
		Protocol:     "nat-pmp",
		ExternalIP:   extIP,
		ExternalPort: mappedPort,
		InternalPort: localPort,
		Lifetime:     lifetime,
	}, nil
}

// probePCP implements RFC 6887 Port Control Protocol (Version 2) mapping request.
func probePCP(ctx context.Context, gwIP net.IP, localPort int) (*PortMappingResult, error) {
	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:5351", gwIP), 600*time.Millisecond)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	_ = conn.SetDeadline(time.Now().Add(600 * time.Millisecond))

	// PCP Request header (24 bytes) + MAP opcode (36 bytes) = 60 bytes
	req := make([]byte, 60)
	req[0] = 2 // Version 2
	req[1] = 1 // Opcode 1 (MAP)
	// Lifetime
	binary.BigEndian.PutUint32(req[4:8], 3600)
	// Internal IP: IPv4-mapped IPv6
	localIP := net.ParseIP("127.0.0.1")
	if laddr, ok := conn.LocalAddr().(*net.UDPAddr); ok && laddr.IP.To4() != nil {
		localIP = laddr.IP.To4()
	}
	copy(req[20:24], localIP.To4())

	// MAP Option Payload
	// Nonce (12 bytes at 24)
	copy(req[24:36], []byte("WORMHOLEPCP1"))
	req[36] = 17                                              // Protocol UDP
	binary.BigEndian.PutUint16(req[40:42], uint16(localPort)) // Internal Port
	binary.BigEndian.PutUint16(req[42:44], uint16(localPort)) // Suggested External Port

	if _, err := conn.Write(req); err != nil {
		return nil, err
	}

	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil || n < 60 || buf[0] != 2 || buf[1] != 129 {
		return nil, fmt.Errorf("pcp map failed")
	}
	resultCode := buf[3]
	if resultCode != 0 {
		return nil, fmt.Errorf("pcp returned code: %d", resultCode)
	}

	extPort := int(binary.BigEndian.Uint16(buf[42:44]))
	extIP := net.IP(buf[44:60])
	if extIP.To4() != nil {
		extIP = extIP.To4()
	}
	lifetime := time.Duration(binary.BigEndian.Uint32(buf[4:8])) * time.Second

	return &PortMappingResult{
		Protocol:     "pcp",
		ExternalIP:   extIP,
		ExternalPort: extPort,
		InternalPort: localPort,
		Lifetime:     lifetime,
	}, nil
}

// probeUPnP sends SSDP M-SEARCH for InternetGatewayDevice and attempts SOAP port mapping.
func probeUPnP(ctx context.Context, localPort int) (*PortMappingResult, error) {
	ssdpReq := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 1\r\n" +
		"ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n"

	addr, err := net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(800 * time.Millisecond))
	if _, err := conn.WriteTo([]byte(ssdpReq), addr); err != nil {
		return nil, err
	}

	buf := make([]byte, 2048)
	n, _, err := conn.ReadFrom(buf)
	if err != nil || n == 0 {
		return nil, fmt.Errorf("no ssdp upnp response")
	}

	respStr := string(buf[:n])
	location := ""
	for _, line := range strings.Split(respStr, "\r\n") {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "location:") {
			location = strings.TrimSpace(line[9:])
			break
		}
	}
	if location == "" {
		return nil, fmt.Errorf("upnp location header not found")
	}

	// Fetch device description XML
	req, err := http.NewRequestWithContext(ctx, "GET", location, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	xmlBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	controlURL, serviceType := parseUPnPControlURL(string(xmlBody), location)
	if controlURL == "" {
		return nil, fmt.Errorf("upnp controlURL not found")
	}

	// Request AddPortMapping via SOAP
	lIP := "127.0.0.1"
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, _ := iface.Addrs()
			for _, a := range addrs {
				if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil && ipnet.IP.IsPrivate() {
					lIP = ipnet.IP.To4().String()
					break
				}
			}
		}
	}

	soapAction := fmt.Sprintf("\"%s#AddPortMapping\"", serviceType)
	soapBody := fmt.Sprintf(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:AddPortMapping xmlns:u="%s">
<NewRemoteHost></NewRemoteHost>
<NewExternalPort>%d</NewExternalPort>
<NewProtocol>UDP</NewProtocol>
<NewInternalPort>%d</NewInternalPort>
<NewInternalClient>%s</NewInternalClient>
<NewEnabled>1</NewEnabled>
<NewPortMappingDescription>Wormhole Sync</NewPortMappingDescription>
<NewLeaseDuration>3600</NewLeaseDuration>
</u:AddPortMapping>
</s:Body>
</s:Envelope>`, serviceType, localPort, localPort, lIP)

	sReq, err := http.NewRequestWithContext(ctx, "POST", controlURL, bytes.NewBufferString(soapBody))
	if err != nil {
		return nil, err
	}
	sReq.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	sReq.Header.Set("SOAPAction", soapAction)

	sResp, err := client.Do(sReq)
	if err != nil {
		return nil, err
	}
	defer sResp.Body.Close()

	if sResp.StatusCode != 200 {
		return nil, fmt.Errorf("upnp soap returned http status %d", sResp.StatusCode)
	}

	return &PortMappingResult{
		Protocol:     "upnp",
		ExternalPort: localPort,
		InternalPort: localPort,
		Lifetime:     3600 * time.Second,
	}, nil
}

func parseUPnPControlURL(descXML, baseURL string) (string, string) {
	services := []string{"WANIPConnection:1", "WANPPPConnection:1", "WANIPConnection:2"}
	for _, s := range services {
		idx := strings.Index(descXML, s)
		if idx != -1 {
			sub := descXML[idx:]
			cIdx := strings.Index(sub, "<controlURL>")
			if cIdx != -1 {
				cEnd := strings.Index(sub[cIdx:], "</controlURL>")
				if cEnd != -1 {
					relURL := sub[cIdx+12 : cIdx+cEnd]
					serviceType := "urn:schemas-upnp-org:service:" + s
					if strings.HasPrefix(relURL, "http://") || strings.HasPrefix(relURL, "https://") {
						return relURL, serviceType
					}
					// Join with baseURL
					if idxSlash := strings.Index(baseURL[8:], "/"); idxSlash != -1 {
						hostRoot := baseURL[:8+idxSlash]
						if strings.HasPrefix(relURL, "/") {
							return hostRoot + relURL, serviceType
						}
						return hostRoot + "/" + relURL, serviceType
					}
					return baseURL + "/" + relURL, serviceType
				}
			}
		}
	}
	return "", ""
}

func parsePort(s string) int {
	p, _ := strconv.Atoi(s)
	return p
}
