package ayenitd_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/llm"
)

type fakeBackend struct {
	models  []llm.ModelInfo
	warmErr error
	listErr error
}

func (f fakeBackend) Model() core.LanguageModel { return nil }
func (f fakeBackend) Embedder() llm.Embedder    { return nil }
func (f fakeBackend) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{}
}
func (f fakeBackend) Health(context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{}, nil
}
func (f fakeBackend) ListModels(context.Context) ([]llm.ModelInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]llm.ModelInfo(nil), f.models...), nil
}
func (f fakeBackend) Warm(context.Context) error { return f.warmErr }
func (f fakeBackend) Close() error               { return nil }
func (f fakeBackend) SetDebugLogging(bool)       {}

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
	results := ayenitd.ProbeWorkspace(probeCfg(absent), fakeBackend{
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
	f, err := os.CreateTemp("", "ayenitd-probe-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	results := ayenitd.ProbeWorkspace(probeCfg(f.Name()), fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	r := findResult(t, results, "workspace_directory")
	if r.OK {
		t.Error("workspace_directory: expected NOT OK when path is a file, not a directory")
	}
}

func TestProbeWorkspace_WorkspaceExists(t *testing.T) {
	results := ayenitd.ProbeWorkspace(probeCfg(t.TempDir()), fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	r := findResult(t, results, "workspace_directory")
	if !r.OK {
		t.Errorf("workspace_directory: expected OK for existing temp dir, got: %s", r.Message)
	}
}

func TestProbeWorkspace_SQLiteWritable(t *testing.T) {
	results := ayenitd.ProbeWorkspace(probeCfg(t.TempDir()), fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	r := findResult(t, results, "sqlite_writable")
	if !r.OK {
		t.Errorf("sqlite_writable: expected OK for writable temp dir, got: %s", r.Message)
	}
}

func TestProbeWorkspace_InferenceUnhealthy(t *testing.T) {
	results := ayenitd.ProbeWorkspace(probeCfg(t.TempDir()), fakeBackend{
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
	results := ayenitd.ProbeWorkspace(func() ayenitd.WorkspaceConfig {
		cfg := probeCfg(t.TempDir())
		cfg.InferenceModel = model
		return cfg
	}(), fakeBackend{
		models: []llm.ModelInfo{{Name: model}},
	})
	r := findResult(t, results, "inference_backend")
	if !r.OK {
		t.Errorf("inference_backend: expected OK when model is in list, got: %s", r.Message)
	}
}

func TestProbeWorkspace_InferenceModelMissing(t *testing.T) {
	results := ayenitd.ProbeWorkspace(func() ayenitd.WorkspaceConfig {
		cfg := probeCfg(t.TempDir())
		cfg.InferenceModel = "qwen2.5-coder:14b"
		return cfg
	}(), fakeBackend{
		models: []llm.ModelInfo{{Name: "other-model:7b"}},
	})
	r := findResult(t, results, "inference_backend")
	if r.OK {
		t.Error("inference_backend: expected NOT OK when model is absent from list")
	}
}

func TestProbeWorkspace_DiskSpaceIsNonRequired(t *testing.T) {
	results := ayenitd.ProbeWorkspace(probeCfg(t.TempDir()), fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	r := findResult(t, results, "disk_space")
	if r.Required {
		t.Error("disk_space: should not be required (warn-only)")
	}
}

func TestProbeWorkspace_AllResultNamesPresent(t *testing.T) {
	results := ayenitd.ProbeWorkspace(probeCfg(t.TempDir()), fakeBackend{
		models: []llm.ModelInfo{{Name: "qwen2.5-coder:14b"}},
	})
	want := []string{"workspace_directory", "sqlite_writable", "inference_backend", "disk_space"}
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
