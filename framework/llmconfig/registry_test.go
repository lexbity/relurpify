package llmconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadProfileRegistry_Precedence(t *testing.T) {
	dir := t.TempDir()
	writeProfileTestFile(t, dir, "default.llm.yaml", `schema: relurpify/model/profile/v1
pattern: "*"
tool_calling:
  intent: auto
context:
  max_tokens: 1024
generation:
  temperature: 0.2
  top_p: 0.9
`)
	writeProfileTestFile(t, dir, "generic.llm.yaml", `schema: relurpify/model/profile/v1
pattern: "model-a"
tool_calling:
  intent: native
context:
  max_tokens: 1024
generation:
  temperature: 0.2
  top_p: 0.9
`)
	writeProfileTestFile(t, dir, "family.llm.yaml", `schema: relurpify/model/profile/v1
pattern: "qwen2.5-coder*"
tool_calling:
  intent: native
context:
  max_tokens: 1024
generation:
  temperature: 0.2
  top_p: 0.9
`)

	reg, err := LoadProfileRegistry(dir)
	require.NoError(t, err)

	res := reg.Resolve("openai-compat", "model-a")
	require.Equal(t, "generic.llm.yaml", filepath.Base(res.SourcePath))
	require.Equal(t, "exact model match for model-a", res.Reason)

	res = reg.Resolve("", "model-a")
	require.Equal(t, "generic.llm.yaml", filepath.Base(res.SourcePath))
	require.Equal(t, "exact model match for model-a", res.Reason)

	res = reg.Resolve("", "qwen2.5-coder-7b")
	require.Equal(t, "family.llm.yaml", filepath.Base(res.SourcePath))
	require.Equal(t, "glob match for qwen2.5-coder-7b", res.Reason)

	res = reg.Resolve("", "missing")
	require.Equal(t, "default.llm.yaml", filepath.Base(res.SourcePath))
	require.Equal(t, "default profile from default.llm.yaml", res.Reason)
}

func TestLoadProfileRegistry_BuiltinDefault(t *testing.T) {
	reg, err := LoadProfileRegistry(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)

	res := reg.Resolve("", "missing")
	require.Equal(t, "builtin-default", res.MatchKind)
	require.NotNil(t, res.Profile)
	require.Equal(t, "*", res.Profile.Pattern)
}

func TestLoadProviderRegistry_Resolve(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ollama.provider.yaml"), []byte(`schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:11434
kind: ollama
request_timeout_seconds: 120
native_tool_calling: true
max_concurrent: 2
`), 0o644))

	reg, err := LoadProviderRegistry(dir)
	require.NoError(t, err)
	def, ok := reg.Resolve("ollama")
	require.True(t, ok, "expected provider to resolve")
	require.Equal(t, "http://localhost:11434", def.Endpoint)
	require.Equal(t, "ollama", def.Kind)
}

func writeProfileTestFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}
