package testhelper

import (
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
}

// WriteCleanWorkspace copies the checked-in relurpify_cfg tree into workspace,
// rewrites the workspace provider, seeds the euclo manifest, and adds any
// requested root files.
func WriteCleanWorkspace(t *testing.T, workspace string, opts WorkspaceOpts) {
	t.Helper()

	provider := strings.TrimSpace(opts.Provider)
	if provider == "" {
		provider = "offline"
	}

	repoRoot := RepoRoot(t)
	copyTree(t, filepath.Join(repoRoot, "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	workspacePath := filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")
	workspaceData, err := os.ReadFile(filepath.Clean(workspacePath))
	if err != nil {
		t.Fatalf("read workspace config: %v", err)
	}
	updated := strings.Replace(string(workspaceData), "provider: ollama", "provider: "+provider, 1)
	if updated == string(workspaceData) {
		t.Fatalf("workspace provider rewrite did not change %s", workspacePath)
	}
	if err := fs.WriteFileSecure(workspacePath, []byte(updated)); err != nil {
		t.Fatalf("rewrite workspace config: %v", err)
	}

	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agents", "euclo.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifestData, err := os.ReadFile(filepath.Clean(filepath.Join(repoRoot, "userconfig", "config", "testdata", "contracts", "document_current.yaml")))
	if err != nil {
		t.Fatalf("read euclo manifest fixture: %v", err)
	}
	manifestText := string(manifestData)
	manifestText = strings.Replace(manifestText,
		"    hitl_required:\n      - filesystem\n",
		"    hitl_required: []\n",
		1,
	)
	manifestText = strings.Replace(manifestText,
		"    capabilities:\n      - capability: cap_net_bind_service\n",
		"    capabilities:\n      - capability: cap_net_bind_service\n",
		1,
	)
	manifestText = strings.Replace(manifestText,
		"path: ${workspace}/docs/**",
		"path: ${workspace}/**",
		1,
	)
	manifestText = strings.Replace(manifestText,
		`    executables:
      - binary: git
        args:
          - status
`,
		`    executables:
      - binary: git
        args:
          - "*"
`,
		1,
	)
	if !strings.Contains(manifestText, "binary: git") {
		t.Fatalf("euclo manifest git permission rewrite failed")
	}
	if err := fs.WriteFileSecure(manifestPath, []byte(manifestText)); err != nil {
		t.Fatalf("write euclo manifest: %v", err)
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
