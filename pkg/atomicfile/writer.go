package atomicfile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// WriteAtomic writes data from reader r to destPath atomically.
// It writes to a temporary file first, verifies the expected SHA-256 hash
// (if provided), and renames the file to destPath upon successful completion.
func WriteAtomic(destPath string, expectedHash string, r io.Reader) (int64, string, error) {
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create temporary file in the same directory to ensure same filesystem for atomic rename
	tmpPath := destPath + ".wormhole.tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return 0, "", fmt.Errorf("failed to create temporary file %s: %w", tmpPath, err)
	}

	// Track cleanup on error
	var success bool
	defer func() {
		if !success {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	n, err := io.Copy(writer, r)
	if err != nil {
		return 0, "", fmt.Errorf("failed to write data to %s: %w", tmpPath, err)
	}

	if err := tmpFile.Sync(); err != nil {
		return 0, "", fmt.Errorf("failed to sync temporary file %s: %w", tmpPath, err)
	}

	if err := tmpFile.Close(); err != nil {
		return 0, "", fmt.Errorf("failed to close temporary file %s: %w", tmpPath, err)
	}

	computedHash := hex.EncodeToString(hasher.Sum(nil))

	if expectedHash != "" && computedHash != expectedHash {
		return 0, "", fmt.Errorf("hash verification failed for %s: expected %s, got %s",
			destPath, expectedHash, computedHash)
	}

	// Atomic rename to final destination
	if err := safeRename(tmpPath, destPath); err != nil {
		return 0, "", fmt.Errorf("failed to replace file %s: %w", destPath, err)
	}

	success = true
	return n, computedHash, nil
}

// safeRename handles platform differences between Linux and Windows when destination exists
func safeRename(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// On Windows, os.Rename fails if the destination file already exists and is locked or in use.
	// If it fails on Windows, try removing the destination first.
	if runtime.GOOS == "windows" {
		_ = os.Remove(dst)
		return os.Rename(src, dst)
	}

	return err
}
