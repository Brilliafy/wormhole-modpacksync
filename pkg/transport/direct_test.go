package transport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"wormhole-sync/pkg/models"
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
	schematicContent := []byte("schematic binary data")

	_ = os.WriteFile(filepath.Join(srcDir, "mods", "jei.jar"), modContent, 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "config", "jei.json"), configContent, 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "zeustemple.schematic"), schematicContent, 0644)

	// Pre-create identical file in dstDir for jei.jar to test delta skipping
	_ = os.MkdirAll(filepath.Join(dstDir, "mods"), 0755)
	_ = os.WriteFile(filepath.Join(dstDir, "mods", "jei.jar"), modContent, 0644)

	// Pre-create an obsolete file in mods to test absolute sync pruning
	_ = os.WriteFile(filepath.Join(dstDir, "mods", "obsolete-mod.jar"), []byte("obsolete jar to delete"), 0644)

	// Pre-create an extra file in root to test that root files are NOT touched
	_ = os.WriteFile(filepath.Join(dstDir, "friend-personal-options.txt"), []byte("do not delete me"), 0644)

	manifest, err := scanner.ScanSelectedFiles(
		context.Background(),
		srcDir,
		[]string{"mods/jei.jar", "config/jei.json", "zeustemple.schematic"},
		[]string{"mods"}, // Only mods is marked for absolute sync
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to scan source files: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	sender := NewDirectSender(srcDir, func() *models.SyncManifest { return manifest }, 0, nil)

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

	// Verify jei.json and zeustemple.schematic were transferred
	receivedConfig, err := os.ReadFile(filepath.Join(dstDir, "config", "jei.json"))
	if err != nil {
		t.Fatalf("Failed to read received config file: %v", err)
	}
	if string(receivedConfig) != string(configContent) {
		t.Errorf("Received config mismatch. Expected %s, got %s", configContent, receivedConfig)
	}

	receivedSchematic, err := os.ReadFile(filepath.Join(dstDir, "zeustemple.schematic"))
	if err != nil {
		t.Fatalf("Failed to read received schematic: %v", err)
	}
	if string(receivedSchematic) != string(schematicContent) {
		t.Errorf("Received schematic mismatch")
	}

	// Verify obsolete-mod.jar was DELETED because mods had Absolute Sync enabled!
	if _, err := os.Stat(filepath.Join(dstDir, "mods", "obsolete-mod.jar")); !os.IsNotExist(err) {
		t.Errorf("Expected obsolete-mod.jar to be pruned from mods/ folder")
	}

	// Verify friend-personal-options.txt in root was PRESERVED!
	personalContent, err := os.ReadFile(filepath.Join(dstDir, "friend-personal-options.txt"))
	if err != nil {
		t.Fatalf("friend-personal-options.txt in root must NOT be deleted: %v", err)
	}
	if string(personalContent) != "do not delete me" {
		t.Errorf("friend-personal-options.txt content corrupted")
	}
}
