package e2e

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/model"
)

// recordingRunner records every command request that reaches it.
type recordingRunner struct {
	mu       sync.Mutex
	requests []sandbox.CommandRequest
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

// fakeSandboxRuntime satisfies governanceports.SandboxRuntime without a real backend.
type fakeSandboxRuntime struct {
	mu     sync.Mutex
	policy governanceports.SandboxPolicy
	runner *recordingRunner
}

func (f *fakeSandboxRuntime) Verify(context.Context) error                              { return nil }
func (f *fakeSandboxRuntime) ValidatePolicy(governanceports.SandboxPolicy) error         { return nil }
func (f *fakeSandboxRuntime) ApplyPolicy(_ context.Context, p governanceports.SandboxPolicy) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.policy = p
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

// scenarioState provides thread-safe scenario injection for the offline model.
type scenarioState struct {
	mu       sync.Mutex
	scenario string
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

// offlineScenarioModel wraps a real model and injects offline_scenario into
// every LLMOptions.Config so the offline model backend returns deterministic
// tool calls.
type offlineScenarioModel struct {
	inner    model.LanguageModel
	scenario func() string
	mu       sync.Mutex
	chatTool int
	tools    []string
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

// autoApproveHITL subscribes to the runtime's HITL bus and approves every
// request. It returns a cancel function that must be called before Close.
func autoApproveHITL(t *testing.T, rt *runtime.Runtime) func() {
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

func safeApproveHITL(rt *runtime.Runtime, requestID string) {
	defer func() { _ = recover() }()
	_ = rt.ApproveHITL(requestID, "e2e", "", 0)
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

func runGit(t *testing.T, workspace string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
