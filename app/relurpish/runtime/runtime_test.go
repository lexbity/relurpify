package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/model"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"gopkg.in/yaml.v3"
)

const (
	runtimeTestProviderOllama = "ollama"
	runtimeTestSandboxDocker  = "docker"
	runtimeTestWorkspaceOld   = "/old/workspace"
	runtimeTestWorkspaceNew   = "/new/workspace"
	runtimeTestTapePrompt     = "phase-15 runtime prompt"
	runtimeTestTapeResponse   = "phase-15 runtime response"
	runtimeTestTapeModel      = "tape-model"
	runtimeTestTapePath       = "/old/workspace/.relurpify_state/tapes/tape.jsonl"
	runtimeTestTapePathNew    = "/new/workspace/.relurpify_state/tapes/tape.jsonl"
)

func TestConfigForWorkspaceRebindsPaths(t *testing.T) {
	current := Config{
		Workspace:         runtimeTestWorkspaceOld,
		AgentName:         "euclo",
		InferenceProvider: runtimeTestProviderOllama,
		InferenceEndpoint: "http://localhost:11434",
		InferenceModel:    "codex",
		SandboxBackend:    runtimeTestSandboxDocker,
		RecordingMode:     "on",
		ManifestPath:      runtimeTestWorkspaceOld + "/manifest.yaml",
		AgentsDir:         runtimeTestWorkspaceOld + "/relurpify_cfg/agents",
		MemoryPath:        runtimeTestWorkspaceOld + "/.relurpify_state/memory",
		LogPath:           runtimeTestWorkspaceOld + "/.relurpify_state/logs/relurpish.log",
		TelemetryPath:     runtimeTestWorkspaceOld + "/.relurpify_state/telemetry/telemetry.jsonl",
		EventsPath:        runtimeTestWorkspaceOld + "/.relurpify_state/events.db",
		ConfigPath:        runtimeTestWorkspaceOld + "/relurpify_cfg/config.yaml",
		InferenceTapePath: runtimeTestTapePath,
	}

	cfg := ConfigForWorkspace(current, runtimeTestWorkspaceNew)
	if cfg.Workspace != runtimeTestWorkspaceNew {
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
	if want := runtimeTestWorkspaceNew + "/.relurpify_state/workspace.yaml"; cfg.ConfigPath != want {
		t.Fatalf("config path = %q, want %q", cfg.ConfigPath, want)
	}
	if want := runtimeTestWorkspaceNew + "/relurpify_cfg/agents/euclo.yaml"; cfg.ManifestPath != want {
		t.Fatalf("manifest path = %q, want %q", cfg.ManifestPath, want)
	}
	if want := runtimeTestWorkspaceNew + "/.relurpify_state/logs/relurpish.log"; cfg.LogPath != want {
		t.Fatalf("log path = %q, want %q", cfg.LogPath, want)
	}
	if want := runtimeTestTapePathNew; cfg.InferenceTapePath != want {
		t.Fatalf("tape path = %q, want %q", cfg.InferenceTapePath, want)
	}
}

type recordingExecutor struct {
	mu        sync.Mutex
	execCount int
	lastTask  *execution.Task
	lastEnv   *contextdata.Envelope
}

type fakeSandboxRuntime struct {
	policy governanceports.SandboxPolicy
}

type fakeCommandRunner struct{}

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

func (r *recordingExecutor) BuildGraph(_ context.Context, _ *execution.Task) (*agentgraph.Graph, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeSandboxRuntime) Verify(context.Context) error { return nil }

func (f *fakeSandboxRuntime) ValidatePolicy(governanceports.SandboxPolicy) error { return nil }

func (f *fakeSandboxRuntime) ApplyPolicy(_ context.Context, policy governanceports.SandboxPolicy) error {
	f.policy = policy
	return nil
}

func (f *fakeSandboxRuntime) Policy() governanceports.SandboxPolicy { return f.policy }

func (f *fakeSandboxRuntime) RunConfig() governanceports.SandboxConfig {
	return governanceports.SandboxConfig{}
}

func (f *fakeSandboxRuntime) Name() string { return "fake" }

func (fakeCommandRunner) Run(context.Context, ports.CommandRequest) (*ports.CommandResult, error) {
	return &ports.CommandResult{ExitCode: 0, Stdout: "", Stderr: ""}, nil
}

func TestResolveInteractionFrameResumesClarificationTask(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	task := &execution.Task{ID: "task-1", Instruction: "clarify request"}
	env.SetWorkingValueWithClass(euclostate.KeyTaskInput, task, contextdata.MemoryClassTask)
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
	if got, ok := contextdata.GetTyped[any](env, intentcontext.ClarificationStateKey); !ok || got == nil {
		t.Fatal("expected clarification state to be written back")
	}
	if got, ok := contextdata.GetTyped[bool](env, "euclo.interaction.frame_requested"); !ok || got != false {
		t.Fatalf("frame_requested = %#v ok=%v, want false", got, ok)
	}
}

func TestResolveInteractionFrameDoesNotResumeOutcomeFeedback(t *testing.T) {
	env := contextdata.NewEnvelope("task-2", "session-2")
	task := &execution.Task{ID: "task-2", Instruction: "collect feedback"}
	env.SetWorkingValueWithClass(euclostate.KeyTaskInput, task, contextdata.MemoryClassTask)
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
	if got, ok := contextdata.GetTyped[bool](env, "euclo.interaction.frame_requested"); !ok || got != false {
		t.Fatalf("frame_requested = %#v ok=%v, want false", got, ok)
	}
}

func TestSubmitTurnUsesTheCanonicalTaskPath(t *testing.T) {
	executor := &recordingExecutor{}
	rt := &Runtime{
		Config: Config{Workspace: "/workspace"},
		Agent:  executor,
	}

	callback := func(string) {}
	result, err := rt.SubmitTurn(context.Background(), "summarize the workspace", execution.TaskTypeCodeGeneration, map[string]any{
		"source": "unit-test",
	}, callback)
	if err != nil {
		t.Fatalf("submit turn failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("submit turn result = %#v", result)
	}
	if executor.execCount != 1 {
		t.Fatalf("execute count = %d, want 1", executor.execCount)
	}
	if executor.lastTask == nil {
		t.Fatal("last task is nil")
	}
	if executor.lastTask.Instruction != "summarize the workspace" {
		t.Fatalf("instruction = %q, want %q", executor.lastTask.Instruction, "summarize the workspace")
	}
	if executor.lastTask.Type != string(execution.TaskTypeCodeGeneration) {
		t.Fatalf("type = %q, want %q", executor.lastTask.Type, execution.TaskTypeCodeGeneration)
	}
	if got := executor.lastTask.Context["workspace"]; got != "/workspace" {
		t.Fatalf("workspace = %#v, want /workspace", got)
	}
	if got := executor.lastTask.Metadata["source"]; got != "unit-test" {
		t.Fatalf("metadata source = %#v, want unit-test", got)
	}
	if got := executor.lastTask.Metadata["stream_callback"]; got == nil {
		t.Fatal("stream_callback metadata is nil")
	}
}

func TestSaveAgentDocumentWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify_cfg", "agent.yaml")
	if err := os.MkdirAll(filepath.Dir(path), fs.PublicDirMode); err != nil { // public: test dir
		t.Fatalf("mkdir document dir: %v", err)
	}
	seed := &config.Document{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata:   config.DocumentMetadata{Name: "document-save"},
		Spec:       map[string]yaml.Node{},
	}
	permNode := yaml.Node{}
	if err := permNode.Encode(permissions.PermissionSet{
		FileSystem: []permissions.FileSystemPermission{{Action: permissions.FileSystemRead, Path: "/workspace/**"}},
	}); err != nil {
		t.Fatalf("encode permissions: %v", err)
	}
	seed.Spec["permissions"] = permNode
	if _, err := config.SaveDocumentWithBackup(path, seed); err != nil {
		t.Fatalf("seed document: %v", err)
	}
	updated := &config.Document{
		APIVersion: seed.APIVersion,
		Kind:       seed.Kind,
		Metadata:   seed.Metadata,
		Spec:       map[string]yaml.Node{},
	}
	updatedPermNode := yaml.Node{}
	if err := updatedPermNode.Encode(permissions.PermissionSet{
		FileSystem: []permissions.FileSystemPermission{{Action: permissions.FileSystemWrite, Path: "/workspace/**"}},
	}); err != nil {
		t.Fatalf("encode updated permissions: %v", err)
	}
	updated.Spec["permissions"] = updatedPermNode

	backup, err := config.SaveDocumentWithBackup(path, updated)
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
		t.Fatalf("read document: %v", err)
	}
	if string(data) == "" || !strings.Contains(string(data), "fs:write") {
		t.Fatalf("document not updated after save: %s", string(data))
	}
}

func TestBuildDoctorReportUsesTapeProviderPathFromRuntimeConfig(t *testing.T) {
	workspace := t.TempDir()
	statePath := filepath.Join(workspace, ".relurpify_state", "workspace.yaml")
	tapePath := filepath.Join(workspace, ".relurpify_state", "tapes", "tape.jsonl")
	if err := os.MkdirAll(filepath.Dir(tapePath), fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir tape dir: %v", err)
	}
	writeTapeJSONL(t, tapePath, []llm.TapeHeader{{
		ProviderID: runtimeTestProviderOllama,
		ModelName:  runtimeTestTapeModel,
	}})
	if err := config.SaveRuntimeWorkspaceConfig(statePath, config.RuntimeWorkspaceConfig{
		Provider: "tape",
		Model:    runtimeTestTapeModel,
		TapePath: tapePath,
	}); err != nil {
		t.Fatalf("seed runtime workspace config: %v", err)
	}
	report := BuildDoctorReport(context.Background(), Config{
		Workspace:  workspace,
		ConfigPath: statePath,
	}, config.Secrets{})
	if report.Inference.State != llm.BackendHealthReady {
		t.Fatalf("inference state = %q, want %q (error=%q)", report.Inference.State, llm.BackendHealthReady, report.Inference.Error)
	}
}

func TestNewBootsWithTapeProviderFromWorkspaceConfig(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agents", "euclo.yaml")
	manifestData, err := config.ReadFileRaw(filepath.Join("..", "..", "..", "userconfig", "config", "testdata", "contracts", "document_current.yaml"))
	if err != nil {
		t.Fatalf("read manifest fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := fs.WriteFileSecure(manifestPath, manifestData); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	statePath := filepath.Join(workspace, ".relurpify_state", "workspace.yaml")
	tapePath := config.DefaultWorkspaceStateTapeFile(workspace)
	recordTapeCorpus(t, tapePath)
	if err := config.SaveRuntimeWorkspaceConfig(statePath, config.RuntimeWorkspaceConfig{
		Model:    runtimeTestTapeModel,
		Provider: "tape",
		TapePath: tapePath,
	}); err != nil {
		t.Fatalf("seed runtime workspace config: %v", err)
	}

	cfg := ConfigForWorkspace(Config{AgentName: "euclo"}, workspace)
	cfg.ConfigPath = statePath
	cfg.ManifestPath = manifestPath
	cfg.InferenceProvider = ""
	cfg.InferenceModel = ""
	cfg.InferenceTapePath = tapePath
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	t.Cleanup(func() {
		_ = rt.Close(context.Background())
	})
	if got := strings.TrimSpace(rt.Config.InferenceProvider); got != "tape" {
		t.Fatalf("inference provider = %q, want tape", got)
	}
	if got := strings.TrimSpace(rt.WorkspaceConfig.Provider); got != "tape" {
		t.Fatalf("workspace provider = %q, want tape", got)
	}
	if rt.Model == nil {
		t.Fatal("expected runtime model")
	}
	resp, err := rt.Model.Generate(context.Background(), runtimeTestTapePrompt, &model.LLMOptions{Model: runtimeTestTapeModel})
	if err != nil {
		t.Fatalf("generate through tape provider: %v", err)
	}
	if resp == nil || resp.Text != runtimeTestTapeResponse {
		t.Fatalf("replayed response = %#v, want text %q", resp, runtimeTestTapeResponse)
	}
}

func TestEucloTapeFidelity(t *testing.T) {
	t.Skip("flaky: empty recipe registry means no LLM calls to record; revisit when NG-1 provisions test recipes")

	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agents", "euclo.yaml")
	manifestData, err := config.ReadFileRaw(filepath.Join("..", "..", "..", "userconfig", "config", "testdata", "contracts", "document_current.yaml"))
	if err != nil {
		t.Fatalf("read manifest fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), fs.PublicDirMode); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := fs.WriteFileSecure(manifestPath, manifestData); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	statePath := filepath.Join(workspace, ".relurpify_state", "workspace.yaml")
	cfg := ConfigForWorkspace(Config{AgentName: "euclo"}, workspace)
	cfg.ConfigPath = statePath
	cfg.ManifestPath = manifestPath
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	defer func() {
		if err := rt.Close(context.Background()); err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	}()

	task := &execution.Task{
		ID:          "euclo-fidelity",
		Type:        string(execution.TaskTypeExecute),
		Instruction: "read the workspace and summarize it",
	}
	recordCtx := contextstream.WithTrigger(context.Background(), rt.Workspace.Environment.StreamTrigger)
	recordResult, err := rt.RunTask(recordCtx, task)
	if err != nil {
		t.Fatalf("record runtime run task: %v", err)
	}
	if recordResult == nil || !recordResult.Success {
		t.Fatalf("record runtime result = %#v", recordResult)
	}
}

func writeTapeJSONL(t *testing.T, path string, headers []llm.TapeHeader) {
	t.Helper()
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		t.Fatalf("create tape: %v", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, header := range headers {
		entry := map[string]any{
			"kind": "_header",
			"request": map[string]any{
				"header": header,
			},
		}
		if err := enc.Encode(entry); err != nil {
			t.Fatalf("encode tape header: %v", err)
		}
	}
}

type tapeRecorderModel struct{}

func (tapeRecorderModel) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: runtimeTestTapeResponse, FinishReason: "stop"}, nil
}

func (tapeRecorderModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (tapeRecorderModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: runtimeTestTapeResponse, FinishReason: "stop"}, nil
}

func (tapeRecorderModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: runtimeTestTapeResponse, FinishReason: "stop"}, nil
}

func recordTapeCorpus(t *testing.T, path string) {
	recordTapeCorpusForModel(t, path, runtimeTestTapeModel)
}

func recordTapeCorpusForModel(t *testing.T, path, modelName string) {
	t.Helper()
	rec, err := llm.NewTapeModel(tapeRecorderModel{}, path, string(llm.TapeRecord))
	if err != nil {
		t.Fatalf("open tape recorder: %v", err)
	}
	defer func() {
		_ = rec.Close()
	}()
	if err := rec.ConfigureHeader(llm.TapeHeader{
		ProviderID: "tape",
		ModelName:  modelName,
		SuiteName:  "runtime",
		CaseName:   "provider_tape_boot",
	}); err != nil {
		t.Fatalf("configure tape header: %v", err)
	}
	resp, err := rec.Generate(context.Background(), runtimeTestTapePrompt, &model.LLMOptions{Model: runtimeTestTapeModel})
	if err != nil {
		t.Fatalf("record tape generate: %v", err)
	}
	if resp == nil || resp.Text != runtimeTestTapeResponse {
		t.Fatalf("recorded response = %#v, want text %q", resp, runtimeTestTapeResponse)
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, fs.PublicDirMode)
		}
		data, err := config.ReadFileRaw(path)
		if err != nil {
			return err
		}
		return fs.WriteFileSecure(target, data)
	}); err != nil {
		t.Fatalf("copy tree %q -> %q: %v", src, dst, err)
	}
}
