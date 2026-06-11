package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestConfigForWorkspaceRebindsPaths(t *testing.T) {
	current := Config{
		Workspace:         "/old/workspace",
		AgentName:         "euclo",
		InferenceProvider: "ollama",
		InferenceEndpoint: "http://localhost:11434",
		InferenceModel:    "codex",
		SandboxBackend:    "docker",
		RecordingMode:     "on",
		ManifestPath:      "/old/workspace/manifest.yaml",
		AgentsDir:         "/old/workspace/relurpify_cfg/agents",
		MemoryPath:        "/old/workspace/.relurpify_state/memory",
		LogPath:           "/old/workspace/.relurpify_state/logs/relurpish.log",
		TelemetryPath:     "/old/workspace/.relurpify_state/telemetry/telemetry.jsonl",
		EventsPath:        "/old/workspace/.relurpify_state/events.db",
		ConfigPath:        "/old/workspace/relurpify_cfg/config.yaml",
	}

	cfg := ConfigForWorkspace(current, "/new/workspace")
	if cfg.Workspace != "/new/workspace" {
		t.Fatalf("workspace = %q, want /new/workspace", cfg.Workspace)
	}
	if cfg.AgentName != current.AgentName {
		t.Fatalf("agent name = %q, want %q", cfg.AgentName, current.AgentName)
	}
	if cfg.InferenceModel != current.InferenceModel || cfg.SandboxBackend != current.SandboxBackend {
		t.Fatalf("config fields were not preserved: %#v", cfg)
	}
	if cfg.ConfigPath == current.ConfigPath || cfg.ManifestPath == current.ManifestPath {
		t.Fatalf("workspace paths were not rebound: %#v", cfg)
	}
	if want := "/new/workspace/.relurpify_state/workspace.yaml"; cfg.ConfigPath != want {
		t.Fatalf("config path = %q, want %q", cfg.ConfigPath, want)
	}
	if want := "/new/workspace/relurpify_cfg/agents/euclo.yaml"; cfg.ManifestPath != want {
		t.Fatalf("manifest path = %q, want %q", cfg.ManifestPath, want)
	}
	if want := "/new/workspace/.relurpify_state/logs/relurpish.log"; cfg.LogPath != want {
		t.Fatalf("log path = %q, want %q", cfg.LogPath, want)
	}
}

type recordingExecutor struct {
	mu        sync.Mutex
	execCount int
	lastTask  *execution.Task
	lastEnv   *contextdata.Envelope
}

func (r *recordingExecutor) Initialize(*execution.Config) error { return nil }

func (r *recordingExecutor) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	r.execCount++
	r.lastTask = task
	r.lastEnv = env
	return &execution.Result{NodeID: "recording", Success: true}, nil
}

func (r *recordingExecutor) Capabilities() []string { return nil }

func (r *recordingExecutor) BuildGraph(ctx context.Context, _ *execution.Task) (*agentgraph.Graph, error) { return nil, errors.New("not implemented") }

func TestResolveInteractionFrameResumesClarificationTask(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	task := &execution.Task{ID: "task-1", Instruction: "clarify request"}
	env.SetWorkingValueWithClass("task.input", task, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("euclo.interaction.frame_seq", 1, contextdata.MemoryClassTask)

	frame := interaction.NewClarificationFrame("task-1", "session-1", "Pick one", []string{"review", "implement"}, nil)
	env.SetWorkingValueWithClass("euclo.interaction.frame.0", frame, contextdata.MemoryClassTask)

	executor := &recordingExecutor{}
	rt := &Runtime{
		Agent:                executor,
		interactionEnvelopes: map[string]*contextdata.Envelope{"task-1": env},
	}

	if err := rt.ResolveInteractionFrame(context.Background(), "task-1", frame.ID, "implement", ""); err != nil {
		t.Fatalf("resolve interaction frame failed: %v", err)
	}
	if executor.execCount != 1 {
		t.Fatalf("execute count = %d, want 1", executor.execCount)
	}
	if executor.lastTask != task {
		t.Fatalf("task pointer mismatch: got %#v want %#v", executor.lastTask, task)
	}
	if executor.lastEnv != env {
		t.Fatalf("envelope pointer mismatch: got %#v want %#v", executor.lastEnv, env)
	}
	if frame.Response == nil || frame.Response.ChosenSlot != "implement" {
		t.Fatalf("frame response = %#v", frame.Response)
	}
	if got, ok := env.GetWorkingValue(intentcontext.ClarificationStateKey); !ok || got == nil {
		t.Fatal("expected clarification state to be written back")
	}
	if got, ok := env.GetWorkingValue("euclo.interaction.frame_requested"); !ok || got != false {
		t.Fatalf("frame_requested = %#v ok=%v, want false", got, ok)
	}
}

func TestResolveInteractionFrameDoesNotResumeOutcomeFeedback(t *testing.T) {
	env := contextdata.NewEnvelope("task-2", "session-2")
	task := &execution.Task{ID: "task-2", Instruction: "collect feedback"}
	env.SetWorkingValueWithClass("task.input", task, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("euclo.interaction.frame_seq", 1, contextdata.MemoryClassTask)

	frame := interaction.NewOutcomeFeedbackFrame("task-2", "session-2", "complete")
	env.SetWorkingValueWithClass("euclo.interaction.frame.0", frame, contextdata.MemoryClassTask)

	executor := &recordingExecutor{}
	rt := &Runtime{
		Agent:                executor,
		interactionEnvelopes: map[string]*contextdata.Envelope{"task-2": env},
	}

	if err := rt.ResolveInteractionFrame(context.Background(), "task-2", frame.ID, "negative", ""); err != nil {
		t.Fatalf("resolve interaction frame failed: %v", err)
	}
	if executor.execCount != 0 {
		t.Fatalf("execute count = %d, want 0", executor.execCount)
	}
	if frame.Response == nil || frame.Response.ChosenSlot != "negative" {
		t.Fatalf("frame response = %#v", frame.Response)
	}
	if got, ok := env.GetWorkingValue("euclo.interaction.frame_requested"); !ok || got != false {
		t.Fatalf("frame_requested = %#v ok=%v, want false", got, ok)
	}
}

func TestSaveAgentManifestWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify_cfg", "agent.yaml")
	if err := os.MkdirAll(filepath.Dir(path), fs.PublicDirMode); err != nil { // public: test dir
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	seed := &config.ManifestSpec{
		Image:   "ghcr.io/example/runtime:0.4.1",
		Runtime: "gvisor",
		Policy: &config.ManifestPolicySpec{
			Permissions: permissions.PermissionSet{
				FileSystem: []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: "/workspace/**"}},
			},
		},
	}
	if err := config.SaveYAML(path, seed); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
	updated := *seed
	updated.Image = "ghcr.io/example/runtime:0.5.0"

	backup, err := SaveManifestSpecWithBackup(path, &updated)
	if err != nil {
		t.Fatalf("save with backup: %v", err)
	}
	if backup == "" {
		t.Fatal("expected backup path")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if string(data) == "" || !strings.Contains(string(data), "0.5.0") {
		t.Fatalf("manifest not updated after save: %s", string(data))
	}
}
