package contextdata

import (
	"context"
	"testing"
	"time"
)

func TestNewEnvelopeCreatesEmptyEnvelope(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	if env == nil {
		t.Fatal("expected envelope to be created")
	}
	if env.TaskID != "task-1" {
		t.Errorf("expected task ID task-1, got %s", env.TaskID)
	}
	if env.SessionID != "session-1" {
		t.Errorf("expected session ID session-1, got %s", env.SessionID)
	}
	if !env.IsEmpty() {
		t.Error("expected new envelope to be empty")
	}
	if env.WorkingData == nil {
		t.Error("expected WorkingData to be initialized")
	}
}

func TestSetWorkingValueStoresAndRetrieves(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)

	val, ok := env.getWorkingValue("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}
}

func TestDeleteWorkingValueRemovesEntry(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)
	env.DeleteWorkingValue("key1")

	_, ok := env.getWorkingValue("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}

	// Reference should also be removed
	if env.References.HasWorkingMemoryKey("task-1", "key1") {
		t.Error("expected reference to be removed")
	}
}

func TestCloneEnvelopeCopiesWorkingData(t *testing.T) {
	parent := NewEnvelope("task-1", "session-1")
	parent.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)
	parent.AddStreamedContextReference(ChunkReference{
		ChunkID: ChunkID("chunk-1"),
		Source:  "test",
		Rank:    1,
	})

	clone := CloneEnvelope(parent, "branch-1")
	if clone == nil {
		t.Fatal("expected clone to be created")
	}

	// Working data should be copied
	val, ok := clone.getWorkingValue("key1")
	if !ok {
		t.Fatal("expected cloned envelope to have key1")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}

	// Streamed context should be copied
	if len(clone.References.StreamedContext) != 1 {
		t.Errorf("expected 1 streamed chunk, got %d", len(clone.References.StreamedContext))
	}

	// Clone should not inherit checkpoint requests
	if clone.CheckpointRequest != nil {
		t.Error("expected clone to not inherit checkpoint request")
	}
}

func TestCloneEnvelopeIsIndependent(t *testing.T) {
	parent := NewEnvelope("task-1", "session-1")
	parent.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)

	clone := CloneEnvelope(parent, "branch-1")

	// Modify clone
	clone.SetWorkingValueWithClass("key2", "value2", MemoryClassEphemeral)

	// Parent should not have key2
	_, ok := parent.getWorkingValue("key2")
	if ok {
		t.Error("expected parent to not have key2 after clone modification")
	}

	// Clone should have both keys
	if _, ok := clone.getWorkingValue("key1"); !ok {
		t.Error("expected clone to have key1")
	}
	if _, ok := clone.getWorkingValue("key2"); !ok {
		t.Error("expected clone to have key2")
	}
}

func TestHandoffCloneCopiesDefaultEnvelopeState(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.NodeID = "node-1"
	env.SetWorkingValueWithClass("key1", "value1", MemoryClassTask)
	env.AddStreamedContextReference(ChunkReference{
		ChunkID: ChunkID("chunk-1"),
		Source:  "test",
		Rank:    1,
	})
	env.AddRetrievalReference(RetrievalReference{
		QueryID:    "query-1",
		ChunkIDs:   []ChunkID{"chunk-1"},
		TotalFound: 1,
	})
	env.References.Checkpoints = append(env.References.Checkpoints, CheckpointReference{
		CheckpointID: "cp-1",
		RequestedBy:  "node-1",
	})
	env.RequestCheckpoint("checkpoint for recovery", 5, true)

	clone := env.HandoffClone()
	if clone == nil {
		t.Fatal("expected handoff clone to be created")
	}
	if clone.NodeID != "node-1" {
		t.Errorf("expected node ID to be preserved, got %s", clone.NodeID)
	}
	if val, ok := clone.getWorkingValue("key1"); !ok || val != "value1" {
		t.Fatalf("expected cloned working value key1=value1, got %v, %v", val, ok)
	}
	if len(clone.References.StreamedContext) != 1 {
		t.Fatalf("expected 1 streamed reference, got %d", len(clone.References.StreamedContext))
	}
	if len(clone.References.Retrieval) != 1 {
		t.Fatalf("expected 1 retrieval reference, got %d", len(clone.References.Retrieval))
	}
	if len(clone.References.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint reference, got %d", len(clone.References.Checkpoints))
	}
	if clone.CheckpointRequest != nil {
		t.Error("expected checkpoint request to be dropped from handoff clone")
	}
}

func TestHandoffSnapshotFiltersByPolicy(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.NodeID = "node-1"
	env.AssemblyMetadata = AssemblyMeta{
		CompilationID:   "compile-1",
		EventLogSeq:     7,
		BudgetTokens:    100,
		ShortfallTokens: 3,
	}
	env.SetWorkingValueWithClass("keep", "value-keep", MemoryClassTask)
	env.SetWorkingValueWithClass("keep.local", "value-prefix", MemoryClassTask)
	env.SetWorkingValueWithClass("drop", "value-drop", MemoryClassTask)
	env.References.WorkingMemory = append(env.References.WorkingMemory,
		WorkingMemoryReference{TaskID: "task-1", Key: "keep", Class: MemoryClassTask},
		WorkingMemoryReference{TaskID: "task-1", Key: "keep.local", Class: MemoryClassTask},
		WorkingMemoryReference{TaskID: "task-1", Key: "drop", Class: MemoryClassTask},
		WorkingMemoryReference{TaskID: "other-task", Key: "keep", Class: MemoryClassTask},
	)
	env.AddStreamedContextReference(ChunkReference{ChunkID: "chunk-1", Source: "test", Rank: 1})
	env.AddRetrievalReference(RetrievalReference{QueryID: "query-1", ChunkIDs: []ChunkID{"chunk-1"}})
	env.References.Checkpoints = append(env.References.Checkpoints, CheckpointReference{
		CheckpointID: "cp-1",
		RequestedBy:  "node-1",
	})

	snapshot := env.HandoffSnapshot(HandoffPolicy{
		PreserveWorkingMemory:    true,
		WorkingKeys:              []string{"keep"},
		WorkingPrefixes:          []string{"keep."},
		PreserveStreamedContext:  true,
		PreserveRetrieval:        true,
		PreserveCheckpoints:      true,
		PreserveAssemblyMetadata: false,
		PreserveNodeID:           false,
	})
	if snapshot == nil {
		t.Fatal("expected handoff snapshot to be created")
	}
	if snapshot.NodeID != "" {
		t.Errorf("expected node ID to be omitted, got %s", snapshot.NodeID)
	}
	if snapshot.AssemblyMetadata != (AssemblyMeta{}) {
		t.Errorf("expected assembly metadata to be omitted, got %#v", snapshot.AssemblyMetadata)
	}
	if _, ok := snapshot.getWorkingValue("keep"); !ok {
		t.Error("expected exact working key to be preserved")
	}
	if _, ok := snapshot.getWorkingValue("keep.local"); !ok {
		t.Error("expected prefix working key to be preserved")
	}
	if _, ok := snapshot.getWorkingValue("drop"); ok {
		t.Error("expected non-matching working key to be filtered out")
	}
	if len(snapshot.References.WorkingMemory) != 4 {
		t.Fatalf("expected 4 working memory refs, got %d", len(snapshot.References.WorkingMemory))
	}
	if !snapshot.References.HasWorkingMemoryKey("task-1", "keep") {
		t.Error("expected task-local working reference to be preserved")
	}
	if snapshot.References.HasWorkingMemoryKey("other-task", "keep") {
		t.Error("expected foreign task working reference to be filtered out")
	}
	if len(snapshot.References.StreamedContext) != 1 {
		t.Fatalf("expected streamed context refs to be preserved, got %d", len(snapshot.References.StreamedContext))
	}
	if len(snapshot.References.Retrieval) != 1 {
		t.Fatalf("expected retrieval refs to be preserved, got %d", len(snapshot.References.Retrieval))
	}
	if len(snapshot.References.Checkpoints) != 1 {
		t.Fatalf("expected checkpoint refs to be preserved, got %d", len(snapshot.References.Checkpoints))
	}
}

func TestMergeBranchEnvelopesUnionsWorkingMemory(t *testing.T) {
	env1 := NewEnvelope("task-1", "session-1")
	env1.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)

	env2 := NewEnvelope("task-1", "session-1")
	env2.SetWorkingValueWithClass("key2", "value2", MemoryClassSession)

	merged, err := MergeBranchEnvelopes("task-1", "session-1", []*Envelope{env1, env2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have both keys
	if _, ok := merged.getWorkingValue("key1"); !ok {
		t.Error("expected merged to have key1")
	}
	if _, ok := merged.getWorkingValue("key2"); !ok {
		t.Error("expected merged to have key2")
	}
}

func TestMergeBranchEnvelopesLastWriteWins(t *testing.T) {
	env1 := NewEnvelope("task-1", "session-1")
	env1.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)

	env2 := NewEnvelope("task-1", "session-1")
	env2.SetWorkingValueWithClass("key1", "value2", MemoryClassEphemeral)

	merged, err := MergeBranchEnvelopes("task-1", "session-1", []*Envelope{env1, env2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Last write wins
	val, _ := merged.getWorkingValue("key1")
	if val != "value2" {
		t.Errorf("expected value2 (last write), got %v", val)
	}
}

func TestMergeBranchEnvelopesDeduplicatesChunks(t *testing.T) {
	env1 := NewEnvelope("task-1", "session-1")
	env1.AddStreamedContextReference(ChunkReference{
		ChunkID: ChunkID("chunk-1"),
		Source:  "test",
		Rank:    1,
	})

	env2 := NewEnvelope("task-1", "session-1")
	env2.AddStreamedContextReference(ChunkReference{
		ChunkID: ChunkID("chunk-1"),
		Source:  "test",
		Rank:    2,
	})
	env2.AddStreamedContextReference(ChunkReference{
		ChunkID: ChunkID("chunk-2"),
		Source:  "test",
		Rank:    3,
	})

	merged, err := MergeBranchEnvelopes("task-1", "session-1", []*Envelope{env1, env2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 unique chunks
	if len(merged.References.StreamedContext) != 2 {
		t.Errorf("expected 2 unique chunks, got %d", len(merged.References.StreamedContext))
	}
}

func TestMergeBranchEnvelopesPreservesCheckpointWorkingMemoryKeys(t *testing.T) {
	env1 := NewEnvelope("task-1", "session-1")
	env1.AddCheckpointReference(CheckpointReference{
		CheckpointID:      "cp-1",
		RequestedBy:       "node-1",
		WorkingMemoryKeys: []string{"euclo.intent.clarification.state", "euclo.intent.clarification.turns"},
	})

	env2 := NewEnvelope("task-1", "session-1")
	env2.AddCheckpointReference(CheckpointReference{
		CheckpointID:      "cp-1",
		RequestedBy:       "node-1",
		WorkingMemoryKeys: []string{"euclo.intent.clarification.confirmed_entities"},
	})

	merged, err := MergeBranchEnvelopes("task-1", "session-1", []*Envelope{env1, env2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(merged.References.Checkpoints) != 1 {
		t.Fatalf("expected 1 merged checkpoint, got %d", len(merged.References.Checkpoints))
	}
	got := merged.References.Checkpoints[0].WorkingMemoryKeys
	if len(got) != 3 {
		t.Fatalf("expected 3 checkpoint working-memory keys, got %v", got)
	}
	want := []string{
		"euclo.intent.clarification.state",
		"euclo.intent.clarification.turns",
		"euclo.intent.clarification.confirmed_entities",
	}
	for i, key := range want {
		if got[i] != key {
			t.Fatalf("checkpoint key %d = %q, want %q (keys=%v)", i, got[i], key, got)
		}
	}
}

func TestReferenceBundleIsEmpty(t *testing.T) {
	empty := ReferenceBundle{}
	if !empty.IsEmpty() {
		t.Error("expected empty bundle to be empty")
	}

	nilBundle := (*ReferenceBundle)(nil)
	if !nilBundle.IsEmpty() {
		t.Error("expected nil bundle to be empty")
	}

	withData := ReferenceBundle{
		WorkingMemory: []WorkingMemoryReference{{TaskID: "t1", Key: "k1"}},
	}
	if withData.IsEmpty() {
		t.Error("expected bundle with data to not be empty")
	}
}

func TestReferenceBundleClone(t *testing.T) {
	original := ReferenceBundle{
		StreamedContext: []ChunkReference{
			{ChunkID: ChunkID("chunk-1"), Rank: 1},
		},
		WorkingMemory: []WorkingMemoryReference{
			{TaskID: "task-1", Key: "key1", Class: MemoryClassEphemeral},
		},
		Retrieval: []RetrievalReference{
			{QueryID: "query-1", ChunkIDs: []ChunkID{"chunk-1", "chunk-2"}},
		},
		Checkpoints: []CheckpointReference{
			{CheckpointID: "cp-1", RequestedBy: "node-1", WorkingMemoryKeys: []string{"euclo.intent.clarification.state"}},
		},
	}

	clone := original.Clone()

	// Modify original retrieval chunk IDs
	original.Retrieval[0].ChunkIDs = append(original.Retrieval[0].ChunkIDs, ChunkID("chunk-3"))
	original.Checkpoints[0].WorkingMemoryKeys[0] = "mutated-key"

	// Clone should not be affected
	if len(clone.Retrieval[0].ChunkIDs) != 2 {
		t.Errorf("expected clone to have 2 chunk IDs, got %d", len(clone.Retrieval[0].ChunkIDs))
	}
	if clone.Checkpoints[0].WorkingMemoryKeys[0] != "euclo.intent.clarification.state" {
		t.Fatalf("expected checkpoint keys to be deep copied, got %v", clone.Checkpoints[0].WorkingMemoryKeys)
	}
}

func TestCheckpointReferenceKeysSurviveHandoffClone(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.AddCheckpointReference(CheckpointReference{
		CheckpointID:      "cp-1",
		RequestedBy:       "node-1",
		WorkingMemoryKeys: []string{"euclo.intent.clarification.state", "euclo.intent.clarification.turns"},
	})

	clone := env.HandoffClone()
	if clone == nil {
		t.Fatal("expected handoff clone to be created")
	}
	if len(clone.References.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint reference, got %d", len(clone.References.Checkpoints))
	}
	got := clone.References.Checkpoints[0].WorkingMemoryKeys
	if len(got) != 2 {
		t.Fatalf("expected 2 checkpoint working-memory keys, got %d", len(got))
	}
	if got[0] != "euclo.intent.clarification.state" || got[1] != "euclo.intent.clarification.turns" {
		t.Fatalf("unexpected checkpoint working-memory keys: %v", got)
	}

	env.References.Checkpoints[0].WorkingMemoryKeys[0] = "changed"
	if clone.References.Checkpoints[0].WorkingMemoryKeys[0] != "euclo.intent.clarification.state" {
		t.Fatal("expected checkpoint keys in clone to remain unchanged after source mutation")
	}
}

func TestCheckpointRequest(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.NodeID = "node-1"

	env.RequestCheckpoint("checkpoint for recovery", 5, true)

	if env.CheckpointRequest == nil {
		t.Fatal("expected checkpoint request to be set")
	}
	if env.CheckpointRequest.RequestedBy != "node-1" {
		t.Errorf("expected requested by node-1, got %s", env.CheckpointRequest.RequestedBy)
	}
	if env.CheckpointRequest.Reason != "checkpoint for recovery" {
		t.Errorf("expected reason 'checkpoint for recovery', got %s", env.CheckpointRequest.Reason)
	}
	if !env.CheckpointRequest.EvictWorkingMemory {
		t.Error("expected EvictWorkingMemory to be true")
	}

	env.ClearCheckpointRequest()
	if env.CheckpointRequest != nil {
		t.Error("expected checkpoint request to be cleared")
	}
}

func TestComputeBranchDelta(t *testing.T) {
	parent := NewEnvelope("task-1", "session-1")
	parent.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)
	parent.SetWorkingValueWithClass("key2", "value2", MemoryClassEphemeral)

	child := NewEnvelope("task-1", "session-1")
	child.SetWorkingValueWithClass("key2", "modified", MemoryClassEphemeral) // Modified
	child.SetWorkingValueWithClass("key3", "value3", MemoryClassEphemeral)   // Added
	// key1 not in child = deleted

	delta := ComputeBranchDelta(parent, child)

	// key2 might be counted as both modified and added depending on implementation
	// key3 is definitely added
	hasKey3 := false
	for _, k := range delta.WorkingMemoryAdded {
		if k == "key3" {
			hasKey3 = true
			break
		}
	}
	if !hasKey3 {
		t.Error("expected key3 to be in added list")
	}

	// key1 should be in deleted list
	hasKey1 := false
	for _, k := range delta.WorkingMemoryDeleted {
		if k == "key1" {
			hasKey1 = true
			break
		}
	}
	if !hasKey1 {
		t.Error("expected key1 to be in deleted list")
	}
}

func TestValidateBranchMerge(t *testing.T) {
	// Single envelope should validate
	env1 := NewEnvelope("task-1", "session-1")
	err := ValidateBranchMerge([]*Envelope{env1})
	if err != nil {
		t.Errorf("expected single envelope to validate, got: %v", err)
	}

	// Multiple envelopes same task should validate
	env2 := NewEnvelope("task-1", "session-1")
	err = ValidateBranchMerge([]*Envelope{env1, env2})
	if err != nil {
		t.Errorf("expected same-task envelopes to validate, got: %v", err)
	}

	// Different tasks should fail
	env3 := NewEnvelope("task-2", "session-1")
	err = ValidateBranchMerge([]*Envelope{env1, env3})
	if err == nil {
		t.Error("expected different-task envelopes to fail validation")
	}
}

func TestDeduplicateChunkReferences(t *testing.T) {
	refs := []ChunkReference{
		{ChunkID: ChunkID("chunk-1"), Source: "ranker-a", Rank: 2},
		{ChunkID: ChunkID("chunk-2"), Source: "ranker-a", Rank: 3},
		{ChunkID: ChunkID("chunk-1"), Source: "ranker-b", Rank: 1}, // Duplicate, better rank
	}

	deduped := DeduplicateChunkReferences(refs)

	if len(deduped) != 2 {
		t.Errorf("expected 2 unique chunks, got %d", len(deduped))
	}

	// chunk-1 should have rank 1 (the better one)
	for _, ref := range deduped {
		if ref.ChunkID == ChunkID("chunk-1") && ref.Rank != 1 {
			t.Errorf("expected chunk-1 to have rank 1, got %d", ref.Rank)
		}
	}

	// Should be sorted by rank
	for i := 1; i < len(deduped); i++ {
		if deduped[i].Rank < deduped[i-1].Rank {
			t.Error("expected deduped references to be sorted by rank")
		}
	}
}

func TestEnvelopeContextStorage(t *testing.T) {
	ctx := WithEnvelope(context.TODO(), NewEnvelope("task-1", "session-1"))

	env, ok := EnvelopeFrom(ctx)
	if !ok {
		t.Fatal("expected to retrieve envelope from context")
	}
	if env.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", env.TaskID)
	}

	// Empty context should return false
	_, ok = EnvelopeFrom(context.TODO())
	if ok {
		t.Error("expected nil context to return false")
	}
}

func TestMustEnvelopeFromPanicsOnMissing(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustEnvelopeFrom to panic on missing envelope")
		}
	}()

	MustEnvelopeFrom(context.TODO())
}

func TestEnvelopeSnapshot(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)

	snapshot := env.Snapshot()
	if len(snapshot) != 1 {
		t.Errorf("expected 1 entry in snapshot, got %d", len(snapshot))
	}

	// Modifying snapshot should not affect envelope
	snapshot["key2"] = "value2"
	if _, ok := env.getWorkingValue("key2"); ok {
		t.Error("expected snapshot modification to not affect envelope")
	}
}

func TestEnvelopeString(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.NodeID = "node-1"
	env.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)
	env.AddStreamedContextReference(ChunkReference{ChunkID: "chunk-1"})

	s := env.String()
	if s == "" || s == "<nil envelope>" {
		t.Error("expected non-empty string representation")
	}
}

func TestMemoryClassConstants(t *testing.T) {
	// Verify memory class constants exist
	classes := []MemoryClass{MemoryClassEphemeral, MemoryClassSession, MemoryClassTask}
	for _, c := range classes {
		if c == "" {
			t.Error("expected memory class to be non-empty")
		}
	}
}

func TestReferenceTypeConstants(t *testing.T) {
	// Verify reference type constants exist
	types := []ReferenceType{
		RefTypeStreamedContext,
		RefTypeWorkingMemory,
		RefTypeRetrieval,
		RefTypeCheckpoint,
	}
	for _, rt := range types {
		if rt == "" {
			t.Error("expected reference type to be non-empty")
		}
	}
}

func TestWorkingMemoryReferenceLookup(t *testing.T) {
	bundle := ReferenceBundle{
		WorkingMemory: []WorkingMemoryReference{
			{TaskID: "task-1", Key: "key1", Class: MemoryClassEphemeral, CreatedAt: time.Now()},
			{TaskID: "task-1", Key: "key2", Class: MemoryClassSession, CreatedAt: time.Now()},
		},
	}

	if !bundle.HasWorkingMemoryKey("task-1", "key1") {
		t.Error("expected to find key1 for task-1")
	}
	if bundle.HasWorkingMemoryKey("task-1", "key3") {
		t.Error("expected not to find key3 for task-1")
	}
	if bundle.HasWorkingMemoryKey("task-2", "key1") {
		t.Error("expected not to find key1 for task-2")
	}

	ref, ok := bundle.GetWorkingMemoryRef("task-1", "key1")
	if !ok {
		t.Error("expected to get ref for key1")
	}
	if ref.Key != "key1" {
		t.Errorf("expected key1, got %s", ref.Key)
	}
	if ref.Class != MemoryClassEphemeral {
		t.Errorf("expected Ephemeral class, got %s", ref.Class)
	}
}

func TestAddRetrievalReference(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	ref := RetrievalReference{
		QueryID:     "query-1",
		QueryText:   "test query",
		ChunkIDs:    []ChunkID{"chunk-1"},
		TotalFound:  5,
		FilteredOut: 2,
		RetrievedAt: time.Now(),
		Duration:    time.Millisecond * 100,
	}

	env.AddRetrievalReference(ref)

	if len(env.References.Retrieval) != 1 {
		t.Errorf("expected 1 retrieval reference, got %d", len(env.References.Retrieval))
	}
}

func TestStreamedChunkIDs(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.AddStreamedContextReference(ChunkReference{ChunkID: "chunk-1"})
	env.AddStreamedContextReference(ChunkReference{ChunkID: "chunk-2"})

	ids := env.StreamedChunkIDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 chunk IDs, got %d", len(ids))
	}

	// Nil envelope
}

func TestWorkingMemoryKeys(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)
	env.SetWorkingValueWithClass("key2", "value2", MemoryClassEphemeral)

	// Add reference for different task (shouldn't appear)
	env.References.WorkingMemory = append(env.References.WorkingMemory, WorkingMemoryReference{
		TaskID: "other-task",
		Key:    "other-key",
	})

	keys := env.WorkingMemoryKeys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys for task-1, got %d", len(keys))
	}

	// Nil envelope
}

// Phase 1 edge-case tests per migration spec

func TestSetWorkingValueUpdatesReferenceNotDuplicate(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")

	// Set initial value
	env.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)
	initialRef, _ := env.References.GetWorkingMemoryRef("task-1", "key1")
	initialCreatedAt := initialRef.CreatedAt

	// Small delay to ensure time difference
	time.Sleep(time.Millisecond)

	// Set same key again - should update existing reference, not create duplicate
	env.SetWorkingValueWithClass("key1", "value2", MemoryClassSession)

	// Should still have only 1 reference for this key
	refCount := 0
	var updatedRef WorkingMemoryReference
	for _, ref := range env.References.WorkingMemory {
		if ref.TaskID == "task-1" && ref.Key == "key1" {
			refCount++
			updatedRef = ref
		}
	}
	if refCount != 1 {
		t.Errorf("expected 1 reference for key1, got %d", refCount)
	}

	// CreatedAt should be unchanged
	if !updatedRef.CreatedAt.Equal(initialCreatedAt) {
		t.Error("expected CreatedAt to remain unchanged on update")
	}

	// UpdatedAt should be newer
	if !updatedRef.UpdatedAt.After(initialCreatedAt) {
		t.Error("expected UpdatedAt to be after CreatedAt")
	}

	// Class should be updated
	if updatedRef.Class != MemoryClassSession {
		t.Errorf("expected class to be updated to Session, got %s", updatedRef.Class)
	}

	// Value should be updated
	val, _ := env.getWorkingValue("key1")
	if val != "value2" {
		t.Errorf("expected value2, got %v", val)
	}
}

func TestMergeBranchEnvelopesSkipsNilEntries(t *testing.T) {
	env1 := NewEnvelope("task-1", "session-1")
	env1.SetWorkingValueWithClass("key1", "value1", MemoryClassEphemeral)

	env2 := NewEnvelope("task-1", "session-1")
	env2.SetWorkingValueWithClass("key2", "value2", MemoryClassEphemeral)

	// Merge with nil entries in the slice
	merged, err := MergeBranchEnvelopes("task-1", "session-1", []*Envelope{env1, nil, env2, nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have both keys, nil entries skipped
	if _, ok := merged.getWorkingValue("key1"); !ok {
		t.Error("expected merged to have key1")
	}
	if _, ok := merged.getWorkingValue("key2"); !ok {
		t.Error("expected merged to have key2")
	}
}

func TestValidateBranchMergeEmptySlice(t *testing.T) {
	// Empty slice should return nil (nothing to validate)
	err := ValidateBranchMerge([]*Envelope{})
	if err != nil {
		t.Errorf("expected nil error for empty slice, got: %v", err)
	}
}

func TestNilEnvelopeMethodsPanic(t *testing.T) {
	assertPanics := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected %s to panic", name)
			}
		}()
		fn()
	}

	var nilEnv *Envelope
	assertPanics("SetWorkingValue", func() { nilEnv.SetWorkingValueWithClass("key", "value", MemoryClassEphemeral) })
	assertPanics("DeleteWorkingValue", func() { nilEnv.DeleteWorkingValue("key") })
	assertPanics("RequestCheckpoint", func() { nilEnv.RequestCheckpoint("reason", 1, true) })
	assertPanics("ClearCheckpointRequest", func() { nilEnv.ClearCheckpointRequest() })
	assertPanics("AddRetrievalReference", func() { nilEnv.AddRetrievalReference(RetrievalReference{}) })
	assertPanics("AddStreamedContextReference", func() { nilEnv.AddStreamedContextReference(ChunkReference{}) })
	assertPanics("GetWorkingValue", func() { _, _ = nilEnv.getWorkingValue("key") })
	assertPanics("StreamedChunkIDs", func() { _ = nilEnv.StreamedChunkIDs() })
	assertPanics("WorkingMemoryKeys", func() { _ = nilEnv.WorkingMemoryKeys() })
	assertPanics("IsEmpty", func() { _ = nilEnv.IsEmpty() })
	assertPanics("Snapshot", func() { _ = nilEnv.Snapshot() })
}
