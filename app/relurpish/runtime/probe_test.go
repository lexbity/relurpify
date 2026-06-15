package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type fakeProbeBackend struct {
	model llm.LanguageModel
}

func (f *fakeProbeBackend) Model() llm.LanguageModel {
	return f.model
}

func (f *fakeProbeBackend) Embedder() llm.Embedder { return nil }

func (f *fakeProbeBackend) Capabilities() llm.BackendCapabilities { return llm.BackendCapabilities{} }

func (f *fakeProbeBackend) ModelContextSize(context.Context) (int, error) { return 0, nil }

func (f *fakeProbeBackend) Health(context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{State: llm.BackendHealthReady}, nil
}

func (f *fakeProbeBackend) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{Name: "gemma4:e4b"}}, nil
}

func (f *fakeProbeBackend) Warm(context.Context) error { return nil }

func (f *fakeProbeBackend) Close() error { return nil }

func (f *fakeProbeBackend) SetDebugLogging(bool) {}

func (f *fakeProbeBackend) SetProfile(*llm.ModelProfile) {}

func (f *fakeProbeBackend) Reset(context.Context, string) error { return nil }

func TestProbeEnvironmentLoadsProfilesViaDiagnostic(t *testing.T) {
	workspace := t.TempDir()
	cfgRoot := filepath.Join(workspace, "relurpify_cfg")
	securityDir := filepath.Join(cfgRoot, "security")
	if err := os.MkdirAll(securityDir, fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir security: %v", err)
	}
	if err := os.WriteFile(filepath.Join(securityDir, "localtool.policy.yaml"), []byte("schema: relurpify/policy/localtool/v1\ntools:\n  cli_git:\n    execute: allow\n"), 0o600); err != nil {
		t.Fatalf("write localtool: %v", err)
	}
	if err := os.WriteFile(filepath.Join(securityDir, "shell.policy.yaml"), []byte("schema: relurpify/policy/shell/v1\nrules: []\n"), 0o600); err != nil {
		t.Fatalf("write shell: %v", err)
	}
	if err := os.WriteFile(filepath.Join(securityDir, "sandbox.policy.yaml"), []byte("schema: relurpify/policy/sandbox/v1\nread_only_root: false\nno_new_privileges: false\n"), 0o600); err != nil {
		t.Fatalf("write sandbox: %v", err)
	}

	cfg := Config{
		Workspace:         workspace,
		InferenceProvider: "ollama",
		InferenceModel:    "gemma4:e4b",
		ConfigPath:        config.DefaultWorkspaceConfigPath(workspace),
	}

	backend := &fakeProbeBackend{}
	env := ProbeEnvironment(context.Background(), cfg, config.Secrets{}, backend)
	if env.Inference.State == "" {
		t.Fatal("expected inference state to be populated")
	}
	if env.Inference.SelectedModel != "gemma4:e4b" {
		t.Fatalf("selected model = %q, want gemma4:e4b", env.Inference.SelectedModel)
	}
}

func TestProbeEnvironmentReportsToolCallingMode(t *testing.T) {
	workspace := t.TempDir()
	cfgRoot := filepath.Join(workspace, "relurpify_cfg")
	securityDir := filepath.Join(cfgRoot, "security")
	if err := os.MkdirAll(securityDir, fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir security: %v", err)
	}
	if err := os.WriteFile(filepath.Join(securityDir, "localtool.policy.yaml"), []byte("schema: relurpify/policy/localtool/v1\ntools:\n  cli_git:\n    execute: allow\n"), 0o600); err != nil {
		t.Fatalf("write localtool: %v", err)
	}
	if err := os.WriteFile(filepath.Join(securityDir, "shell.policy.yaml"), []byte("schema: relurpify/policy/shell/v1\nrules: []\n"), 0o600); err != nil {
		t.Fatalf("write shell: %v", err)
	}
	if err := os.WriteFile(filepath.Join(securityDir, "sandbox.policy.yaml"), []byte("schema: relurpify/policy/sandbox/v1\nread_only_root: false\nno_new_privileges: false\n"), 0o600); err != nil {
		t.Fatalf("write sandbox: %v", err)
	}

	cfg := Config{
		Workspace:         workspace,
		InferenceProvider: "ollama",
		InferenceModel:    "gemma4:e4b",
		ConfigPath:        config.DefaultWorkspaceConfigPath(workspace),
	}

	backend := &fakeProbeBackend{model: probeToolModel{native: false}}
	env := ProbeEnvironment(context.Background(), cfg, config.Secrets{}, backend)
	if env.Inference.ToolCallingMode != "fallback" {
		t.Fatalf("tool calling mode = %q, want fallback", env.Inference.ToolCallingMode)
	}
}

type probeToolModel struct {
	native bool
}

func (m probeToolModel) Generate(context.Context, string, *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Text: "ok"}, nil
}

func (m probeToolModel) GenerateStream(context.Context, string, *llm.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m probeToolModel) Chat(context.Context, []llm.Message, *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Text: "ok"}, nil
}

func (m probeToolModel) ChatWithTools(context.Context, []llm.Message, []llm.LLMToolSpec, *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Text: "ok"}, nil
}

func (m probeToolModel) ToolRepairStrategy() string {
	return "heuristic-only"
}

func (m probeToolModel) MaxToolsPerCall() int {
	return 0
}

func (m probeToolModel) UsesNativeToolCalling() bool {
	return m.native
}
