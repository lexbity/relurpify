package llm

import (
	"os"
	"path/filepath"
	"testing"

	_ "codeburg.org/lexbit/relurpify/framework/cfgload"
)

func TestProviderRegistryResolve(t *testing.T) {
	dir := t.TempDir()
	writeProviderRegistryTestFile(t, filepath.Join(dir, "ollama.provider.yaml"), `schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:11434
kind: ollama
request_timeout_seconds: 120
native_tool_calling: true
max_concurrent: 2
`)

	reg, err := NewProviderRegistry(dir)
	if err != nil {
		t.Fatalf("load registry failed: %v", err)
	}
	def, ok := reg.Resolve("ollama")
	if !ok {
		t.Fatal("expected provider to resolve")
	}
	if def.Endpoint != "http://localhost:11434" || def.Kind != "ollama" {
		t.Fatalf("unexpected provider definition: %#v", def)
	}
}

func writeProviderRegistryTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}
