package intentcontext

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

func TestResolveEntityByFilePathAndName(t *testing.T) {
	core := newIntentCoreForTest(t)
	_, err := core.ChunkStore.Save(knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("chunk:envelope"),
		WorkspaceID: "ws",
		Provenance:  knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: time.Now().UTC()},
		Body: knowledge.ChunkBody{
			Raw: "type Envelope struct{}",
			Fields: map[string]any{
				"name":      "Envelope",
				"file_path": "framework/contextdata/envelope.go",
			},
		},
	})
	if err != nil {
		t.Fatalf("save chunk: %v", err)
	}

	ref, err := core.ResolveEntity(context.Background(), "Envelope", EntityKindType)
	if err != nil {
		t.Fatalf("ResolveEntity failed: %v", err)
	}
	if ref.Name != "Envelope" {
		t.Fatalf("unexpected entity name: %q", ref.Name)
	}
	if ref.FilePath != "framework/contextdata/envelope.go" {
		t.Fatalf("unexpected file path: %q", ref.FilePath)
	}
}

func TestGroundConfirmedDedupesAnchorsAndWritesState(t *testing.T) {
	core := newIntentCoreForTest(t)
	_, err := core.ChunkStore.Save(knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("chunk:contextdata"),
		WorkspaceID: "ws",
		Provenance:  knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: time.Now().UTC()},
		Body: knowledge.ChunkBody{
			Raw: "package contextdata",
			Fields: map[string]any{
				"name":      "contextdata",
				"file_path": "framework/contextdata/envelope.go",
			},
		},
	})
	if err != nil {
		t.Fatalf("save scope chunk: %v", err)
	}
	env := contextdata.NewEnvelope("task-1", "session-1")
	state := NewState("task-1", "session-1")
	state.StateVersion = 3
	state.ConfirmedEntities = []ConfirmedEntity{
		{
			Name:        "Envelope",
			Kind:        EntityKindType,
			ResolverKey: "framework/contextdata:Envelope",
			EntityRef: EntityRef{
				EntityID: "chunk:envelope",
				ChunkID:  "chunk:envelope",
				Kind:     EntityKindType,
				Name:     "Envelope",
				FilePath: "framework/contextdata/envelope.go",
			},
			SourceTurnID: "turn-1",
		},
	}
	if err := core.Store.Write(context.Background(), env, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	grounding, err := core.GroundConfirmed(context.Background(), env, ScopeDeclaration{
		Kind:        ScopeKindPackageSubtree,
		Name:        "framework/contextdata",
		AnchorClass: AnchorClassClarifiedScope,
		Selector:    "framework/contextdata",
	})
	if err != nil {
		t.Fatalf("GroundConfirmed failed: %v", err)
	}
	if grounding.StateVersion != 4 {
		t.Fatalf("expected state version 4, got %d", grounding.StateVersion)
	}
	if len(grounding.Added) == 0 {
		t.Fatal("expected added anchors")
	}
	if len(grounding.Reused) != 0 {
		t.Fatalf("unexpected reused anchors: %#v", grounding.Reused)
	}

	updated, err := core.Store.Read(context.Background(), env)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if len(updated.GroundedAnchors) == 0 {
		t.Fatal("expected grounded anchors in state")
	}
	if updated.StateVersion != 4 {
		t.Fatalf("expected incremented state version, got %d", updated.StateVersion)
	}
}

func TestBuildClarificationRequestIncludesTraversalAndAnchors(t *testing.T) {
	core := newIntentCoreForTest(t)
	env := contextdata.NewEnvelope("task-2", "session-2")
	state := NewState("task-2", "session-2")
	state.StateVersion = 8
	state.CurrentTurnID = "turn-8"
	state.Ambiguity = &AmbiguityCharacterization{
		Kind:       AmbiguityKindUnderspecified,
		Confidence: 0.42,
		Rationale:  "missing module",
	}
	state.GroundedAnchors = []retrieval.AnchorRef{
		{
			AnchorID:  "anchor-1",
			ChunkID:   "chunk:root",
			Term:      "Envelope",
			Class:     string(AnchorClassClarifiedEntity),
			Active:    true,
			CreatedAt: "2026-05-07T12:00:00Z",
		},
	}
	if err := core.Store.Write(context.Background(), env, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	req, err := core.BuildClarificationRequest(context.Background(), env, "Which module should change?", 256, contextstream.ModeBlocking)
	if err != nil {
		t.Fatalf("BuildClarificationRequest failed: %v", err)
	}
	if req.Query.Text != "Which module should change?" {
		t.Fatalf("unexpected query text: %q", req.Query.Text)
	}
	if req.Query.Traversal == nil || len(req.Query.Traversal.AnchorIDs) != 1 {
		t.Fatalf("expected traversal anchors, got %#v", req.Query.Traversal)
	}
	if req.Metadata["state_version"] != uint64(8) {
		t.Fatalf("unexpected state version metadata: %#v", req.Metadata["state_version"])
	}
}

func TestBuildProjectionPlanAndApply(t *testing.T) {
	core := newIntentCoreForTest(t)
	env := contextdata.NewEnvelope("task-3", "session-3")
	state := NewState("task-3", "session-3")
	state.StateVersion = 5
	state.CurrentTurnID = "turn-5"
	state.PendingRelationIntents = []RelationIntent{
		{
			StableID:       "relation-1",
			SourceEntityID: "chunk:src",
			TargetEntityID: "chunk:dst",
			RelationKind:   "clarification_related_to",
			Direction:      "out",
			SourceTurnID:   "turn-5",
		},
	}
	if err := core.Store.Write(context.Background(), env, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	plan, err := core.BuildProjectionPlan(context.Background(), env)
	if err != nil {
		t.Fatalf("BuildProjectionPlan failed: %v", err)
	}
	if len(plan.Intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(plan.Intents))
	}
	if plan.Intents[0].MutationKind != "upsert_edge" {
		t.Fatalf("unexpected mutation kind: %q", plan.Intents[0].MutationKind)
	}

	result, err := core.ApplyProjection(context.Background(), env, plan)
	if err != nil {
		t.Fatalf("ApplyProjection failed: %v", err)
	}
	if len(result.Applied) != 1 {
		t.Fatalf("expected 1 applied record, got %d", len(result.Applied))
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %#v", result.Conflicts)
	}
	if result.Mutation == nil {
		t.Fatal("expected projection mutation summary")
	}
	if result.Mutation.Scope != graphdb.MutationScopeProjection {
		t.Fatalf("unexpected mutation scope: %q", result.Mutation.Scope)
	}
	if result.Mutation.Status != graphdb.MutationStatusCreated {
		t.Fatalf("unexpected mutation status: %q", result.Mutation.Status)
	}
	if result.Mutation.StableID == "" {
		t.Fatal("expected mutation stable id")
	}
	if got := core.Graph.GetOutEdges("chunk:src", graphdb.EdgeKind("clarification_related_to")); len(got) != 1 {
		t.Fatalf("expected projected edge, got %#v", got)
	}
	updated, err := core.Store.Read(context.Background(), env)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if len(updated.AppliedMutations) != 1 {
		t.Fatalf("expected applied mutation in state, got %d", len(updated.AppliedMutations))
	}
	if updated.StateVersion != 6 {
		t.Fatalf("expected incremented state version, got %d", updated.StateVersion)
	}

	reapply, err := core.ApplyProjection(context.Background(), env, plan)
	if err != nil {
		t.Fatalf("reapply ApplyProjection failed: %v", err)
	}
	if reapply.Mutation == nil {
		t.Fatal("expected reapply mutation summary")
	}
	if reapply.Mutation.Status != graphdb.MutationStatusMatched {
		t.Fatalf("expected matched status on replay, got %q", reapply.Mutation.Status)
	}
	reapplied, err := core.Store.Read(context.Background(), env)
	if err != nil {
		t.Fatalf("read reapplied state: %v", err)
	}
	if reapplied.StateVersion != 6 {
		t.Fatalf("expected state version to remain 6 on replay, got %d", reapplied.StateVersion)
	}
	if got := core.Graph.GetOutEdges("chunk:src", graphdb.EdgeKind("clarification_related_to")); len(got) != 1 {
		t.Fatalf("expected one active projected edge after replay, got %#v", got)
	}
}

func TestProjectionPlanValidateRejectsDuplicateStableIDs(t *testing.T) {
	plan := &ProjectionPlan{
		PlanID: "plan-dup",
		Intents: []ProjectionIntent{
			{
				StableID:     "intent-1",
				MutationKind: "upsert_node",
				SubjectIDs:   []string{"node-1"},
				NodeKind:     "type",
			},
			{
				StableID:     "intent-1",
				MutationKind: "upsert_node",
				SubjectIDs:   []string{"node-2"},
				NodeKind:     "type",
			},
		},
	}
	if err := plan.Validate(); err == nil {
		t.Fatal("expected duplicate stable_id validation failure")
	}
}

func newIntentCoreForTest(t *testing.T) *IntentCore {
	t.Helper()
	graph, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	t.Cleanup(func() { _ = graph.Close() })
	return &IntentCore{
		Store:      NewStateStore(),
		ChunkStore: &knowledge.ChunkStore{Graph: graph},
		Graph:      graph,
	}
}
