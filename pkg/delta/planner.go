package delta

import (
	"os"
	"path/filepath"

	"wormhole-sync/pkg/models"
	"wormhole-sync/pkg/scanner"
)

// PlanDelta compares the sender's manifest against local destination directory
// and determines which files actually need to be downloaded vs which already match.
func PlanDelta(destDir string, manifest *models.SyncManifest, onProgress func(checked int, total int, current string)) (*models.DeltaPlan, error) {
	plan := &models.DeltaPlan{
		FilesToSync:   make([]models.FileInfo, 0),
		FilesUpToDate: make([]models.FileInfo, 0),
		TotalFiles:    manifest.TotalFiles,
		TotalBytes:    manifest.TotalBytes,
	}

	for i, file := range manifest.Files {
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
	return plan, nil
}
