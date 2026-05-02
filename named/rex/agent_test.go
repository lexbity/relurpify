package rex

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/platform/contracts"

	// "codeburg.org/lexbit/relurpify/framework/memory/db" // TODO: package does not exist
	"codeburg.org/lexbit/relurpify/named/rex/proof"
	"codeburg.org/lexbit/relurpify/named/rex/reconcile"
	"codeburg.org/lexbit/relurpify/named/rex/retrieval"
	"codeburg.org/lexbit/relurpify/named/rex/route"
	"codeburg.org/lexbit/relurpify/named/rex/state"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
)

type stubModel struct{}

func (stubModel) Generate(context.Context, string, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}
func (stubModel) GenerateStream(context.Context, string, *contracts.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}
func (stubModel) Chat(context.Context, []contracts.Message, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: "{}"}, nil
}
func (stubModel) ChatWithTools(context.Context, []contracts.Message, []contracts.LLMToolSpec, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func testEnv(t *testing.T) *agentenv.WorkspaceEnvironment {
	t.Helper()
	return &agentenv.WorkspaceEnvironment{
		Model:         stubModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config:        &core.Config{Name: "rex-test", Model: "stub", MaxIterations: 1},
	}
}

func testRuntimeEnv(t *testing.T) *agentenv.WorkspaceEnvironment {
	t.Helper()
	return &agentenv.WorkspaceEnvironment{
		Model:         stubModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config:        &core.Config{Name: "rex-test", Model: "stub", MaxIterations: 1},
	}
}

func TestAgentImplementsWorkflowExecutor(t *testing.T) {
	var executor interface{} = New(testEnv(t))
	if _, ok := executor.(interface {
		Initialize(*core.Config) error
		Execute(context.Context, *core.Task, *contextdata.Envelope) (*core.Result, error)
		Capabilities() []string
	}); !ok {
		t.Fatalf("agent does not satisfy workflow executor shape")
	}
}

func TestAgentUsesReconcilerHelpers(t *testing.T) {
	agent := New(testEnv(t))
	record := agent.RecordAmbiguity("wf-1", "run-1", "missing ack")
	if record.WorkflowID != "wf-1" || record.RunID != "run-1" {
		t.Fatalf("record = %+v", record)
	}
	resolved := agent.ResolveAmbiguity(record, reconcile.OutcomeRepaired, "confirmed")
	if resolved.Status != reconcile.StatusRepaired {
		t.Fatalf("resolved = %+v", resolved)
	}
	if !agent.ShouldRetryAmbiguity(resolved) {
		t.Fatalf("expected repaired ambiguity to be retryable")
	}
}

type rexNoopCompiler struct{}

func (rexNoopCompiler) Compile(_ context.Context, _ compiler.CompilationRequest) (*compiler.CompilationResult, *compiler.CompilationRecord, error) {
	return &compiler.CompilationResult{}, &compiler.CompilationRecord{}, nil
}

func TestAgentExecuteBuildsProofSurface(t *testing.T) {
	agent := New(testEnv(t))
	env := contextdata.NewEnvelope("task-1", "")
	ctx := contextstream.WithTrigger(context.Background(), contextstream.NewTrigger(rexNoopCompiler{}))
	result, err := agent.Execute(ctx, &core.Task{
		ID:          "task-1",
		Instruction: "review the code",
		Context:     map[string]any{"workspace": t.TempDir(), "edit_permitted": false},
	}, env)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, ok := result.Data["rex.proof_surface"]; !ok {
		t.Fatalf("missing proof surface")
	}
}

func TestAgentExecuteRejectsCapabilityProjectionThatBlocksRequiredCapability(t *testing.T) {
	env := testRuntimeEnv(t)
	agent := New(env)
	env2 := contextdata.NewEnvelope("task-1", "")
	env2.SetWorkingValue("fmp.capability_projection", fwfmp.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{"execute"},
		AllowedTaskClasses:   []string{"agent.run"},
	}, contextdata.MemoryClassTask)

	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-1",
		Instruction: "write code",
		Type:        string(core.TaskTypeCodeGeneration),
		Context:     map[string]any{"workspace": t.TempDir(), "edit_permitted": true},
	}, env2)
	if err == nil {
		t.Fatal("Execute() error = nil, want capability projection rejection")
	}
}

func TestAgentCapabilitiesBuildGraphAndManagedAdapter(t *testing.T) {
	agent := New(testEnv(t))
	caps := agent.Capabilities()
	if len(caps) == 0 || caps[0] != "plan" {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
	graphTask := &core.Task{
		ID:          "task-build",
		Instruction: "review the code",
		Context:     map[string]any{"workspace": t.TempDir(), "edit_permitted": false},
	}
	if g, err := agent.BuildGraph(graphTask); err != nil || g == nil {
		t.Fatalf("BuildGraph err=%v graph=%v", err, g)
	}
	if projection := agent.RuntimeProjection(); projection.Health == "" {
		t.Fatalf("expected runtime projection: %+v", projection)
	}
	if adapter := agent.ManagedAdapter(); adapter == nil || adapter.Registration().Name != "rex" {
		t.Fatalf("unexpected managed adapter: %+v", adapter)
	}
}

func TestInitializeEnvironmentAndHelpers(t *testing.T) {
	env := testEnv(t)
	agent := &Agent{}
	if err := agent.InitializeEnvironment(env, ""); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	if agent.Workspace == "" || agent.Delegates == nil || agent.Runtime == nil || agent.Reconciler == nil {
		t.Fatalf("agent not initialized: %+v", agent)
	}
	if err := agent.Initialize(nil); err != nil {
		t.Fatalf("Initialize(nil): %v", err)
	}
	if len(uniqueStrings([]string{"a", "", "a", "b"})) != 2 {
		t.Fatalf("unexpected uniqueStrings result")
	}
	if got := resolveWorkspaceRoot(" /tmp/workspace "); got == "" {
		t.Fatalf("expected cleaned workspace root")
	}
	root := t.TempDir()
	cfgDir := filepath.Join(root, "relurpify_cfg")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("MkdirAll child: %v", err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()
	if err := os.Chdir(child); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	if got := resolveWorkspaceRoot(""); got != root {
		t.Fatalf("expected workspace root search to find %q, got %q", root, got)
	}
}

func TestAgentReconcilerWrappersAndPersistProof(t *testing.T) {
	agent := &Agent{}
	record := agent.RecordAmbiguity("wf-1", "run-1", "missing ack")
	if record.Status != reconcile.StatusOperatorReview {
		t.Fatalf("unexpected ambiguity record: %+v", record)
	}
	resolved := agent.ResolveAmbiguity(record, reconcile.OutcomeRepaired, "confirmed")
	if resolved.Status != reconcile.StatusRepaired {
		t.Fatalf("unexpected resolved record: %+v", resolved)
	}
	if !agent.ShouldRetryAmbiguity(resolved) {
		t.Fatalf("expected repaired ambiguity to retry")
	}

	store := &stubArtifactStore{}
	identity := state.Identity{WorkflowID: "wf-1", RunID: "run-1"}
	env := contextdata.NewEnvelope("wf-1", "")
	if err := persistProof(context.Background(), store, identity, route.RouteDecision{Family: route.FamilyArchitect, Mode: "mutation", Profile: "managed"}, proof.ProofSurface{RouteFamily: "architect"}, []proof.ActionLogEntry{{Kind: "route"}}, proof.CompletionDecision{Allowed: true}, env); err != nil {
		t.Fatalf("persistProof: %v", err)
	}
	if len(store.artifacts) == 0 {
		t.Fatalf("expected persisted artifacts")
	}
}

func TestAgentNilReceiverWrappersAndPersistenceHelpers(t *testing.T) {
	var agent *Agent
	if got := agent.RecordAmbiguity("wf-1", "run-1", "reason"); got != (reconcile.Record{}) {
		t.Fatalf("expected zero record from nil agent, got %+v", got)
	}
	if got := agent.ResolveAmbiguity(reconcile.Record{ID: "r"}, reconcile.OutcomeVerified, "notes"); got.ID != "r" {
		t.Fatalf("expected passthrough resolve from nil agent, got %+v", got)
	}
	if agent.ShouldRetryAmbiguity(reconcile.Record{ID: "r"}) {
		t.Fatalf("expected nil agent to not retry")
	}

	env2 := contextdata.NewEnvelope("wf", "")
	if err := persistProof(context.Background(), nil, state.Identity{WorkflowID: "wf", RunID: "run"}, route.RouteDecision{}, proof.ProofSurface{}, nil, proof.CompletionDecision{}, env2); err != nil {
		t.Fatalf("nil store should be ignored: %v", err)
	}

	store := &stubArtifactStore{}
	env3 := contextdata.NewEnvelope("wf-2", "")
	env3.SetWorkingValue("rex.verification_policy", proof.VerificationPolicy{PolicyID: "policy-1", ModeID: "mutation"}, contextdata.MemoryClassTask)
	env3.SetWorkingValue("rex.verification", proof.VerificationEvidenceRecord{Status: "pass", EvidencePresent: true}, contextdata.MemoryClassTask)
	env3.SetWorkingValue("rex.success_gate", proof.SuccessGateResult{Allowed: true, Reason: "verification_accepted"}, contextdata.MemoryClassTask)
	env3.SetWorkingValue("rex.recovery_attempts", 1, contextdata.MemoryClassTask)
	env3.SetWorkingValue("rex.artifact_kinds", []string{"rex.proof_surface"}, contextdata.MemoryClassTask)
	if err := persistProof(context.Background(), store, state.Identity{WorkflowID: "wf-2", RunID: "run-2"}, route.RouteDecision{Family: route.FamilyArchitect, Mode: "mutation", Profile: "managed"}, proof.ProofSurface{}, []proof.ActionLogEntry{{Kind: "route"}}, proof.CompletionDecision{Allowed: true}, env3); err != nil {
		t.Fatalf("persistProof with artifacts: %v", err)
	}
	if len(store.artifacts) < 5 {
		t.Fatalf("expected multiple persisted artifacts, got %d", len(store.artifacts))
	}

	if err := persistContextExpansion(context.Background(), store, state.Identity{WorkflowID: "wf-3", RunID: "run-3"}, retrieval.Expansion{Summary: "summary", ExpansionStrategy: "local_first"}); err != nil {
		t.Fatalf("persistContextExpansion: %v", err)
	}
}

type stubArtifactStore struct {
	artifacts []memory.WorkflowArtifactRecord
}

func (s *stubArtifactStore) UpsertWorkflowArtifact(_ context.Context, artifact memory.WorkflowArtifactRecord) error {
	s.artifacts = append(s.artifacts, artifact)
	return nil
}
