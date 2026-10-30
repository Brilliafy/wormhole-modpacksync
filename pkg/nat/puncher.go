package nat

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrHolePunchTimeout = errors.New("hole punching timed out")
	ErrDoubleSymmetric  = errors.New("double symmetric NAT cannot be hole-punched without relay")
)

const (
	punchMagic    = "WHPUNCH1"
	probePacingMs = 16 // ~60 packets/second
	maxSprays     = 512
	socketPoolLen = 256
)

// Candidate holds network endpoint & NAT information exchanged over signaling.
type Candidate struct {
	PublicIP    net.IP      `json:"publicIP"`
	Port1       int         `json:"port1"`
	Port2       int         `json:"port2"`
	MappingType MappingType `json:"mappingType"`
	HasStride   bool        `json:"hasStride"`
	Stride      int         `json:"stride"`
	IPv6        net.IP      `json:"ipv6,omitempty"`
	GatewayPort int         `json:"gatewayPort,omitempty"`
}

type punchMsgType byte

const (
	msgProbe    punchMsgType = 1
	msgProbeAck punchMsgType = 2
	msgUpgrade  punchMsgType = 3
)

func makePunchPacket(code string, msgType punchMsgType) []byte {
	// 8 bytes Magic + 8 bytes Code + 1 byte MsgType + 8 bytes Nonce = 25 bytes
	pkt := make([]byte, 25)
	copy(pkt[0:8], []byte(punchMagic))
	codeBytes := []byte(code)
	if len(codeBytes) > 8 {
		codeBytes = codeBytes[:8]
	}
	copy(pkt[8:16], codeBytes)
	pkt[16] = byte(msgType)
	_, _ = rand.Read(pkt[17:25])
	return pkt
}

func parsePunchPacket(data []byte, expectedCode string) (punchMsgType, bool) {
	if len(data) < 25 {
		return 0, false
	}
	if string(data[0:8]) != punchMagic {
		return 0, false
	}
	codeBytes := []byte(expectedCode)
	if len(codeBytes) > 8 {
		codeBytes = codeBytes[:8]
	}
	for i := 0; i < len(codeBytes); i++ {
		if data[8+i] != codeBytes[i] {
			return 0, false
		}
	}
	return punchMsgType(data[16]), true
}

// HolePunch attempts Stages 1-4 to establish a direct P2P UDP connection.
func HolePunch(ctx context.Context, localCand *Candidate, remoteCand *Candidate, code string) (net.Conn, error) {
	if localCand == nil || remoteCand == nil {
		return nil, errors.New("missing local or remote candidate")
	}

	punchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	// Stage 1: Native IPv6 direct bypass
	if localCand.IPv6 != nil && remoteCand.IPv6 != nil {
		if conn, err := punchIPv6(punchCtx, localCand.IPv6, remoteCand.IPv6, remoteCand.Port1, code); err == nil && conn != nil {
			return conn, nil
		}
	}

	// Stage 2: Gateway Mapped Port
	if remoteCand.GatewayPort > 0 {
		if conn, err := punchDirectMapped(punchCtx, remoteCand.PublicIP, remoteCand.GatewayPort, code); err == nil && conn != nil {
			return conn, nil
		}
	}

	// Stage 3: Cone <-> Cone standard hole punch
	if localCand.MappingType == MappingCone && remoteCand.MappingType == MappingCone {
		if conn, err := punchConeToCone(punchCtx, localCand.Port1, remoteCand.PublicIP, remoteCand.Port1, code); err == nil && conn != nil {
			return conn, nil
		}
	}

	// Stage 4: Symmetric <-> Cone (Birthday Paradox & Stride Sweep)
	if localCand.MappingType == MappingSymmetric && remoteCand.MappingType == MappingCone {
		return runSymmetricPool(punchCtx, remoteCand.PublicIP, remoteCand.Port1, code)
	}

	if localCand.MappingType == MappingCone && remoteCand.MappingType == MappingSymmetric {
		return runConeSpray(punchCtx, localCand.Port1, remoteCand.PublicIP, remoteCand.Port1, remoteCand.Stride, remoteCand.HasStride, code)
	}

	// Double Symmetric NAT -> Mathematical limit for direct hole punching without relay
	return nil, ErrDoubleSymmetric
}

func punchIPv6(ctx context.Context, localIP, remoteIP net.IP, remotePort int, code string) (net.Conn, error) {
	conn, err := net.ListenUDP("udp6", &net.UDPAddr{IP: localIP, Port: 0})
	if err != nil {
		return nil, err
	}
	rAddr := &net.UDPAddr{IP: remoteIP, Port: remotePort}
	return runHandshake(ctx, conn, rAddr, code)
}

func punchDirectMapped(ctx context.Context, remoteIP net.IP, remotePort int, code string) (net.Conn, error) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	rAddr := &net.UDPAddr{IP: remoteIP, Port: remotePort}
	return runHandshake(ctx, conn, rAddr, code)
}

func punchConeToCone(ctx context.Context, localPort int, remoteIP net.IP, remotePort int, code string) (net.Conn, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: localPort})
	if err != nil {
		conn, err = net.ListenUDP("udp4", nil)
		if err != nil {
			return nil, err
		}
	}
	rAddr := &net.UDPAddr{IP: remoteIP, Port: remotePort}
	return runHandshake(ctx, conn, rAddr, code)
}

// runSymmetricPool opens N=256 sockets and sends probes to Cone's fixed (remoteIP, remotePort).
func runSymmetricPool(ctx context.Context, remoteIP net.IP, remotePort int, code string) (net.Conn, error) {
	target := &net.UDPAddr{IP: remoteIP, Port: remotePort}
	sockets := make([]*net.UDPConn, socketPoolLen)
	var opened int

	for i := 0; i < socketPoolLen; i++ {
		c, err := net.ListenUDP("udp4", nil)
		if err == nil {
			sockets[i] = c
			opened++
		}
	}
	if opened == 0 {
		return nil, errors.New("failed to open socket pool")
	}

	closeRemaining := func(winner *net.UDPConn) {
		for _, s := range sockets {
			if s != nil && s != winner {
				_ = s.Close()
			}
		}
	}

	probePkt := makePunchPacket(code, msgProbe)
	ackPkt := makePunchPacket(code, msgProbeAck)

	type winResult struct {
		conn *net.UDPConn
		from *net.UDPAddr
	}
	winChan := make(chan winResult, 1)
	var won int32

	// Listen loop on all sockets
	for _, s := range sockets {
		if s == nil {
			continue
		}
		go func(conn *net.UDPConn) {
			buf := make([]byte, 256)
			for atomic.LoadInt32(&won) == 0 {
				_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				n, from, err := conn.ReadFromUDP(buf)
				if err != nil {
					select {
					case <-ctx.Done():
						return
					default:
						continue
					}
				}
				if mType, ok := parsePunchPacket(buf[:n], code); ok {
					if mType == msgProbe || mType == msgProbeAck {
						if atomic.CompareAndSwapInt32(&won, 0, 1) {
							// Send ACK back
							_, _ = conn.WriteTo(ackPkt, from)
							select {
							case winChan <- winResult{conn: conn, from: from}:
							default:
							}
							return
						}
					}
				}
			}
		}(s)
	}

	// Probe loop: periodically ping Cone peer from all sockets
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			closeRemaining(nil)
			return nil, ErrHolePunchTimeout
		case win := <-winChan:
			closeRemaining(win.conn)
			_ = win.conn.SetDeadline(time.Time{})
			return &PunchedConn{UDPConn: win.conn, rAddr: win.from}, nil
		case <-ticker.C:
			for _, s := range sockets {
				if s != nil {
					_, _ = s.WriteTo(probePkt, target)
				}
			}
		}
	}
}

// runConeSpray sends probes from a single socket: first sweeping stride window, then spraying 512 random ephemeral ports.
func runConeSpray(ctx context.Context, localPort int, remoteIP net.IP, portBase int, stride int, hasStride bool, code string) (net.Conn, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: localPort})
	if err != nil {
		conn, err = net.ListenUDP("udp4", nil)
		if err != nil {
			return nil, err
		}
	}

	probePkt := makePunchPacket(code, msgProbe)
	ackPkt := makePunchPacket(code, msgProbeAck)

	winChan := make(chan *net.UDPAddr, 1)
	var won int32

	// Listener
	go func() {
		buf := make([]byte, 256)
		for atomic.LoadInt32(&won) == 0 {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			if mType, ok := parsePunchPacket(buf[:n], code); ok {
				if mType == msgProbe || mType == msgProbeAck {
					if atomic.CompareAndSwapInt32(&won, 0, 1) {
						_, _ = conn.WriteTo(ackPkt, from)
						select {
						case winChan <- from:
						default:
						}
						return
					}
				}
			}
		}
	}()

	// Build target ports list
	var targetPorts []int

	// 1. Stride window
	if hasStride && stride != 0 {
		for i := 1; i <= 16; i++ {
			p := portBase + (i * stride)
			if p > 1024 && p < 65535 {
				targetPorts = append(targetPorts, p)
			}
		}
	}

	// 2. Birthday Paradox randomized port spray (512 ports)
	seen := make(map[int]bool)
	for _, p := range targetPorts {
		seen[p] = true
	}
	for len(targetPorts) < maxSprays {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(64511))
		p := int(nBig.Int64()) + 1024
		if !seen[p] {
			seen[p] = true
			targetPorts = append(targetPorts, p)
		}
	}

	paceTicker := time.NewTicker(probePacingMs * time.Millisecond)
	defer paceTicker.Stop()

	portIdx := 0
	for {
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return nil, ErrHolePunchTimeout
		case winAddr := <-winChan:
			_ = conn.SetDeadline(time.Time{})
			return &PunchedConn{UDPConn: conn, rAddr: winAddr}, nil
		case <-paceTicker.C:
			if portIdx < len(targetPorts) {
				tgt := &net.UDPAddr{IP: remoteIP, Port: targetPorts[portIdx]}
				_, _ = conn.WriteTo(probePkt, tgt)
				portIdx++
			} else {
				// Loop back and keep sending keepalive probes
				portIdx = 0
			}
		}
	}
}

func runHandshake(ctx context.Context, conn *net.UDPConn, rAddr *net.UDPAddr, code string) (net.Conn, error) {
	probePkt := makePunchPacket(code, msgProbe)
	ackPkt := makePunchPacket(code, msgProbeAck)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	buf := make([]byte, 256)
	var confirmed bool

	for {
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return nil, ErrHolePunchTimeout
		case <-ticker.C:
			_, _ = conn.WriteTo(probePkt, rAddr)
		default:
			_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, from, err := conn.ReadFromUDP(buf)
			if err == nil && n > 0 {
				if mType, ok := parsePunchPacket(buf[:n], code); ok {
					if mType == msgProbe || mType == msgProbeAck {
						_, _ = conn.WriteTo(ackPkt, from)
						confirmed = true
					}
				}
			}
			if confirmed {
				_ = conn.SetDeadline(time.Time{})
				return &PunchedConn{UDPConn: conn, rAddr: rAddr}, nil
			}
		}
	}
}

// PunchedConn wraps a UDP socket into a standard streaming net.Conn.
type PunchedConn struct {
	*net.UDPConn
	rAddr   *net.UDPAddr
	readBuf []byte
	mu      sync.Mutex
}

func (c *PunchedConn) RemoteAddr() net.Addr {
	return c.rAddr
}

func (c *PunchedConn) Write(b []byte) (int, error) {
	return c.UDPConn.WriteTo(b, c.rAddr)
}

func (c *PunchedConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.readBuf) > 0 {
		n := copy(b, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	buf := make([]byte, 65536)
	for {
		n, from, err := c.UDPConn.ReadFromUDP(buf)
		if err != nil {
			return 0, err
		}
		// Filter out residual punch handshake packets
		if n >= 8 && string(buf[:8]) == punchMagic {
			continue
		}
		// Accept packets matching remote IP
		if from.IP.Equal(c.rAddr.IP) {
			copied := copy(b, buf[:n])
			if copied < n {
				c.readBuf = buf[copied:n]
			}
			return copied, nil
		}
	}
}
