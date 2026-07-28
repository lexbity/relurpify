package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// severityBlocking mirrors the diagnostic severity string used by loaders to
// signal a halting error. It is local to keep these tests self-contained.
const severityBlocking = "blocking"

func TestLoadFailsOnMalformedWorkspaceYAML(t *testing.T) {
	workspace := t.TempDir()
	copyDir(t, filepath.Join("..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "relurpify_cfg", "workspace.yaml"), []byte("schema: relurpify/workspace/v1\nmodel:\n  provider: ollama\nmalformed_key: [\n"), 0644)) //nolint:gosec // test fixture under temp dir

	_, _, err := Load(LoadOptions{WorkspaceRoot: workspace})
	require.Error(t, err, "Load must return error on malformed workspace.yaml")
}

func TestLoadFailsOnUnreadableWorkspaceYAML(t *testing.T) {
	workspace := t.TempDir()
	copyDir(t, filepath.Join("..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))
	require.NoError(t, os.Chmod(filepath.Join(workspace, "relurpify_cfg", "workspace.yaml"), 0000)) //nolint:gosec // test fixture intentionally made unreadable

	_, _, err := Load(LoadOptions{WorkspaceRoot: workspace})
	require.Error(t, err, "Load must return error on unreadable workspace.yaml")

	require.NoError(t, os.Chmod(filepath.Join(workspace, "relurpify_cfg", "workspace.yaml"), 0644)) //nolint:gosec // test fixture under temp dir
}

func TestLoadUsesDefaultsWhenWorkspaceAbsent(t *testing.T) {
	workspace := t.TempDir()
	copyDir(t, filepath.Join("..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))
	require.NoError(t, os.Remove(filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")))

	// When workspace.yaml is absent, Load uses defaults. Provider must be
	// specified via env override or the load fails with no provider specified.
	env := []string{"RELURPIFY_MODEL_PROVIDER=ollama", "RELURPIFY_MODEL_NAME=gemma4:e4b"}
	_, _, err := Load(LoadOptions{WorkspaceRoot: workspace, EnvOverrides: env})
	require.NoError(t, err, "Load must use defaults when workspace.yaml does not exist")
}

func TestLoadFailsOnInvalidWorkspaceSchema(t *testing.T) {
	workspace := t.TempDir()
	copyDir(t, filepath.Join("..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "relurpify_cfg", "workspace.yaml"), []byte("schema: relurpify/workspace/v1\nmodel:\n  provider: nonexistent-provider\nsandbox:\n  backend: invalid\n"), 0644)) //nolint:gosec // test fixture under temp dir

	_, _, err := Load(LoadOptions{WorkspaceRoot: workspace})
	require.Error(t, err, "Load must return error when workspace config sets invalid model provider")
}

func TestLoadDiagnosticReturnsPartialOnMalformedWorkspace(t *testing.T) {
	workspace := t.TempDir()
	copyDir(t, filepath.Join("..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "relurpify_cfg", "workspace.yaml"), []byte("schema: relurpify/workspace/v1\nmodel:\n  provider: ollama\nmalformed: [\n"), 0644)) //nolint:gosec // test fixture under temp dir

	bundle, diags, err := LoadDiagnostic(LoadOptions{WorkspaceRoot: workspace})
	require.NoError(t, err, "LoadDiagnostic must not return error on malformed workspace")
	require.NotNil(t, bundle.Config, "LoadDiagnostic must return partial config")
	require.NotNil(t, bundle.Secrets, "LoadDiagnostic must return secrets")
	require.NotEmpty(t, diags, "LoadDiagnostic must report blocking diagnostics")
	hasBlocking := false
	for _, d := range diags {
		if d.Severity == severityBlocking {
			hasBlocking = true
			break
		}
	}
	require.True(t, hasBlocking, "LoadDiagnostic must include blocking diagnostic for malformed workspace")
}
