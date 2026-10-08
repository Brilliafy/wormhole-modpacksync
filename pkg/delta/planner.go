package delta

import (
	"os"
	"path/filepath"
	"strings"

	"wormhole-sync/pkg/models"
	"wormhole-sync/pkg/scanner"
)

// PlanDelta compares the sender's manifest against local destination directory
// and determines which files actually need to be downloaded vs which already match.
// If manifest has AbsoluteSyncFolders, it also detects extra local files in those folders for deletion.
func PlanDelta(destDir string, manifest *models.SyncManifest, onProgress func(checked int, total int, current string)) (*models.DeltaPlan, error) {
	plan := &models.DeltaPlan{
		FilesToSync:   make([]models.FileInfo, 0),
		FilesUpToDate: make([]models.FileInfo, 0),
		FilesToDelete: make([]string, 0),
		TotalFiles:    manifest.TotalFiles,
		TotalBytes:    manifest.TotalBytes,
	}

	manifestFileSet := make(map[string]bool)

	for i, file := range manifest.Files {
		manifestFileSet[file.RelPath] = true

		if onProgress != nil {
			onProgress(i+1, manifest.TotalFiles, file.RelPath)
		}

		localPath := filepath.Join(destDir, filepath.FromSlash(file.RelPath))
		fi, err := os.Stat(localPath)

		if err != nil || fi.IsDir() {
			// File does not exist locally -> must download
			plan.FilesToSync = append(plan.FilesToSync, file)
			plan.SyncBytes += file.Size
			continue
		}

		// Fast check: if size differs, it definitely needs syncing
		if fi.Size() != file.Size {
			plan.FilesToSync = append(plan.FilesToSync, file)
			plan.SyncBytes += file.Size
			continue
		}

		// If size is 0 and exists, it's matching
		if file.Size == 0 {
			plan.FilesUpToDate = append(plan.FilesUpToDate, file)
			plan.SavedBytes += file.Size
			continue
		}

		// Size matches: compute local hash to be 100% sure
		localHash, err := scanner.HashFile(localPath)
		if err == nil && localHash == file.Hash {
			// Already up-to-date! Zero download needed!
			plan.FilesUpToDate = append(plan.FilesUpToDate, file)
			plan.SavedBytes += file.Size
		} else {
			// Hash mismatch -> download
			plan.FilesToSync = append(plan.FilesToSync, file)
			plan.SyncBytes += file.Size
		}
	}

	plan.SyncFiles = len(plan.FilesToSync)

	// Check AbsoluteSyncFolders: find local files inside these folders that are not in manifest
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
