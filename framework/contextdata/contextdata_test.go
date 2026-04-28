package contextdata

import (
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
	env.SetWorkingValue("key1", "value1", MemoryClassEphemeral)

	val, ok := env.GetWorkingValue("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}
}

func TestDeleteWorkingValueRemovesEntry(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("key1", "value1", MemoryClassEphemeral)
	env.DeleteWorkingValue("key1")

	_, ok := env.GetWorkingValue("key1")
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
	parent.SetWorkingValue("key1", "value1", MemoryClassEphemeral)
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
	val, ok := clone.GetWorkingValue("key1")
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
	parent.SetWorkingValue("key1", "value1", MemoryClassEphemeral)

	clone := CloneEnvelope(parent, "branch-1")

	// Modify clone
	clone.SetWorkingValue("key2", "value2", MemoryClassEphemeral)

	// Parent should not have key2
	_, ok := parent.GetWorkingValue("key2")
	if ok {
		t.Error("expected parent to not have key2 after clone modification")
	}

	// Clone should have both keys
	if _, ok := clone.GetWorkingValue("key1"); !ok {
		t.Error("expected clone to have key1")
	}
	if _, ok := clone.GetWorkingValue("key2"); !ok {
		t.Error("expected clone to have key2")
	}
}

func TestMergeBranchEnvelopesUnionsWorkingMemory(t *testing.T) {
	env1 := NewEnvelope("task-1", "session-1")
	env1.SetWorkingValue("key1", "value1", MemoryClassEphemeral)

	env2 := NewEnvelope("task-1", "session-1")
	env2.SetWorkingValue("key2", "value2", MemoryClassSession)

	merged, err := MergeBranchEnvelopes("task-1", "session-1", []*Envelope{env1, env2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have both keys
	if _, ok := merged.GetWorkingValue("key1"); !ok {
		t.Error("expected merged to have key1")
	}
	if _, ok := merged.GetWorkingValue("key2"); !ok {
		t.Error("expected merged to have key2")
	}
}

func TestMergeBranchEnvelopesLastWriteWins(t *testing.T) {
	env1 := NewEnvelope("task-1", "session-1")
	env1.SetWorkingValue("key1", "value1", MemoryClassEphemeral)

	env2 := NewEnvelope("task-1", "session-1")
	env2.SetWorkingValue("key1", "value2", MemoryClassEphemeral)

	merged, err := MergeBranchEnvelopes("task-1", "session-1", []*Envelope{env1, env2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Last write wins
	val, _ := merged.GetWorkingValue("key1")
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
			{CheckpointID: "cp-1", RequestedBy: "node-1"},
		},
	}

	clone := original.Clone()

	// Modify original retrieval chunk IDs
	original.Retrieval[0].ChunkIDs = append(original.Retrieval[0].ChunkIDs, ChunkID("chunk-3"))

	// Clone should not be affected
	if len(clone.Retrieval[0].ChunkIDs) != 2 {
		t.Errorf("expected clone to have 2 chunk IDs, got %d", len(clone.Retrieval[0].ChunkIDs))
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
	parent.SetWorkingValue("key1", "value1", MemoryClassEphemeral)
	parent.SetWorkingValue("key2", "value2", MemoryClassEphemeral)

	child := NewEnvelope("task-1", "session-1")
	child.SetWorkingValue("key2", "modified", MemoryClassEphemeral) // Modified
	child.SetWorkingValue("key3", "value3", MemoryClassEphemeral)   // Added
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
	ctx := WithEnvelope(nil, NewEnvelope("task-1", "session-1"))

	env, ok := EnvelopeFrom(ctx)
	if !ok {
		t.Fatal("expected to retrieve envelope from context")
	}
	if env.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", env.TaskID)
	}

	// Empty context should return false
	_, ok = EnvelopeFrom(nil)
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

	MustEnvelopeFrom(nil)
}

func TestEnvelopeSnapshot(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("key1", "value1", MemoryClassEphemeral)

	snapshot := env.Snapshot()
	if len(snapshot) != 1 {
		t.Errorf("expected 1 entry in snapshot, got %d", len(snapshot))
	}

	// Modifying snapshot should not affect envelope
	snapshot["key2"] = "value2"
	if _, ok := env.GetWorkingValue("key2"); ok {
		t.Error("expected snapshot modification to not affect envelope")
	}
}

func TestEnvelopeString(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.NodeID = "node-1"
	env.SetWorkingValue("key1", "value1", MemoryClassEphemeral)
	env.AddStreamedContextReference(ChunkReference{ChunkID: "chunk-1"})

	s := env.String()
	if s == "" || s == "<nil envelope>" {
		t.Error("expected non-empty string representation")
	}

	// Nil envelope
	var nilEnv *Envelope
	if nilEnv.String() != "<nil envelope>" {
		t.Error("expected nil envelope string representation")
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
	var nilEnv *Envelope
	ids = nilEnv.StreamedChunkIDs()
	if ids != nil {
		t.Error("expected nil from nil envelope")
	}
}

func TestWorkingMemoryKeys(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("key1", "value1", MemoryClassEphemeral)
	env.SetWorkingValue("key2", "value2", MemoryClassEphemeral)

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
	var nilEnv *Envelope
	keys = nilEnv.WorkingMemoryKeys()
	if keys != nil {
		t.Error("expected nil from nil envelope")
	}
}

// Phase 1 edge-case tests per migration spec

func TestSetWorkingValueUpdatesReferenceNotDuplicate(t *testing.T) {
	env := NewEnvelope("task-1", "session-1")

	// Set initial value
	env.SetWorkingValue("key1", "value1", MemoryClassEphemeral)
	initialRef, _ := env.References.GetWorkingMemoryRef("task-1", "key1")
	initialCreatedAt := initialRef.CreatedAt

	// Small delay to ensure time difference
	time.Sleep(time.Millisecond)

	// Set same key again - should update existing reference, not create duplicate
	env.SetWorkingValue("key1", "value2", MemoryClassSession)

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
	val, _ := env.GetWorkingValue("key1")
	if val != "value2" {
		t.Errorf("expected value2, got %v", val)
	}
}

func TestMergeBranchEnvelopesSkipsNilEntries(t *testing.T) {
	env1 := NewEnvelope("task-1", "session-1")
	env1.SetWorkingValue("key1", "value1", MemoryClassEphemeral)

	env2 := NewEnvelope("task-1", "session-1")
	env2.SetWorkingValue("key2", "value2", MemoryClassEphemeral)

	// Merge with nil entries in the slice
	merged, err := MergeBranchEnvelopes("task-1", "session-1", []*Envelope{env1, nil, env2, nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have both keys, nil entries skipped
	if _, ok := merged.GetWorkingValue("key1"); !ok {
		t.Error("expected merged to have key1")
	}
	if _, ok := merged.GetWorkingValue("key2"); !ok {
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

func TestNilEnvelopeMethods(t *testing.T) {
	// All methods that accept *Envelope should handle nil gracefully
	var nilEnv *Envelope

	// Methods that should return without panic
	nilEnv.SetWorkingValue("key", "value", MemoryClassEphemeral)
	nilEnv.DeleteWorkingValue("key")
	nilEnv.RequestCheckpoint("reason", 1, true)
	nilEnv.ClearCheckpointRequest()
	nilEnv.AddRetrievalReference(RetrievalReference{})
	nilEnv.AddStreamedContextReference(ChunkReference{})

	// Methods that should return zero values
	if val, ok := nilEnv.GetWorkingValue("key"); ok || val != nil {
		t.Error("expected GetWorkingValue to return (nil, false) for nil envelope")
	}
	if ids := nilEnv.StreamedChunkIDs(); ids != nil {
		t.Error("expected StreamedChunkIDs to return nil for nil envelope")
	}
	if keys := nilEnv.WorkingMemoryKeys(); keys != nil {
		t.Error("expected WorkingMemoryKeys to return nil for nil envelope")
	}
	if !nilEnv.IsEmpty() {
		t.Error("expected IsEmpty to return true for nil envelope")
	}
	if snap := nilEnv.Snapshot(); snap != nil {
		t.Error("expected Snapshot to return nil for nil envelope")
	}
}
