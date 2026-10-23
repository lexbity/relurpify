package critic_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/critic"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	agentgraph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
)

// TestEnsembleGraphRunsAndMerges runs the exported ensemble builder with
// three independent critics — one flags issues — and asserts the whole
// pipeline: the parallel merge unions all three verdict keys without
// conflicts, the ensemble verdict is conservative, one revision fires,
// and the re-review passes clean.
func TestEnsembleGraphRunsAndMerges(t *testing.T) {
	cfg := critic.EnsembleConfig{
		DraftModel: critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"a rough draft"}}),
		CriticModels: []model.LanguageModel{
			critic.NewFakeModel(
				critic.FakeTurn{Chunks: []string{"issues: no error handling"}},
				critic.FakeTurn{Chunks: []string{"clean"}},
			),
			critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"clean"}}),
			critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"clean"}}),
		},
		ReviseModel:  critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"a revised draft"}}),
		MaxRevisions: 2,
	}

	g, err := critic.BuildEnsembleGraph(cfg)
	if err != nil {
		t.Fatalf("BuildEnsembleGraph: %v", err)
	}

	env := newTourEnvelope()
	result, err := g.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %+v, want success", result)
	}

	// The run ends after the re-review, so every branch's key holds its
	// second-round verdict — the parallel merge unioned them, and the
	// re-review passed clean.
	for i := range cfg.CriticModels {
		key := fmt.Sprintf("critic.verdict.%d", i)
		if got, ok := contextdata.GetTyped[string](env, key); !ok || got != "clean" {
			t.Fatalf("%s = %q, %v, want the re-review's clean", key, got, ok)
		}
	}
	if got, _ := contextdata.GetTyped[string](env, "critic.verdict"); got != "clean" {
		t.Fatalf("ensemble verdict = %q, want clean after the re-review", got)
	}
	// The first round did flag issues — that is why exactly one revision
	// fired before the re-review passed.
	if got, _ := contextdata.GetTyped[int](env, "critic.revisions"); got != 1 {
		t.Fatalf("revisions = %d, want 1", got)
	}
	if got, _ := contextdata.GetTyped[string](env, "critic.draft"); got != "a revised draft" {
		t.Fatalf("final draft = %q, want the revision", got)
	}
	if env.CheckpointRequestSnapshot() == nil {
		t.Fatal("revise node did not request a checkpoint")
	}
}

// TestParallelMergeResolvesConflictsByDeclarationOrder proves the merge
// rule the doc states: branches run on cloned envelopes, and on a
// conflicting key the branch later in edge-declaration order wins.
// Conflicts are recorded, not rejected.
func TestParallelMergeResolvesConflictsByDeclarationOrder(t *testing.T) {
	// A branch node that writes a fixed value to a shared key.
	conflicting := func(id, value string) agentgraph.Node {
		return agentgraph.NewSystemNode(id, func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
			env.SetWorkingValueWithOrigin("conflict.key", value,
				contextdata.MemoryClassTask, contextdata.OriginTool)
			return &execution.Result{Success: true}, nil
		})
	}

	readBack := agentgraph.NewSystemNode("readback", func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		winner, _ := contextdata.GetTyped[string](env, "conflict.key")
		env.SetWorkingValueWithOrigin("conflict.winner", winner,
			contextdata.MemoryClassTask, contextdata.OriginTool)
		return &execution.Result{Success: true}, nil
	})

	g := agentgraph.NewGraph()
	mustBuild(t,
		g.AddNode(agentgraph.NewSystemNode("fork", nil)), // pass-through fork point
		g.AddNode(conflicting("b0", "first-declared")),
		g.AddNode(conflicting("b1", "second-declared")),
		g.AddNode(readBack),
		g.AddNode(agentgraph.NewTerminalNode("done")),
		g.AddEdge("fork", "b0", nil, true),
		g.AddEdge("fork", "b1", nil, true),
		g.AddEdge("b0", "done", nil, false),
		g.AddEdge("b1", "done", nil, false),
		g.AddEdge("fork", "readback", nil, false),
		g.AddEdge("readback", "done", nil, false),
		g.SetStart("fork"),
	)

	env := newTourEnvelope()
	if _, err := g.Execute(context.Background(), env); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	winner, _ := contextdata.GetTyped[string](env, "conflict.winner")
	if winner != "second-declared" {
		t.Fatalf("conflict winner = %q, want the later-declared branch", winner)
	}
}

// TestRetrievalNodeFailsVisibly shows the retrieval node's honest
// behavior when its retriever is not wired: a failed result with a named
// cause, not a panic and not a silent pass. The node stands alone in the
// graph — a failed intermediate result does not stop a run, so the
// failure is only the final result when nothing follows it.
func TestRetrievalNodeFailsVisibly(t *testing.T) {
	g := agentgraph.NewGraph()
	mustBuild(t,
		g.AddNode(agentgraph.NewRetrievalNode("retrieve", nil, "task.instruction", "retrieval.results")),
		g.SetStart("retrieve"),
	)

	result, err := g.Execute(context.Background(), newTourEnvelope())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Success {
		t.Fatal("retrieval without a retriever should fail visibly")
	}
	if !strings.Contains(result.Error, "retriever not configured") {
		t.Fatalf("error = %q, want retriever not configured", result.Error)
	}
}
