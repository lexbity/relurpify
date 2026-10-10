//go:build live
// +build live

package agenttest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	a_txt          = "a.txt"
	agents         = "agents"
	coding_go_yaml = "coding-go.yaml"
	manifest_yaml  = "manifest.yaml"
	templates_dir  = "templates"
	workspace      = "workspace"
)

func TestSnapshotAndDiffWorkspace(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	mustWrite := func(rel, content string) {
		path := filepath.Join(root, rel)
		if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFileSecure(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(a_txt, "one")
	mustWrite("skip/b.txt", "nope")
	before, err := SnapshotWorkspace(root, []string{"skip/**"})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(a_txt, "two")
	after, err := SnapshotWorkspace(root, []string{"skip/**"})
	if err != nil {
		t.Fatal(err)
	}
	changed := DiffSnapshots(before, after)
	if len(changed) != 1 || changed[0] != a_txt {
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

	profileRoot := filepath.Join(shared, templates_dir, testsuite, "default", config.DirName)
	agentTemplate := filepath.Join(shared, templates_dir, agents, coding_go_yaml)
	for _, dir := range []string{profileRoot, filepath.Dir(agentTemplate)} {
		if err := fs.MkdirAllSecure(dir); err != nil {
			t.Fatal(err)
		}
	}
	if err := fs.WriteFileSecure(filepath.Join(profileRoot, manifest_yaml), []byte("model: derived\n")); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(filepath.Join(profileRoot, agent_yaml), []byte("path: ${workspace}\n")); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(agentTemplate, []byte("path: ${workspace}\n")); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	if err := fs.WriteFileSecure(filepath.Join(target, "README.md"), []byte(workspace)); err != nil {
		t.Fatal(err)
	}
	if err := fs.MkdirAllSecure(filepath.Join(target, config.DirName)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(filepath.Join(target, config.DirName, manifest_yaml), []byte("model: live\n")); err != nil {
		t.Fatal(err)
	}

	derived := filepath.Join(t.TempDir(), "run", workspace)
	err := MaterializeDerivedWorkspace(
		target,
		derived,
		shared,
		"default",
		filepath.ToSlash(filepath.Join(config.DirName, manifest_yaml)),
		nil,
		[]SetupFileSpec{{Path: filepath.ToSlash(filepath.Join(config.DirName, manifest_yaml)), Content: "model: override\n"}},
	)
	if err != nil {
		t.Fatalf("MaterializeDerivedWorkspace() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(derived, "README.md")); err != nil {
		t.Fatalf("expected copied workspace file: %v", err)
	}
	configPath := filepath.Join(derived, config.DirName, manifest_yaml)
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read derived config: %v", err)
	}
	if string(configData) != "model: override\n" {
		t.Fatalf("derived config = %q", string(configData))
	}
	// The template profile is served from the embedded bundle: the derived
	// workspace must receive the full checked-in config tree, with ${workspace}
	// rendered to the derived root (regression guard for the empty-copy bug).
	for _, embedded := range []string{
		filepath.Join(config.DirName, "workspace.yaml"),
		filepath.Join(config.DirName, "security", "sandbox.policy.yaml"),
		filepath.Join(config.DirName, "model", "provider", "ollama.provider.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(derived, embedded)); err != nil {
			t.Fatalf("expected embedded template file in derived workspace: %v", err)
		}
	}
	toolManifest, err := os.ReadFile(filepath.Join(derived, config.DirName, "tools", "file", "file_read.tool.yaml"))
	if err != nil {
		t.Fatalf("read derived tool manifest: %v", err)
	}
	if !strings.Contains(string(toolManifest), filepath.ToSlash(derived)) {
		t.Fatalf("expected ${workspace} rendered to derived root in tool manifest, got:\n%s", toolManifest)
	}
	if _, err := os.Stat(filepath.Join(derived, ".relurpify_state", "logs")); err != nil {
		t.Fatalf("expected derived logs dir: %v", err)
	}
}

func TestApplyWorkspaceFilesUsesConfiguredFileMode(t *testing.T) {
	root := t.TempDir()

	err := applyWorkspaceFiles(root, root, []SetupFileSpec{{
		Path:    "bin/run.sh",
		Content: "#!/bin/sh\n",
		Mode:    "0755",
	}})
	if err != nil {
		t.Fatalf("applyWorkspaceFiles: %v", err)
	}

	info, err := os.Stat(filepath.Join(root, "bin", "run.sh"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("expected 0755 perms, got %#o", got)
	}
}
