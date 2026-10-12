package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"wormhole-sync/pkg/models"
)

// Default exclusions for Minecraft instances to avoid overwriting personal user data
var defaultExcludedNames = map[string]bool{
	"saves":              true,
	"screenshots":        true,
	"logs":               true,
	"crash-reports":      true,
	"options.txt":        true,
	"servers.dat":        true,
	"usercache.json":     true,
	"usernamecache.json": true,
	".fabric":            true,
	".forge":             true,
	".quilt":             true,
	".neoforge":          true,
	".git":               true,
	".DS_Store":          true,
	"Thumbs.db":          true,
}

// BuildFileTree recursively scans a directory and produces a hierarchical
// FileTreeNode. Directories are walked in parallel with a bounded worker pool,
// which makes large instances (tens of thousands of files) scan dramatically
// faster than a sequential walk.
func BuildFileTree(rootPath string) (*models.FileTreeNode, error) {
	if _, err := os.Stat(rootPath); err != nil {
		return nil, err
	}

	rootName := filepath.Base(rootPath)
	if rootName == "" || rootName == "." || rootName == "/" {
		rootName = "Instance Root"
	}

	rootNode := &models.FileTreeNode{
		ID:       "",
		Name:     rootName,
		RelPath:  "",
		IsDir:    true,
		Checked:  true,
		Children: make([]*models.FileTreeNode, 0),
	}

	workers := runtime.NumCPU() * 2
	if workers < 4 {
		workers = 4
	}
	sem := make(chan struct{}, workers)

	var wg sync.WaitGroup
	var walkDir func(dirAbs, dirRel string, node *models.FileTreeNode)

	// walkDir fills node.Children. Every node is written only by the
	// goroutine that owns it, so no extra locking is needed. Each level waits
	// for its direct child directories, and those wait for their own children,
	// so by the time the top-level call returns the whole tree is complete.
	walkDir = func(dirAbs, dirRel string, node *models.FileTreeNode) {
		defer wg.Done()

		entries, err := os.ReadDir(dirAbs)
		if err != nil {
			return // Unreadable directories are skipped gracefully
		}

		children := make([]*models.FileTreeNode, 0, len(entries))
		firstComponent := dirRel
		if idx := strings.Index(dirRel, "/"); idx >= 0 {
			firstComponent = dirRel[:idx]
		}

		for _, entry := range entries {
			name := entry.Name()

			// Skip hidden dotfiles like .git (except .minecraft)
			if strings.HasPrefix(name, ".") && name != ".minecraft" {
				continue
			}

			childRel := name
			if dirRel != "" {
				childRel = dirRel + "/" + name
			}

			checked := true
			lowerName := strings.ToLower(name)
			if defaultExcludedNames[lowerName] || defaultExcludedNames[strings.ToLower(firstComponent)] {
				checked = false
			}

			if entry.IsDir() {
				child := &models.FileTreeNode{
					ID:           childRel,
					Name:         name,
					RelPath:      childRel,
					IsDir:        true,
					Checked:      checked,
					AbsoluteSync: lowerName == "mods" || lowerName == "config",
					Children:     make([]*models.FileTreeNode, 0),
				}
				children = append(children, child)

				wg.Add(1)
				sem <- struct{}{}
				go func(c *models.FileTreeNode, abs string) {
					defer func() { <-sem }()
					walkDir(abs, c.RelPath, c)
				}(child, filepath.Join(dirAbs, name))
				continue
			}

			fi, err := entry.Info()
			if err != nil {
				continue
			}
			children = append(children, &models.FileTreeNode{
				ID:      childRel,
				Name:    name,
				RelPath: childRel,
				IsDir:   false,
				Size:    fi.Size(),
				Checked: checked,
			})
		}

		node.Children = children
	}

	wg.Add(1)
	walkDir(rootPath, "", rootNode)
	wg.Wait()

	// Sort children (directories first, then alphabetically) and compute directory sizes
	sortAndComputeSizes(rootNode)

	return rootNode, nil
}

// sortAndComputeSizes sorts tree nodes (directories first) and recursively sums sizes
func sortAndComputeSizes(node *models.FileTreeNode) int64 {
	if !node.IsDir {
		return node.Size
	}

	var totalSize int64 = 0
	for _, child := range node.Children {
		totalSize += sortAndComputeSizes(child)
	}
	node.Size = totalSize

	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsDir != node.Children[j].IsDir {
			return node.Children[i].IsDir // Dirs first
		}
		return strings.ToLower(node.Children[i].Name) < strings.ToLower(node.Children[j].Name)
	})

	return totalSize
}
