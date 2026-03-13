package agenttest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	appruntime "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	namedfactory "github.com/lexcodex/relurpify/named/factory"
)

func TestBuildAgentUsesBootstrappedEnvironmentConfig(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := writeAgenttestManifest(t, workspace, "testfu")

	agentName := "testfu"
	var capturedCfg *core.Config
	namedfactory.RegisterNamedAgent(agentName, func(_ string, env agentenv.AgentEnvironment) graph.Agent {
		capturedCfg = env.Config
		return &stubNamedAgent{}
	})

	origBootstrap := bootstrapAgentRuntime
	bootstrapAgentRuntime = func(_ string, opts appruntime.AgentBootstrapOptions) (*appruntime.BootstrappedAgentRuntime, error) {
		registry := capability.NewRegistry()
		cfg := &core.Config{
			Name:              "bootstrapped",
			Model:             "boot-model",
			MaxIterations:     17,
			OllamaToolCalling: true,
		}
		return &appruntime.BootstrappedAgentRuntime{
			Registry:    registry,
			Memory:      opts.Memory,
			AgentConfig: cfg,
			Environment: agentenv.AgentEnvironment{
				Model:    opts.Model,
				Registry: registry,
				Memory:   opts.Memory,
				Config:   cfg,
			},
		}, nil
	}
	defer func() { bootstrapAgentRuntime = origBootstrap }()

	_, _, err := buildAgent(
		context.Background(),
		workspace,
		manifestPath,
		agentName,
		nil,
		nil,
		nil,
		RunOptions{},
		nil,
		nil,
		CaseSpec{},
		nil,
	)
	if err != nil {
		t.Fatalf("buildAgent: %v", err)
	}
	if capturedCfg == nil {
		t.Fatal("expected named agent to receive bootstrapped config")
	}
	if capturedCfg.MaxIterations != 17 {
		t.Fatalf("expected MaxIterations 17, got %d", capturedCfg.MaxIterations)
	}
	if !capturedCfg.OllamaToolCalling {
		t.Fatal("expected OllamaToolCalling to come from bootstrapped config")
	}
	if capturedCfg.Model != "boot-model" {
		t.Fatalf("expected model boot-model, got %q", capturedCfg.Model)
	}
}

func TestBuildAgentPropagatesSkipASTIndexToBootstrap(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := writeAgenttestManifest(t, workspace, "testfu")

	var capturedSkipASTIndex bool
	origBootstrap := bootstrapAgentRuntime
	bootstrapAgentRuntime = func(_ string, opts appruntime.AgentBootstrapOptions) (*appruntime.BootstrappedAgentRuntime, error) {
		capturedSkipASTIndex = opts.SkipASTIndex
		registry := capability.NewRegistry()
		cfg := &core.Config{Name: "bootstrapped", MaxIterations: 3}
		return &appruntime.BootstrappedAgentRuntime{
			Registry:    registry,
			Memory:      opts.Memory,
			AgentConfig: cfg,
			Environment: agentenv.AgentEnvironment{
				Model:    opts.Model,
				Registry: registry,
				Memory:   opts.Memory,
				Config:   cfg,
			},
		}, nil
	}
	defer func() { bootstrapAgentRuntime = origBootstrap }()

	_, _, err := buildAgent(
		context.Background(),
		workspace,
		manifestPath,
		"testfu",
		nil,
		nil,
		nil,
		RunOptions{SkipASTIndex: true},
		nil,
		nil,
		CaseSpec{},
		nil,
	)
	if err != nil {
		t.Fatalf("buildAgent: %v", err)
	}
	if !capturedSkipASTIndex {
		t.Fatal("expected SkipASTIndex to propagate to bootstrap")
	}
}

func TestBuildAgentExposesDefaultAgenttestToolsForCoding(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := writeCodingAgenttestManifest(t, workspace)

	agent, _, err := buildAgent(
		context.Background(),
		workspace,
		manifestPath,
		"coding",
		nil,
		nil,
		nil,
		RunOptions{SkipASTIndex: true},
		nil,
		defaultAgenttestAllowedCapabilities(),
		CaseSpec{},
		nil,
	)
	if err != nil {
		t.Fatalf("buildAgent: %v", err)
	}

	reactAgent, ok := agent.(*reactpkg.ReActAgent)
	if !ok {
		t.Fatalf("expected ReActAgent, got %T", agent)
	}
	if reactAgent.Tools == nil {
		t.Fatal("expected bootstrapped tool registry")
	}
	for _, required := range []string{"file_read", "file_write", "file_list"} {
		if _, ok := reactAgent.Tools.Get(required); !ok {
			t.Fatalf("expected %s to remain registered", required)
		}
	}
	tools := reactAgent.Tools.ModelCallableTools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	for _, required := range []string{"file_read", "file_write", "file_list"} {
		if !slices.Contains(names, required) {
			t.Fatalf("expected %s in model-callable tools, got %v", required, names)
		}
	}
}

func TestResolveExecutionAgentNameRoutesCodingArchitectAndWorkflowCases(t *testing.T) {
	if got := resolveExecutionAgentName("coding", CaseSpec{
		TaskType: string(core.TaskTypeAnalysis),
		Context:  map[string]any{"mode": "architect"},
	}); got != "architect" {
		t.Fatalf("expected architect routing, got %q", got)
	}
	if got := resolveExecutionAgentName("coding", CaseSpec{
		TaskType: string(core.TaskTypeAnalysis),
		Context:  map[string]any{"mode": "architect", "workflow_id": "wf-1"},
	}); got != "coding" {
		t.Fatalf("expected coding routing for workflow-backed architect case, got %q", got)
	}
	if got := resolveExecutionAgentName("coding", CaseSpec{
		TaskType: string(core.TaskTypeCodeModification),
		Context:  map[string]any{"workflow_id": "wf-1"},
	}); got != "coding" {
		t.Fatalf("expected coding routing for workflow-backed case, got %q", got)
	}
	if got := resolveExecutionAgentName("coding", CaseSpec{
		TaskType: string(core.TaskTypeCodeModification),
		Context:  map[string]any{},
	}); got != "coding" {
		t.Fatalf("expected coding routing without workflow id, got %q", got)
	}
	if got := resolveExecutionAgentName("coding", CaseSpec{
		TaskType: string(core.TaskTypeAnalysis),
		Context:  map[string]any{"mode": "ask"},
	}); got != "coding" {
		t.Fatalf("expected coding routing for ask mode, got %q", got)
	}
}

func TestDefaultAgenttestAllowlistKeepsRuntimeFileTools(t *testing.T) {
	workspace := t.TempDir()
	bundle, err := appruntime.BuildBuiltinCapabilityBundle(workspace, fsandbox.NewLocalCommandRunner(workspace, nil), appruntime.CapabilityRegistryOptions{
		AgentID: "coding",
		AgentSpec: &core.AgentRuntimeSpec{
			Implementation: "coding",
			Model:          core.AgentModelConfig{Name: "test-model"},
		},
		SkipASTIndex: true,
	})
	if err != nil {
		t.Fatalf("BuildCapabilityRegistry: %v", err)
	}

	for _, required := range []string{"file_read", "file_write", "file_list"} {
		if _, ok := bundle.Registry.Get(required); !ok {
			t.Fatalf("expected runtime registry to include %s before restriction", required)
		}
	}

	applyAgentTestCapabilityDefaults(bundle.Registry, defaultAgenttestAllowedCapabilities())

	for _, required := range []string{"file_read", "file_write", "file_list"} {
		if _, ok := bundle.Registry.Get(required); !ok {
			t.Fatalf("expected runtime registry to include %s after restriction", required)
		}
	}
}

func TestBuildAgentRetainsCaseOverrideAllowedCapabilitiesDuringBootstrap(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := writeCodingAgenttestManifest(t, workspace)
	writeSkillManifest(t, workspace, "system", `apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: system
spec:
  allowed_capabilities:
    - name: file_read
      kind: tool
`)
	writeSkillManifest(t, workspace, "coding", `apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: coding
spec:
  allowed_capabilities:
    - name: file_write
      kind: tool
`)

	allowed := mergeCapabilitySelectors(defaultAgenttestAllowedCapabilities(), []core.CapabilitySelector{{
		Name: "cli_git",
		Kind: core.CapabilityKindTool,
	}})
	agent, _, err := buildAgent(
		context.Background(),
		workspace,
		manifestPath,
		"coding",
		nil,
		nil,
		nil,
		RunOptions{SkipASTIndex: true},
		nil,
		allowed,
		CaseSpec{},
		nil,
	)
	if err != nil {
		t.Fatalf("buildAgent: %v", err)
	}

	reactAgent, ok := agent.(*reactpkg.ReActAgent)
	if !ok {
		t.Fatalf("expected ReActAgent, got %T", agent)
	}
	if _, ok := reactAgent.Tools.Get("cli_git"); !ok {
		t.Fatal("expected cli_git to survive bootstrap restriction")
	}
}

func TestRunCaseRetryRebuildsWorkspace(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := writeAgenttestManifest(t, workspace, "testfu")

	agentName := "testfu"
	shared := &retryCheckShared{}
	namedfactory.RegisterNamedAgent(agentName, func(workspace string, env agentenv.AgentEnvironment) graph.Agent {
		return &retryCheckAgent{
			workspace: workspace,
			shared:    shared,
		}
	})

	origBootstrap := bootstrapAgentRuntime
	bootstrapAgentRuntime = func(_ string, opts appruntime.AgentBootstrapOptions) (*appruntime.BootstrappedAgentRuntime, error) {
		registry := capability.NewRegistry()
		cfg := &core.Config{Name: "retry", MaxIterations: 3, OllamaToolCalling: true}
		return &appruntime.BootstrappedAgentRuntime{
			Registry:    registry,
			Memory:      opts.Memory,
			AgentConfig: cfg,
			Environment: agentenv.AgentEnvironment{
				Model:    opts.Model,
				Registry: registry,
				Memory:   opts.Memory,
				Config:   cfg,
			},
		}, nil
	}
	defer func() { bootstrapAgentRuntime = origBootstrap }()

	suite := &Suite{
		SourcePath: filepath.Join(workspace, "testsuite", "agenttests", "retry.testsuite.yaml"),
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "retry"},
		Spec: SuiteSpec{
			AgentName: agentName,
			Manifest:  filepath.ToSlash(strings.TrimPrefix(manifestPath, workspace+string(os.PathSeparator))),
			Workspace: WorkspaceSpec{Strategy: "derived"},
			Models: []ModelSpec{{
				Name:     "fake-model",
				Endpoint: "http://localhost:11434",
			}},
			Cases: []CaseSpec{{
				Name:   "retry_reset",
				Prompt: "exercise retry",
				Setup: SetupSpec{
					Files: []SetupFileSpec{{
						Path:    "note.txt",
						Content: "seed\n",
					}},
				},
				Expect: ExpectSpec{
					MustSucceed:    true,
					OutputContains: []string{"retry ok"},
				},
			}},
		},
	}
	if err := suite.Validate(); err != nil {
		t.Fatalf("suite.Validate: %v", err)
	}

	report := (&Runner{}).runCase(
		context.Background(),
		suite,
		suite.Spec.Cases[0],
		suite.Spec.Models[0],
		RunOptions{
			TargetWorkspace: workspace,
			OutputDir:       t.TempDir(),
			OllamaResetOn:   []string{"reset requested"},
		},
		workspace,
		t.TempDir(),
	)

	if !report.Success {
		t.Fatalf("expected retry case to succeed, got error %q", report.Error)
	}
	if report.RetryCount != 1 {
		t.Fatalf("expected one retry, got %d", report.RetryCount)
	}
	if shared.secondAttemptSaw != "seed\n" {
		t.Fatalf("expected clean workspace on retry, got %q", shared.secondAttemptSaw)
	}
}

func TestRunCaseDoesNotRetryNonInfraFailureEvenWhenPatternMatches(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := writeAgenttestManifest(t, workspace, "testfu")

	shared := &retryCheckShared{}
	namedfactory.RegisterNamedAgent("testfu", func(workspace string, env agentenv.AgentEnvironment) graph.Agent {
		return &nonInfraRetryAgent{shared: shared}
	})

	origBootstrap := bootstrapAgentRuntime
	bootstrapAgentRuntime = func(_ string, opts appruntime.AgentBootstrapOptions) (*appruntime.BootstrappedAgentRuntime, error) {
		registry := capability.NewRegistry()
		cfg := &core.Config{Name: "retry", MaxIterations: 3, OllamaToolCalling: true}
		return &appruntime.BootstrappedAgentRuntime{
			Registry:    registry,
			Memory:      opts.Memory,
			AgentConfig: cfg,
			Environment: agentenv.AgentEnvironment{
				Model:    opts.Model,
				Registry: registry,
				Memory:   opts.Memory,
				Config:   cfg,
			},
		}, nil
	}
	defer func() { bootstrapAgentRuntime = origBootstrap }()

	suite := &Suite{
		SourcePath: filepath.Join(workspace, "testsuite", "agenttests", "retry.testsuite.yaml"),
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "retry"},
		Spec: SuiteSpec{
			AgentName: "testfu",
			Manifest:  filepath.ToSlash(strings.TrimPrefix(manifestPath, workspace+string(os.PathSeparator))),
			Workspace: WorkspaceSpec{Strategy: "derived"},
			Models:    []ModelSpec{{Name: "fake-model", Endpoint: "http://localhost:11434"}},
			Cases:     []CaseSpec{{Name: "no_retry", Prompt: "no retry"}},
		},
	}

	report := (&Runner{}).runCase(
		context.Background(),
		suite,
		suite.Spec.Cases[0],
		suite.Spec.Models[0],
		RunOptions{
			TargetWorkspace: workspace,
			OutputDir:       t.TempDir(),
			OllamaResetOn:   []string{"reset requested"},
		},
		workspace,
		t.TempDir(),
	)

	if report.RetryCount != 0 {
		t.Fatalf("expected no retry for non-infra failure, got %d", report.RetryCount)
	}
	if shared.attempt != 1 {
		t.Fatalf("expected one execution attempt, got %d", shared.attempt)
	}
}

func TestRunCaseExecutionTimeoutDoesNotIncludeBootstrapTime(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := writeAgenttestManifest(t, workspace, "testfu")

	shared := &executionTimeoutShared{}
	namedfactory.RegisterNamedAgent("testfu", func(_ string, _ agentenv.AgentEnvironment) graph.Agent {
		return &executionTimeoutAgent{shared: shared}
	})

	origBootstrap := bootstrapAgentRuntime
	bootstrapAgentRuntime = func(_ string, opts appruntime.AgentBootstrapOptions) (*appruntime.BootstrappedAgentRuntime, error) {
		time.Sleep(30 * time.Millisecond)
		registry := capability.NewRegistry()
		cfg := &core.Config{Name: "timeout", MaxIterations: 3, OllamaToolCalling: true}
		return &appruntime.BootstrappedAgentRuntime{
			Registry:    registry,
			Memory:      opts.Memory,
			AgentConfig: cfg,
			Environment: agentenv.AgentEnvironment{
				Model:    opts.Model,
				Registry: registry,
				Memory:   opts.Memory,
				Config:   cfg,
			},
		}, nil
	}
	defer func() { bootstrapAgentRuntime = origBootstrap }()

	suite := &Suite{
		SourcePath: filepath.Join(workspace, "testsuite", "agenttests", "timeout.testsuite.yaml"),
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "timeout"},
		Spec: SuiteSpec{
			AgentName: "testfu",
			Manifest:  filepath.ToSlash(strings.TrimPrefix(manifestPath, workspace+string(os.PathSeparator))),
			Workspace: WorkspaceSpec{Strategy: "derived"},
			Models:    []ModelSpec{{Name: "fake-model", Endpoint: "http://localhost:11434"}},
			Cases:     []CaseSpec{{Name: "separate_timeout", Prompt: "wait for context cancellation"}},
		},
	}

	report := (&Runner{}).runCase(
		context.Background(),
		suite,
		suite.Spec.Cases[0],
		suite.Spec.Models[0],
		RunOptions{
			TargetWorkspace:  workspace,
			OutputDir:        t.TempDir(),
			Timeout:          40 * time.Millisecond,
			BootstrapTimeout: 100 * time.Millisecond,
		},
		workspace,
		t.TempDir(),
	)

	if report.Success {
		t.Fatal("expected execution timeout failure")
	}
	if shared.elapsed < 35*time.Millisecond {
		t.Fatalf("expected execute to receive full timeout budget, got %s", shared.elapsed)
	}
}

func TestExtractOutputPrefersStructuredFinalOutput(t *testing.T) {
	state := core.NewContext()
	state.AddInteraction("assistant", "intermediate plan", nil)
	state.AddInteraction("assistant", "another intermediate step", nil)
	state.Set("pipeline.final_output", map[string]any{
		"summary": "final pipeline answer",
	})

	if got := extractOutput(state, nil); got != "final pipeline answer" {
		t.Fatalf("expected structured final output, got %q", got)
	}
}

func TestExtractOutputDoesNotUseAmbiguousAssistantHistory(t *testing.T) {
	state := core.NewContext()
	state.AddInteraction("assistant", "first step", nil)
	state.AddInteraction("assistant", "second step", nil)

	if got := extractOutput(state, nil); got != "" {
		t.Fatalf("expected empty output for ambiguous assistant history, got %q", got)
	}
}

type stubNamedAgent struct{}

func (s *stubNamedAgent) Initialize(_ *core.Config) error { return nil }
func (s *stubNamedAgent) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	return &core.Result{Success: true}, nil
}
func (s *stubNamedAgent) Capabilities() []core.Capability { return nil }
func (s *stubNamedAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	return nil, nil
}

type retryCheckShared struct {
	attempt          int
	secondAttemptSaw string
}

type retryCheckAgent struct {
	workspace string
	shared    *retryCheckShared
}

func (a *retryCheckAgent) Initialize(_ *core.Config) error { return nil }

func (a *retryCheckAgent) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	a.shared.attempt++
	target := filepath.Join(a.workspace, "note.txt")
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	if a.shared.attempt == 1 {
		if err := os.WriteFile(target, []byte("mutated\n"), 0o644); err != nil {
			return nil, err
		}
		return nil, errors.New("ollama error: reset requested")
	}
	a.shared.secondAttemptSaw = string(data)
	if string(data) != "seed\n" {
		return nil, fmt.Errorf("workspace not reset: %q", string(data))
	}
	return &core.Result{
		Success: true,
		Data: map[string]any{
			"final_output": "retry ok",
		},
	}, nil
}

func (a *retryCheckAgent) Capabilities() []core.Capability { return nil }
func (a *retryCheckAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	return nil, nil
}

type nonInfraRetryAgent struct {
	shared *retryCheckShared
}

func (a *nonInfraRetryAgent) Initialize(_ *core.Config) error { return nil }
func (a *nonInfraRetryAgent) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	a.shared.attempt++
	return nil, errors.New("reset requested")
}
func (a *nonInfraRetryAgent) Capabilities() []core.Capability { return nil }
func (a *nonInfraRetryAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	return nil, nil
}

type executionTimeoutShared struct {
	elapsed time.Duration
}

type executionTimeoutAgent struct {
	shared *executionTimeoutShared
}

func (a *executionTimeoutAgent) Initialize(_ *core.Config) error { return nil }
func (a *executionTimeoutAgent) Execute(ctx context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	start := time.Now()
	<-ctx.Done()
	a.shared.elapsed = time.Since(start)
	return nil, ctx.Err()
}
func (a *executionTimeoutAgent) Capabilities() []core.Capability { return nil }
func (a *executionTimeoutAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	return nil, nil
}

func writeAgenttestManifest(t *testing.T, workspace, agentName string) string {
	t.Helper()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifestData := fmt.Sprintf(`apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: %s
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  agent:
    implementation: coding
    mode: primary
    model:
      provider: ollama
      name: test-model
  defaults:
    permissions:
      filesystem:
        - action: fs:read
          path: %q
          justification: Read workspace
        - action: fs:list
          path: %q
          justification: List workspace
        - action: fs:write
          path: %q
          justification: Write workspace
`, agentName, filepath.ToSlash(filepath.Join(workspace, "**")), filepath.ToSlash(filepath.Join(workspace, "**")), filepath.ToSlash(filepath.Join(workspace, "**")))
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifestPath
}

func writeCodingAgenttestManifest(t *testing.T, workspace string) string {
	t.Helper()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifestData := fmt.Sprintf(`apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: coding
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  agent:
    implementation: coding
    mode: primary
    model:
      provider: ollama
      name: test-model
  defaults:
    permissions:
      filesystem:
        - action: fs:read
          path: %q
          justification: Read workspace
        - action: fs:list
          path: %q
          justification: List workspace
        - action: fs:write
          path: %q
          justification: Write workspace
  skills:
    - system
    - coding
`, filepath.ToSlash(filepath.Join(workspace, "**")), filepath.ToSlash(filepath.Join(workspace, "**")), filepath.ToSlash(filepath.Join(workspace, "**")))
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifestPath
}

func writeSkillManifest(t *testing.T, workspace, name, contents string) {
	t.Helper()
	path := filepath.Join(workspace, "relurpify_cfg", "skills", name, "skill.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write skill manifest: %v", err)
	}
}
