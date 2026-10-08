package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"wormhole-sync/pkg/models"
)

// ScanSelectedFiles gathers selected files from rootPath, filters them, hashes them, and builds a SyncManifest
func ScanSelectedFiles(rootPath string, selectedPaths []string, onProgress func(done int, total int, current string)) (*models.SyncManifest, error) {
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
			fi, err := d.Info()
			if err != nil {
				return nil
			}

			candidateFiles = append(candidateFiles, models.FileInfo{
				RelPath:  relSlash,
				Size:     fi.Size(),
				ModTime:  fi.ModTime().Unix(),
				IsDir:    false,
				Checked:  true,
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Compute hashes in parallel
	hashedFiles, err := HashFilesParallel(rootPath, candidateFiles, onProgress)
	if err != nil {
		return nil, err
	}

	var totalBytes int64 = 0
	for _, f := range hashedFiles {
		totalBytes += f.Size
	}

	return &models.SyncManifest{
		InstanceName: filepath.Base(rootPath),
		CreatedAt:    time.Now().Unix(),
		Files:        hashedFiles,
		TotalFiles:   len(hashedFiles),
		TotalBytes:   totalBytes,
	}, nil
}
