package critic_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/critic"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	agentgraph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

// criticNode is the independent critique phase: its own model instance,
// its own script, its own prompt. It reads the draft from working memory
// and returns a verdict — it never edits the draft, and it never sees the
// drafter's model. The separation is the mechanism.
type criticNode struct {
	id    string
	model *critic.FakeModel
}

func (n *criticNode) ID() string                { return n.id }
func (n *criticNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeObservation }
func (n *criticNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectNone,
		Idempotency:     agentgraph.IdempotencyReplaySafe,
	}
}

func (n *criticNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	draft, ok := contextdata.GetTyped[string](env, "critic.draft")
	if !ok {
		return nil, errors.New("critic: no draft in working memory")
	}
	ch, err := n.model.GenerateStream(ctx, "critique this draft: "+draft, nil)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for chunk := range ch {
		b.WriteString(chunk)
	}
	verdict := b.String() // scripted: "issues" or "clean"
	env.SetWorkingValueWithOrigin("critic.verdict", verdict,
		contextdata.MemoryClassTask, contextdata.OriginLLM)
	return &execution.Result{
		Success: true,
		Data:    execution.NewToolResultPayload(map[string]any{"verdict": verdict}),
	}, nil
}

// reviseNode applies a revision: a third model turn under a required
// checkpoint policy. It mirrors the shipped ReWOO pattern of a node
// requesting a checkpoint on the envelope ("rewoo:<phase>") — the request
// is the node's; materialization belongs to the persistence wiring.
type reviseNode struct {
	id    string
	model *critic.FakeModel
}

func (n *reviseNode) ID() string                { return n.id }
func (n *reviseNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeObservation }
func (n *reviseNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass:  agentgraph.SideEffectContext,
		Idempotency:      agentgraph.IdempotencyReplaySafe,
		CheckpointPolicy: agentgraph.CheckpointPolicyRequired,
		Recoverability:   agentgraph.NodeRecoverabilityPersisted,
	}
}

func (n *reviseNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	env.RequestCheckpoint("critic:revise", 50, false)
	revision, _ := contextdata.GetTyped[int](env, "critic.revisions")
	env.SetWorkingValueWithOrigin("critic.revisions", revision+1,
		contextdata.MemoryClassTask, contextdata.OriginLLM)

	draft, _ := contextdata.GetTyped[string](env, "critic.draft")
	ch, err := n.model.GenerateStream(ctx, "revise this draft: "+draft, nil)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for chunk := range ch {
		b.WriteString(chunk)
	}
	revised := b.String()
	env.SetWorkingValueWithOrigin("critic.draft", revised,
		contextdata.MemoryClassTask, contextdata.OriginLLM)
	return &execution.Result{
		Success: true,
		Data:    execution.NewToolResultPayload(map[string]any{"revised": revised}),
	}, nil
}

// approvalGate is the human pause. It mirrors the shipped HumanNode's
// contract exactly (SideEffectHuman, single-shot) — HumanNode itself
// cannot be constructed outside the agentgraph package as of this tree
// (unexported id field, no constructor), and mirroring it is the honest
// builder's move. The decision callback is synchronous, as HumanNode's is.
type approvalGate struct {
	id       string
	prompt   string
	decide   func(*contextdata.Envelope) error
	decision string
}

func (n *approvalGate) ID() string                { return n.id }
func (n *approvalGate) Type() agentgraph.NodeType { return agentgraph.NodeTypeHuman }
func (n *approvalGate) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectHuman,
		Idempotency:     agentgraph.IdempotencySingleShot,
	}
}

func (n *approvalGate) Execute(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n.decide != nil {
		if err := n.decide(env); err != nil {
			return nil, err
		}
	}
	return &execution.Result{
		Success: true,
		Data:    execution.NewToolResultPayload(map[string]any{"prompt": n.prompt}),
	}, nil
}

// criticGraph wires the full pipeline:
//
//	drafter → critic → gate ──(revise)──→ approve ──(approved)──→ revise ──→ critic
//	                     └─(finish)──→ done         └─(denied)────→ done
//
// The gate is a system node with ConditionalNode semantics: it decides,
// the edges route on its decision. The loop is bounded twice — by the
// scripted critique eventually saying "clean", and by the graph's visit
// cap as backstop.
func criticGraph(t *testing.T, drafter, criticModel, reviseModel *critic.FakeModel, approval string) *agentgraph.Graph {
	t.Helper()

	gate := agentgraph.NewSystemNode("gate", func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		next := "finish"
		if verdict, _ := contextdata.GetTyped[string](env, "critic.verdict"); verdict == "issues" {
			if revision, _ := contextdata.GetTyped[int](env, "critic.revisions"); revision < 2 {
				next = "revise"
			}
		}
		return &execution.Result{
			Success: true,
			Data:    execution.NewToolResultPayload(map[string]any{"next": next}),
		}, nil
	})

	approve := &approvalGate{
		id:     "approve",
		prompt: "approve the revision?",
		decide: func(env *contextdata.Envelope) error {
			env.SetWorkingValueWithOrigin("approval.revise", approval,
				contextdata.MemoryClassTask, contextdata.OriginUser)
			return nil
		},
	}

	g := agentgraph.NewGraph()
	mustBuild(t,
		g.AddNode(&draftNode{id: "drafter", model: drafter}),
		g.AddNode(&criticNode{id: "critic", model: criticModel}),
		g.AddNode(gate),
		g.AddNode(approve),
		g.AddNode(&reviseNode{id: "revise", model: reviseModel}),
		g.AddNode(agentgraph.NewTerminalNode("done")),
		g.AddEdge("drafter", "critic", nil, false),
		g.AddEdge("critic", "gate", nil, false),
		g.AddEdge("gate", "approve", func(r *execution.Result, _ *contextdata.Envelope) bool {
			next, _ := execution.ResultFields(r.Data)["next"].(string)
			return next == "revise"
		}, false),
		g.AddEdge("gate", "done", func(r *execution.Result, _ *contextdata.Envelope) bool {
			next, _ := execution.ResultFields(r.Data)["next"].(string)
			return next == "finish"
		}, false),
		g.AddEdge("approve", "revise", func(_ *execution.Result, env *contextdata.Envelope) bool {
			decision, _ := contextdata.GetTyped[string](env, "approval.revise")
			return decision == "approved"
		}, false),
		g.AddEdge("approve", "done", func(_ *execution.Result, env *contextdata.Envelope) bool {
			decision, _ := contextdata.GetTyped[string](env, "approval.revise")
			return decision == "denied"
		}, false),
		g.AddEdge("revise", "critic", nil, false),
		g.SetStart("drafter"),
	)
	return g
}

// TestCriticPipelineRevisesWhenFlagged runs the happy revision: draft is
// critiqued, flagged, approved by the human pause, revised once, and the
// re-review comes back clean.
func TestCriticPipelineRevisesWhenFlagged(t *testing.T) {
	drafter := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"a draft with issues"}})
	criticModel := critic.NewFakeModel(
		critic.FakeTurn{Chunks: []string{"issues"}}, // first critique
		critic.FakeTurn{Chunks: []string{"clean"}},  // re-review after revise
	)
	reviseModel := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"a better draft"}})

	g := criticGraph(t, drafter, criticModel, reviseModel, "approved")
	env := newTourEnvelope()
	result, err := g.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("result = %+v, want success", result)
	}

	if got, _ := contextdata.GetTyped[string](env, "critic.verdict"); got != "clean" {
		t.Fatalf("final verdict = %q, want clean", got)
	}
	if got, _ := contextdata.GetTyped[int](env, "critic.revisions"); got != 1 {
		t.Fatalf("revisions = %d, want 1", got)
	}
	if got, _ := contextdata.GetTyped[string](env, "critic.draft"); got != "a better draft" {
		t.Fatalf("final draft = %q, want the revision", got)
	}
	if criticModel.Calls() != 2 {
		t.Fatalf("critic model calls = %d, want 2 (initial + re-review)", criticModel.Calls())
	}
	if reviseModel.Calls() != 1 {
		t.Fatalf("revise model calls = %d, want 1", reviseModel.Calls())
	}
	if got, _ := contextdata.GetTyped[string](env, "approval.revise"); got != "approved" {
		t.Fatalf("approval = %q, want approved", got)
	}
	// The revise node requested a checkpoint on the envelope.
	if env.CheckpointRequestSnapshot() == nil {
		t.Fatal("revise node did not request a checkpoint")
	}
	// Bounded: three distinct models, three phases, one loop iteration.
	if drafter.Calls() != 1 || criticModel.Calls()+reviseModel.Calls() != 3 {
		t.Fatalf("call budget blown: drafter=%d critic=%d revise=%d",
			drafter.Calls(), criticModel.Calls(), reviseModel.Calls())
	}
}

// TestCriticPipelineDenialSkipsRevision: the human pause says no, the
// revision never runs, and the run ends honestly at the terminal.
func TestCriticPipelineDenialSkipsRevision(t *testing.T) {
	drafter := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"a draft with issues"}})
	criticModel := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"issues"}})
	reviseModel := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"unused"}})

	g := criticGraph(t, drafter, criticModel, reviseModel, "denied")
	env := newTourEnvelope()
	if _, err := g.Execute(context.Background(), env); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, _ := contextdata.GetTyped[string](env, "approval.revise"); got != "denied" {
		t.Fatalf("approval = %q, want denied", got)
	}
	if reviseModel.Calls() != 0 {
		t.Fatalf("revise ran %d times after a denial, want 0", reviseModel.Calls())
	}
	if _, ok := contextdata.GetTyped[int](env, "critic.revisions"); ok {
		t.Fatal("revisions counter moved despite a denial")
	}
}

// TestContractDefaultsAreTyped shows the shipped nodes arriving with
// governance already declared: the graph resolves a contract for every
// node, typed or not. HumanNode's zero value is readable here even though
// its id field is unexported — the contract comes from the type.
func TestContractDefaultsAreTyped(t *testing.T) {
	terminal := agentgraph.ResolveNodeContract(agentgraph.NewTerminalNode("done"))
	if terminal.SideEffectClass != agentgraph.SideEffectNone ||
		terminal.Idempotency != agentgraph.IdempotencyReplaySafe {
		t.Fatalf("terminal contract = %+v, want none/replay-safe", terminal)
	}
	human := agentgraph.ResolveNodeContract(&agentgraph.HumanNode{})
	if human.SideEffectClass != agentgraph.SideEffectHuman ||
		human.Idempotency != agentgraph.IdempotencySingleShot {
		t.Fatalf("human contract = %+v, want human/single-shot", human)
	}
	tool := agentgraph.ResolveNodeContract(
		agentgraph.NewToolNode("t", &stringkitReadTool{}, nil, governedInvoker{reg: governedRegistry(t, false)}))
	// A tool's contract is classified from what the tool declares: our
	// read-only tool declares no effect classes, so it classifies
	// none/replay-safe — and its required capability is still pinned.
	if tool.SideEffectClass != agentgraph.SideEffectNone ||
		tool.Idempotency != agentgraph.IdempotencyReplaySafe {
		t.Fatalf("tool contract = %+v, want none/replay-safe for a no-effects tool", tool)
	}
	if len(tool.RequiredCapabilities) != 1 || tool.RequiredCapabilities[0].ID != "tool:read_stringkit" {
		t.Fatalf("tool RequiredCapabilities = %+v, want [tool:read_stringkit]", tool.RequiredCapabilities)
	}
}

// opinionatedNode is a ContractNode with an arbitrary contract — the
// vehicle for proving validation refuses garbage.
type opinionatedNode struct {
	id       string
	nodeType agentgraph.NodeType
	contract agentgraph.NodeContract
}

func (n *opinionatedNode) ID() string                { return n.id }
func (n *opinionatedNode) Type() agentgraph.NodeType { return n.nodeType }
func (n *opinionatedNode) Contract() agentgraph.NodeContract {
	return n.contract
}
func (n *opinionatedNode) Execute(_ context.Context, _ *contextdata.Envelope) (*execution.Result, error) {
	return &execution.Result{Success: true}, nil
}

// TestContractValidationRefusesGarbage: an invalid class spelling is a
// build-time refusal with coordinates, and a tool-typed node that
// declares no capabilities is rejected on sight.
func TestContractValidationRefusesGarbage(t *testing.T) {
	t.Run("invalid side effect class", func(t *testing.T) {
		g := agentgraph.NewGraph()
		mustBuild(t,
			g.AddNode(&opinionatedNode{id: "wild", nodeType: agentgraph.NodeTypeSystem,
				contract: agentgraph.NodeContract{
					SideEffectClass: agentgraph.SideEffectClass("wing-it"),
				}}),
			g.SetStart("wild"),
		)
		err := g.Validate()
		if err == nil || !strings.Contains(err.Error(), "invalid side effect class") {
			t.Fatalf("err = %v, want invalid side effect class", err)
		}
	})
	t.Run("tool node must declare capabilities", func(t *testing.T) {
		g := agentgraph.NewGraph()
		mustBuild(t,
			g.AddNode(&opinionatedNode{id: "toolish", nodeType: agentgraph.NodeTypeTool,
				contract: agentgraph.NodeContract{
					SideEffectClass: agentgraph.SideEffectExternal,
				}}),
			g.SetStart("toolish"),
		)
		err := g.Validate()
		if err == nil || !strings.Contains(err.Error(), "declares no required capabilities") {
			t.Fatalf("err = %v, want declares no required capabilities", err)
		}
	})
}
