package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wormhole-sync/pkg/models"
)

func TestBuildFileTreeParallelCorrectness(t *testing.T) {
	src := t.TempDir()

	// Nested structure across multiple top-level dirs to exercise the
	// parallel walk: many files in mods, nested config, excluded saves.
	for _, d := range []string{"mods", "config/sub", "saves/world", "resourcepacks"} {
		if err := os.MkdirAll(filepath.Join(src, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	wantFiles := map[string]int64{}
	for i := 0; i < 30; i++ {
		name := string(rune('a' + i))
		p := filepath.Join(src, "mods", "mod"+name+".jar")
		if err := os.WriteFile(p, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
		wantFiles["mods/mod"+name+".jar"] = 7
	}
	_ = os.WriteFile(filepath.Join(src, "config", "sub", "main.toml"), []byte("cfg"), 0644)
	wantFiles["config/sub/main.toml"] = 3
	_ = os.WriteFile(filepath.Join(src, "saves", "world", "level.dat"), []byte("world"), 0644)
	wantFiles["saves/world/level.dat"] = 5
	_ = os.WriteFile(filepath.Join(src, "options.txt"), []byte("opt"), 0644)
	wantFiles["options.txt"] = 3

	root, err := BuildFileTree(src)
	if err != nil {
		t.Fatalf("BuildFileTree failed: %v", err)
	}

	gotFiles := map[string]int64{}
	var visit func(n *models.FileTreeNode)
	visit = func(n *models.FileTreeNode) {
		if !n.IsDir {
			gotFiles[n.RelPath] = n.Size
		}
		for _, c := range n.Children {
			visit(c)
		}
	}
	visit(root)

	if len(gotFiles) != len(wantFiles) {
		t.Fatalf("file count mismatch: got %d want %d", len(gotFiles), len(wantFiles))
	}
	for path, size := range wantFiles {
		if got, ok := gotFiles[path]; !ok || got != size {
			t.Errorf("file %q: got (%d, %v), want size %d", path, got, ok, size)
		}
	}

	// Excluded entries must still be present but unchecked
	var find func(n *models.FileTreeNode, rel string) *models.FileTreeNode
	find = func(n *models.FileTreeNode, rel string) *models.FileTreeNode {
		if n.RelPath == rel {
			return n
		}
		for _, c := range n.Children {
			if hit := find(c, rel); hit != nil {
				return hit
			}
		}
		return nil
	}

	savesDir := find(root, "saves")
	if savesDir == nil {
		t.Fatal("saves dir missing from tree")
	}
	if savesDir.Checked {
		t.Error("saves dir should default to unchecked")
	}
	optionsFile := find(root, "options.txt")
	if optionsFile == nil {
		t.Fatal("options.txt missing from tree")
	}
	if optionsFile.Checked {
		t.Error("options.txt should default to unchecked")
	}

	// mods/config should be flagged for absolute sync by default
	modsDir := find(root, "mods")
	if modsDir == nil || !modsDir.AbsoluteSync {
		t.Error("mods dir should have AbsoluteSync enabled")
	}
}

func TestBuildFileTreeEmptyDir(t *testing.T) {
	root, err := BuildFileTree(t.TempDir())
	if err != nil {
		t.Fatalf("BuildFileTree failed: %v", err)
	}
	if len(root.Children) != 0 {
		t.Errorf("expected no children, got %d", len(root.Children))
	}
}

func TestBuildFileTreeStopsAtDepthAndMarksTruncated(t *testing.T) {
	src := t.TempDir()

	// A chain deeper than maxTreeDepth: root/mods/deep/a/b/c/file.jar
	deep := filepath.Join(src, "mods", "deep", "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "file.jar"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	root, err := BuildFileTree(src)
	if err != nil {
		t.Fatalf("BuildFileTree failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("shallow scan took %s, expected well under 3s", elapsed)
	}

	var find func(n *models.FileTreeNode, rel string) *models.FileTreeNode
	find = func(n *models.FileTreeNode, rel string) *models.FileTreeNode {
		if n.RelPath == rel {
			return n
		}
		for _, c := range n.Children {
			if hit := find(c, rel); hit != nil {
				return hit
			}
		}
		return nil
	}

	node := root
	names := []string{"mods", "deep", "a", "b"}
	for i := 0; i < len(names); i++ {
		var next *models.FileTreeNode
		for _, c := range node.Children {
			if c.Name == names[i] {
				next = c
				break
			}
		}
		if next == nil {
			t.Fatalf("missing node %q at level %d", names[i], i+1)
		}
		node = next
	}
	if !node.Truncated {
		t.Errorf("node at depth %d should be marked Truncated", len(names))
	}
	if len(node.Children) != 0 {
		t.Errorf("truncated node should not have children, got %d", len(node.Children))
	}
}

func TestBuildFileTreeMarksOversizeDirTruncated(t *testing.T) {
	src := t.TempDir()
	bulk := filepath.Join(src, "worlds")
	if err := os.MkdirAll(bulk, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= maxDirEntries; i++ {
		if err := os.WriteFile(filepath.Join(bulk, fmt.Sprintf("f%05d.dat", i)), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	root, err := BuildFileTree(src)
	if err != nil {
		t.Fatalf("BuildFileTree failed: %v", err)
	}
	if len(root.Children) != 1 || !root.Children[0].Truncated {
		t.Fatalf("oversize dir should be a single truncated child")
	}
}

func TestScanSelectedFilesAcceptsWholeFolder(t *testing.T) {
	src := t.TempDir()
	deep := filepath.Join(src, "mods", "big", "nested", "deeper")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(src, "mods", "a.jar"),
		filepath.Join(deep, "data.dat"),
		filepath.Join(src, "config", "skip.toml"),
	} {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	m, err := ScanSelectedFiles(context.Background(), src, []string{"mods/big"}, nil, nil)
	if err != nil {
		t.Fatalf("ScanSelectedFiles failed: %v", err)
	}
	if m.TotalFiles != 1 || m.Files[0].RelPath != "mods/big/nested/deeper/data.dat" {
		t.Fatalf("unexpected manifest: %+v", m.Files)
	}
}
