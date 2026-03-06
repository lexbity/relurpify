package agenttest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotAndDiffWorkspace(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	mustWrite := func(rel, content string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("a.txt", "one")
	mustWrite("skip/b.txt", "nope")
	before, err := SnapshotWorkspace(root, []string{"skip/**"})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite("a.txt", "two")
	after, err := SnapshotWorkspace(root, []string{"skip/**"})
	if err != nil {
		t.Fatal(err)
	}
	changed := DiffSnapshots(before, after)
	if len(changed) != 1 || changed[0] != "a.txt" {
		t.Fatalf("unexpected changed files: %v", changed)
	}
}

func TestFilterChangedFilesIgnoresGeneratedArtifacts(t *testing.T) {
	changed := []string{
		"pkg/file.go",
		"pkg/target/debug/app",
		"pkg/__pycache__/mod.cpython-313.pyc",
	}

	filtered := FilterChangedFiles(changed, []string{"**/target/**", "**/__pycache__/**"})

	if len(filtered) != 1 || filtered[0] != "pkg/file.go" {
		t.Fatalf("unexpected filtered files: %v", filtered)
	}
}
