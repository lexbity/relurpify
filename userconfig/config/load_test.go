package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestLoadDiagnosticKeepsPartialProfilesAndLoadStaysStrictOnSharedBundle(t *testing.T) {
	workspace := t.TempDir()
	copyDir(t, filepath.Join("..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))
	require.NoError(t, fs.MkdirAllSecure(filepath.Join(workspace, "relurpify_cfg", "model", "profiles")))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(workspace, "relurpify_cfg", "model", "profiles", "broken.llm.yaml"), []byte(`schema: relurpify/model/profile/v1
pattern: "broken*"
tool_calling:
  intent: native
context:
  max_tokens: 0
generation:
  temperature: 0.2
  top_p: 0.9
`)))

	diagBundle, diags, err := LoadDiagnostic(LoadOptions{WorkspaceRoot: workspace})
	require.NoError(t, err)
	require.NotNil(t, diagBundle.Config)
	require.NotNil(t, diagBundle.Secrets)
	require.NotEmpty(t, diagBundle.Config.Model.Profiles)
	require.True(t, hasDiagnostic(diags, "profile", "warning", "broken.llm.yaml"))

	cfg, secrets, err := Load(LoadOptions{WorkspaceRoot: workspace})
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, secrets)
	require.Len(t, cfg.Model.Profiles, len(diagBundle.Config.Model.Profiles))
}

func TestLoadDiagnosticMatchesLoadOnCleanWorkspace(t *testing.T) {
	workspace := t.TempDir()
	copyDir(t, filepath.Join("..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	diagBundle, diags, err := LoadDiagnostic(LoadOptions{WorkspaceRoot: workspace})
	require.NoError(t, err)
	require.NotNil(t, diagBundle.Config)
	require.NotNil(t, diagBundle.Secrets)
	require.Empty(t, diags)

	cfg, secrets, err := Load(LoadOptions{WorkspaceRoot: workspace})
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, secrets)
	require.Len(t, cfg.Model.Profiles, len(diagBundle.Config.Model.Profiles))
}

func TestLoadDiagnosticReturnsPartialBundleForBrokenDefaultProfile(t *testing.T) {
	workspace := t.TempDir()
	copyDir(t, filepath.Join("..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(workspace, "relurpify_cfg", "model", "profiles", "default.llm.yaml"), []byte(`schema: relurpify/model/profile/v1
pattern: ""
tool_calling:
  intent: auto
context:
  max_tokens: 0
generation:
  temperature: 0.2
  top_p: 0.95
`)))

	diagBundle, diags, err := LoadDiagnostic(LoadOptions{WorkspaceRoot: workspace})
	require.NoError(t, err)
	require.NotNil(t, diagBundle.Config)
	require.NotNil(t, diagBundle.Secrets)
	require.True(t, hasDiagnostic(diags, "profile", "blocking", "default.llm.yaml"))
	require.NotEmpty(t, diagBundle.Config.Model.Profiles)

	cfg, _, err := Load(LoadOptions{WorkspaceRoot: workspace})
	require.Error(t, err)
	require.Nil(t, cfg)
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return fs.MkdirAllSecure(target)
		}
		data, err := ReadFileRaw(path)
		if err != nil {
			return err
		}
		return fs.WriteFileSecure(target, data)
	})
	require.NoError(t, err)
}

func hasDiagnostic(diags []ConfigDiagnostic, section, severity, pathFragment string) bool {
	for _, diag := range diags {
		if strings.EqualFold(diag.Section, section) && strings.EqualFold(diag.Severity, severity) && strings.Contains(diag.Path, pathFragment) {
			return true
		}
	}
	return false
}
