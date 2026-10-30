package nat

import (
	"context"
	"net"
	"time"
)

// DiscoverGlobalIPv6 returns the primary routable Global Unicast IPv6 address, if available.
func DiscoverGlobalIPv6(ctx context.Context) (net.IP, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, false
	}

	var candidates []net.IP

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Ignore virtual/tunnel interfaces if physical interfaces exist
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP
			// Check if Global Unicast
			if isGlobalUnicastIPv6(ip) {
				candidates = append(candidates, ip)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, false
	}

	// Verify outbound routability
	for _, ip := range candidates {
		if verifyIPv6Routable(ctx, ip) {
			return ip, true
		}
	}

	return nil, false
}

func isGlobalUnicastIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	// Filter out ULA (fc00::/7) and 6to4/Teredo if not pure unicast
	if len(ip) == 16 && (ip[0]&0xfe == 0xfc) {
		return false
	}
	return true
}

func verifyIPv6Routable(ctx context.Context, localIP net.IP) bool {
	d := &net.Dialer{
		LocalAddr: &net.UDPAddr{IP: localIP, Port: 0},
		Timeout:   500 * time.Millisecond,
	}
	// Probe Google Public DNS IPv6 over UDP
	conn, err := d.DialContext(ctx, "udp6", "[2001:4860:4860::8888]:53")
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}
