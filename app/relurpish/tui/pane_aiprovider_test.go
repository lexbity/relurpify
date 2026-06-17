package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeProviderYAML writes a minimal provider YAML for testing.
func writeProviderYAML(t *testing.T, dir, name, kind, endpoint string, native bool, hint string) {
	t.Helper()
	content := "schema: relurpify/model/provider/v1\n" +
		"name: " + name + "\n" +
		"endpoint: " + endpoint + "\n" +
		"kind: " + kind + "\n" +
		"native_tool_calling: " + map[bool]string{true: "true", false: "false"}[native] + "\n"
	if hint != "" {
		content += "setup_hint: \"" + hint + "\"\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".provider.yaml"), []byte(content), 0o600))
}

type testAIProviderRuntime struct {
	workspace string
}

func (r *testAIProviderRuntime) SessionInfo() SessionInfo {
	return SessionInfo{Workspace: r.workspace}
}

func (r *testAIProviderRuntime) ReloadWorkspace(_ context.Context, _ string) error {
	return nil
}

func TestAIProviderPaneCatalogPicker_EmptyCatalog(t *testing.T) {
	workspace := t.TempDir()
	// No provider YAMLs — catalog is empty
	pane := NewAIProviderPane(&testAIProviderRuntime{workspace: workspace})
	// cycleProvider should not panic with empty catalog
	pane.cycleProvider(1)
	pane.cycleProvider(-1)
}

func TestAIProviderPaneCatalogPicker_CycleProviders(t *testing.T) {
	workspace := t.TempDir()
	provDir := filepath.Join(workspace, "relurpify_cfg", "model", "provider")
	require.NoError(t, os.MkdirAll(provDir, 0755))
	writeProviderYAML(t, provDir, "ollama", "ollama", "http://localhost:11434", true, "")
	writeProviderYAML(t, provDir, "lmstudio", "lmstudio", "http://localhost:1234", false, "")
	writeProviderYAML(t, provDir, "openai_compatible", "openai_compatible", "http://custom:8080/v1", true, "")

	pane := NewAIProviderPane(&testAIProviderRuntime{workspace: workspace})

	if len(pane.catalog) == 0 {
		t.Fatal("expected catalog to be populated")
	}
	if len(pane.catalog) < 2 {
		t.Fatal("expected at least 2 catalog providers")
	}

	// Record the initial catalog index and provider
	initialIdx := pane.catalogIdx

	// Cycle right once — index should advance by 1
	pane.cycleProvider(1)
	expectedIdx := (initialIdx + 1) % len(pane.catalog)
	if pane.catalogIdx != expectedIdx {
		t.Fatalf("expected catalogIdx = %d after cycle right, got %d", expectedIdx, pane.catalogIdx)
	}
	if pane.profile.Provider != pane.catalog[expectedIdx].Name {
		t.Fatalf("expected profile.Provider = %q after cycle, got %q", pane.catalog[expectedIdx].Name, pane.profile.Provider)
	}

	// Cycle right again
	pane.cycleProvider(1)
	expectedIdx = (expectedIdx + 1) % len(pane.catalog)
	if pane.catalogIdx != expectedIdx {
		t.Fatalf("expected catalogIdx = %d after second cycle right, got %d", expectedIdx, pane.catalogIdx)
	}

	// Cycle right until we wrap — should return to initial
	targetIdx := initialIdx
	for pane.catalogIdx != targetIdx {
		pane.cycleProvider(1)
	}
	if pane.catalogIdx != initialIdx {
		t.Fatalf("expected catalogIdx = %d after wrap, got %d", initialIdx, pane.catalogIdx)
	}

	// Cycle left once — index should go back by one (with wrap)
	pane.cycleProvider(-1)
	expectedLeft := (initialIdx - 1 + len(pane.catalog)) % len(pane.catalog)
	if pane.catalogIdx != expectedLeft {
		t.Fatalf("expected catalogIdx = %d after cycle left, got %d", expectedLeft, pane.catalogIdx)
	}
}

func TestAIProviderPaneAutoFillOnSelect(t *testing.T) {
	workspace := t.TempDir()
	provDir := filepath.Join(workspace, "relurpify_cfg", "model", "provider")
	require.NoError(t, os.MkdirAll(provDir, 0755))
	writeProviderYAML(t, provDir, "ollama", "ollama", "http://localhost:11434", true, "Start ollama serve")
	writeProviderYAML(t, provDir, "custom-api", "openai_compatible", "http://custom:8080/v1", false, "Set your API key")

	pane := NewAIProviderPane(&testAIProviderRuntime{workspace: workspace})

	// Find custom-api index
	customIdx := -1
	ollamaIdx := -1
	for i, cp := range pane.catalog {
		if cp.Name == "custom-api" {
			customIdx = i
		}
		if cp.Name == "ollama" {
			ollamaIdx = i
		}
	}
	if customIdx < 0 {
		t.Fatal("expected custom-api in catalog")
	}
	if ollamaIdx < 0 {
		t.Fatal("expected ollama in catalog")
	}

	// Cycle to custom-api
	for pane.catalogIdx != customIdx {
		pane.cycleProvider(1)
	}

	require.Equal(t, "custom-api", pane.profile.Provider)
	require.Equal(t, "http://custom:8080/v1", pane.profile.Endpoint)
	require.False(t, pane.profile.NativeToolCalling)
	require.Contains(t, pane.setupHint, "API key")

	// Cycle back to ollama
	for pane.catalogIdx != ollamaIdx {
		pane.cycleProvider(-1)
	}

	require.Equal(t, "ollama", pane.profile.Provider)
	require.Equal(t, "http://localhost:11434", pane.profile.Endpoint)
	require.True(t, pane.profile.NativeToolCalling)
}

func TestAIProviderPaneLoadCatalog_InvalidDir(t *testing.T) {
	workspace := t.TempDir()
	pane := NewAIProviderPane(&testAIProviderRuntime{workspace: workspace})
	require.Empty(t, pane.catalog)
	require.Equal(t, 0, pane.catalogIdx)
}

func TestAIProviderPaneToggleInfrastructureDeleted(t *testing.T) {
	// Verify that toggleInfrastructure no longer exists
	// by checking that the provider pane uses cycleProvider instead.
	workspace := t.TempDir()
	provDir := filepath.Join(workspace, "relurpify_cfg", "model", "provider")
	require.NoError(t, os.MkdirAll(provDir, 0755))
	writeProviderYAML(t, provDir, "ollama", "ollama", "http://localhost:11434", true, "")
	writeProviderYAML(t, provDir, "openai_compatible", "openai_compatible", "http://custom:8080/v1", true, "")

	pane := NewAIProviderPane(&testAIProviderRuntime{workspace: workspace})
	// Left/right arrows should cycle through catalog, not toggle between two hardcoded providers
	pane.cycleProvider(1)
	require.Equal(t, "openai_compatible", pane.profile.Provider)
}
