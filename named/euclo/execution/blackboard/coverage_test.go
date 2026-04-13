package blackboard

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentblackboard "github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type stubModel struct {
	prompt   string
	response *core.LLMResponse
	err      error
}

type errorKnowledgeSource struct {
	name     string
	priority int
	err      error
}

func (m *stubModel) Generate(_ context.Context, prompt string, _ *core.LLMOptions) (*core.LLMResponse, error) {
	m.prompt = prompt
	return m.response, m.err
}

func (m *stubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (m *stubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *stubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (s errorKnowledgeSource) Name() string { return s.name }
func (s errorKnowledgeSource) CanActivate(*agentblackboard.Blackboard) bool {
	return true
}
func (s errorKnowledgeSource) Execute(context.Context, *agentblackboard.Blackboard, *capability.Registry, core.LanguageModel, core.AgentSemanticContext) error {
	return s.err
}
func (s errorKnowledgeSource) Priority() int { return s.priority }

func TestKnowledgeSourceHelpersAndResponseParsing(t *testing.T) {
	board := agentblackboard.NewBlackboard("repair the build")
	if !board.AddFact("fact:one", `{"nested":["value"]}`, "seed") {
		t.Fatal("expected fact to be added")
	}
	if err := board.AddArtifact("artifact-1", "debug:root_cause", `{"status":"open"}`, "seed"); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}

	tmpl := newTemplateKnowledgeSource(
		"  analysis  ",
		"fact:fact:one",
		[]string{"fact:one"},
		[]string{"tool-a", "  "},
		"Source {{knowledge_source}} for {{goal}} tools={{available_tools}} entries={{entries}} input={{input_entries}}",
		templateKindAnalysis,
	).(*templateKnowledgeSource)
	if got := tmpl.Name(); got != "analysis" {
		t.Fatalf("unexpected name: %q", got)
	}
	if got := tmpl.Priority(); got != 90 {
		t.Fatalf("unexpected priority: %d", got)
	}
	spec := tmpl.KnowledgeSourceSpec()
	if spec.Name != "analysis" || spec.Priority != 90 || len(spec.RequiredCapabilities) != 1 {
		t.Fatalf("unexpected spec: %#v", spec)
	}
	if spec.Contract.SideEffectClass != graph.SideEffectNone {
		t.Fatalf("unexpected side effect class: %#v", spec.Contract.SideEffectClass)
	}
	if !tmpl.CanActivate(board) {
		t.Fatal("expected activation through fact predicate")
	}
	if !evaluateActivationPredicate(board, "always") || !evaluateActivationPredicate(board, "not artifact:missing") {
		t.Fatal("expected predicate helpers to match")
	}
	if !evaluateActivationPredicate(board, "artifact:debug:root_cause") || !evaluateActivationPredicate(board, "fact:fact:one") {
		t.Fatal("expected artifact/fact predicates to match")
	}
	if evaluateActivationPredicate(board, "missing exists") {
		t.Fatal("expected missing entry predicate to be false")
	}
	if got := selectedEntries(map[string]any{"a": 1, "b": 2}, []string{"b", "c"}); len(got) != 1 || got["b"] != 2 {
		t.Fatalf("unexpected selected entries: %#v", got)
	}
	if got := stringSliceFromAny([]any{" a ", 1, nil, "b"}); len(got) != 3 || got[0] != "a" || got[2] != "b" {
		t.Fatalf("unexpected string slice conversion: %#v", got)
	}
	if floatValue(float64(1.5)) != 1.5 || floatValue(float32(2.5)) != 2.5 || floatValue(3) != 3 || floatValue("nope") != 0 {
		t.Fatal("unexpected float conversion")
	}
	if mustJSON(make(chan int)) != "{}" {
		t.Fatal("expected invalid JSON fallback")
	}

	rendered := renderKnowledgeSourcePrompt(tmpl, board)
	if !strings.Contains(rendered, "analysis") || !strings.Contains(rendered, "repair the build") || !strings.Contains(rendered, "tool-a") {
		t.Fatalf("unexpected rendered prompt: %s", rendered)
	}

	model := &stubModel{
		response: &core.LLMResponse{
			Text: `{"facts":[{"key":"fact:two","value":{"status":"done"}}],"hypotheses":[{"summary":"first","confidence":0.2},{"id":"h2","title":"second","confidence":0.9}]}`,
		},
	}
	if err := tmpl.Execute(context.Background(), board, capability.NewRegistry(), model, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !boardHasFactKey(board, "fact:two") {
		t.Fatal("expected response facts to be applied")
	}
	if len(board.Hypotheses) != 2 || board.Hypotheses[0].ID != "h2" {
		t.Fatalf("expected hypotheses to be sorted by confidence, got %#v", board.Hypotheses)
	}
	if !strings.Contains(model.prompt, "analysis") || !strings.Contains(model.prompt, "fact:one") {
		t.Fatalf("expected prompt to include rendered data, got %s", model.prompt)
	}

	if err := applyKnowledgeSourceResponse(board, "analysis", "```json\n{\"facts\":[{\"key\":\"fact:three\",\"value\":\"ok\"}]}\n```"); err != nil {
		t.Fatalf("applyKnowledgeSourceResponse fenced JSON: %v", err)
	}
	if !boardHasFactKey(board, "fact:three") {
		t.Fatal("expected fenced response to be applied")
	}
}

func TestArtifactBridgeRoundTripAndHelpers(t *testing.T) {
	board := agentblackboard.NewBlackboard("goal")
	bridge := NewArtifactBridge(board)

	if got := firstNonEmpty("", "  ", "value"); got != "value" {
		t.Fatalf("unexpected firstNonEmpty: %q", got)
	}
	if setBoardEntry(nil, "a", 1, "src") {
		t.Fatal("expected nil board write to fail")
	}
	if _, ok := boardEntryValue(nil, "a"); ok {
		t.Fatal("expected nil board lookup to fail")
	}

	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "v1", Kind: euclotypes.ArtifactKindVerification, Payload: map[string]any{"status": "ok"}},
		{ID: "t1", Kind: euclotypes.ArtifactKindTrace, Payload: "trace"},
	})
	if err := bridge.SeedFromArtifacts(artifacts); err != nil {
		t.Fatalf("SeedFromArtifacts: %v", err)
	}
	if got, ok := boardEntryValue(board, "verify:result"); !ok || got == nil {
		t.Fatalf("expected verification entry to be seeded, got %#v", got)
	}

	if err := board.AddArtifact("trace-1", "trace:raw_output", `{"detail":"x"}`, "source"); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if !setBoardEntry(board, "custom:fact", []string{"x", "y"}, "source") {
		t.Fatal("expected board fact to be written")
	}
	if !boardHasEntry(board, "custom:fact") {
		t.Fatal("expected board entry to exist")
	}

	harvested := bridge.HarvestToArtifacts()
	if len(harvested) == 0 {
		t.Fatal("expected harvested artifacts")
	}
	var seenVerification, seenTrace bool
	for _, artifact := range harvested {
		switch artifact.Kind {
		case euclotypes.ArtifactKindVerification:
			seenVerification = true
		case euclotypes.ArtifactKindTrace:
			seenTrace = true
		}
	}
	if !seenVerification || !seenTrace {
		t.Fatalf("expected verification and trace artifacts, got %#v", harvested)
	}
}

func TestExecutorBranches(t *testing.T) {
	env := euclotypes.ExecutionEnvelope{
		Task:     &core.Task{Instruction: "verify"},
		State:    core.NewContext(),
		Registry: capability.NewRegistry(),
		Environment: agentenv.AgentEnvironment{
			Model: &stubModel{},
		},
	}

	if _, err := Execute(context.Background(), env, euclotypes.ExecutorSemanticContext{}, nil, 1, nil); err == nil {
		t.Fatal("expected error without knowledge sources")
	}

	env.State.Set("euclo.blackboard_seed_facts", map[string]any{"seed:fact": "seeded"})
	env.State.Set("euclo.artifacts", []euclotypes.Artifact{
		{ID: "existing", Kind: euclotypes.ArtifactKindTrace, Payload: map[string]any{"status": "seeded"}},
	})

	runs := 0
	sources := []agentblackboard.KnowledgeSource{
		executableKnowledgeSource{stubKnowledgeSource{
			name:     "writer",
			priority: 10,
			canRun:   func(*agentblackboard.Blackboard) bool { return true },
			executeFn: func(bb *agentblackboard.Blackboard) {
				runs++
				_ = setBoardEntry(bb, "verify:result", map[string]any{"status": "ok"}, "test")
				bb.Artifacts = append(bb.Artifacts, agentblackboard.Artifact{ID: "trace-1", Kind: "trace:raw_output", Content: `{"ok":true}`, Source: "test", Verified: true})
			},
		}},
	}
	result, err := Execute(context.Background(), env, euclotypes.ExecutorSemanticContext{}, sources, 2, func(bb *agentblackboard.Blackboard) bool {
		return boardHasFactKey(bb, "verify:result")
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Termination != "predicate_satisfied" {
		t.Fatalf("unexpected termination: %#v", result)
	}
	if runs != 1 {
		t.Fatalf("expected one run, got %d", runs)
	}
	if result.Cycles != 1 || !boardHasFactKey(result.Board, "verify:result") {
		t.Fatalf("unexpected result board: %#v", result)
	}
	if got, _ := env.State.GetKnowledge("blackboard.last_source"); got != "writer" {
		t.Fatalf("expected state publish, got %q", got)
	}

	noEligible := []agentblackboard.KnowledgeSource{
		stubKnowledgeSource{name: "inactive", priority: 1, canRun: func(*agentblackboard.Blackboard) bool { return false }},
	}
	if res, err := Execute(context.Background(), env, euclotypes.ExecutorSemanticContext{}, noEligible, 1, nil); err != nil || res.Termination != "no_eligible_sources" {
		t.Fatalf("expected no eligible source termination, got res=%#v err=%v", res, err)
	}

	failedEnv := euclotypes.ExecutionEnvelope{
		Task:     &core.Task{Instruction: "verify"},
		State:    core.NewContext(),
		Registry: capability.NewRegistry(),
		Environment: agentenv.AgentEnvironment{
			Model: &stubModel{},
		},
	}
	failure := errors.New("boom")
	if res, err := Execute(context.Background(), failedEnv, euclotypes.ExecutorSemanticContext{}, []agentblackboard.KnowledgeSource{
		errorKnowledgeSource{name: "fail", priority: 10, err: failure},
	}, 1, nil); err == nil || res != nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected source failure, got res=%#v err=%v", res, err)
	}

	limitedEnv := euclotypes.ExecutionEnvelope{
		Task:     &core.Task{Instruction: "verify"},
		State:    core.NewContext(),
		Registry: capability.NewRegistry(),
		Environment: agentenv.AgentEnvironment{
			Model: &stubModel{},
		},
	}
	limited := []agentblackboard.KnowledgeSource{
		stubKnowledgeSource{name: "idle", priority: 1, canRun: func(*agentblackboard.Blackboard) bool { return true }},
	}
	if res, err := Execute(context.Background(), limitedEnv, euclotypes.ExecutorSemanticContext{}, limited, 2, nil); err != nil || res.Termination != "cycle_limit" || res.Cycles != 2 {
		t.Fatalf("expected cycle limit termination, got res=%#v err=%v", res, err)
	}
}
