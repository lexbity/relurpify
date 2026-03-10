package agenttest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/config"
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

func TestMaterializeDerivedWorkspaceCreatesIsolatedConfigFromTemplate(t *testing.T) {
	shared := t.TempDir()
	t.Setenv("RELURPIFY_SHARED_DIR", shared)

	profileRoot := filepath.Join(shared, "templates", "testsuite", "default", config.DirName)
	agentTemplate := filepath.Join(shared, "templates", "agents", "coding-go.yaml")
	for _, dir := range []string{profileRoot, filepath.Dir(agentTemplate)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(profileRoot, "config.yaml"), []byte("model: derived\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileRoot, "agent.manifest.yaml"), []byte("path: ${workspace}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentTemplate, []byte("path: ${workspace}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, config.DirName, "config.yaml"), []byte("model: live\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	derived := filepath.Join(t.TempDir(), "run", "workspace")
	err := MaterializeDerivedWorkspace(
		target,
		derived,
		"default",
		filepath.ToSlash(filepath.Join(config.DirName, "agents", "coding-go.yaml")),
		nil,
		[]SetupFileSpec{{Path: filepath.ToSlash(filepath.Join(config.DirName, "config.yaml")), Content: "model: override\n"}},
	)
	if err != nil {
		t.Fatalf("MaterializeDerivedWorkspace() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(derived, "README.md")); err != nil {
		t.Fatalf("expected copied workspace file: %v", err)
	}
	configPath := filepath.Join(derived, config.DirName, "config.yaml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read derived config: %v", err)
	}
	if string(configData) != "model: override\n" {
		t.Fatalf("derived config = %q", string(configData))
	}
	agentPath := filepath.Join(derived, config.DirName, "agents", "coding-go.yaml")
	agentData, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read derived agent: %v", err)
	}
	if string(agentData) != "path: "+filepath.ToSlash(derived)+"\n" {
		t.Fatalf("derived agent = %q", string(agentData))
	}
	if _, err := os.Stat(filepath.Join(derived, config.DirName, "logs")); err != nil {
		t.Fatalf("expected derived logs dir: %v", err)
	}
}
