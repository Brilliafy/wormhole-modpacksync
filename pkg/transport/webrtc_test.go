package transport

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"wormhole-sync/pkg/scanner"
	"wormhole-sync/pkg/signaling"
)

// TestWebRTCSignalingAndTransfer verifies the full hole-punch pipeline:
// offer/answer exchanged through the embedded signaling relay, ICE
// connectivity, DataChannel open, manifest + delta exchange, chunked
// transfer, atomic write and ACK round trips.
func TestWebRTCSignalingAndTransfer(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	_ = os.MkdirAll(filepath.Join(srcDir, "mods"), 0755)
	content := []byte("webrtc hole punch verification payload 0123456789")
	_ = os.WriteFile(filepath.Join(srcDir, "mods", "punch.jar"), content, 0644)

	manifest, err := scanner.ScanSelectedFiles(srcDir, []string{"mods/punch.jar"}, nil, nil)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Shared signaling relay stands in for a rendezvous point both peers can
	// reach (this is exactly what a LAN peer or a self-hosted relay does).
	relay := signaling.NewEmbeddedRelayServer()
	server := httptest.NewServer(relay.Handler())
	defer server.Close()

	sender := NewWebRTCSender(srcDir, manifest, nil)
	offer, err := sender.CreateOffer()
	if err != nil {
		t.Fatalf("create offer failed: %v", err)
	}

	senderClient := signaling.NewSignalingClient(server.URL)
	code := signaling.GenerateRoomCode()
	ctx := context.Background()
	if err := senderClient.PublishOffer(ctx, code, offer, ""); err != nil {
		t.Fatalf("publish offer failed: %v", err)
	}

	receiver := NewWebRTCReceiver(dstDir, nil)
	receiverClient := signaling.NewSignalingClient(server.URL)
	gotOffer, _, err := receiverClient.FetchOffer(ctx, code)
	if err != nil {
		t.Fatalf("fetch offer failed: %v", err)
	}
	answer, err := receiver.CreateAnswer(gotOffer)
	if err != nil {
		t.Fatalf("create answer failed: %v", err)
	}
	if err := receiverClient.PublishAnswer(ctx, code, answer); err != nil {
		t.Fatalf("publish answer failed: %v", err)
	}

	sessionCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var senderErr, receiverErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		gotAnswer, err := senderClient.WaitForAnswer(sessionCtx, code)
		if err != nil {
			senderErr = err
			return
		}
		if err := sender.SetAnswer(gotAnswer); err != nil {
			senderErr = err
			return
		}
		senderErr = sender.StartSession(sessionCtx)
	}()
	receiverErr = receiver.StartSession(sessionCtx)
	wg.Wait()

	if senderErr != nil {
		t.Errorf("sender error: %v", senderErr)
	}
	if receiverErr != nil {
		t.Errorf("receiver error: %v", receiverErr)
	}

	received, err := os.ReadFile(filepath.Join(dstDir, "mods", "punch.jar"))
	if err != nil {
		t.Fatalf("received file missing: %v", err)
	}
	if string(received) != string(content) {
		t.Errorf("received content mismatch: got %q want %q", received, content)
	}
}
