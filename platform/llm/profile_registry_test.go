package llm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileRegistryResolve_Precendence(t *testing.T) {
	dir := t.TempDir()
	writeProfileTestFile(t, dir, "default.yaml", `pattern: "*"
repair:
  strategy: heuristic-only
`)
	writeProfileTestFile(t, dir, "generic.yaml", `model: "model-a"
repair:
  strategy: llm
`)
	writeProfileTestFile(t, dir, "family.yaml", `pattern: "qwen2.5-coder*"
repair:
  strategy: llm
`)
	writeProfileTestFile(t, dir, "provider.yaml", `provider: "openai-compat"
model: "model-a"
repair:
  strategy: heuristic-only
`)

	reg, err := NewProfileRegistry(dir)
	require.NoError(t, err)

	res := reg.Resolve("openai-compat", "model-a")
	require.Equal(t, "provider.yaml", filepath.Base(res.SourcePath))
	require.Equal(t, "provider/model exact match for openai-compat/model-a", res.Reason)

	res = reg.Resolve("", "model-a")
	require.Equal(t, "generic.yaml", filepath.Base(res.SourcePath))
	require.Equal(t, "exact model match for model-a", res.Reason)

	res = reg.Resolve("", "qwen2.5-coder-7b")
	require.Equal(t, "family.yaml", filepath.Base(res.SourcePath))
	require.Equal(t, "glob match for qwen2.5-coder-7b", res.Reason)

	res = reg.Resolve("", "missing")
	require.Equal(t, "default.yaml", filepath.Base(res.SourcePath))
	require.Equal(t, "default profile from default.yaml", res.Reason)
}

func TestProfileRegistryResolve_BuiltinDefault(t *testing.T) {
	reg, err := NewProfileRegistry(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)

	res := reg.Resolve("", "missing")
	require.Equal(t, "builtin-default", res.MatchKind)
	require.NotNil(t, res.Profile)
	require.Equal(t, "*", res.Profile.Pattern)
}

func writeProfileTestFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}
