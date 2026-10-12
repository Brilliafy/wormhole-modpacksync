package scanner

import (
	"context"
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

// HashFilesParallel computes hashes for a slice of FileInfo using a worker
// pool. Files that cannot be read are dropped from the result instead of
// silently producing an empty hash, which would poison delta planning and
// abort the transfer later. Cancelling ctx stops hashing at the next job.
func HashFilesParallel(ctx context.Context, rootPath string, files []models.FileInfo, onProgress func(done int, total int, current string)) ([]models.FileInfo, error) {
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
	var dropped int64

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if result[j.index].Size == 0 {
					result[j.index].Hash = emptySHA256
				} else {
					fullPath := filepath.Join(rootPath, filepath.FromSlash(result[j.index].RelPath))
					hash, err := HashFile(fullPath)
					if err == nil {
						result[j.index].Hash = hash
					} else {
						// Unreadable: mark for removal so the manifest only
						// contains files both sides can actually transfer.
						atomic.AddInt64(&dropped, 1)
						result[j.index].Size = -1
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

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if dropped == 0 {
		return result, nil
	}

	kept := make([]models.FileInfo, 0, len(result)-int(dropped))
	for _, f := range result {
		if f.Size >= 0 {
			kept = append(kept, f)
		}
	}
	return kept, nil
}
