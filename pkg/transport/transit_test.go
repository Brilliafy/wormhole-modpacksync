package transport

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
)

func TestWrapEncryptedConn(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p1.Close()
	defer p2.Close()

	code := "ROOM-CRYPTO-TEST-99"
	sender, err := WrapEncryptedConn(p1, code, true)
	if err != nil {
		t.Fatalf("WrapEncryptedConn sender failed: %v", err)
	}
	receiver, err := WrapEncryptedConn(p2, code, false)
	if err != nil {
		t.Fatalf("WrapEncryptedConn receiver failed: %v", err)
	}

	testData := make([]byte, 256*1024) // 256KB
	_, _ = rand.Read(testData)

	var wg sync.WaitGroup
	wg.Add(2)

	var receivedData []byte
	var sendErr, recvErr error

	go func() {
		defer wg.Done()
		_, sendErr = sender.Write(testData)
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, len(testData))
		_, recvErr = io.ReadFull(receiver, buf)
		receivedData = buf
	}()

	wg.Wait()

	if sendErr != nil {
		t.Fatalf("sender.Write failed: %v", sendErr)
	}
	if recvErr != nil {
		t.Fatalf("receiver.Read failed: %v", recvErr)
	}
	if !bytes.Equal(testData, receivedData) {
		t.Fatalf("decrypted payload does not match original payload")
	}
}

func TestEncryptedConnTampering(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p1.Close()
	defer p2.Close()

	code := "ROOM-TAMPER-TEST"
	sender, err := WrapEncryptedConn(p1, code, true)
	if err != nil {
		t.Fatalf("WrapEncryptedConn sender failed: %v", err)
	}

	tamperedP1, tamperedP2 := net.Pipe()
	defer tamperedP1.Close()
	defer tamperedP2.Close()

	go func() {
		// Read 4-byte length prefix
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(p2, lenBuf); err != nil {
			return
		}
		cipherLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
		cipherBuf := make([]byte, cipherLen)
		if _, err := io.ReadFull(p2, cipherBuf); err != nil {
			return
		}
		// Flip a bit in the ciphertext payload
		if len(cipherBuf) > 0 {
			cipherBuf[len(cipherBuf)-1] ^= 0xAA
		}
		_, _ = tamperedP1.Write(lenBuf)
		_, _ = tamperedP1.Write(cipherBuf)
	}()

	go func() {
		_, _ = sender.Write([]byte("Hello confidential world"))
	}()

	receiver, err := WrapEncryptedConn(tamperedP2, code, false)
	if err != nil {
		t.Fatalf("WrapEncryptedConn receiver failed: %v", err)
	}

	buf := make([]byte, 100)
	_, err = receiver.Read(buf)
	if err == nil {
		t.Fatalf("expected decryption error due to tampering, got nil")
	}
	t.Logf("Tampering successfully detected and rejected: %v", err)
}
