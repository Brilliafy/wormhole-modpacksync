package nat

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

type MappingType string

const (
	MappingCone      MappingType = "cone"      // Endpoint-Independent Mapping
	MappingSymmetric MappingType = "symmetric" // Endpoint-Dependent Mapping
)

type NATReport struct {
	MappingType MappingType
	PublicIP    net.IP
	Port1       int
	Port2       int
	Stride      int
	HasStride   bool
	LocalPort   int
}

var DefaultSTUNServers = []string{
	"stun.l.google.com:19302",
	"stun.cloudflare.com:3478",
}

// CharacterizeNAT queries two distinct STUN servers from the same local socket.
func CharacterizeNAT(ctx context.Context, localConn *net.UDPConn) (*NATReport, error) {
	var conn *net.UDPConn
	var err error

	if localConn != nil {
		conn = localConn
	} else {
		conn, err = net.ListenUDP("udp4", nil)
		if err != nil {
			return nil, fmt.Errorf("failed to bind local udp socket: %w", err)
		}
		defer conn.Close()
	}

	localPort := conn.LocalAddr().(*net.UDPAddr).Port

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Query STUN Server 1
	ip1, port1, err := querySTUN(ctx, conn, DefaultSTUNServers[0])
	if err != nil {
		return nil, fmt.Errorf("STUN server 1 (%s) failed: %w", DefaultSTUNServers[0], err)
	}

	// Query STUN Server 2
	_, port2, err := querySTUN(ctx, conn, DefaultSTUNServers[1])
	if err != nil {
		// Fallback: report what server 1 gave us
		return &NATReport{
			MappingType: MappingCone,
			PublicIP:    ip1,
			Port1:       port1,
			Port2:       port1,
			LocalPort:   localPort,
		}, nil
	}

	report := &NATReport{
		PublicIP:  ip1,
		Port1:     port1,
		Port2:     port2,
		LocalPort: localPort,
	}

	if port1 == port2 {
		report.MappingType = MappingCone
	} else {
		report.MappingType = MappingSymmetric
		stride := port2 - port1
		if stride >= -16 && stride <= 16 && stride != 0 {
			report.HasStride = true
			report.Stride = stride
		}
	}

	return report, nil
}

func querySTUN(ctx context.Context, conn *net.UDPConn, server string) (net.IP, int, error) {
	dst, err := net.ResolveUDPAddr("udp4", server)
	if err != nil {
		return nil, 0, err
	}

	txID := make([]byte, 12)
	if _, err := rand.Read(txID); err != nil {
		return nil, 0, err
	}

	// RFC 5389 Binding Request Header (20 bytes)
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req[0:2], 0x0001)     // Binding Request
	binary.BigEndian.PutUint16(req[2:4], 0x0000)     // Length
	binary.BigEndian.PutUint32(req[4:8], 0x2112A442) // Magic Cookie
	copy(req[8:20], txID)                            // Transaction ID

	_ = conn.SetDeadline(time.Now().Add(1200 * time.Millisecond))
	if _, err := conn.WriteTo(req, dst); err != nil {
		return nil, 0, err
	}

	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		default:
		}

		_ = conn.SetDeadline(time.Now().Add(1200 * time.Millisecond))
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			return nil, 0, err
		}
		if n < 20 {
			continue
		}

		// Verify Magic Cookie & Transaction ID
		if binary.BigEndian.Uint32(buf[4:8]) != 0x2112A442 {
			continue
		}
		match := true
		for i := 0; i < 12; i++ {
			if buf[8+i] != txID[i] {
				match = false
				break
			}
		}
		if !match {
			continue
		}

		// Parse STUN attributes
		msgLen := int(binary.BigEndian.Uint16(buf[2:4]))
		offset := 20
		end := 20 + msgLen
		if end > n {
			end = n
		}

		for offset+4 <= end {
			attrType := binary.BigEndian.Uint16(buf[offset : offset+2])
			attrLen := int(binary.BigEndian.Uint16(buf[offset+2 : offset+4]))
			valOffset := offset + 4
			offset = valOffset + ((attrLen + 3) & ^3) // 4-byte padding

			if valOffset+attrLen > end {
				break
			}

			// XOR-MAPPED-ADDRESS (0x0020)
			if attrType == 0x0020 && attrLen >= 8 {
				family := buf[valOffset+1]
				xPort := binary.BigEndian.Uint16(buf[valOffset+2 : valOffset+4])
				port := int(xPort ^ 0x2112)

				if family == 0x01 && attrLen >= 8 { // IPv4
					xIP := binary.BigEndian.Uint32(buf[valOffset+4 : valOffset+8])
					ipUint := xIP ^ 0x2112A442
					ip := net.IPv4(byte(ipUint>>24), byte(ipUint>>16), byte(ipUint>>8), byte(ipUint))
					return ip, port, nil
				}
			}

			// MAPPED-ADDRESS (0x0001) fallback
			if attrType == 0x0001 && attrLen >= 8 {
				family := buf[valOffset+1]
				port := int(binary.BigEndian.Uint16(buf[valOffset+2 : valOffset+4]))
				if family == 0x01 && attrLen >= 8 {
					ip := net.IPv4(buf[valOffset+4], buf[valOffset+5], buf[valOffset+6], buf[valOffset+7])
					return ip, port, nil
				}
			}
		}

		_ = from
		break
	}

	return nil, 0, fmt.Errorf("no mapped address attribute in STUN response from %s", server)
}
