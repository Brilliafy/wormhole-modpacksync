package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"wormhole-sync/pkg/models"
)

// emptySHA256 is the SHA-256 digest of a zero-byte file. Zero-size files are
// extremely common in modpack instances (placeholder files), and hashing them
// through the generic path wastes syscalls on every scan.
var emptySHA256 = func() string {
	sum := sha256.Sum256(nil)
	return hex.EncodeToString(sum[:])
}()

// HashFile computes the SHA-256 hash of a file using a 1MB buffer, which is
// large enough to keep modern disks and NVMe drives saturated.
func HashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	buf := make([]byte, 1024*1024)
	if _, err := io.CopyBuffer(hasher, file, buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// HashFilesParallel computes hashes for a slice of FileInfo using a worker pool
func HashFilesParallel(rootPath string, files []models.FileInfo, onProgress func(done int, total int, current string)) ([]models.FileInfo, error) {
	numWorkers := runtime.NumCPU()
	if numWorkers < 2 {
		numWorkers = 2
	}
	if numWorkers > 32 {
		numWorkers = 32
	}

	total := len(files)
	result := make([]models.FileInfo, total)
	copy(result, files)

	type job struct {
		index int
	}

	jobs := make(chan job, total)
	for i := 0; i < total; i++ {
		jobs <- job{index: i}
	}
	close(jobs)

	var wg sync.WaitGroup
	var completedCount int64

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if result[j.index].Size == 0 {
					result[j.index].Hash = emptySHA256
				} else {
					fullPath := filepath.Join(rootPath, filepath.FromSlash(result[j.index].RelPath))
					hash, err := HashFile(fullPath)
					if err == nil {
						result[j.index].Hash = hash
					}
				}
				done := int(atomic.AddInt64(&completedCount, 1))
				if onProgress != nil {
					onProgress(done, total, result[j.index].RelPath)
				}
			}
		}()
	}

	wg.Wait()
	return result, nil
}
