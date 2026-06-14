//go:build live
// +build live

package agenttest

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	a_txt          = "a.txt"
	agents         = "agents"
	coding_go_yaml = "coding-go.yaml"
	manifest_yaml  = "manifest.yaml"
	templates      = "templates"
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

	profileRoot := filepath.Join(shared, templates, testsuite, "default", config.DirName)
	agentTemplate := filepath.Join(shared, templates, agents, coding_go_yaml)
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
		filepath.ToSlash(filepath.Join(config.DirName, agents, coding_go_yaml)),
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
	agentPath := filepath.Join(derived, config.DirName, agents, coding_go_yaml)
	agentData, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read derived agent: %v", err)
	}
	if string(agentData) != "path: "+filepath.ToSlash(derived)+"\n" {
		t.Fatalf("derived agent = %q", string(agentData))
	}
	if _, err := os.Stat(filepath.Join(derived, ".relurpify_state", "logs")); err != nil {
		t.Fatalf("expected derived logs dir: %v", err)
	}
}

func TestMaterializeDerivedWorkspace(t *testing.T) {
	shared := t.TempDir()

	profileRoot := filepath.Join(shared, templates, testsuite, "default", config.DirName)
	if err := fs.MkdirAllSecure(profileRoot); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(filepath.Join(profileRoot, agent_yaml), []byte("name: ${workspace}\n")); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	manifestPath := filepath.Join(target, config.DirName, agent_yaml)
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(manifestPath, []byte(`schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: coding
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  agent:
    implementation: coding
    mode: primary
    model:
      provider: ollama
      name: test-model
  defaults:
    permissions:
      filesystem:
        - action: fs:read
          path: /tmp/**
          justification: Read workspace
`)); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadDocument(manifestPath); err != nil {
		t.Fatalf("LoadDocument: %v", err)
	}

	derived := filepath.Join(t.TempDir(), "run", workspace)
	if err := MaterializeDerivedWorkspace(
		target,
		derived,
		shared,
		"default",
		filepath.ToSlash(filepath.Join(config.DirName, agent_yaml)),
		nil,
		nil,
	); err != nil {
		t.Fatalf("MaterializeDerivedWorkspace() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(derived, config.DirName, "skills", "system", "skill.yaml")); err != nil {
		t.Fatalf("expected referenced skill to be copied into derived workspace: %v", err)
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
