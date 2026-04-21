package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/agents"
	appruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"gopkg.in/yaml.v3"
)

type noopPatternStore struct{}

func (noopPatternStore) Save(context.Context, patterns.PatternRecord) error { return nil }
func (noopPatternStore) Load(context.Context, string) (*patterns.PatternRecord, error) {
	return nil, nil
}
func (noopPatternStore) ListByStatus(context.Context, patterns.PatternStatus, string) ([]patterns.PatternRecord, error) {
	return nil, nil
}
func (noopPatternStore) ListByKind(context.Context, patterns.PatternKind, string) ([]patterns.PatternRecord, error) {
	return nil, nil
}
func (noopPatternStore) UpdateStatus(context.Context, string, patterns.PatternStatus, string) error {
	return nil
}
func (noopPatternStore) Supersede(context.Context, string, patterns.PatternRecord) error { return nil }

func writeCodingAgentManifestFixture(t *testing.T, ws, name string) string {
	t.Helper()
	path := filepath.Join(ws, "relurpify_cfg", "agents", name+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	m := manifest.AgentManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata: manifest.ManifestMetadata{
			Name:        name,
			Version:     "1.0.0",
			Description: "coding test agent",
		},
		Spec: manifest.ManifestSpec{
			Image:   "ghcr.io/example/runtime:latest",
			Runtime: "gvisor",
			Agent: &core.AgentRuntimeSpec{
				Implementation: "coding",
				Mode:           core.AgentModePrimary,
				Version:        "1.0.0",
				Model: core.AgentModelConfig{
					Provider:    "ollama",
					Name:        "qwen2.5-coder:14b",
					Temperature: 0.1,
					MaxTokens:   2048,
				},
			},
			Defaults: &manifest.ManifestDefaults{
				Permissions: &core.PermissionSet{
					FileSystem: []core.FileSystemPermission{{
						Action:        core.FileSystemRead,
						Path:          filepath.ToSlash(filepath.Join(ws, "**")),
						Justification: "read workspace",
					}},
				},
			},
		},
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildAndWireEucloAgent_WiresWorkspaceServices(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	retrievalDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retrievalDB.Close() })

	graphEngine := &graphdb.Engine{}
	learningBroker := archaeolearning.NewBroker(0)
	ws := &ayenitd.Workspace{
		Environment: ayenitd.WorkspaceEnvironment{
			Config:   &core.Config{Name: "euclo"},
			Registry: capability.NewRegistry(),
			IndexManager: &ast.IndexManager{
				GraphDB: graphEngine,
			},
			PatternStore:   noopPatternStore{},
			CommentStore:   nil,
			RetrievalDB:    retrievalDB,
			GuidanceBroker: guidance.NewGuidanceBroker(0),
		},
	}

	executor := buildAndWireEucloAgent(ws, learningBroker)
	agent, ok := executor.(*euclo.Agent)
	if !ok {
		t.Fatalf("buildAndWireEucloAgent returned %T", executor)
	}
	if agent.GraphDB != graphEngine {
		t.Fatalf("GraphDB = %p, want %p", agent.GraphDB, graphEngine)
	}
	if agent.RetrievalDB != retrievalDB {
		t.Fatalf("RetrievalDB not wired from workspace")
	}
	if agent.GuidanceBroker != ws.Environment.GuidanceBroker {
		t.Fatalf("GuidanceBroker not wired from workspace")
	}
	if agent.LearningBroker != learningBroker {
		t.Fatalf("LearningBroker not wired from caller")
	}
	if agent.ConvVerifier == nil {
		t.Fatal("expected convergence verifier to be wired")
	}
	if agent.DeferralPolicy.MaxBlastRadiusForDefer == 0 && len(agent.DeferralPolicy.DeferrableKinds) == 0 {
		t.Fatal("expected default deferral policy to be applied")
	}
}

func TestStartCmdUsesEucloBuilderForCodingManifests(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)
	writeCodingAgentManifestFixture(t, ws, "coding")
	stubStartWorkspaceFn(t, ws, true)

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

	var relurpicOptCount int
	registerBuiltinProvidersFn = func(ctx context.Context, rt *appruntime.Runtime) error { return nil }
	registerBuiltinRelurpicCapabilitiesFn = func(registry *capability.Registry, model core.LanguageModel, cfg *core.Config, opts ...agents.RelurpicOption) error {
		relurpicOptCount = len(opts)
		return nil
	}
	registerAgentCapabilitiesFn = func(registry *capability.Registry, env agents.AgentEnvironment) error { return nil }
	buildFromSpecCalls := 0
	buildFromSpecFn = func(env agents.AgentEnvironment, spec core.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
		buildFromSpecCalls++
		return nil, nil
	}
	builderCalls := 0
	buildAndWireEucloAgentFn = func(ws *ayenitd.Workspace, broker *archaeolearning.Broker) graph.WorkflowExecutor {
		builderCalls++
		if broker == nil {
			t.Fatal("expected learning broker")
		}
		return &stubWorkflowExecutor{}
	}

	cmd := newStartCmd()
	if err := cmd.Flags().Set("agent", "coding"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("mode", "code"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("instruction", "validate euclo branch"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if builderCalls != 1 {
		t.Fatalf("buildAndWireEucloAgentFn calls = %d, want 1", builderCalls)
	}
	if buildFromSpecCalls != 0 {
		t.Fatalf("buildFromSpecFn calls = %d, want 0", buildFromSpecCalls)
	}
	if relurpicOptCount != 8 {
		t.Fatalf("relurpic option count = %d, want 8", relurpicOptCount)
	}
}
