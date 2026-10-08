package transport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"wormhole-sync/pkg/scanner"
)

func TestDirectSenderReceiverSync(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source files
	_ = os.MkdirAll(filepath.Join(srcDir, "mods"), 0755)
	_ = os.MkdirAll(filepath.Join(srcDir, "config"), 0755)

	modContent := []byte("mod file content here - test bytes 123456789")
	configContent := []byte(`{"enabled": true, "key": "value"}`)

	_ = os.WriteFile(filepath.Join(srcDir, "mods", "jei.jar"), modContent, 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "config", "jei.json"), configContent, 0644)

	// Pre-create identical file in dstDir for jei.jar to test delta skipping
	_ = os.MkdirAll(filepath.Join(dstDir, "mods"), 0755)
	_ = os.WriteFile(filepath.Join(dstDir, "mods", "jei.jar"), modContent, 0644)

	manifest, err := scanner.ScanSelectedFiles(srcDir, []string{"mods/jei.jar", "config/jei.json"}, nil)
	if err != nil {
		t.Fatalf("Failed to scan source files: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	sender := NewDirectSender(srcDir, manifest, 0, nil)

	var senderErr error
	var receiverErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		senderErr = sender.Start(ctx)
	}()

	// Wait briefly for sender listener to bind port
	time.Sleep(100 * time.Millisecond)

	port := sender.Port()
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	receiver := NewDirectReceiver(dstDir, addr, nil)
	receiverErr = receiver.Start(ctx)

	wg.Wait()

	if senderErr != nil {
		t.Errorf("Sender error: %v", senderErr)
	}
	if receiverErr != nil {
		t.Errorf("Receiver error: %v", receiverErr)
	}

	// Verify jei.json was transferred
	receivedConfig, err := os.ReadFile(filepath.Join(dstDir, "config", "jei.json"))
	if err != nil {
		t.Fatalf("Failed to read received config file: %v", err)
	}
	if string(receivedConfig) != string(configContent) {
		t.Errorf("Received config mismatch. Expected %s, got %s", configContent, receivedConfig)
	}

	// Verify jei.jar matches
	receivedMod, err := os.ReadFile(filepath.Join(dstDir, "mods", "jei.jar"))
	if err != nil {
		t.Fatalf("Failed to read received mod file: %v", err)
	}
	if string(receivedMod) != string(modContent) {
		t.Errorf("Received mod mismatch")
	}
}
