package learning

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBrokerRequestResolves(t *testing.T) {
	broker := NewBroker(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan *Interaction, 1)
	go func() {
		resolved, err := broker.Request(ctx, Interaction{
			ID:            "learn-1",
			WorkflowID:    "wf-1",
			Title:         "Confirm pattern",
			Status:        StatusPending,
			DefaultChoice: string(ResolutionConfirm),
		})
		require.NoError(t, err)
		done <- resolved
	}()

	require.Eventually(t, func() bool {
		return len(broker.PendingInteractions()) == 1
	}, time.Second, 5*time.Millisecond)
	require.NoError(t, broker.Resolve(Interaction{
		ID:         "learn-1",
		Status:     StatusResolved,
		Resolution: &Resolution{Kind: ResolutionConfirm, ChoiceID: "confirm"},
	}))
	resolved := <-done
	require.Equal(t, StatusResolved, resolved.Status)
	require.NotNil(t, resolved.Resolution)
	require.Equal(t, ResolutionConfirm, resolved.Resolution.Kind)
}

func TestBrokerTimeoutUseDefault(t *testing.T) {
	broker := NewBroker(5 * time.Millisecond)
	resolved, err := broker.Request(context.Background(), Interaction{
		ID:              "learn-2",
		WorkflowID:      "wf-1",
		Title:           "Confirm pattern",
		Status:          StatusPending,
		DefaultChoice:   string(ResolutionConfirm),
		TimeoutBehavior: TimeoutUseDefault,
	})
	require.NoError(t, err)
	require.Equal(t, StatusResolved, resolved.Status)
	require.NotNil(t, resolved.Resolution)
	require.Equal(t, "timeout-default", resolved.Resolution.ResolvedBy)
}

func TestBrokerTimeoutDefer(t *testing.T) {
	broker := NewBroker(5 * time.Millisecond)
	resolved, err := broker.Request(context.Background(), Interaction{
		ID:              "learn-3",
		WorkflowID:      "wf-1",
		Title:           "Confirm pattern",
		Status:          StatusPending,
		DefaultChoice:   string(ResolutionConfirm),
		TimeoutBehavior: TimeoutDefer,
	})
	require.NoError(t, err)
	require.Equal(t, StatusDeferred, resolved.Status)
	require.NotNil(t, resolved.Resolution)
	require.Equal(t, ResolutionDefer, resolved.Resolution.Kind)
}

func TestBrokerSubscribeAsyncFlow(t *testing.T) {
	broker := NewBroker(50 * time.Millisecond)
	events, cancel := broker.Subscribe(4)
	defer cancel()

	require.NoError(t, broker.SubmitAsync(Interaction{
		ID:         "learn-4",
		WorkflowID: "wf-1",
		Title:      "Review anchor drift",
		Status:     StatusPending,
	}))
	requested := <-events
	require.Equal(t, EventRequested, requested.Type)
	require.Equal(t, "learn-4", requested.Interaction.ID)

	require.NoError(t, broker.Resolve(Interaction{
		ID:         "learn-4",
		WorkflowID: "wf-1",
		Title:      "Review anchor drift",
		Status:     StatusDeferred,
		Resolution: &Resolution{Kind: ResolutionDefer},
	}))
	deferred := <-events
	require.Equal(t, EventDeferred, deferred.Type)
	require.Equal(t, StatusDeferred, deferred.Interaction.Status)
}
