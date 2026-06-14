package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	executioncompiler "codeburg.org/lexbit/relurpify/execution/compiler"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/euclokeys"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type recordingRunner struct {
	mu       sync.Mutex
	requests []sandbox.CommandRequest
}

type fakeSandboxRuntime struct {
	mu     sync.Mutex
	policy governanceports.SandboxPolicy
	runner *recordingRunner
}

type scenarioState struct {
	mu       sync.Mutex
	scenario string
}

type offlineScenarioModel struct {
	inner    model.LanguageModel
	scenario func() string
	mu       sync.Mutex
	chatTool int
	tools    []string
}

func (r *recordingRunner) Run(_ context.Context, req sandbox.CommandRequest) (*ports.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
	return &ports.CommandResult{
		Stdout:      "sandbox output",
		StdoutBytes: int64(len("sandbox output")),
		ExitCode:    0,
	}, nil
}

func (r *recordingRunner) snapshot() []sandbox.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]sandbox.CommandRequest, len(r.requests))
	copy(out, r.requests)
	return out
}

func (r *recordingRunner) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
}

func (f *fakeSandboxRuntime) Verify(context.Context) error { return nil }

func (f *fakeSandboxRuntime) ValidatePolicy(governanceports.SandboxPolicy) error { return nil }

func (f *fakeSandboxRuntime) ApplyPolicy(_ context.Context, policy governanceports.SandboxPolicy) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.policy = policy
	return nil
}

func (f *fakeSandboxRuntime) Policy() governanceports.SandboxPolicy {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.policy
}

func (f *fakeSandboxRuntime) RunConfig() governanceports.SandboxConfig {
	return governanceports.SandboxConfig{}
}

func (f *fakeSandboxRuntime) Name() string { return "fake" }

func (f *fakeSandboxRuntime) NewCommandRunner(*sandbox.CommandRunnerConfig) (sandbox.CommandRunner, error) {
	if f.runner == nil {
		return nil, nil
	}
	return f.runner, nil
}

func (s *scenarioState) set(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scenario = strings.TrimSpace(value)
}

func (s *scenarioState) get() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scenario
}

func (m *offlineScenarioModel) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	m.inject(options)
	return m.inner.Generate(ctx, prompt, options)
}

func (m *offlineScenarioModel) GenerateStream(ctx context.Context, prompt string, options *model.LLMOptions) (<-chan string, error) {
	m.inject(options)
	return m.inner.GenerateStream(ctx, prompt, options)
}

func (m *offlineScenarioModel) Chat(ctx context.Context, messages []model.Message, options *model.LLMOptions) (*model.LLMResponse, error) {
	m.inject(options)
	return m.inner.Chat(ctx, messages, options)
}

func (m *offlineScenarioModel) ChatWithTools(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	m.inject(options)
	m.mu.Lock()
	m.chatTool++
	m.tools = m.tools[:0]
	for _, tool := range tools {
		m.tools = append(m.tools, tool.Name)
	}
	m.mu.Unlock()
	return m.inner.ChatWithTools(ctx, messages, tools, options)
}

func (m *offlineScenarioModel) chatWithToolsCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chatTool
}

func (m *offlineScenarioModel) lastToolNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.tools))
	copy(out, m.tools)
	return out
}

func (m *offlineScenarioModel) ToolRepairStrategy() string {
	if profiled, ok := m.inner.(model.ProfiledModel); ok {
		return profiled.ToolRepairStrategy()
	}
	return "heuristic"
}

func (m *offlineScenarioModel) MaxToolsPerCall() int {
	if profiled, ok := m.inner.(model.ProfiledModel); ok {
		return profiled.MaxToolsPerCall()
	}
	return 1
}

func (m *offlineScenarioModel) UsesNativeToolCalling() bool {
	if profiled, ok := m.inner.(model.ProfiledModel); ok {
		return profiled.UsesNativeToolCalling()
	}
	return true
}

func (m *offlineScenarioModel) inject(options *model.LLMOptions) {
	if options == nil {
		return
	}
	if options.Config == nil {
		options.Config = make(map[string]any)
	}
	if scenario := strings.TrimSpace(m.scenario()); scenario != "" {
		options.Config["offline_scenario"] = scenario
	} else {
		delete(options.Config, "offline_scenario")
	}
}

func TestBootTurnCompilePersistOfflineEucloRecipes(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"notes.txt":      "initial workspace notes\n",
			"workspace.md":   "# Workspace\n\nseeded for slice 4\n",
			"scratch/readme": "scratchpad\n",
		},
		Recipes: map[string]string{
			"clarify_scope.euclo": `thoughtrecipe clarify_scope
"Clarify the workspace scope."

trigger as intent:
  family ["slice4_clarify"]
  keyword ["clarify", "scope", "question"]
  handoff ["intent_clarify"]
  may read workspace

agent clarifier uses react

run clarifier:
  goal "Summarize the workspace scope that should be reviewed."
`,
			"review_changes.euclo": `thoughtrecipe review_changes
"Review workspace changes."

trigger as capability:
  family ["review"]
  keyword ["review", "changes", "diff"]
  may read workspace
  may invoke ["cli_git"]

agent reviewer uses react

run reviewer:
  may invoke ["cli_git"]
  goal "Review the workspace changes and summarize the diff."
`,
		},
	})
	testhelper.InitGitRepo(t, workspace)

	runner := &recordingRunner{}
	scenario := &scenarioState{}
	offline := &offlineScenarioModel{}
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.InferenceNativeToolCalling = true
	cfg.ModelFactoryWrapper = func(base model.ModelFactory) model.ModelFactory {
		return func(tel model.Telemetry, debug bool) model.LanguageModel {
			offline.inner = base(tel, debug)
			offline.scenario = func() string { return scenario.get() }
			return offline
		}
	}
	cfg.SecurityRunner = runner
	cfg.CommandPolicy = sandbox.CommandPolicyFunc(func(context.Context, sandbox.CommandRequest) error { return nil })
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{runner: runner}, nil
	}

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	cancelHITL := autoApproveHITL(t, rt)
	defer func() {
		cancelHITL()
		if err := rt.Close(context.Background()); err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	}()

	t.Run("intent recipe", func(t *testing.T) {
		runner.reset()
		scenario.set("tool:cli_git:diff --stat HEAD")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := rt.SubmitTurn(ctx, "clarify the workspace scope before reviewing changes", execution.TaskTypeAnalysis, map[string]any{
			"source": "slice4",
		}, nil)
		if err != nil {
			t.Fatalf("submit intent turn: %v", err)
		}
		if result == nil || !result.Success {
			t.Fatalf("intent result = %#v", result)
		}
		if got := runner.snapshot(); len(got) != 0 {
			t.Fatalf("intent turn unexpectedly executed commands: %#v", got)
		}
	})

	t.Run("capability recipe", func(t *testing.T) {
		notesPath := filepath.Join(workspace, "notes.txt")
		if err := os.WriteFile(notesPath, []byte("initial workspace notes\nupdated for review\n"), 0o600); err != nil {
			t.Fatalf("update notes: %v", err)
		}
		runGit(t, workspace, "add", "notes.txt")
		runGit(t, workspace, "commit", "-m", "slice 4 workspace update")

		runner.reset()
		loadedRecipes, err := thoughtrecipepkg.NewLoader().LoadWorkspace(workspace)
		if err != nil {
			t.Fatalf("load thoughtrecipe workspace: %v", err)
		}
		if loadedRecipes == nil || loadedRecipes.Registry == nil {
			t.Fatal("expected thoughtrecipe registry from workspace")
		}
		if plan, ok := loadedRecipes.Registry.GetPlan("review_changes"); ok && len(plan.Steps) > 0 {
			t.Logf("review_changes effective tools: %v", plan.Steps[0].EffectiveToolNames)
		}
		deps := &paradigm.Deps{
			Config: &execution.Config{
				Model:             rt.Config.InferenceModel,
				NativeToolCalling: true,
			},
			Model:          rt.Model,
			Registry:       rt.Tools,
			CommandRunner:  rt.Workspace.Environment.CommandRunner,
			CommandPolicy:  rt.Workspace.Environment.CommandPolicy,
			WorkingMemory:  rt.Memory,
			IndexManager:   rt.IndexManager,
			SearchEngine:   rt.SearchEngine,
			StreamTrigger:  rt.Workspace.Environment.StreamTrigger,
			OutputIngester: rt.Workspace.Environment.OutputIngester,
			IngestOutputs:  rt.Workspace.Environment.IngestOutputs,
			PromptRegistry: rt.Workspace.Environment.PromptRegistry,
			AgentLifecycle: rt.AgentLifecycle,
		}
		if rt.Workspace == nil || rt.Workspace.Environment.CommandRunner == nil {
			t.Fatalf("runtime command runner unavailable: workspace=%v runner=%T", rt.Workspace, rt.Workspace.Environment.CommandRunner)
		}
		graph, err := orchestrate.NewRootGraph(context.Background(), orchestrate.RootGraphDeps{
			Workspace:            workspace,
			DispatchCapabilities: rt.Tools,
			ThoughtRecipes:       loadedRecipes.Registry,
			Paradigm:             deps,
			StreamTrigger:        rt.Workspace.Environment.StreamTrigger,
			Checkpoints:          rt.AgentLifecycle,
		})
		if err != nil {
			t.Fatalf("build root graph: %v", err)
		}
		env := contextdata.NewEnvelope("task-capability", "session-capability")
		task := &execution.Task{
			ID:          "task-capability",
			Type:        string(execution.TaskTypeExecute),
			Instruction: "review the workspace changes with cli_git and summarize the diff",
			Context: map[string]any{
				"workspace":    workspace,
				"euclo.family": "review",
				"euclo.user_files": []string{
					"notes.txt",
				},
			},
			Metadata: map[string]any{
				"source": "slice4",
			},
		}
		contextdata.SetTyped(env, euclokeys.KeyTaskInput, task)
		contextdata.SetTyped(env, euclokeys.KeyTaskRaw, task)
		euclostate.SetRouteSelection(env, &euclotypes.RouteSelection{
			RouteKind:       euclotypes.RouteKindThoughtRecipe,
			ThoughtRecipeID: "review_changes",
		})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := graph.Execute(ctx, env); err != nil {
			t.Fatalf("execute capability graph: %v", err)
		}
		visibleTools := offline.lastToolNames()
		t.Logf("offline tools visible: %v", visibleTools)
		if rt.Tools != nil {
			callable := rt.Tools.ModelCallableTools(context.Background())
			t.Logf("runtime callable tools: %v", toolNames(callable))
	}
	if got := offline.chatWithToolsCalls(); got == 0 {
		t.Fatal("expected the offline react model to issue at least one tool call")
	}
		if !hasString(visibleTools, "cli_git") {
			t.Fatalf("expected cli_git in visible tools, got %v", visibleTools)
		}

		requests := runner.snapshot()
		if len(requests) == 0 {
			t.Fatal("expected a sandbox command request")
		}
		foundDiff := false
		for _, req := range requests {
			if req.Workdir != workspace {
				t.Fatalf("sandbox workdir = %q, want %q", req.Workdir, workspace)
			}
			if len(req.Args) >= 2 && req.Args[0] == "git" && req.Args[1] == "diff" {
				foundDiff = true
			}
		}
		if !foundDiff {
			t.Fatalf("expected one sandbox request to be git diff, got %#v", requests)
		}

		repo := executioncompiler.NewCompilerRepository(rt.GraphDB)
		records, err := repo.ListCompilationRecords(context.Background(), 0)
		if err != nil {
			t.Fatalf("list compilation records: %v", err)
		}
		if len(records) == 0 {
			t.Fatal("expected persisted compilation record")
		}
		if got := strings.TrimSpace(records[0].RequestID); got == "" {
			t.Fatal("persisted compilation record missing request ID")
		}
	})
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func toolNames(tools []ports.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		names = append(names, tool.Name())
	}
	return names
}

func autoApproveHITL(t *testing.T, rt *relurpishruntime.Runtime) func() {
	t.Helper()

	ch, cancel := rt.SubscribeHITL()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range ch {
			if ev.Request == nil {
				continue
			}
			safeApproveHITL(rt, ev.Request.ID)
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func safeApproveHITL(rt *relurpishruntime.Runtime, requestID string) {
	defer func() {
		_ = recover()
	}()
	_ = rt.ApproveHITL(requestID, "e2e", "", 0)
}

func runGit(t *testing.T, workspace string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
