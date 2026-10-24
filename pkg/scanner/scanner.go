package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wormhole-sync/pkg/models"
)

// ScanSelectedFiles gathers selected files from rootPath, filters them, hashes them, and builds a SyncManifest.
// Cancelling ctx aborts hashing early instead of grinding through the whole selection.
// onScan (optional) reports walk progress; onProgress (optional) reports hashing progress.
func ScanSelectedFiles(ctx context.Context, rootPath string, selectedPaths []string, absoluteSyncFolders []string, onScan func(found int, current string), onProgress func(done int, total int, current string)) (*models.SyncManifest, error) {
	selectedSet := make(map[string]bool)
	for _, p := range selectedPaths {
		norm := filepath.ToSlash(filepath.Clean(p))
		selectedSet[norm] = true
	}

	var candidateFiles []models.FileInfo

	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || path == rootPath {
			return nil
		}

		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil
		}

		relSlash := filepath.ToSlash(rel)
		name := d.Name()

		// Skip hidden dotfiles
		if strings.HasPrefix(name, ".") && name != ".minecraft" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// If selection is specified, check if this file or any of its parent folders are selected
		if len(selectedSet) > 0 {
			included := false
			if selectedSet[relSlash] {
				included = true
			} else {
				// Check if any ancestor folder is selected
				parts := strings.Split(relSlash, "/")
				curr := ""
				for i := 0; i < len(parts)-1; i++ {
					if i == 0 {
						curr = parts[i]
					} else {
						curr = curr + "/" + parts[i]
					}
					if selectedSet[curr] {
						included = true
						break
					}
				}
			}

			if !included {
				if d.IsDir() {
					// Check if any selected item starts with this dir prefix
					hasChild := false
					for sel := range selectedSet {
						if strings.HasPrefix(sel, relSlash+"/") {
							hasChild = true
							break
						}
					}
					if !hasChild {
						return filepath.SkipDir
					}
				}
				return nil
			}
		}

		if !d.IsDir() {
			// Size and mtime are filled in while hashing: one open() there
			// covers both the stat and the read, which halves the syscalls on
			// slow FUSE mounts. The old d.Info() here cost ~0.2ms per file,
			// i.e. tens of seconds on 100k+ file instances.
			candidateFiles = append(candidateFiles, models.FileInfo{
				RelPath: relSlash,
				Size:    -1,
				IsDir:   false,
				Checked: true,
			})

			if onScan != nil && len(candidateFiles)%500 == 0 {
				onScan(len(candidateFiles), relSlash)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Compute hashes in parallel
	hashedFiles, err := HashFilesParallel(ctx, rootPath, candidateFiles, onProgress)
	if err != nil {
		return nil, err
	}

	var totalBytes int64 = 0
	for _, f := range hashedFiles {
		totalBytes += f.Size
	}

	if onScan != nil {
		onScan(len(candidateFiles), "")
	}

	normalizedAbsFolders := make([]string, 0, len(absoluteSyncFolders))
	for _, folder := range absoluteSyncFolders {
		clean := filepath.ToSlash(filepath.Clean(folder))
		if clean != "" && clean != "." {
			normalizedAbsFolders = append(normalizedAbsFolders, clean)
		}
	}

	return &models.SyncManifest{
		InstanceName:        filepath.Base(rootPath),
		CreatedAt:           time.Now().Unix(),
		Files:               hashedFiles,
		TotalFiles:          len(hashedFiles),
		TotalBytes:          totalBytes,
		AbsoluteSyncFolders: normalizedAbsFolders,
	}, nil
}
