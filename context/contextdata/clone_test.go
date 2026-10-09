package contextdata

import (
	"reflect"
	"testing"
)

// TestCloneEnvelopeMatchesEnvelopeClone pins the consolidation promised by
// Phase 1: the free function is a nil-safe delegate to Envelope.Clone, so both
// produce identical snapshots for the same source.
func TestCloneEnvelopeMatchesEnvelopeClone(t *testing.T) {
	env := NewEnvelope(testTaskID, testSessionID)
	env.NodeID = testNodeID
	env.SetWorkingValueWithClass(testKey1, testValue1, MemoryClassTask)
	env.SetWorkingValueWithClass("key2", 42, MemoryClassSession)
	env.AddStreamedContextReference(ChunkReference{ChunkID: testChunkID1, Source: testSource, Rank: 1})
	env.AddRetrievalReference(RetrievalReference{QueryID: testQueryID, ChunkIDs: []ChunkID{testChunkID1}})
	env.AddCheckpointReference(CheckpointReference{
		CheckpointID:      testCheckpointID,
		RequestedBy:       testNodeID,
		WorkingMemoryKeys: []string{testKey1},
	})
	meta := env.AssemblyMetadataSnapshot()
	meta.EventLogSeq = 7
	meta.CompilationID = "compile-1"
	env.SetAssemblyMetadata(meta)

	fromFreeFunction := CloneEnvelope(env)
	fromMethod := env.Clone()

	assertSameEnvelopeSnapshot(t, fromFreeFunction, fromMethod)
}

func TestCloneEnvelopeNilReturnsNil(t *testing.T) {
	if got := CloneEnvelope(nil); got != nil {
		t.Fatalf("CloneEnvelope(nil) = %v, want nil", got)
	}
}

func TestCloneEnvelopeDoesNotInheritCheckpointRequest(t *testing.T) {
	env := NewEnvelope(testTaskID, testSessionID)
	env.NodeID = testNodeID
	env.RequestCheckpoint("checkpoint for recovery", 5, true)

	clone := CloneEnvelope(env)
	if clone == nil {
		t.Fatal("expected clone to be created")
	}
	if clone.CheckpointRequest != nil {
		t.Fatalf("clone inherited checkpoint request: %#v", clone.CheckpointRequest)
	}
	if clone.NodeID != testNodeID {
		t.Fatalf("clone lost NodeID: %q", clone.NodeID)
	}
	if env.CheckpointRequest == nil {
		t.Fatal("source checkpoint request was consumed")
	}
}

func assertSameEnvelopeSnapshot(t *testing.T, a, b *Envelope) {
	t.Helper()
	if a.TaskID != b.TaskID || a.SessionID != b.SessionID || a.NodeID != b.NodeID {
		t.Fatalf("identity mismatch: %q/%q/%q vs %q/%q/%q",
			a.TaskID, a.SessionID, a.NodeID, b.TaskID, b.SessionID, b.NodeID)
	}
	if !reflect.DeepEqual(a.WorkingDataSnapshot(), b.WorkingDataSnapshot()) {
		t.Fatalf("working data mismatch: %#v vs %#v", a.WorkingDataSnapshot(), b.WorkingDataSnapshot())
	}
	if !reflect.DeepEqual(a.ReferencesSnapshot(), b.ReferencesSnapshot()) {
		t.Fatalf("references mismatch: %#v vs %#v", a.ReferencesSnapshot(), b.ReferencesSnapshot())
	}
	if !reflect.DeepEqual(a.AssemblyMetadataSnapshot(), b.AssemblyMetadataSnapshot()) {
		t.Fatalf("assembly metadata mismatch: %#v vs %#v", a.AssemblyMetadataSnapshot(), b.AssemblyMetadataSnapshot())
	}
	if a.CheckpointRequest != nil || b.CheckpointRequest != nil {
		t.Fatalf("expected no checkpoint request on either clone: %#v vs %#v", a.CheckpointRequest, b.CheckpointRequest)
	}
}
