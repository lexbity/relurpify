package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/agents"
	appruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/policybundle"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	euclotypes "codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

type eucloSummaryExecutor struct {
	result  *core.Result
	stateFn func(*core.Context)
}

func (e *eucloSummaryExecutor) Initialize(config *core.Config) error { return nil }

func (e *eucloSummaryExecutor) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if e.stateFn != nil {
		e.stateFn(state)
	}
	if e.result != nil {
		return e.result, nil
	}
	return &core.Result{NodeID: "done", Success: true}, nil
}

func (e *eucloSummaryExecutor) Capabilities() []core.Capability { return nil }

func (e *eucloSummaryExecutor) BuildGraph(task *core.Task) (*graph.Graph, error) { return nil, nil }

func stubEucloWorkspaceFn(t *testing.T, ws string, skillResults []frameworkskills.SkillResolution) {
	t.Helper()
	origOpenWorkspace := openWorkspaceFn
	t.Cleanup(func() { openWorkspaceFn = origOpenWorkspace })
	openWorkspaceFn = func(ctx context.Context, cfg ayenitd.WorkspaceConfig) (*ayenitd.Workspace, error) {
		manifestPath := cfg.ManifestPath
		if manifestPath == "" {
			manifestPath = filepath.Join(ws, "relurpify_cfg", "agents", "coding.yaml")
		}
		loaded, err := manifest.LoadAgentManifest(manifestPath)
		if err != nil {
			return nil, err
		}
		effectivePerms, err := manifest.ResolveEffectivePermissions(ws, loaded)
		if err != nil {
			return nil, err
		}
		hitl := authorization.NewHITLBroker(cfg.HITLTimeout)
		perms, err := authorization.NewPermissionManager(ws, &effectivePerms, nil, hitl)
		if err != nil {
			return nil, err
		}
		env := ayenitd.WorkspaceEnvironment{
			Config: &core.Config{
				Name:              cfg.AgentName,
				Model:             cfg.InferenceModel,
				InferenceEndpoint: cfg.InferenceEndpoint,
				MaxIterations:     cfg.MaxIterations,
				NativeToolCalling: loaded.Spec.Agent != nil && loaded.Spec.Agent.NativeToolCallingEnabled(),
				AgentSpec:         loaded.Spec.Agent,
				DebugLLM:          cfg.DebugLLM,
				DebugAgent:        cfg.DebugAgent,
			},
			Registry:          capability.NewRegistry(),
			PermissionManager: perms,
			IndexManager:      &ast.IndexManager{},
			GuidanceBroker:    nil,
		}
		engine, err := authorization.FromAgentSpecWithConfig(loaded.Spec.Agent, loaded.Metadata.Name, perms)
		if err != nil {
			return nil, err
		}
		compiled := &policybundle.CompiledPolicyBundle{
			AgentID: loaded.Metadata.Name,
			Spec:    loaded.Spec.Agent,
			Engine:  engine,
		}
		env.Registry.SetPolicyEngine(engine)
		return &ayenitd.Workspace{
			Environment:    env,
			Registration:   &authorization.AgentRegistration{ID: loaded.Metadata.Name, Manifest: loaded, Permissions: perms, HITL: hitl, Policy: compiledEngine(compiled)},
			AgentSpec:      loaded.Spec.Agent,
			CompiledPolicy: compiled,
			SkillResults:   skillResults,
			ServiceManager: ayenitd.NewServiceManager(),
		}, nil
	}
}

func TestStartCmdEucloReadyHintWhenModeMissing(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeCodingAgentManifestFixture(t, ws, "coding")

	cmd := newStartCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("agent", "coding"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Available modes:") {
		t.Fatalf("expected euclo mode hint, got %q", out.String())
	}
	if strings.Contains(out.String(), "default") && !strings.Contains(out.String(), "Use --mode") {
		t.Fatalf("hint should not advertise default mode fallback: %q", out.String())
	}
}

func TestStartCmdRejectsUnknownEucloMode(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeCodingAgentManifestFixture(t, ws, "coding")

	cmd := newStartCmd()
	if err := cmd.Flags().Set("agent", "coding"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("mode", "bananas"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("instruction", "validate mode rejection"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected invalid mode error")
	} else if !strings.Contains(err.Error(), "unknown euclo mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartCmdEucloJSONSummary(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeCodingAgentManifestFixture(t, ws, "coding")
	stubEucloWorkspaceFn(t, ws, []frameworkskills.SkillResolution{{
		Name:    "demo-skill",
		Applied: true,
		Paths: frameworkskills.SkillPaths{
			Root: filepath.Join(ws, "skills", "demo-skill"),
		},
	}})

	origRegisterProviders := registerBuiltinProvidersFn
	origRegisterRelurpic := registerBuiltinRelurpicCapabilitiesFn
	origRegisterAgentCaps := registerAgentCapabilitiesFn
	origBuildFromSpec := buildFromSpecFn
	origEucloBuilder := buildAndWireEucloAgentFn
	t.Cleanup(func() {
		registerBuiltinProvidersFn = origRegisterProviders
		registerBuiltinRelurpicCapabilitiesFn = origRegisterRelurpic
		registerAgentCapabilitiesFn = origRegisterAgentCaps
		buildFromSpecFn = origBuildFromSpec
		buildAndWireEucloAgentFn = origEucloBuilder
	})

	registerBuiltinProvidersFn = func(ctx context.Context, rt *appruntime.Runtime) error { return nil }
	registerBuiltinRelurpicCapabilitiesFn = func(registry *capability.Registry, model core.LanguageModel, cfg *core.Config, opts ...agents.RelurpicOption) error {
		return nil
	}
	registerAgentCapabilitiesFn = func(registry *capability.Registry, env agents.AgentEnvironment) error { return nil }
	buildFromSpecFn = func(env agents.AgentEnvironment, spec core.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
		return nil, nil
	}
	buildAndWireEucloAgentFn = func(ws *ayenitd.Workspace, broker *archaeolearning.Broker) graph.WorkflowExecutor {
		return &eucloSummaryExecutor{
			result: &core.Result{NodeID: "done", Success: true, Data: map[string]any{"status": "ok"}},
			stateFn: func(state *core.Context) {
				state.Set("euclo.mode_resolution", eucloruntime.ModeResolution{ModeID: "code"})
				state.Set("euclo.interaction_recording", map[string]any{"event_count": 2})
				state.Set("euclo.artifacts", []euclotypes.Artifact{
					{Kind: euclotypes.ArtifactKindFinalReport},
					{Kind: euclotypes.ArtifactKindExecutionStatus},
				})
			},
		}
	}

	cmd := newStartCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("agent", "coding"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("mode", "code"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("instruction", "summarize euclo output"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	var summary executionSummary
	if err := json.Unmarshal(out.Bytes(), &summary); err != nil {
		t.Fatalf("output was not valid JSON: %v\n%s", err, out.String())
	}
	if summary.TaskID == "" {
		t.Fatal("expected task_id to be populated")
	}
	if summary.Mode != "code" {
		t.Fatalf("mode = %q, want code", summary.Mode)
	}
	if summary.ResultNode != "done" {
		t.Fatalf("result_node = %q, want done", summary.ResultNode)
	}
	if !summary.Success {
		t.Fatal("expected success=true")
	}
	if !summary.Recorded {
		t.Fatal("expected interaction recording flag")
	}
	if len(summary.ArtifactPaths) != 1 {
		t.Fatalf("artifact_paths = %v, want 1 path", summary.ArtifactPaths)
	}
	if len(summary.ArtifactKinds) != 2 {
		t.Fatalf("artifact_kinds = %v, want 2 kinds", summary.ArtifactKinds)
	}
	if !strings.Contains(strings.Join(summary.ArtifactKinds, ","), string(euclotypes.ArtifactKindFinalReport)) {
		t.Fatalf("artifact kinds missing final report: %v", summary.ArtifactKinds)
	}
	if !strings.Contains(out.String(), "\"task_id\"") {
		t.Fatalf("expected JSON output, got %q", out.String())
	}
}
