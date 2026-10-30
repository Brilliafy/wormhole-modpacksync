package transport

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	DefaultTransitRelay = "transit.magic-wormhole.io:4001"
)

func transitRelayAddr() string {
	if v := strings.TrimSpace(os.Getenv("WORMHOLE_TRANSIT_RELAY")); v != "" {
		return v
	}
	return DefaultTransitRelay
}

func randomHex16() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// DialTransitRelay connects to the public transit relay server, matches on the room code,
// and wraps the established stream in end-to-end AES-256-GCM encryption.
func DialTransitRelay(ctx context.Context, code string, isSender bool) (net.Conn, error) {
	relayAddr := transitRelayAddr()

	// Derive 64-hex-char channel token from room code
	sum := sha256.Sum256([]byte("wormhole-sync-transit-v1:" + code))
	channel := hex.EncodeToString(sum[:])
	side := randomHex16()

	var d net.Dialer
	dialCtx, cancelDial := context.WithTimeout(ctx, 10*time.Second)
	defer cancelDial()

	rawConn, err := d.DialContext(dialCtx, "tcp", relayAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial transit relay %s: %w", relayAddr, err)
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = rawConn.Close()
		case <-done:
		}
	}()

	_ = rawConn.SetDeadline(time.Now().Add(25 * time.Second))

	// Send Magic Wormhole transit relay handshake line: "please relay <64-hex> for side <16-hex>\n"
	req := fmt.Sprintf("please relay %s for side %s\n", channel, side)
	if _, err := rawConn.Write([]byte(req)); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("failed to send handshake to transit relay: %w", err)
	}

	reader := bufio.NewReader(rawConn)
	line, err := reader.ReadString('\n')
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("failed to read response from transit relay: %w", err)
	}
	if line != "ok\n" {
		rawConn.Close()
		return nil, fmt.Errorf("transit relay returned unexpected response: %q", line)
	}

	// Reset deadline for normal streaming transfer
	_ = rawConn.SetDeadline(time.Time{})

	// Wrap in end-to-end AES-256-GCM encryption
	encConn, err := WrapEncryptedConn(rawConn, code, isSender)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("failed to initialize e2e encryption: %w", err)
	}

	return encConn, nil
}

// EncryptedConn wraps any net.Conn with directional AES-256-GCM authenticated encryption.
type EncryptedConn struct {
	net.Conn
	encGCM   cipher.AEAD
	decGCM   cipher.AEAD
	writeSeq uint64
	readSeq  uint64
	readBuf  []byte
	writeMu  sync.Mutex
	readMu   sync.Mutex
}

func WrapEncryptedConn(conn net.Conn, code string, isSender bool) (*EncryptedConn, error) {
	// Derive distinct directional keys using SHA-256 from the shared room code
	s2rKey := sha256.Sum256([]byte("wormhole-sync-s2r-key-v1:" + code))
	r2sKey := sha256.Sum256([]byte("wormhole-sync-r2s-key-v1:" + code))

	var encKey, decKey [32]byte
	if isSender {
		encKey = s2rKey
		decKey = r2sKey
	} else {
		encKey = r2sKey
		decKey = s2rKey
	}

	blockEnc, err := aes.NewCipher(encKey[:])
	if err != nil {
		return nil, err
	}
	encGCM, err := cipher.NewGCM(blockEnc)
	if err != nil {
		return nil, err
	}

	blockDec, err := aes.NewCipher(decKey[:])
	if err != nil {
		return nil, err
	}
	decGCM, err := cipher.NewGCM(blockDec)
	if err != nil {
		return nil, err
	}

	return &EncryptedConn{
		Conn:   conn,
		encGCM: encGCM,
		decGCM: decGCM,
	}, nil
}

func (c *EncryptedConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	const maxChunk = 64 * 1024
	total := len(p)
	for len(p) > 0 {
		chunkSize := len(p)
		if chunkSize > maxChunk {
			chunkSize = maxChunk
		}
		chunk := p[:chunkSize]
		p = p[chunkSize:]

		// 12-byte nonce: 4-byte zero prefix + 8-byte big-endian sequence counter
		nonce := make([]byte, 12)
		binary.BigEndian.PutUint64(nonce[4:], c.writeSeq)
		c.writeSeq++

		ciphertext := c.encGCM.Seal(nil, nonce, chunk, nil)

		// 4-byte length prefix
		lenBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(ciphertext)))

		if _, err := c.Conn.Write(lenBuf); err != nil {
			return 0, err
		}
		if _, err := c.Conn.Write(ciphertext); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (c *EncryptedConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if len(c.readBuf) > 0 {
		n := copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.Conn, lenBuf); err != nil {
		return 0, err
	}
	cipherLen := binary.BigEndian.Uint32(lenBuf)
	if cipherLen > 128*1024 { // safety boundary per record
		return 0, fmt.Errorf("ciphertext frame exceeds safety limit: %d bytes", cipherLen)
	}

	ciphertext := make([]byte, cipherLen)
	if _, err := io.ReadFull(c.Conn, ciphertext); err != nil {
		return 0, err
	}

	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce[4:], c.readSeq)
	c.readSeq++

	plaintext, err := c.decGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return 0, fmt.Errorf("e2e decryption/authentication error: %w", err)
	}

	n := copy(p, plaintext)
	if n < len(plaintext) {
		c.readBuf = plaintext[n:]
	}
	return n, nil
}
