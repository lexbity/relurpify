package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiagnosticPolicyReadsAreConfinedAndFailuresRemainPartial(t *testing.T) {
	for _, filename := range []string{"sandbox.policy.yaml", "shell.policy.yaml", "localtool.policy.yaml", "workspaceingestion.policy.yaml"} {
		t.Run(filename, func(t *testing.T) {
			workspace := t.TempDir()
			copyDir(t, filepath.Join("..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))
			path := filepath.Join(workspace, "relurpify_cfg", "security", filename)
			source, err := os.ReadFile(path)
			require.NoError(t, err)
			outside := filepath.Join(t.TempDir(), filename)
			require.NoError(t, os.WriteFile(outside, source, 0o600))
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Symlink(outside, path))
			bundle, diags, err := LoadDiagnostic(LoadOptions{WorkspaceRoot: workspace})
			require.NoError(t, err)
			require.NotNil(t, bundle.Config)
			require.NotEmpty(t, bundle.Config.Model.Profiles, "unsafe policy must not erase unrelated diagnostics")
			found := false
			for _, diag := range diags {
				if diag.Path == path && diag.Section == "security" && diag.Severity == "blocking" {
					found = true
				}
			}
			require.True(t, found, "unsafe policy was not a blocking diagnostic: %v", diags)
			switch filename {
			case "sandbox.policy.yaml":
				require.Nil(t, bundle.Config.Security.Sandbox)
			case "shell.policy.yaml":
				require.Nil(t, bundle.Config.Security.Shell)
			case "localtool.policy.yaml":
				require.Nil(t, bundle.Config.Security.LocalTool)
			case "workspaceingestion.policy.yaml":
				require.Nil(t, bundle.Config.Security.Ingestion)
			}
			data, err := os.ReadFile(outside)
			require.NoError(t, err)
			require.Equal(t, source, data)
		})
	}
}

func TestWorkspacePolicySourceAllowlistAndReadOnlyGuard(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "relurpify_cfg", "security"), 0o700))
	path := filepath.Join(workspace, "relurpify_cfg", "security", "shell.policy.yaml")
	source := "schema: relurpify/policy/shell/v1\nrules: []\n"
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	document, err := ReadGuardPolicy(t.Context(), workspace, GuardShell)
	require.NoError(t, err)
	require.Equal(t, source, document.Source)
	require.NotEmpty(t, document.Revision)
	_, err = ReadGuardPolicy(t.Context(), workspace, GuardIngestion)
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, name := range []string{"../shell.policy.yaml", path, "workspace.yaml", ""} {
		_, _, err := ReadWorkspacePolicySource(t.Context(), workspace, name)
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = ReadWorkspacePolicySource(ctx, workspace, "shell.policy.yaml")
	require.ErrorIs(t, err, context.Canceled)
	_, err = ParseGuardPolicy(GuardShell, strings.Repeat("x", PolicySourceLimit+1))
	require.Error(t, err)
	_, err = ParseGuardPolicy(GuardShell, "schema: relurpify/policy/ingestion/v1\nrules: []\n")
	require.Error(t, err)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "inspection created policy/backup/lock files")
}

func TestGuardInspectionRejectsAmbiguousRuleSelection(t *testing.T) {
	for _, rules := range []string{"- id: ''\n  pattern: test\n  action: block\n", "- id: same\n  pattern: first\n  action: block\n- id: same\n  pattern: second\n  action: block\n"} {
		_, err := ParseGuardPolicy(GuardShell, "schema: relurpify/policy/shell/v1\nrules:\n"+rules)
		require.ErrorContains(t, err, "nonempty and unique")
	}
}
