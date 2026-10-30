package nat

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestPunchPacketCodec(t *testing.T) {
	code := "TEST-PUNCH-CODE"
	pkt := makePunchPacket(code, msgProbe)
	mType, ok := parsePunchPacket(pkt, code)
	if !ok || mType != msgProbe {
		t.Fatalf("expected msgProbe, got %v, ok=%v", mType, ok)
	}

	pktAck := makePunchPacket(code, msgProbeAck)
	mType, ok = parsePunchPacket(pktAck, code)
	if !ok || mType != msgProbeAck {
		t.Fatalf("expected msgProbeAck, got %v, ok=%v", mType, ok)
	}

	// Mismatched code should fail
	_, ok = parsePunchPacket(pkt, "WRONG-CODE")
	if ok {
		t.Fatalf("expected failure on mismatched code")
	}
}

func TestConeToConeSimulated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	code := "SIM-PUNCH-01"
	c1, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()

	c2, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	p1 := c1.LocalAddr().(*net.UDPAddr).Port
	p2 := c2.LocalAddr().(*net.UDPAddr).Port

	var conn1, conn2 net.Conn
	var err1, err2 error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		conn1, err1 = runHandshake(ctx, c1, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: p2}, code)
	}()

	go func() {
		defer wg.Done()
		conn2, err2 = runHandshake(ctx, c2, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: p1}, code)
	}()

	wg.Wait()

	if err1 != nil || err2 != nil {
		t.Fatalf("handshake failed: err1=%v, err2=%v", err1, err2)
	}
	if conn1 == nil || conn2 == nil {
		t.Fatalf("expected non-nil connections")
	}

	// Test message passing through punched conn
	msg := []byte("hello from peer 1")
	if _, err := conn1.Write(msg); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 100)
	n, err := conn2.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != string(msg) {
		t.Fatalf("expected %q, got %q", string(msg), string(buf[:n]))
	}
}
