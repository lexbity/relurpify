package configcheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateWorkspaceTreeRepoRoot(t *testing.T) {
	report := ValidateWorkspaceTree(repoRoot(t))
	require.False(t, report.HasErrors(), report.Error())
	// Agents are declared in workspace.yaml; relurpify_cfg/agents/ no longer exists.
	require.NoDirExists(t, filepath.Join(repoRoot(t), "relurpify_cfg", "agents"))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join("..", ".."))
}

func TestNoCommittedRuntimeArtifacts(t *testing.T) {
	root := repoRoot(t)
	cfgRoot := filepath.Join(root, "relurpify_cfg")

	err := filepath.WalkDir(cfgRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".jsonl") {
			t.Fatalf("runtime artifact committed under relurpify_cfg: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		if strings.Contains(content, "/home/") || strings.Contains(content, "/Users/") {
			t.Fatalf("absolute home path committed under relurpify_cfg: %s", path)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestFrameworkManifestDirectoryAbsent(t *testing.T) {
	_, err := os.Stat(filepath.Join(repoRoot(t), "framework", "manifest"))
	require.Error(t, err)
	require.True(t, os.IsNotExist(err), "expected framework/manifest to remain absent")
}

func TestValidateWorkspaceTreeCatchesBoundaryViolations(t *testing.T) {
	workspace := t.TempDir()

	// 1. Create a valid relurpify_cfg layout so semantic loading works
	cfgRoot := filepath.Join(workspace, "relurpify_cfg")
	require.NoError(t, os.MkdirAll(filepath.Join(cfgRoot, "security"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(cfgRoot, "model", "provider"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(cfgRoot, "model", "profiles"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(cfgRoot, "tools"), 0o755))

	// Write mock workspace.yaml
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "workspace.yaml"), []byte(`schema: relurpify/workspace/v1
model:
  provider: ollama
  name: qwen2.5-coder:14b
`), 0o644))

	// Write mock security policies
	policyContent := []byte(`schema: relurpify/security/sandbox/v1
protected_paths: []
`)
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "security", "sandbox.policy.yaml"), policyContent, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "security", "shell.policy.yaml"), []byte(`schema: relurpify/security/shell/v1`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "security", "localtool.policy.yaml"), []byte(`schema: relurpify/security/localtool/v1`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "security", "workspaceingestion.policy.yaml"), []byte(`schema: relurpify/security/workspaceingestion/v1`), 0o644))

	// 2. Create framework/cfgload directory to identify it as a codebase root
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "framework", "cfgload"), 0o755))

	// 3. Write a file with a direct environment violation (os.Getenv)
	violationDir := filepath.Join(workspace, "app", "relurpish")
	require.NoError(t, os.MkdirAll(violationDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(violationDir, "violation.go"), []byte(`package relurpish
import "os"
func get() string {
	return os.Getenv("SECRET_KEY")
}
`), 0o644))

	// 4. Run validation
	report := ValidateWorkspaceTree(workspace)
	require.True(t, report.HasErrors())

	errStr := report.Error()
	require.Contains(t, errStr, "direct environment boundary violation")
	require.Contains(t, errStr, "os.Getenv")
	require.Contains(t, errStr, "app/relurpish/violation.go")

	// 5. Clean environment violation and write a config file boundary violation (reading relurpify_cfg directly)
	require.NoError(t, os.WriteFile(filepath.Join(violationDir, "violation.go"), []byte(`package relurpish
import "os"
func read() {
	_, _ = os.ReadFile("relurpify_cfg/workspace.yaml")
}
`), 0o644))

	report2 := ValidateWorkspaceTree(workspace)
	require.True(t, report2.HasErrors())

	errStr2 := report2.Error()
	require.Contains(t, errStr2, "direct config path boundary violation")
	require.Contains(t, errStr2, "os.ReadFile")
	require.Contains(t, errStr2, "app/relurpish/violation.go")
}

// TestBoundaryFlagsLookupEnv asserts that os.LookupEnv outside allowed
// packages is flagged as a boundary violation.
func TestBoundaryFlagsLookupEnv(t *testing.T) {
	workspace := writeMinimalWorkspace(t)
	violationDir := filepath.Join(workspace, "app", "relurpish")
	require.NoError(t, os.MkdirAll(violationDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(violationDir, "violation.go"), []byte(`package relurpish
import "os"
func lookup() string {
	v, _ := os.LookupEnv("SECRET")
	return v
}
`), 0o644))

	report := ValidateWorkspaceTree(workspace)
	require.True(t, report.HasErrors())
	require.Contains(t, report.Error(), "direct environment boundary violation")
	require.Contains(t, report.Error(), "os.LookupEnv")
}

// TestBoundaryFlagsSetenv asserts that os.Setenv outside allowed packages
// is flagged as a boundary violation.
func TestBoundaryFlagsSetenv(t *testing.T) {
	workspace := writeMinimalWorkspace(t)
	violationDir := filepath.Join(workspace, "app", "relurpish")
	require.NoError(t, os.MkdirAll(violationDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(violationDir, "violation.go"), []byte(`package relurpish
import "os"
func set() {
	_ = os.Setenv("SECRET", "value")
}
`), 0o644))

	report := ValidateWorkspaceTree(workspace)
	require.True(t, report.HasErrors())
	require.Contains(t, report.Error(), "direct environment boundary violation")
	require.Contains(t, report.Error(), "os.Setenv")
}

// TestBoundaryFlagsConstBuiltConfigPath asserts that os.ReadFile with a
// const-built path (e.g. filepath.Join(ws, "relurpify_cfg", "file.yaml"))
// is flagged even though no single string literal contains "relurpify_cfg".
func TestBoundaryFlagsConstBuiltConfigPath(t *testing.T) {
	workspace := writeMinimalWorkspace(t)
	violationDir := filepath.Join(workspace, "app", "relurpish")
	require.NoError(t, os.MkdirAll(violationDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(violationDir, "violation.go"), []byte(`package relurpish
import (
	"os"
	"path/filepath"
)
func read() {
	_, _ = os.ReadFile(filepath.Join("/workspace", "relurpify_cfg", "workspace.yaml"))
}
`), 0o644))

	report := ValidateWorkspaceTree(workspace)
	require.True(t, report.HasErrors())
	require.Contains(t, report.Error(), "direct config path boundary violation")
	require.Contains(t, report.Error(), "os.ReadFile")
}

// TestTestsuiteExemptionIsPrefixNotSubstring asserts that only paths
// containing a "testsuite" directory component are exempt, not paths
// that happen to contain "testsuite" as part of a directory name.
func TestTestsuiteExemptionIsPrefixNotSubstring(t *testing.T) {
	workspace := writeMinimalWorkspace(t)
	violationDir := filepath.Join(workspace, "app", "relurpish")
	require.NoError(t, os.MkdirAll(violationDir, 0o755))
	// Create a file inside a directory that contains "testsuite" as a substring
	// but is not a "/testsuite/" path component.
	require.NoError(t, os.WriteFile(filepath.Join(violationDir, "violation.go"), []byte(`package relurpish
import "os"
func get() string {
	return os.Getenv("SECRET_KEY")
}
`), 0o644))

	report := ValidateWorkspaceTree(workspace)
	require.True(t, report.HasErrors())
}

// writeMinimalWorkspace creates a temporary workspace with a minimal
// relurpify_cfg layout and framework/cfgload directory so that the
// boundary auditor runs.
func writeMinimalWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	cfgRoot := filepath.Join(workspace, "relurpify_cfg")
	require.NoError(t, os.MkdirAll(filepath.Join(cfgRoot, "security"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(cfgRoot, "model", "provider"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(cfgRoot, "model", "profiles"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(cfgRoot, "tools"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "workspace.yaml"), []byte(`schema: relurpify/workspace/v1
model:
  provider: ollama
  name: qwen2.5-coder:14b
`), 0o644))
	policyContent := []byte(`schema: relurpify/security/sandbox/v1
protected_paths: []
`)
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "security", "sandbox.policy.yaml"), policyContent, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "security", "shell.policy.yaml"), []byte(`schema: relurpify/security/shell/v1`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "security", "localtool.policy.yaml"), []byte(`schema: relurpify/security/localtool/v1`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgRoot, "security", "workspaceingestion.policy.yaml"), []byte(`schema: relurpify/security/workspaceingestion/v1`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "framework", "cfgload"), 0o755))
	return workspace
}
