package agentgraph

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/persistence"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/identity"
	"codeburg.org/lexbit/relurpify/telemetry"
	"github.com/stretchr/testify/require"
)

// TestCheckpointNodeMirrorFailureIsHonest proves a failed checkpoint mirror is
// surfaced as checkpoint.mirror_failed telemetry and checkpoint_mirror_failed
// result metadata instead of a swallowed error (the Phase 7 honesty fix): the
// primary checkpoint is still created, but the mirror failure is not silent.
func TestCheckpointNodeMirrorFailureIsHonest(t *testing.T) {
	repo := &checkpointRepoStub{}
	tel := &recordingGraphTelemetry{}
	node := NewCheckpointNode("checkpoint-mirror-fail").
		WithRepository(repo).
		WithWriter(persistence.NewWriter(&knowledge.ChunkStore{Graph: nil}, nil, nil, nil)).
		WithPrincipalResolver(func(*contextdata.Envelope) (identity.SubjectRef, bool) {
			return identity.SubjectRef{ID: "principal-1"}, true
		}).
		WithTelemetry(tel)

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.RequestCheckpoint("materialize", 7, true)
	env.SetWorkingValueWithClass("contextstream.result", &contextstream.Result{
		Request: contextstream.Request{ID: "stream-1", Mode: contextstream.ModeBlocking},
		Trim:    contextstream.TrimMetadata{ShortfallTokens: 3},
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	created, _ := execution.ResultField(result.Data, "checkpoint_created")
	require.Equal(t, true, created, "primary checkpoint must still be recorded as created")
	failed, ok := execution.ResultField(result.Data, "checkpoint_mirror_failed")
	require.True(t, ok, "mirror failure must be surfaced in the result")
	require.NotEmpty(t, failed)

	require.Equal(t, 1, tel.count(telemetry.EventCheckpointMirrorFailed),
		"checkpoint.mirror_failed must reach the telemetry trail")
}

// TestCheckpointNodeEmitMirrorFailedNilsafe proves the honesty emitter is safe
// with a nil receiver and a nil sink.
func TestCheckpointNodeEmitMirrorFailedNilsafe(t *testing.T) {
	var n *CheckpointNode
	n.emitMirrorFailed(context.Background(), "task-1", context.DeadlineExceeded)                          // nil receiver, must not panic
	(&CheckpointNode{id: "x"}).emitMirrorFailed(context.Background(), "task-1", context.DeadlineExceeded) // nil sink, must not panic
}
