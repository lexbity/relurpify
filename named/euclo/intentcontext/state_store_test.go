package intentcontext

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

func TestStateStoreWriteReadRoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	store := NewStateStore()

	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	state := NewState("task-1", "session-1")
	state.StateVersion = 4
	state.CurrentTurnID = "turn-1"
	state.ActiveThoughtRecipeID = "thoughtrecipe-clarify"
	state.Ambiguity = &AmbiguityCharacterization{
		Kind:               AmbiguityKindUnderspecified,
		Confidence:         0.33,
		Rationale:          "workspace-specific target missing",
		CandidateFamilies:  []string{"code", "docs"},
		NeedsClarification: true,
	}
	state.Turns = []ClarificationTurn{
		{
			TurnID:       "turn-1",
			PromptID:     "intent.clarify.question.v1",
			PromptFamily: "intent.clarify.question",
			Question:     "Which module should be updated?",
			Answer:       "framework/contextdata",
			ResponseKind: ResponseKindConfirm,
			StateVersion: 4,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
	state.ConfirmedEntities = []ConfirmedEntity{
		{
			Name:         "Envelope",
			Kind:         EntityKindType,
			ResolverKey:  "framework/contextdata:Envelope",
			EntityRef:    EntityRef{EntityID: "entity-1", ChunkID: "chunk-1", Kind: EntityKindType, Name: "Envelope", FilePath: "framework/contextdata/envelope.go", Package: "framework/contextdata"},
			SourceTurnID: "turn-1",
			ConfirmedAt:  now,
		},
	}
	state.ConfirmedScopes = []ConfirmedScope{
		{
			Name:         "framework/contextdata",
			AnchorClass:  AnchorClassClarifiedScope,
			Entities:     append([]ConfirmedEntity(nil), state.ConfirmedEntities...),
			Rationale:    "clarified by user response",
			SourceTurnID: "turn-1",
			ConfirmedAt:  now,
		},
	}
	state.GroundedAnchors = []retrieval.AnchorRef{
		{
			AnchorID:   "anchor-1",
			ChunkID:    "chunk-1",
			Term:       "Envelope",
			Definition: "Clarified type",
			Class:      string(AnchorClassClarifiedEntity),
			Active:     true,
			CreatedAt:  now.Format(time.RFC3339),
		},
	}
	state.PendingProjection = []ProjectionIntent{
		{
			RevisionRootID: "root-1",
			MutationKind:   "upsert_edge",
			SubjectIDs:     []string{"entity-1"},
			ObjectIDs:      []string{"scope-1"},
			EdgeKind:       "clarification_scopes",
			IdempotencyKey: "idem-1",
			Provenance: ProjectionProvenance{
				TaskID:       "task-1",
				SessionID:    "session-1",
				TurnID:       "turn-1",
				StateVersion: 4,
				AnswerText:   "framework/contextdata",
				Extractor:    "test",
			},
		},
	}
	state.AppliedMutations = []ProjectionRecord{
		{
			RevisionRootID: "root-1",
			IdempotencyKey: "idem-1",
			GraphRecordIDs: []string{"edge-1"},
			AppliedAt:      now,
			AppliedBy:      "test",
			Result:         ProjectionStatusApplied,
		},
	}
	state.LastCheckpointID = "checkpoint-1"
	state.LastCheckpointSeq = 99

	if err := store.Write(context.Background(), env, state); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if got := len(env.WorkingMemoryKeys()); got != len(ClarificationWorkingMemoryKeys()) {
		t.Fatalf("expected %d clarification keys, got %d", len(ClarificationWorkingMemoryKeys()), got)
	}

	state.ConfirmedEntities[0].Name = "Changed"
	state.GroundedAnchors[0].Term = "Changed"

	readBack, err := store.Read(context.Background(), env)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if readBack.StateVersion != 4 {
		t.Fatalf("expected version 4, got %d", readBack.StateVersion)
	}
	if readBack.TaskID != "task-1" || readBack.SessionID != "session-1" {
		t.Fatalf("unexpected task/session: %s/%s", readBack.TaskID, readBack.SessionID)
	}
	if readBack.ConfirmedEntities[0].Name != "Envelope" {
		t.Fatalf("expected read clone to preserve original entity name, got %q", readBack.ConfirmedEntities[0].Name)
	}
	if readBack.GroundedAnchors[0].Term != "Envelope" {
		t.Fatalf("expected read clone to preserve original anchor term, got %q", readBack.GroundedAnchors[0].Term)
	}
	if readBack.LastCheckpointID != "checkpoint-1" || readBack.LastCheckpointSeq != 99 {
		t.Fatalf("unexpected checkpoint boundary: %s/%d", readBack.LastCheckpointID, readBack.LastCheckpointSeq)
	}

	readBack.ConfirmedEntities[0].Name = "MutatedRead"
	again, err := store.Read(context.Background(), env)
	if err != nil {
		t.Fatalf("second read failed: %v", err)
	}
	if again.ConfirmedEntities[0].Name != "Envelope" {
		t.Fatalf("expected stored state to remain immutable across reads, got %q", again.ConfirmedEntities[0].Name)
	}
}

func TestNextStateVersionIsMonotonic(t *testing.T) {
	if got := NextStateVersion(0); got != 1 {
		t.Fatalf("expected 1 for zero version, got %d", got)
	}
	if got := NextStateVersion(4); got != 5 {
		t.Fatalf("expected 5 for version 4, got %d", got)
	}
}

func TestStateStoreSurvivesCloneAndCheckpointKeys(t *testing.T) {
	env := contextdata.NewEnvelope("task-2", "session-2")
	store := NewStateStore()

	state := NewState("task-2", "session-2")
	state.StateVersion = 2
	state.CurrentTurnID = "turn-2"
	state.GroundedAnchors = []retrieval.AnchorRef{
		{
			AnchorID:   "anchor-2",
			ChunkID:    "chunk-2",
			Term:       "Resolver",
			Definition: "Clarification anchor",
			Class:      "clarified_entity",
			Active:     true,
			CreatedAt:  "2026-05-07T12:00:00Z",
		},
	}

	if err := store.Write(context.Background(), env, state); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	env.References.Checkpoints = append(env.References.Checkpoints, contextdata.CheckpointReference{
		CheckpointID:      "checkpoint-2",
		RequestedBy:       "node-2",
		WorkingMemoryKeys: ClarificationWorkingMemoryKeys(),
	})

	clone := env.HandoffClone()
	if clone == nil {
		t.Fatal("expected handoff clone to be created")
	}
	if len(clone.References.Checkpoints) != 1 {
		t.Fatalf("expected one checkpoint reference, got %d", len(clone.References.Checkpoints))
	}
	if len(clone.References.Checkpoints[0].WorkingMemoryKeys) != len(ClarificationWorkingMemoryKeys()) {
		t.Fatalf("expected checkpoint working-memory keys to survive clone, got %v", clone.References.Checkpoints[0].WorkingMemoryKeys)
	}

	readBack, err := store.Read(context.Background(), clone)
	if err != nil {
		t.Fatalf("read after clone failed: %v", err)
	}
	if readBack.StateVersion != 2 {
		t.Fatalf("expected version 2 after clone read, got %d", readBack.StateVersion)
	}
	if readBack.CurrentTurnID != "turn-2" {
		t.Fatalf("expected current turn turn-2, got %s", readBack.CurrentTurnID)
	}
	if got := len(readBack.GroundedAnchors); got != 1 {
		t.Fatalf("expected 1 grounded anchor, got %d", got)
	}
	if readBack.GroundedAnchors[0].CreatedAt != "2026-05-07T12:00:00Z" {
		t.Fatalf("expected created-at string to survive round-trip, got %q", readBack.GroundedAnchors[0].CreatedAt)
	}
}

func TestClarificationStateValidateRejectsMissingIdentity(t *testing.T) {
	state := NewState("task-3", "session-3")
	state.TaskID = ""
	if err := state.Validate(); err == nil {
		t.Fatal("expected validation error for missing task id")
	}

	state = NewState("task-3", "session-3")
	state.SessionID = ""
	if err := state.Validate(); err == nil {
		t.Fatal("expected validation error for missing session id")
	}

	state = NewState("task-3", "session-3")
	state.StateVersion = 0
	if err := state.Validate(); err == nil {
		t.Fatal("expected validation error for missing version")
	}
}

func TestStateStoreReadRejectsCorruptState(t *testing.T) {
	env := contextdata.NewEnvelope("task-4", "session-4")
	env.SetWorkingValue(ClarificationStateKey, &ClarificationState{
		TaskID:       "task-4",
		SessionID:    "session-4",
		StateVersion: 1,
		Turns: []ClarificationTurn{
			{StableID: "dup"},
			{StableID: "dup"},
		},
	}, contextdata.MemoryClassTask)

	_, err := NewStateStore().Read(context.Background(), env)
	if err == nil {
		t.Fatal("expected read to reject duplicate stable ids")
	}
}

func TestStateStoreWriteRejectsCorruptState(t *testing.T) {
	env := contextdata.NewEnvelope("task-5", "session-5")
	state := NewState("task-5", "session-5")
	state.Turns = []ClarificationTurn{
		{StableID: "turn-1"},
		{StableID: "turn-1"},
	}

	if err := NewStateStore().Write(context.Background(), env, state); err == nil {
		t.Fatal("expected write to reject duplicate stable ids")
	}
}

func TestCanonicalWorkingMemoryKeysIncludeRouteState(t *testing.T) {
	keys := CanonicalWorkingMemoryKeys()
	want := map[string]bool{
		IntentEvidenceKey:       true,
		IntentInterpretationKey: true,
		RouteResolutionKey:      true,
	}
	for _, key := range keys {
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing canonical working-memory keys: %#v", want)
	}
}
