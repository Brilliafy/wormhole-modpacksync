package transport

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"wormhole-sync/pkg/models"
)

// TestStartSenderImmediateCode verifies the share code is returned instantly
// while hashing still runs in the background, and that a receiver connecting
// during hashing still gets a correct transfer (the manifest provider blocks
// until preparation finishes).
func TestStartSenderImmediateCode(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	content := []byte("immediate code verification payload")
	if err := os.WriteFile(filepath.Join(srcDir, "bigmod.jar"), content, 0644); err != nil {
		t.Fatal(err)
	}

	statsCh := make(chan models.SyncStats, 64)
	sm := NewSessionManager(func(stats models.SyncStats) {
		statsCh <- stats
	})
	defer sm.Reset()

	start := time.Now()
	result, err := sm.StartSenderSession(srcDir, []string{"bigmod.jar"}, nil, "direct")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("StartSenderSession failed: %v", err)
	}
	if result.Code == "" {
		t.Fatal("expected a generated code immediately")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("code took %v to appear; expected near-instant", elapsed)
	}

	// Give the listener a moment, then connect the receiver.
	time.Sleep(100 * time.Millisecond)

	directAddrs := result.DirectAddresses
	if len(directAddrs) == 0 {
		t.Fatal("expected direct addresses")
	}

	var wg sync.WaitGroup
	var receiverErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		receiver := NewDirectReceiver(dstDir, directAddrs[0], nil)
		receiverErr = receiver.Start(context.Background())
	}()

	// Sender side runs inside the manager; wait for the receiver goroutine.
	wg.Wait()
	if receiverErr != nil {
		t.Fatalf("receiver error: %v", receiverErr)
	}

	// Wait for the manager session to finish.
	deadline := time.After(10 * time.Second)
	for {
		stats := sm.GetStats()
		if stats.Status == "finished" || stats.Status == "error" {
			if stats.Status == "error" {
				t.Fatalf("session error: %s", stats.ErrorMessage)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("session did not finish; last status: %s (%s)", stats.Status, stats.Message)
		case <-time.After(100 * time.Millisecond):
		}
	}

	received, err := os.ReadFile(filepath.Join(dstDir, "bigmod.jar"))
	if err != nil {
		t.Fatalf("received file missing: %v", err)
	}
	if string(received) != string(content) {
		t.Error("received content mismatch")
	}
}

// TestSenderPortFallbackAdvertisement verifies the advertised direct address
// uses the actual bound port when the default one is taken.
func TestSenderPortFallbackAdvertisement(t *testing.T) {
	// Occupy the default port
	ln, err := net.Listen("tcp", "0.0.0.0:42424")
	if err != nil {
		t.Skipf("cannot occupy default port: %v", err)
	}
	defer ln.Close()

	sm := NewSessionManager(func(models.SyncStats) {})
	defer sm.Reset()

	result, err := sm.StartSenderSession(t.TempDir(), nil, nil, "direct")
	if err != nil {
		t.Fatalf("StartSenderSession failed: %v", err)
	}
	if len(result.DirectAddresses) == 0 {
		t.Skip("no non-loopback local IPs detected in this environment")
	}
	for _, addr := range result.DirectAddresses {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			t.Errorf("malformed advertised address %q: %v", addr, err)
			continue
		}
		if port == "42424" {
			t.Errorf("advertised address %q still uses the occupied default port", addr)
		}
	}
}
