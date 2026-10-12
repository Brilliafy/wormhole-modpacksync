package delta

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wormhole-sync/pkg/models"
	"wormhole-sync/pkg/scanner"
)

// PlanDelta compares the sender's manifest against local destination directory
// and determines which files actually need to be downloaded vs which already match.
// If manifest has AbsoluteSyncFolders, it also detects extra local files in those folders for deletion.
// Local files are verified in parallel with a bounded worker pool, so receivers
// with large instances do not sit through a serial hashing pass. Cancelling
// ctx aborts verification at the next job.
func PlanDelta(ctx context.Context, destDir string, manifest *models.SyncManifest, onProgress func(checked int, total int, current string)) (*models.DeltaPlan, error) {
	plan := &models.DeltaPlan{
		FilesToSync:   make([]models.FileInfo, 0),
		FilesUpToDate: make([]models.FileInfo, 0),
		FilesToDelete: make([]string, 0),
		TotalFiles:    manifest.TotalFiles,
		TotalBytes:    manifest.TotalBytes,
	}

	total := len(manifest.Files)

	// needsSync[i] records whether manifest.Files[i] must be downloaded.
	needsSync := make([]bool, total)

	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	if workers > 32 {
		workers = 32
	}

	var completed int64
	var lastEmit int64 // Unix nanos of last progress event
	emitInterval := int64(100 * time.Millisecond)

	progress := func(current string) {
		if onProgress == nil || total == 0 {
			return
		}
		done := int(atomic.AddInt64(&completed, 1))
		now := time.Now().UnixNano()
		last := atomic.LoadInt64(&lastEmit)
		if now-last < emitInterval && done != total {
			return
		}
		if atomic.CompareAndSwapInt64(&lastEmit, last, now) {
			onProgress(done, total, current)
		}
	}

	jobs := make(chan int, total)
	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				file := manifest.Files[i]

				localPath := filepath.Join(destDir, filepath.FromSlash(file.RelPath))
				fi, err := os.Stat(localPath)

				if err != nil || fi.IsDir() {
					// File does not exist locally -> must download
					needsSync[i] = true
					progress(file.RelPath)
					continue
				}

				// Fast check: if size differs, it definitely needs syncing
				if fi.Size() != file.Size {
					needsSync[i] = true
					progress(file.RelPath)
					continue
				}

				// If size is 0 and exists, it's matching
				if file.Size == 0 {
					progress(file.RelPath)
					continue
				}

				// Size matches: compute local hash to be 100% sure
				localHash, err := scanner.HashFile(localPath)
				if err == nil && localHash == file.Hash {
					// Already up-to-date! Zero download needed!
					progress(file.RelPath)
				} else {
					// Hash mismatch -> download
					needsSync[i] = true
					progress(file.RelPath)
				}
			}
		}()
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Build the plan in manifest order so the transfer sequence is
	// deterministic no matter how the workers interleaved.
	for i, file := range manifest.Files {
		if needsSync[i] {
			plan.FilesToSync = append(plan.FilesToSync, file)
			plan.SyncBytes += file.Size
		} else {
			plan.FilesUpToDate = append(plan.FilesUpToDate, file)
			plan.SavedBytes += file.Size
		}
	}

	if onProgress != nil && total > 0 {
		onProgress(total, total, "")
	}

	plan.SyncFiles = len(plan.FilesToSync)

	// Check AbsoluteSyncFolders: find local files inside these folders that are not in manifest
	manifestFileSet := make(map[string]bool)
	for _, file := range manifest.Files {
		manifestFileSet[file.RelPath] = true
	}

	for _, absFolder := range manifest.AbsoluteSyncFolders {
		cleanFolder := filepath.ToSlash(filepath.Clean(absFolder))
		if cleanFolder == "" || cleanFolder == "." {
			continue // Never allow pruning entire root folder
		}

		localFolderDir := filepath.Join(destDir, filepath.FromSlash(cleanFolder))
		if _, err := os.Stat(localFolderDir); os.IsNotExist(err) {
			continue
		}

		_ = filepath.WalkDir(localFolderDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}

			rel, err := filepath.Rel(destDir, path)
			if err != nil {
				return nil
			}

			relSlash := filepath.ToSlash(rel)
			// Ignore temporary wormhole files
			if strings.HasSuffix(relSlash, ".wormhole.tmp") {
				return nil
			}

			// If file does NOT exist in sender's manifest, it is an extra file in an absolute-sync folder!
			if !manifestFileSet[relSlash] {
				plan.FilesToDelete = append(plan.FilesToDelete, relSlash)
			}
			return nil
		})
	}

	return plan, nil
}
