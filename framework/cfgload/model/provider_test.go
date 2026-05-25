package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProviderFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ollama.provider.yaml")
	writeModelTestFile(t, path, `schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:11434
kind: ollama
request_timeout_seconds: 120
available_models:
  - gemma4:e4b
native_tool_calling: true
max_concurrent: 2
`)

	provider, err := LoadProviderFile(path)
	if err != nil {
		t.Fatalf("load provider failed: %v", err)
	}
	if provider.Name != "ollama" || provider.Endpoint != "http://localhost:11434" {
		t.Fatalf("unexpected provider: %#v", provider)
	}
}

func TestLoadProviderFileRejectsSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openai.provider.yaml")
	writeModelTestFile(t, path, `schema: relurpify/model/provider/v1
name: openai
endpoint: https://example.com
kind: openai_compatible
api_key: secret
`)

	if _, err := LoadProviderFile(path); err == nil {
		t.Fatal("expected provider load to reject secret field")
	}
}

func TestLoadProviderDir(t *testing.T) {
	dir := t.TempDir()
	writeModelTestFile(t, filepath.Join(dir, "a.provider.yaml"), `schema: relurpify/model/provider/v1
name: a
endpoint: http://localhost:11434
kind: ollama
`)
	writeModelTestFile(t, filepath.Join(dir, "b.provider.yaml"), `schema: relurpify/model/provider/v1
name: b
endpoint: http://localhost:1234
kind: lmstudio
`)

	providers, err := LoadProviderDir(dir)
	if err != nil {
		t.Fatalf("load provider dir failed: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
}

func writeModelTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}
