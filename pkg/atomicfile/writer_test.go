package atomicfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicSuccess(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "subdir", "testfile.txt")
	content := []byte("Hello Minecraft Wormhole Sync!")

	hasher := sha256.New()
	hasher.Write(content)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	n, computedHash, err := WriteAtomic(targetPath, expectedHash, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("WriteAtomic failed: %v", err)
	}

	if n != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), n)
	}
	if computedHash != expectedHash {
		t.Errorf("Expected hash %s, got %s", expectedHash, computedHash)
	}

	readBack, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read back file: %v", err)
	}
	if string(readBack) != string(content) {
		t.Errorf("Expected content %s, got %s", content, readBack)
	}

	// Verify temporary file does not remain
	tmpPath := targetPath + ".wormhole.tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("Temporary file %s was not cleaned up", tmpPath)
	}
}

func TestWriteAtomicHashMismatch(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "mismatch.txt")
	content := []byte("Corrupted content")

	_, _, err := WriteAtomic(targetPath, "invalidhash123", bytes.NewReader(content))
	if err == nil {
		t.Fatal("Expected error on hash mismatch, got nil")
	}

	// Verify target file was not created
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Errorf("Target file %s should not exist after hash mismatch", targetPath)
	}
}
