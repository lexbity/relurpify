package ayenitd_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

type fakeBackend struct {
	models  []llm.ModelInfo
	warmErr error
	listErr error
}

func (f fakeBackend) Model() model.LanguageModel { return nil }
func (f fakeBackend) Embedder() llm.Embedder     { return nil }
func (f fakeBackend) Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{}
}
func (f fakeBackend) ModelContextSize(context.Context) (int, error) { return 0, nil }
func (f fakeBackend) Health(context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{}, nil
}
func (f fakeBackend) ListModels(context.Context) ([]llm.ModelInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]llm.ModelInfo(nil), f.models...), nil
}
func (f fakeBackend) Warm(context.Context) error          { return f.warmErr }
func (f fakeBackend) Close() error                        { return nil }
func (f fakeBackend) SetDebugLogging(bool)                {}
func (f fakeBackend) SetProfile(*llm.ModelProfile)        {}
func (f fakeBackend) Reset(context.Context, string) error { return nil }

// findResult returns the ProbeResult with the given name, or fails the test.
func findResult(t *testing.T, results []ayenitd.ProbeResult, name string) ayenitd.ProbeResult {
	t.Helper()
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("probe result %q not found in %v", name, results)
	return ayenitd.ProbeResult{}
}

func probeCfg(workspace string) ayenitd.WorkspaceConfig {
	return ayenitd.WorkspaceConfig{
		Workspace:         workspace,
		InferenceProvider: "ollama",
		InferenceEndpoint: "http://127.0.0.1:11435",
		InferenceModel:    "qwen2.5-coder:14b",
	}
}

func TestProbeWorkspace_WorkspaceNotFound(t *testing.T) {
	absent := t.TempDir()
	if err := os.RemoveAll(absent); err != nil {
		t.Fatal(err)
	}
	results := ayenitd.ProbeWorkspace(context.Background(), probeCfg(absent), llm.ProviderSecrets{}, fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	r := findResult(t, results, "workspace_directory")
	if r.OK {
		t.Error("workspace_directory: expected NOT OK for missing directory")
	}
	if !r.Required {
		t.Error("workspace_directory: should be required")
	}
}

func TestProbeWorkspace_WorkspaceIsFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "ayenitd-probe-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	results := ayenitd.ProbeWorkspace(context.Background(), probeCfg(f.Name()), llm.ProviderSecrets{}, fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	r := findResult(t, results, "workspace_directory")
	if r.OK {
		t.Error("workspace_directory: expected NOT OK when path is a file, not a directory")
	}
}

func TestProbeWorkspace_WorkspaceExists(t *testing.T) {
	results := ayenitd.ProbeWorkspace(context.Background(), probeCfg(t.TempDir()), llm.ProviderSecrets{}, fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	r := findResult(t, results, "workspace_directory")
	if !r.OK {
		t.Errorf("workspace_directory: expected OK for existing temp dir, got: %s", r.Message)
	}
}

func TestProbeWorkspace_InferenceUnhealthy(t *testing.T) {
	results := ayenitd.ProbeWorkspace(context.Background(), probeCfg(t.TempDir()), llm.ProviderSecrets{}, fakeBackend{
		warmErr: errors.New("backend unavailable"),
	})
	r := findResult(t, results, "inference_backend")
	if r.OK {
		t.Error("inference_backend: expected NOT OK when backend warmup fails")
	}
	if !r.Required {
		t.Error("inference_backend: should be required")
	}
}

func TestProbeWorkspace_InferenceModelPresent(t *testing.T) {
	const model = "qwen2.5-coder:14b"
	results := ayenitd.ProbeWorkspace(context.Background(), func() ayenitd.WorkspaceConfig {
		cfg := probeCfg(t.TempDir())
		cfg.InferenceModel = model
		return cfg
	}(), llm.ProviderSecrets{}, fakeBackend{
		models: []llm.ModelInfo{{Name: model}},
	})
	r := findResult(t, results, "inference_backend")
	if !r.OK {
		t.Errorf("inference_backend: expected OK when model is in list, got: %s", r.Message)
	}
}

func TestProbeWorkspace_InferenceModelMissing(t *testing.T) {
	results := ayenitd.ProbeWorkspace(context.Background(), func() ayenitd.WorkspaceConfig {
		cfg := probeCfg(t.TempDir())
		cfg.InferenceModel = "qwen2.5-coder:14b"
		return cfg
	}(), llm.ProviderSecrets{}, fakeBackend{
		models: []llm.ModelInfo{{Name: "other-model:7b"}},
	})
	r := findResult(t, results, "inference_backend")
	if r.OK {
		t.Error("inference_backend: expected NOT OK when model is absent from list")
	}
}

func TestProbeWorkspace_DiskSpaceIsNonRequired(t *testing.T) {
	results := ayenitd.ProbeWorkspace(context.Background(), probeCfg(t.TempDir()), llm.ProviderSecrets{}, fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	r := findResult(t, results, "disk_space")
	if r.Required {
		t.Error("disk_space: should not be required (warn-only)")
	}
}

func TestProbeWorkspace_AllResultNamesPresent(t *testing.T) {
	results := ayenitd.ProbeWorkspace(context.Background(), probeCfg(t.TempDir()), llm.ProviderSecrets{}, fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	want := []string{"workspace_directory", "inference_backend", "disk_space"}
	got := make(map[string]bool, len(results))
	for _, r := range results {
		got[r.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing expected probe result: %q", name)
		}
	}
}
