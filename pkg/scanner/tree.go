package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wormhole-sync/pkg/models"
)

// Default exclusions for Minecraft instances to avoid overwriting personal user data
var defaultExcludedNames = map[string]bool{
	"saves":         true,
	"screenshots":   true,
	"logs":          true,
	"crash-reports": true,
	"options.txt":   true,
	"servers.dat":   true,
	"usercache.json":true,
	"usernamecache.json": true,
	".fabric":       true,
	".forge":        true,
	".quilt":        true,
	".neoforge":     true,
	".git":          true,
	".DS_Store":     true,
	"Thumbs.db":     true,
}

// BuildFileTree recursively scans a directory and produces a hierarchical FileTreeNode
func BuildFileTree(rootPath string) (*models.FileTreeNode, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
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
		Size:     info.Size(),
		Checked:  true,
		Children: make([]*models.FileTreeNode, 0),
	}

	dirMap := make(map[string]*models.FileTreeNode)
	dirMap[""] = rootNode

	err = filepath.WalkDir(rootPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // Skip unreadable entries gracefully
		}

		if path == rootPath {
			return nil
		}

		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil
		}

		// Normalize slash for cross-platform compatibility
		relSlash := filepath.ToSlash(rel)
		name := d.Name()

		// Skip hidden dotfiles like .git
		if strings.HasPrefix(name, ".") && name != ".minecraft" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		parentRel := filepath.ToSlash(filepath.Dir(rel))
		if parentRel == "." {
			parentRel = ""
		}

		parentNode, ok := dirMap[parentRel]
		if !ok {
			parentNode = rootNode
		}

		// Determine default checked state
		firstComponent := strings.Split(relSlash, "/")[0]
		checked := true
		if defaultExcludedNames[strings.ToLower(name)] || defaultExcludedNames[strings.ToLower(firstComponent)] {
			checked = false
		}

		absSync := false
		if d.IsDir() {
			lowerName := strings.ToLower(name)
			if lowerName == "mods" || lowerName == "config" {
				absSync = true
			}
		}

		node := &models.FileTreeNode{
			ID:           relSlash,
			Name:         name,
			RelPath:      relSlash,
			IsDir:        d.IsDir(),
			Checked:      checked,
			AbsoluteSync: absSync,
			Children:     make([]*models.FileTreeNode, 0),
		}

		if !d.IsDir() {
			fi, err := d.Info()
			if err == nil {
				node.Size = fi.Size()
			}
		} else {
			dirMap[relSlash] = node
		}

		parentNode.Children = append(parentNode.Children, node)
		return nil
	})

	if err != nil {
		return nil, err
	}

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
