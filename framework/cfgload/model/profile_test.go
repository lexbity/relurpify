package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfileFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "default.llm.yaml")
	writeProfileTestFile(t, path, `schema: relurpify/model/profile/v1
pattern: "*"
tool_calling:
  intent: auto
  max_concurrent_tools: 4
  double_encode_args: false
context:
  max_tokens: 8192
  response_reserve_tokens: 512
generation:
  temperature: 0.2
  top_p: 0.9
`)

	profile, err := LoadProfileFile(path)
	if err != nil {
		t.Fatalf("load profile failed: %v", err)
	}
	if profile.Pattern != "*" || profile.Context.MaxTokens != 8192 {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

func TestLoadProfileFileRejectsInvalidIntent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.llm.yaml")
	writeProfileTestFile(t, path, `schema: relurpify/model/profile/v1
pattern: "model*"
tool_calling:
  intent: forbidden
context:
  max_tokens: 1024
generation:
  temperature: 0.2
  top_p: 0.9
`)
	if _, err := LoadProfileFile(path); err == nil {
		t.Fatal("expected profile load to fail")
	}
}

func TestLoadProfileDir(t *testing.T) {
	dir := t.TempDir()
	writeProfileTestFile(t, filepath.Join(dir, "default.llm.yaml"), `schema: relurpify/model/profile/v1
pattern: "*"
tool_calling:
  intent: auto
context:
  max_tokens: 1024
generation:
  temperature: 0.2
  top_p: 0.9
`)
	writeProfileTestFile(t, filepath.Join(dir, "gemma.llm.yaml"), `schema: relurpify/model/profile/v1
pattern: "gemma*"
tool_calling:
  intent: native
context:
  max_tokens: 2048
generation:
  temperature: 0.2
  top_p: 0.9
`)
	profiles, err := LoadProfileDir(dir)
	if err != nil {
		t.Fatalf("load profile dir failed: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
}

func writeProfileTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}
