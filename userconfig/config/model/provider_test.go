package model

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestLoadProviderDir_Valid(t *testing.T) {
	dir := t.TempDir()
	writeModelTestFile(t, filepath.Join(dir, "relurpify_cfg", "model", "provider", "ollama.provider.yaml"), `schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:11434
kind: ollama
available_models:
  - gemma4:e4b
native_tool_calling: true
max_concurrent: 2
`)
	writeModelTestFile(t, filepath.Join(dir, "relurpify_cfg", "model", "provider", "lmstudio.provider.yaml"), `schema: relurpify/model/provider/v1
name: lmstudio
endpoint: http://localhost:1234
kind: lmstudio
`)

	providers, err := LoadProviderDir(filepath.Join(dir, "relurpify_cfg", "model", "provider"), testDecode)
	require.NoError(t, err)
	require.Len(t, providers, 2)
	names := []string{providers[0].Name, providers[1].Name}
	require.ElementsMatch(t, []string{"ollama", "lmstudio"}, names)
}

func TestLoadProviderDir_ForbiddenField(t *testing.T) {
	dir := t.TempDir()
	writeModelTestFile(t, filepath.Join(dir, "relurpify_cfg", "model", "provider", "openai.provider.yaml"), `schema: relurpify/model/provider/v1
name: openai
endpoint: https://example.com
kind: openai_compatible
api_key: secret
`)

	_, err := LoadProviderDir(filepath.Join(dir, "relurpify_cfg", "model", "provider"), testDecode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field=api_key")
}

func TestLoadProviderDir_DuplicateName(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "relurpify_cfg", "model", "provider")
	writeModelTestFile(t, filepath.Join(base, "a.provider.yaml"), `schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:11434
kind: ollama
`)
	writeModelTestFile(t, filepath.Join(base, "b.provider.yaml"), `schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:1234
kind: lmstudio
`)

	_, err := LoadProviderDir(base, testDecode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate provider name")
}

func TestLoadProviderDir_InvalidURL(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "relurpify_cfg", "model", "provider")
	writeModelTestFile(t, filepath.Join(base, "bad.provider.yaml"), `schema: relurpify/model/provider/v1
name: bad
endpoint: not-a-url
kind: ollama
`)

	_, err := LoadProviderDir(base, testDecode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "endpoint invalid")
}

func TestLoadProviderDirDetailedKeepsValidProvidersWhenOneBroken(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "relurpify_cfg", "model", "provider")
	writeModelTestFile(t, filepath.Join(base, "ollama.provider.yaml"), `schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:11434
kind: ollama
`)
	writeModelTestFile(t, filepath.Join(base, "broken.provider.yaml"), `schema: relurpify/model/provider/v1
name: broken
endpoint: not-a-url
kind: ollama
`)

	providers, diags, err := LoadProviderDirDetailed(base, testDecode)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Len(t, diags, 1)
	require.Equal(t, "blocking", diags[0].Severity)
	require.Contains(t, diags[0].Path, "broken.provider.yaml")
	require.Equal(t, "ollama", providers[0].Name)
}

func TestLoadProviderDir_MissingDir(t *testing.T) {
	_, err := LoadProviderDir(filepath.Join(t.TempDir(), "missing"), testDecode)
	require.Error(t, err)
}

func TestLoadProviderDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "relurpify_cfg", "model", "provider")
	require.NoError(t, fs.MkdirAllSecure(base))

	_, err := LoadProviderDir(base, testDecode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is empty")
}

func writeModelTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fs.WriteFileSecure(path, []byte(contents)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}
