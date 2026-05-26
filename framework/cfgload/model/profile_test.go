package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadProfileDir(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "relurpify_cfg", "model", "profiles")
	writeProfileTestFile(t, filepath.Join(base, "default.llm.yaml"), `schema: relurpify/model/profile/v1
pattern: "*"
tool_calling:
  intent: auto
  max_concurrent_tools: 4
context:
  max_tokens: 1024
generation:
  temperature: 0.2
  top_p: 0.9
`)
	writeProfileTestFile(t, filepath.Join(base, "gemma.llm.yaml"), `schema: relurpify/model/profile/v1
pattern: "gemma*"
tool_calling:
  intent: native
  max_concurrent_tools: 1
context:
  max_tokens: 2048
generation:
  temperature: 0.1
  top_p: 0.95
`)

	profiles, err := LoadProfileDir(base, testDecode)
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	require.Equal(t, "gemma*", profiles[0].Pattern)
	require.Equal(t, "*", profiles[1].Pattern)
}

func TestLoadProfileDirRequiresDefault(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "relurpify_cfg", "model", "profiles")
	writeProfileTestFile(t, filepath.Join(base, "gemma.llm.yaml"), `schema: relurpify/model/profile/v1
pattern: "gemma*"
tool_calling:
  intent: native
context:
  max_tokens: 1024
generation:
  temperature: 0.1
  top_p: 0.9
`)

	_, err := LoadProfileDir(base, testDecode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "default.llm.yaml required")
}

func TestMatchProfile_SpecificBeforeDefault(t *testing.T) {
	profiles := []*ModelProfileConfig{
		{Pattern: "gemma*", SourcePath: "relurpify_cfg/model/profiles/gemma.llm.yaml"},
		{Pattern: "*", SourcePath: "relurpify_cfg/model/profiles/default.llm.yaml"},
	}
	require.Equal(t, "gemma*", MatchProfile(profiles, "gemma4:e4b").Pattern)
}

func TestMatchProfile_FallsBackToDefault(t *testing.T) {
	profiles := []*ModelProfileConfig{
		{Pattern: "gemma*", SourcePath: "relurpify_cfg/model/profiles/gemma.llm.yaml"},
		{Pattern: "*", SourcePath: "relurpify_cfg/model/profiles/default.llm.yaml"},
	}
	require.Equal(t, "*", MatchProfile(profiles, "unknown").Pattern)
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
