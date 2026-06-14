package testhelper

import (
	"fmt"
	"os"
	"path/filepath"
	"os/exec"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

// WorkspaceOpts controls how a test workspace fixture is materialized.
type WorkspaceOpts struct {
	Provider  string
	SeedFiles map[string]string
	Recipes   map[string]string
	// CliGitExec controls the cli_git execute policy written to
	// relurpify_cfg/security/localtool.policy.yaml.
	// Default: "allow" | "ask" | "deny".
	CliGitExec string
}

// WriteCleanWorkspace copies the checked-in relurpify_cfg tree into workspace,
// rewrites the workspace provider, writes the split localtool policy from
// opts.CliGitExec, and adds any requested seed files and recipes.
// No per-agent YAML manifest is written.
func WriteCleanWorkspace(t *testing.T, workspace string, opts WorkspaceOpts) {
	t.Helper()

	provider := strings.TrimSpace(opts.Provider)
	if provider == "" {
		provider = "offline"
	}

	repoRoot := RepoRoot(t)
	copyTree(t, filepath.Join(repoRoot, "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	// Write workspace.yaml with the requested provider.
	wsPath := filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")
	wsContent := fmt.Sprintf(`schema: relurpify/workspace/v1

paths:
  state_dir: .relurpify_state

model:
  provider: %s
  name: gemma4:e4b

sandbox:
  backend: gvisor

logging:
  level: info
  format: json

audit:
  retention_days: 7

telemetry:
  enabled: false
`, provider)
	if err := fs.WriteFileSecure(wsPath, []byte(wsContent)); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	// Write the split localtool policy.
	cliGitExec := strings.TrimSpace(opts.CliGitExec)
	if cliGitExec == "" {
		cliGitExec = "allow"
	}
	policyDir := filepath.Join(workspace, "relurpify_cfg", "security")
	if err := os.MkdirAll(policyDir, fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir security dir: %v", err)
	}
	policyContent := fmt.Sprintf(`schema: relurpify/policy/localtool/v1

tools:
  cli_git:
    execute: %s
  bash:
    execute: ask
`, cliGitExec)
	policyPath := filepath.Join(policyDir, "localtool.policy.yaml")
	if err := fs.WriteFileSecure(policyPath, []byte(policyContent)); err != nil {
		t.Fatalf("write localtool policy: %v", err)
	}

	for name, content := range opts.SeedFiles {
		name = strings.TrimSpace(name)
		if name == "" {
			t.Fatal("seed file name required")
		}
		path := filepath.Join(workspace, filepath.Clean(name))
		if err := os.MkdirAll(filepath.Dir(path), fs.PublicDirMode); err != nil {
			t.Fatalf("mkdir seed dir for %s: %v", name, err)
		}
		if err := fs.WriteFileSecure(path, []byte(content)); err != nil {
			t.Fatalf("write seed file %s: %v", name, err)
		}
	}

	if len(opts.Recipes) > 0 {
		recipeRoot := filepath.Join(workspace, "relurpify_cfg", "euclo")
		if err := os.MkdirAll(recipeRoot, fs.PublicDirMode); err != nil {
			t.Fatalf("mkdir recipe root: %v", err)
		}
		for name, content := range opts.Recipes {
			name = strings.TrimSpace(name)
			if name == "" {
				t.Fatal("recipe file name required")
			}
			path := filepath.Join(recipeRoot, filepath.Clean(name))
			if err := os.MkdirAll(filepath.Dir(path), fs.PublicDirMode); err != nil {
				t.Fatalf("mkdir recipe dir for %s: %v", name, err)
			}
			if err := fs.WriteFileSecure(path, []byte(content)); err != nil {
				t.Fatalf("write recipe %s: %v", name, err)
			}
		}
	}
}

// WriteCleanWorkspaceAsk is a convenience wrapper that writes the workspace
// with cli_git: execute: ask.
func WriteCleanWorkspaceAsk(t *testing.T, workspace string, opts WorkspaceOpts) {
	t.Helper()
	opts.CliGitExec = "ask"
	WriteCleanWorkspace(t, workspace, opts)
}

// InitGitRepo initializes a git repository in workspace and creates an initial commit.
func InitGitRepo(t *testing.T, workspace string) {
	t.Helper()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workspace
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	run("init")
	run("config", "user.email", "slice4@example.com")
	run("config", "user.name", "Slice Four")
	run("add", ".")
	run("commit", "-m", "initial workspace")
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()

	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read source tree %s: %v", src, err)
	}
	if err := os.MkdirAll(dst, fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir target tree %s: %v", dst, err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyTree(t, srcPath, dstPath)
			continue
		}
		data, err := os.ReadFile(filepath.Clean(srcPath))
		if err != nil {
			t.Fatalf("read source file %s: %v", srcPath, err)
		}
		if err := fs.WriteFileSecure(dstPath, data); err != nil {
			t.Fatalf("write target file %s: %v", dstPath, err)
		}
	}
}
