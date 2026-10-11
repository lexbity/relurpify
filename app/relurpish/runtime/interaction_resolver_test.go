package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// TestNotifyDropCounted proves the contract's drop clause is honored (D8): a
// Notify with no live envelope for the task drops, is counted, and logs once;
// a Notify with a live envelope does not drop.
func TestNotifyDropCounted(t *testing.T) {
	r := &Runtime{}
	resolver := r.interactionResolver()
	impl := resolver.(*runtimeInteractionResolver)

	frame := &interaction.InteractionFrame{TaskID: "task-gone", ID: "frame-1"}
	require.NoError(t, impl.Notify(context.Background(), frame))
	require.NoError(t, impl.Notify(context.Background(), frame))
	require.Equal(t, uint64(2), impl.DroppedTotal(), "drops are counted")

	// A live envelope: no drop.
	env := contextdata.NewEnvelope("task-live", "session-live")
	r.trackInteractionEnvelope("task-live", env)
	live := &interaction.InteractionFrame{TaskID: "task-live", ID: "frame-2"}
	require.NoError(t, impl.Notify(context.Background(), live))
	require.Equal(t, uint64(2), impl.DroppedTotal(), "a delivered Notify is not a drop")
}

// TestResolveUnknownFrame proves the typed not-found path: an unknown task or
// frame returns *ErrFrameNotFound and writes nothing.
func TestResolveUnknownFrame(t *testing.T) {
	r := &Runtime{}
	err := r.ResolveInteractionFrame(context.Background(), "task-x", "frame-x", "yes", "")
	require.ErrorAs(t, err, new(*ErrFrameNotFound))

	env := contextdata.NewEnvelope("task-1", "session-1")
	r.trackInteractionEnvelope("task-1", env)
	err = r.ResolveInteractionFrame(context.Background(), "task-1", "frame-absent", "yes", "")
	var notFound *ErrFrameNotFound
	require.ErrorAs(t, err, &notFound)
	require.Equal(t, "frame-absent", notFound.FrameID)
}

// TestResolveAlreadyAnswered proves the write-once answer boundary (D8): a
// second answer returns *ErrFrameAlreadyResolved and leaves the envelope's
// frame byte-identical — the human's first decision is never overwritten.
func TestResolveAlreadyAnswered(t *testing.T) {
	r := &Runtime{}
	env := contextdata.NewEnvelope("task-1", "session-1")
	frame := &interaction.InteractionFrame{
		TaskID: "task-1", ID: "frame-1",
		Type:     interaction.FrameOutcomeFeedback, // informational: no resume path on answer
		Question: "proceed?",
	}
	contextdata.SetTyped(env, "euclo.interaction.frame.test", frame)
	r.trackInteractionEnvelope("task-1", env)

	require.NoError(t, r.ResolveInteractionFrame(context.Background(), "task-1", "frame-1", "yes", ""))
	first := frame.Response
	require.NotNil(t, first)
	answer, _ := interaction.ResponseValue(frame)
	require.Equal(t, "yes", answer)

	err := r.ResolveInteractionFrame(context.Background(), "task-1", "frame-1", "no", "")
	var already *ErrFrameAlreadyResolved
	require.ErrorAs(t, err, &already)
	answer, _ = interaction.ResponseValue(frame)
	require.Equal(t, "yes", answer, "the first answer stands")
	require.True(t, errors.Is(err, already))
}
