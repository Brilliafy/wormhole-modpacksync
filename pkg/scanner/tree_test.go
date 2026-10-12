package scanner

import (
	"os"
	"path/filepath"
	"testing"

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
