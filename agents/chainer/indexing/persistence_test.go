package indexing_test

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents/chainer/indexing"
)

func TestPersistedCodeIndexStore_SaveAndLoadSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)

	snapshot := &indexing.CodeIndexSnapshot{
		SnapshotID:    "snap-1",
		TaskID:        "task-1",
		Timestamp:     time.Now(),
		LinkName:      "link-1",
		TotalSnippets: 1,
		ErrorCount:    0,
		SuccessCount:  1,
		Snippets: []*indexing.IndexedCodeSnippet{
			{
				ID:       "snippet-1",
				Source:   "code",
				Language: "go",
			},
		},
	}

	err := store.SaveSnapshot(snapshot)
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	loaded, err := store.LoadSnapshot("task-1", "snap-1")
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}

	if loaded.SnapshotID != "snap-1" {
		t.Errorf("snapshot ID not preserved")
	}

	if len(loaded.Snippets) != 1 {
		t.Errorf("expected 1 snippet, got %d", len(loaded.Snippets))
	}
}

func TestPersistedCodeIndexStore_SaveNilSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)

	err := store.SaveSnapshot(nil)
	if err == nil {
		t.Fatal("expected error for nil snapshot")
	}
}

func TestPersistedCodeIndexStore_SaveSnapshotNoID(t *testing.T) {
	tmpDir := t.TempDir()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)

	snapshot := &indexing.CodeIndexSnapshot{
		TaskID: "task-1",
	}

	err := store.SaveSnapshot(snapshot)
	if err == nil {
		t.Fatal("expected error for snapshot without ID")
	}
}

func TestPersistedCodeIndexStore_LoadMissingSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)

	_, err := store.LoadSnapshot("task-1", "missing")
	if err == nil {
		t.Fatal("expected error for missing snapshot")
	}
}

func TestPersistedCodeIndexStore_ListSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)

	// Save multiple snapshots
	for i := 1; i <= 3; i++ {
		snapshot := &indexing.CodeIndexSnapshot{
			SnapshotID: "snap-" + string(rune(i+'0'-1)),
			TaskID:     "task-1",
			Timestamp:  time.Now(),
		}
		store.SaveSnapshot(snapshot)
	}

	snapshots, err := store.ListSnapshots("task-1")
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snapshots) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(snapshots))
	}
}

func TestPersistedCodeIndexStore_ListEmptySnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)

	snapshots, err := store.ListSnapshots("task-1")
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snapshots))
	}
}

func TestPersistedCodeIndexStore_DeleteSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)

	snapshot := &indexing.CodeIndexSnapshot{
		SnapshotID: "snap-1",
		TaskID:     "task-1",
	}

	store.SaveSnapshot(snapshot)

	err := store.DeleteSnapshot("task-1", "snap-1")
	if err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	_, err = store.LoadSnapshot("task-1", "snap-1")
	if err == nil {
		t.Fatal("snapshot should be deleted")
	}
}

func TestPersistedCodeIndexStore_DeleteMissingSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)

	// Should not error on missing snapshot
	err := store.DeleteSnapshot("task-1", "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPersistenceManager_CreateSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	idx := indexing.NewCodeIndex()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)
	pm := indexing.NewPersistenceManager(idx, store, "task-1")

	// Index a snippet
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-1",
		Source:   "code",
		Language: "go",
	})

	snapID, err := pm.CreateSnapshot("link-1")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if snapID == "" {
		t.Fatal("expected snapshot ID")
	}

	// Verify snapshot was saved
	snap, err := store.LoadSnapshot("task-1", snapID)
	if err != nil {
		t.Fatalf("snapshot not found: %v", err)
	}

	if len(snap.Snippets) != 1 {
		t.Errorf("expected 1 snippet in snapshot, got %d", len(snap.Snippets))
	}
}

func TestPersistenceManager_RestoreSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	idx := indexing.NewCodeIndex()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)
	pm := indexing.NewPersistenceManager(idx, store, "task-1")

	// Create and save a snapshot
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-1",
		Source: "code1",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-2",
		Source: "code2",
	})

	snapID, _ := pm.CreateSnapshot("link-1")

	// Clear the index
	idx.Clear()
	if idx.Count() != 0 {
		t.Fatal("index should be empty")
	}

	// Restore from snapshot
	err := pm.RestoreSnapshot(snapID)
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	if idx.Count() != 2 {
		t.Errorf("expected 2 snippets after restore, got %d", idx.Count())
	}
}

func TestPersistenceManager_LatestSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	idx := indexing.NewCodeIndex()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)
	pm := indexing.NewPersistenceManager(idx, store, "task-1")

	// Create multiple snapshots
	idx.Index(&indexing.IndexedCodeSnippet{ID: "snippet-1", Source: "code"})
	_, _ = pm.CreateSnapshot("link-1")

	time.Sleep(10 * time.Millisecond) // Ensure different timestamp

	idx.Index(&indexing.IndexedCodeSnippet{ID: "snippet-2", Source: "code"})
	snap2, _ := pm.CreateSnapshot("link-2")

	latest, err := pm.LatestSnapshot()
	if err != nil {
		t.Fatalf("LatestSnapshot failed: %v", err)
	}

	if latest != snap2 {
		t.Errorf("expected latest to be %s, got %s", snap2, latest)
	}
}

func TestPersistenceManager_ListAllSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	idx := indexing.NewCodeIndex()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)
	pm := indexing.NewPersistenceManager(idx, store, "task-1")

	// Create multiple snapshots
	for i := 0; i < 3; i++ {
		idx.Index(&indexing.IndexedCodeSnippet{
			ID:     string(rune('a' + i)),
			Source: "code",
		})
		pm.CreateSnapshot("link")
		time.Sleep(5 * time.Millisecond)
	}

	snapshots, err := pm.ListAllSnapshots()
	if err != nil {
		t.Fatalf("ListAllSnapshots failed: %v", err)
	}

	if len(snapshots) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(snapshots))
	}
}

func TestPersistenceManager_NilManager(t *testing.T) {
	var pm *indexing.PersistenceManager

	_, err := pm.CreateSnapshot("link")
	if err == nil {
		t.Fatal("expected error for nil manager")
	}

	err = pm.RestoreSnapshot("snap")
	if err == nil {
		t.Fatal("expected error for nil manager")
	}

	_, err = pm.LatestSnapshot()
	if err == nil {
		t.Fatal("expected error for nil manager")
	}

	_, err = pm.ListAllSnapshots()
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestReportGenerator_GenerateExecutionReport(t *testing.T) {
	tmpDir := t.TempDir()
	idx := indexing.NewCodeIndex()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)
	pm := indexing.NewPersistenceManager(idx, store, "task-1")

	// Add snippets and create snapshots
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-1",
		Source:   "code1",
		Language: "go",
		FilePath: "/src/main.go",
	})
	pm.CreateSnapshot("link-1")

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-2",
		Source:   "code2",
		Language: "python",
		FilePath: "/src/main.py",
	})
	pm.CreateSnapshot("link-2")

	rg := indexing.NewReportGenerator(idx, store)
	report, err := rg.GenerateExecutionReport("task-1")
	if err != nil {
		t.Fatalf("GenerateExecutionReport failed: %v", err)
	}

	if report.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", report.TaskID)
	}

	if report.SnapshotCount != 2 {
		t.Errorf("expected 2 snapshots, got %d", report.SnapshotCount)
	}

	if report.TotalSnippets == 0 {
		t.Fatal("expected total snippets > 0")
	}
}

func TestReportGenerator_GenerateReportNoSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	idx := indexing.NewCodeIndex()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)

	rg := indexing.NewReportGenerator(idx, store)
	report, err := rg.GenerateExecutionReport("task-1")
	if err != nil {
		t.Fatalf("GenerateExecutionReport failed: %v", err)
	}

	if report == nil {
		t.Fatal("expected report")
	}

	if report.SnapshotCount != 0 {
		t.Errorf("expected 0 snapshots, got %d", report.SnapshotCount)
	}
}

func TestReportGenerator_FormatReport(t *testing.T) {
	report := &indexing.ExecutionReport{
		TaskID:          "task-1",
		GeneratedAt:     time.Now(),
		TotalSnippets:   10,
		ErrorSnippets:   2,
		SuccessSnippets: 8,
		SnapshotCount:   3,
		UniqueLanguages: map[string]int{"go": 6, "python": 4},
		UniqueFiles:     map[string]int{"/src/main.go": 6, "/src/util.py": 4},
	}

	rg := indexing.NewReportGenerator(nil, nil)
	formatted := rg.FormatReport(report)

	if formatted == "" {
		t.Fatal("expected formatted report")
	}

	if !contains(formatted, "task-1") {
		t.Fatal("report missing task ID")
	}

	if !contains(formatted, "Total Snippets:   10") {
		t.Fatal("report missing snippet count")
	}

	if !contains(formatted, "go:") {
		t.Fatal("report missing language")
	}
}

func TestReportGenerator_FormatNilReport(t *testing.T) {
	rg := indexing.NewReportGenerator(nil, nil)
	formatted := rg.FormatReport(nil)

	if formatted != "" {
		t.Fatal("expected empty string for nil report")
	}
}

func TestSnapshotErrorTracking(t *testing.T) {
	tmpDir := t.TempDir()
	idx := indexing.NewCodeIndex()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)
	pm := indexing.NewPersistenceManager(idx, store, "task-1")

	// Add success and error snippets
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:      "snippet-1",
		Source:  "good code",
		IsError: false,
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:           "snippet-2",
		Source:       "bad code",
		IsError:      true,
		ErrorMessage: "syntax error",
	})

	snapID, _ := pm.CreateSnapshot("link-1")
	snap, _ := store.LoadSnapshot("task-1", snapID)

	if snap.TotalSnippets != 2 {
		t.Errorf("expected 2 total snippets, got %d", snap.TotalSnippets)
	}

	if snap.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", snap.ErrorCount)
	}

	if snap.SuccessCount != 1 {
		t.Errorf("expected 1 success, got %d", snap.SuccessCount)
	}
}

func TestSnapshotIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	idx := indexing.NewCodeIndex()
	store := indexing.NewPersistedCodeIndexStore(tmpDir)
	pm := indexing.NewPersistenceManager(idx, store, "task-1")

	// Create first snapshot
	idx.Index(&indexing.IndexedCodeSnippet{ID: "snippet-1", Source: "code1"})
	snap1ID, _ := pm.CreateSnapshot("link-1")

	// Add more and create second snapshot
	idx.Index(&indexing.IndexedCodeSnippet{ID: "snippet-2", Source: "code2"})
	snap2ID, _ := pm.CreateSnapshot("link-2")

	// Load both snapshots and verify they're independent
	snap1, _ := store.LoadSnapshot("task-1", snap1ID)
	snap2, _ := store.LoadSnapshot("task-1", snap2ID)

	if snap1 == nil || len(snap1.Snippets) != 1 {
		t.Errorf("snap1 should have 1 snippet, has %d", len(snap1.Snippets))
	}

	if len(snap2.Snippets) != 2 {
		t.Errorf("snap2 should have 2 snippets, has %d", len(snap2.Snippets))
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
