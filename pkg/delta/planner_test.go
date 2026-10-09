package delta

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"wormhole-sync/pkg/models"
)

func TestPlanDelta(t *testing.T) {
	destDir := t.TempDir()

	// Create an existing identical file
	identicalContent := []byte("mod1 v1.0.0")
	hasher := sha256.New()
	hasher.Write(identicalContent)
	identicalHash := hex.EncodeToString(hasher.Sum(nil))

	_ = os.MkdirAll(filepath.Join(destDir, "mods"), 0755)
	_ = os.WriteFile(filepath.Join(destDir, "mods", "mod1.jar"), identicalContent, 0644)

	// Create an outdated file
	_ = os.WriteFile(filepath.Join(destDir, "mods", "mod2.jar"), []byte("mod2 old"), 0644)

	manifest := &models.SyncManifest{
		TotalFiles: 3,
		TotalBytes: 300,
		Files: []models.FileInfo{
			{
				RelPath: "mods/mod1.jar",
				Size:    int64(len(identicalContent)),
				Hash:    identicalHash,
			},
			{
				RelPath: "mods/mod2.jar",
				Size:    100,
				Hash:    "newmod2hash",
			},
			{
				RelPath: "mods/mod3.jar",
				Size:    100,
				Hash:    "newmod3hash",
			},
		},
	}

	plan, err := PlanDelta(destDir, manifest, nil)
	if err != nil {
		t.Fatalf("PlanDelta failed: %v", err)
	}

	if len(plan.FilesUpToDate) != 1 {
		t.Errorf("Expected 1 file up to date, got %d", len(plan.FilesUpToDate))
	}
	if plan.FilesUpToDate[0].RelPath != "mods/mod1.jar" {
		t.Errorf("Expected mod1.jar up to date, got %s", plan.FilesUpToDate[0].RelPath)
	}

	if len(plan.FilesToSync) != 2 {
		t.Errorf("Expected 2 files to sync, got %d", len(plan.FilesToSync))
	}
	if plan.SavedBytes != int64(len(identicalContent)) {
		t.Errorf("Expected saved bytes %d, got %d", len(identicalContent), plan.SavedBytes)
	}
}

func TestPlanDeltaAbsoluteSync(t *testing.T) {
	destDir := t.TempDir()

	// Set up receiver directories and files
	_ = os.MkdirAll(filepath.Join(destDir, "mods"), 0755)
	_ = os.MkdirAll(filepath.Join(destDir, "config"), 0755)

	// mod1.jar is kept
	_ = os.WriteFile(filepath.Join(destDir, "mods", "mod1.jar"), []byte("mod1 content"), 0644)
	// obsolete-mod.jar is extra in mods/ -> should be deleted
	_ = os.WriteFile(filepath.Join(destDir, "mods", "obsolete-mod.jar"), []byte("old mod to delete"), 0644)
	// obsolete-config.json is extra in config/ -> should be deleted
	_ = os.WriteFile(filepath.Join(destDir, "config", "obsolete-config.json"), []byte("old config to delete"), 0644)
	// important-root-file.txt is extra in root -> should NOT be touched!
	_ = os.WriteFile(filepath.Join(destDir, "important-root-file.txt"), []byte("do not touch me"), 0644)

	manifest := &models.SyncManifest{
		TotalFiles:          2,
		TotalBytes:          200,
		AbsoluteSyncFolders: []string{"mods", "config"},
		Files: []models.FileInfo{
			{
				RelPath: "mods/mod1.jar",
				Size:    int64(len("mod1 content")),
				Hash:    "dummy-hash",
			},
			{
				RelPath: "zeustemple.schematic",
				Size:    100,
				Hash:    "schematic-hash",
			},
		},
	}

	plan, err := PlanDelta(destDir, manifest, nil)
	if err != nil {
		t.Fatalf("PlanDelta failed: %v", err)
	}

	// Verify extra files detected in mods and config
	deleteMap := make(map[string]bool)
	for _, del := range plan.FilesToDelete {
		deleteMap[del] = true
	}

	if !deleteMap["mods/obsolete-mod.jar"] {
		t.Errorf("Expected mods/obsolete-mod.jar to be marked for deletion")
	}
	if !deleteMap["config/obsolete-config.json"] {
		t.Errorf("Expected config/obsolete-config.json to be marked for deletion")
	}
	if deleteMap["important-root-file.txt"] {
		t.Errorf("important-root-file.txt must NOT be marked for deletion")
	}
	if len(plan.FilesToDelete) != 2 {
		t.Errorf("Expected exactly 2 files to delete, got %d: %v", len(plan.FilesToDelete), plan.FilesToDelete)
	}
}
