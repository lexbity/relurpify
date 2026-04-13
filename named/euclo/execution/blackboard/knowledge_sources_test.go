package blackboard

import (
	"context"
	"strings"
	"testing"

	agentblackboard "github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestApplyKnowledgeSourceResponseAcceptsFencedJSON(t *testing.T) {
	bb := agentblackboard.NewBlackboard()
	raw := "```json\n{\"facts\":[{\"key\":\"archaeology:patterns\",\"value\":[\"normalize\"]}]}\n```"

	if err := applyKnowledgeSourceResponse(bb, "Pattern Mapper", raw); err != nil {
		t.Fatalf("applyKnowledgeSourceResponse returned error: %v", err)
	}
	if len(bb.Facts) != 1 {
		t.Fatalf("expected one blackboard fact, got %#v", bb.Facts)
	}
	if bb.Facts[0].Key != "archaeology:patterns" {
		t.Fatalf("unexpected fact key: %#v", bb.Facts[0])
	}
	if !strings.Contains(bb.Facts[0].Value, "normalize") {
		t.Fatalf("unexpected recorded fact: %#v", bb.Facts[0].Value)
	}
}

type stubKnowledgeSource struct {
	name      string
	priority  int
	canRun    func(*agentblackboard.Blackboard) bool
	executeFn func(*agentblackboard.Blackboard)
}

func (s stubKnowledgeSource) Name() string  { return s.name }
func (s stubKnowledgeSource) Priority() int { return s.priority }
func (s stubKnowledgeSource) CanActivate(bb *agentblackboard.Blackboard) bool {
	if s.canRun != nil {
		return s.canRun(bb)
	}
	return true
}
func (s stubKnowledgeSource) Execute(context.Context, *agentblackboard.Blackboard, *capability.Registry, core.LanguageModel, core.AgentSemanticContext) error {
	return nil
}
func (s stubKnowledgeSource) KnowledgeSourceSpec() agentblackboard.KnowledgeSourceSpec {
	return agentblackboard.KnowledgeSourceSpec{
		Name:     s.name,
		Priority: s.priority,
	}
}

type executableKnowledgeSource struct {
	stubKnowledgeSource
}

func (s executableKnowledgeSource) Execute(_ context.Context, bb *agentblackboard.Blackboard, _ *capability.Registry, _ core.LanguageModel, _ core.AgentSemanticContext) error {
	if s.executeFn != nil {
		s.executeFn(bb)
	}
	return nil
}

func TestExecuteRunsKnowledgeSourceAndPublishesArtifacts(t *testing.T) {
	state := core.NewContext()
	result, err := Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task:        &core.Task{Instruction: "verify this change"},
		State:       state,
		Registry:    capability.NewRegistry(),
		Environment: testutil.Env(t),
	}, euclotypes.ExecutorSemanticContext{}, []agentblackboard.KnowledgeSource{
		executableKnowledgeSource{stubKnowledgeSource{
			name:     "verify",
			priority: 100,
			executeFn: func(bb *agentblackboard.Blackboard) {
				_ = setBoardEntry(bb, "verify:result", map[string]any{"status": "ok"}, "test")
			},
		}},
	}, 2, func(bb *agentblackboard.Blackboard) bool {
		return boardHasEntry(bb, "verify:result")
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || result.Termination != "predicate_satisfied" {
		t.Fatalf("unexpected result: %+v", result)
	}
	raw, ok := state.Get("pipeline.verify")
	if !ok {
		t.Fatal("expected verification payload in state")
	}
	payload, ok := raw.(map[string]any)
	if !ok || payload["status"] != "ok" {
		t.Fatalf("unexpected verification payload: %#v", raw)
	}
}

func TestEligibleKnowledgeSourcesPrefersHigherPriority(t *testing.T) {
	board := agentblackboard.NewBlackboard()
	sources := []agentblackboard.KnowledgeSource{
		stubKnowledgeSource{name: "low", priority: 10},
		stubKnowledgeSource{name: "high", priority: 90},
	}

	eligible := eligibleKnowledgeSources(sources, board)
	if len(eligible) != 2 {
		t.Fatalf("expected two eligible sources, got %d", len(eligible))
	}
	if eligible[0].Name() != "high" {
		t.Fatalf("expected highest priority source first, got %q", eligible[0].Name())
	}
}

func TestExecuteRequiresAtLeastOneKnowledgeSource(t *testing.T) {
	_, err := Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task:        &core.Task{Instruction: "nothing"},
		State:       core.NewContext(),
		Registry:    capability.NewRegistry(),
		Environment: testutil.Env(t),
	}, euclotypes.ExecutorSemanticContext{}, nil, 1, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
