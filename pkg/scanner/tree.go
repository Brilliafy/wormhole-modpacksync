package scanner

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// Scan limits. The tree in the sender UI is a picker, not a manifest: it only
// needs enough depth to pick folders. Fully walking huge instance trees (some
// mods ship 100k+ data files, e.g. mods/OpenTerrainGenerator/worlds) over
// FUSE/NTFS mounts takes tens of seconds. Anything past these limits is marked
// Truncated and stays selectable as a whole folder, and the real manifest is
// still built by walking the selection at StartSender time.
const (
	maxTreeDepth    = 4     // root = 0, so files are listed 4 levels deep
	maxDirEntries   = 4000  // a single directory with more entries is not expanded
	maxTreeNodes    = 60000 // hard ceiling on scanned entries
	maxScanDuration = 2500 * time.Millisecond
)

type scanJob struct {
	abs   string
	node  *models.FileTreeNode
	depth int
}

// BuildFileTree scans a directory and produces a hierarchical FileTreeNode for
// the sender UI. The walk is breadth-first, parallel across directories, and
// bounded by depth, entry count and wall-clock limits so picking a folder stays
// fast on slow removable/network filesystems.
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

	var scanned int64
	deadline := time.Now().Add(maxScanDuration)

	level := []scanJob{{abs: rootPath, node: rootNode, depth: 0}}
	for len(level) > 0 {
		var next []scanJob
		var mu sync.Mutex

		workers := runtime.NumCPU() * 2
		if workers < 4 {
			workers = 4
		}
		if workers > len(level) {
			workers = len(level)
		}

		jobs := make(chan scanJob)
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range jobs {
					children := scanDir(j, deadline, &scanned)
					if len(children) > 0 {
						mu.Lock()
						next = append(next, children...)
						mu.Unlock()
					}
				}
			}()
		}
		for _, j := range level {
			jobs <- j
		}
		close(jobs)
		wg.Wait()

		level = next
	}

	// Sort children (directories first, then alphabetically) and compute directory sizes
	sortAndComputeSizes(rootNode)

	return rootNode, nil
}

// scanDir fills job.node.Children and returns the child directories that still
// need scanning. Every node is written only by the goroutine that owns it.
func scanDir(j scanJob, deadline time.Time, scanned *int64) []scanJob {
	if j.depth >= maxTreeDepth ||
		time.Now().After(deadline) ||
		atomic.LoadInt64(scanned) >= maxTreeNodes {
		j.node.Truncated = true
		return nil
	}

	f, err := os.Open(j.abs)
	if err != nil {
		return nil // Unreadable directories are skipped gracefully
	}
	defer f.Close()

	// Read one entry past the cap so oversize directories are detected without
	// enumerating all 100k+ of their children.
	entries, err := f.ReadDir(maxDirEntries + 1)
	if err != nil && err != io.EOF {
		return nil
	}
	if len(entries) > maxDirEntries {
		j.node.Truncated = true
		return nil
	}

	atomic.AddInt64(scanned, int64(len(entries)))

	dirRel := j.node.RelPath
	firstComponent := dirRel
	if idx := strings.Index(dirRel, "/"); idx >= 0 {
		firstComponent = dirRel[:idx]
	}

	children := make([]*models.FileTreeNode, 0, len(entries))
	childJobs := make([]scanJob, 0, 8)

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

		lowerName := strings.ToLower(name)
		checked := !(defaultExcludedNames[lowerName] || defaultExcludedNames[strings.ToLower(firstComponent)])

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
			childJobs = append(childJobs, scanJob{
				abs:   filepath.Join(j.abs, name),
				node:  child,
				depth: j.depth + 1,
			})
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

	j.node.Children = children
	return childJobs
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
