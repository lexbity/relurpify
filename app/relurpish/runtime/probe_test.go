package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/llm"
)

type probeBackend struct {
	health *llm.HealthReport
	models []llm.ModelInfo
	err    error
}

func (p probeBackend) Model() core.LanguageModel                         { return nil }
func (p probeBackend) Embedder() llm.Embedder                            { return nil }
func (p probeBackend) Capabilities() core.BackendCapabilities            { return core.BackendCapabilities{} }
func (p probeBackend) Health(context.Context) (*llm.HealthReport, error) { return p.health, p.err }
func (p probeBackend) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return append([]llm.ModelInfo(nil), p.models...), p.err
}
func (p probeBackend) Warm(context.Context) error { return p.err }
func (p probeBackend) Close() error               { return nil }
func (p probeBackend) SetDebugLogging(bool)       {}

func TestProbeEnvironment_HealthyBackend(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agent.manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("metadata:\n  name: coding\nspec:\n  runtime: react\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Workspace = dir
	cfg.ManifestPath = manifestPath
	cfg.ConfigPath = filepath.Join(dir, "relurpify.yaml")
	cfg.InferenceProvider = "ollama"
	cfg.InferenceEndpoint = "http://127.0.0.1:11434"
	cfg.InferenceModel = "qwen2.5-coder:14b"

	report := ProbeEnvironment(context.Background(), cfg, probeBackend{
		health: &llm.HealthReport{
			State:   llm.BackendHealthReady,
			Message: "ready",
		},
		models: []llm.ModelInfo{{Name: cfg.InferenceModel}},
	})
	if report.Inference.State != llm.BackendHealthReady {
		t.Fatalf("expected ready state, got %s", report.Inference.State)
	}
	if report.Inference.SelectedModel != cfg.InferenceModel {
		t.Fatalf("expected selected model %q, got %q", cfg.InferenceModel, report.Inference.SelectedModel)
	}
	if len(report.Inference.Models) != 1 || report.Inference.Models[0] != cfg.InferenceModel {
		t.Fatalf("expected models to include %q, got %#v", cfg.InferenceModel, report.Inference.Models)
	}
	if report.Inference.Resources != nil {
		t.Fatalf("expected no resources snapshot, got %#v", report.Inference.Resources)
	}
}

func TestProbeEnvironment_UnhealthyBackend(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agent.manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("metadata:\n  name: coding\nspec:\n  runtime: react\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Workspace = dir
	cfg.ManifestPath = manifestPath
	cfg.ConfigPath = filepath.Join(dir, "relurpify.yaml")
	cfg.InferenceProvider = "ollama"
	cfg.InferenceEndpoint = "http://127.0.0.1:11434"
	cfg.InferenceModel = "qwen2.5-coder:14b"

	report := ProbeEnvironment(context.Background(), cfg, probeBackend{
		health: &llm.HealthReport{
			State:      llm.BackendHealthUnhealthy,
			Message:    "backend unavailable",
			LastError:  "backend unavailable",
			ErrorCount: 1,
		},
	})
	if report.Inference.State != llm.BackendHealthUnhealthy {
		t.Fatalf("expected unhealthy state, got %s", report.Inference.State)
	}
	if report.Inference.Error == "" {
		t.Fatal("expected error to be populated")
	}
}

func TestProbeEnvironment_ModelsListed(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agent.manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("metadata:\n  name: coding\nspec:\n  runtime: react\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Workspace = dir
	cfg.ManifestPath = manifestPath
	cfg.ConfigPath = filepath.Join(dir, "relurpify.yaml")
	cfg.InferenceProvider = "ollama"
	cfg.InferenceEndpoint = "http://127.0.0.1:11434"
	cfg.InferenceModel = "qwen2.5-coder:14b"

	report := ProbeEnvironment(context.Background(), cfg, probeBackend{
		health: &llm.HealthReport{State: llm.BackendHealthReady},
		models: []llm.ModelInfo{{Name: "a"}, {Name: "b"}},
	})
	if len(report.Inference.Models) != 2 {
		t.Fatalf("expected two models, got %#v", report.Inference.Models)
	}
	if report.Inference.Models[0] != "a" || report.Inference.Models[1] != "b" {
		t.Fatalf("unexpected models: %#v", report.Inference.Models)
	}
}

func TestProbeEnvironment_NoBackend_FallbackConstruct(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agent.manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("metadata:\n  name: coding\nspec:\n  runtime: react\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Workspace = dir
	cfg.ManifestPath = manifestPath
	cfg.ConfigPath = filepath.Join(dir, "relurpify.yaml")
	cfg.InferenceProvider = "ollama"
	cfg.InferenceEndpoint = "http://127.0.0.1:11434"
	cfg.InferenceModel = "qwen2.5-coder:14b"

	orig := newManagedBackend
	defer func() { newManagedBackend = orig }()
	called := false
	newManagedBackend = func(pc llm.ProviderConfig) (llm.ManagedBackend, error) {
		called = true
		return probeBackend{
			health: &llm.HealthReport{State: llm.BackendHealthReady},
			models: []llm.ModelInfo{{Name: cfg.InferenceModel}},
		}, nil
	}

	report := ProbeEnvironment(context.Background(), cfg, nil)
	if !called {
		t.Fatal("expected fallback backend construction")
	}
	if report.Inference.State != llm.BackendHealthReady {
		t.Fatalf("expected ready state from fallback backend, got %s", report.Inference.State)
	}
}

func TestProbeEnvironment_PropagatesBuilderError(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agent.manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("metadata:\n  name: coding\nspec:\n  runtime: react\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Workspace = dir
	cfg.ManifestPath = manifestPath
	cfg.ConfigPath = filepath.Join(dir, "relurpify.yaml")
	cfg.InferenceProvider = "ollama"
	cfg.InferenceEndpoint = "http://127.0.0.1:11434"
	cfg.InferenceModel = "qwen2.5-coder:14b"

	orig := newManagedBackend
	defer func() { newManagedBackend = orig }()
	newManagedBackend = func(pc llm.ProviderConfig) (llm.ManagedBackend, error) {
		return nil, errors.New("boom")
	}

	report := ProbeEnvironment(context.Background(), cfg, nil)
	if report.Inference.State != llm.BackendHealthUnhealthy {
		t.Fatalf("expected unhealthy state on builder error, got %s", report.Inference.State)
	}
	if report.Inference.Error == "" {
		t.Fatal("expected error on builder failure")
	}
}
