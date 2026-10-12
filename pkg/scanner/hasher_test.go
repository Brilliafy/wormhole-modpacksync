package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"wormhole-sync/pkg/models"
)

func TestScanSelectedFilesCancel(t *testing.T) {
	src := t.TempDir()
	for i := 0; i < 50; i++ {
		if err := os.WriteFile(filepath.Join(src, "f"+string(rune('a'+i))+".bin"), make([]byte, 4096), 0644); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before hashing starts

	_, err := ScanSelectedFiles(ctx, src, nil, nil, nil)
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
}

func TestHashFilesParallelDropsUnreadable(t *testing.T) {
	src := t.TempDir()
	good := filepath.Join(src, "good.bin")
	bad := filepath.Join(src, "bad.bin")
	if err := os.WriteFile(good, []byte("readable"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("secret"), 0000); err != nil {
		t.Fatal(err)
	}

	files := []models.FileInfo{
		{RelPath: "good.bin", Size: 8},
		{RelPath: "bad.bin", Size: 6},
	}

	result, err := HashFilesParallel(context.Background(), src, files, nil)
	if err != nil {
		t.Fatalf("HashFilesParallel failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 kept file, got %d", len(result))
	}
	if result[0].RelPath != "good.bin" || result[0].Hash == "" {
		t.Errorf("kept file wrong: %+v", result[0])
	}
}
