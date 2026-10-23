package critic

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	agentgraph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
)

// EnsembleConfig describes the critic ensemble: one drafter, N
// independent critics (one model each — the separation is the mechanism),
// one reviser, and the revision bound.
type EnsembleConfig struct {
	DraftModel   model.LanguageModel
	CriticModels []model.LanguageModel
	ReviseModel  model.LanguageModel
	MaxRevisions int
}

// BuildEnsembleGraph wires the critic pipeline with an ensemble flavor:
//
//	drafter ──▶ critic-0 ──▶ branch-done ┐
//	   │    ──▶ critic-1 ──▶ branch-done ┤ (parallel; deltas merged
//	   │    ──▶ critic-2 ──▶ branch-done ┘  in edge-declaration order)
//	   └─serial─▶ merge ──▶ gate ──(revise)──▶ revise ──▶ critic(s)…
//	                          └─(finish)──▶ done
//
// Each critic writes its verdict to its own working-memory key, so the
// parallel merge unions them without conflicts; the merge node computes
// the ensemble verdict conservatively (any critic flagging issues flags
// the ensemble). The revision loop is bounded by MaxRevisions, with the
// graph's visit cap as backstop.
//
// Every external action in this graph is a model call — data, not an
// action — so no capability invocation, sandbox, or approval gate is
// required here. Capability-backed nodes route through the governed
// registry (see the graph tour in the book's Chapter 4).
func BuildEnsembleGraph(cfg EnsembleConfig) (*agentgraph.Graph, error) {
	if cfg.DraftModel == nil {
		return nil, fmt.Errorf("critic ensemble: DraftModel is required")
	}
	if len(cfg.CriticModels) == 0 {
		return nil, fmt.Errorf("critic ensemble: at least one CriticModel is required")
	}
	if cfg.ReviseModel == nil {
		return nil, fmt.Errorf("critic ensemble: ReviseModel is required")
	}
	maxRevisions := cfg.MaxRevisions
	if maxRevisions <= 0 {
		maxRevisions = 2
	}

	g := agentgraph.NewGraph()

	drafter := &modelPhaseNode{
		id:        "drafter",
		model:     cfg.DraftModel,
		prompt:    "draft",
		outputKey: "critic.draft",
	}
	if err := g.AddNode(drafter); err != nil {
		return nil, err
	}

	for i, criticModel := range cfg.CriticModels {
		critic := &modelPhaseNode{
			id:        fmt.Sprintf("critic-%d", i),
			model:     criticModel,
			prompt:    "critique",
			inputKey:  "critic.draft",
			outputKey: fmt.Sprintf("critic.verdict.%d", i),
			contract: agentgraph.NodeContract{
				SideEffectClass: agentgraph.SideEffectNone,
				Idempotency:     agentgraph.IdempotencyReplaySafe,
			},
		}
		done := agentgraph.NewTerminalNode(fmt.Sprintf("branch-done-%d", i))
		// Parallel edges fan out to the critics; their deltas merge in
		// declaration order. The one serial edge is where the run
		// continues after the merge — a node with parallel edges and no
		// serial successor ends the traversal instead.
		if err := g.AddNode(critic); err != nil {
			return nil, err
		}
		if err := g.AddNode(done); err != nil {
			return nil, err
		}
		if err := g.AddEdge("drafter", critic.ID(), nil, true); err != nil {
			return nil, err
		}
		if err := g.AddEdge(critic.ID(), done.ID(), nil, false); err != nil {
			return nil, err
		}
	}

	merge := agentgraph.NewSystemNode("merge", ensembleMerge(len(cfg.CriticModels)))
	revise := &modelPhaseNode{
		id:        "revise",
		model:     cfg.ReviseModel,
		prompt:    "revise",
		inputKey:  "critic.draft",
		outputKey: "critic.draft",
		contract: agentgraph.NodeContract{
			SideEffectClass:  agentgraph.SideEffectContext,
			Idempotency:      agentgraph.IdempotencyReplaySafe,
			CheckpointPolicy: agentgraph.CheckpointPolicyRequired,
			Recoverability:   agentgraph.NodeRecoverabilityPersisted,
		},
		onExecute: func(env *contextdata.Envelope) {
			env.RequestCheckpoint("critic:revise", 50, false)
			revision, _ := contextdata.GetTyped[int](env, "critic.revisions")
			env.SetWorkingValueWithOrigin("critic.revisions", revision+1,
				contextdata.MemoryClassTask, contextdata.OriginLLM)
		},
	}

	gate := agentgraph.NewSystemNode("gate", ensembleGate(maxRevisions))

	if err := g.AddNode(merge); err != nil {
		return nil, err
	}
	if err := g.AddNode(gate); err != nil {
		return nil, err
	}
	if err := g.AddNode(revise); err != nil {
		return nil, err
	}
	if err := g.AddNode(agentgraph.NewTerminalNode("done")); err != nil {
		return nil, err
	}

	// The serial edge off the fork node continues the run after the
	// parallel branches merge.
	if err := g.AddEdge("drafter", "merge", nil, false); err != nil {
		return nil, err
	}
	// After a revision, the critics re-review: the revise node fans back
	// out to the same parallel branches, so the merge never works from
	// stale verdicts.
	for i := range cfg.CriticModels {
		if err := g.AddEdge("revise", fmt.Sprintf("critic-%d", i), nil, true); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge("revise", "merge", nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge("merge", "gate", nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge("gate", "revise", func(r *execution.Result, _ *contextdata.Envelope) bool {
		next, _ := execution.ResultFields(r.Data)["next"].(string)
		return next == "revise"
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge("gate", "done", func(r *execution.Result, _ *contextdata.Envelope) bool {
		next, _ := execution.ResultFields(r.Data)["next"].(string)
		return next == "finish"
	}, false); err != nil {
		return nil, err
	}
	if err := g.SetStart("drafter"); err != nil {
		return nil, err
	}
	return g, nil
}

// ensembleMerge computes the ensemble verdict: any critic flagging issues
// flags the ensemble (the conservative reading of independent
// verification), and the verdict keys are unioned by the parallel merge.
func ensembleMerge(n int) agentgraph.SystemAction {
	return func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		verdicts := make([]string, 0, n)
		verdict := "clean"
		for i := 0; i < n; i++ {
			v, _ := contextdata.GetTyped[string](env, fmt.Sprintf("critic.verdict.%d", i))
			verdicts = append(verdicts, v)
			if strings.Contains(v, "issues") {
				verdict = "issues"
			}
		}
		env.SetWorkingValueWithOrigin("critic.verdict", verdict,
			contextdata.MemoryClassTask, contextdata.OriginLLM)
		env.SetWorkingValueWithOrigin("critic.ensemble.size", n,
			contextdata.MemoryClassTask, contextdata.OriginTool)
		return &execution.Result{
			Success: true,
			Data: execution.NewToolResultPayload(map[string]any{
				"verdict":  verdict,
				"verdicts": strings.Join(verdicts, "|"),
			}),
		}, nil
	}
}

// ensembleGate routes to revise while the ensemble flags issues and the
// revision bound holds.
func ensembleGate(maxRevisions int) agentgraph.SystemAction {
	return func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		next := "finish"
		if verdict, _ := contextdata.GetTyped[string](env, "critic.verdict"); verdict == "issues" {
			if revision, _ := contextdata.GetTyped[int](env, "critic.revisions"); revision < maxRevisions {
				next = "revise"
			}
		}
		return &execution.Result{
			Success: true,
			Data:    execution.NewToolResultPayload(map[string]any{"next": next}),
		}, nil
	}
}

// modelPhaseNode is one model-backed phase of the pipeline: it streams a
// prompt (with the phase's input key, when set, appended as the material
// under discussion), writes the joined output to its working-memory key
// with LLM provenance, and returns it as a result field — which the graph
// runtime captures as "<node>.<key>".
type modelPhaseNode struct {
	id        string
	model     model.LanguageModel
	prompt    string
	inputKey  string
	outputKey string
	contract  agentgraph.NodeContract
	onExecute func(*contextdata.Envelope)
}

func (n *modelPhaseNode) ID() string                { return n.id }
func (n *modelPhaseNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeObservation }
func (n *modelPhaseNode) Contract() agentgraph.NodeContract {
	return n.contract
}

func (n *modelPhaseNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n.onExecute != nil {
		n.onExecute(env)
	}
	prompt := n.prompt
	if n.inputKey != "" {
		if material, ok := contextdata.GetTyped[string](env, n.inputKey); ok {
			prompt = fmt.Sprintf("%s: %s", n.prompt, material)
		}
	}
	ch, err := n.model.GenerateStream(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for chunk := range ch {
		b.WriteString(chunk)
	}
	output := b.String()
	env.SetWorkingValueWithOrigin(n.outputKey, output,
		contextdata.MemoryClassTask, contextdata.OriginLLM)
	return &execution.Result{
		Success: true,
		Data:    execution.NewToolResultPayload(map[string]any{n.outputKey: output}),
	}, nil
}
