package guidance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGuidanceBrokerRequestResolves(t *testing.T) {
	broker := newTestBroker(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var requestID string
	done := make(chan *GuidanceDecision, 1)
	go func() {
		decision, err := broker.Request(ctx, testRequest())
		require.NoError(t, err)
		done <- decision
	}()

	require.Eventually(t, func() bool {
		pending := broker.PendingRequests()
		if len(pending) != 1 {
			return false
		}
		requestID = pending[0].ID
		return requestID != ""
	}, time.Second, 5*time.Millisecond)

	require.NoError(t, broker.Resolve(GuidanceDecision{RequestID: requestID, ChoiceID: "skip"}))
	decision := <-done
	require.Equal(t, requestID, decision.RequestID)
	require.Equal(t, "skip", decision.ChoiceID)
	require.Equal(t, "user", decision.DecidedBy)
}

func TestGuidanceBrokerRequestTimeoutUseDefault(t *testing.T) {
	broker := newTestBroker(5 * time.Millisecond)
	decision, err := broker.Request(context.Background(), testRequest())
	require.NoError(t, err)
	require.Equal(t, "proceed", decision.ChoiceID)
	require.Equal(t, "timeout-default", decision.DecidedBy)
}

func TestGuidanceBrokerRequestTimeoutDefer(t *testing.T) {
	broker := newTestBroker(5 * time.Millisecond)
	dp := &DeferralPlan{ID: "dp-1", WorkflowID: "wf-1"}
	broker.SetDeferralPlan(dp)

	req := testRequest()
	req.TimeoutBehavior = GuidanceTimeoutDefer
	decision, err := broker.Request(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "proceed", decision.ChoiceID)
	require.Equal(t, "deferred", decision.DecidedBy)

	observations := dp.PendingObservations()
	require.Len(t, observations, 1)
	require.Equal(t, req.Kind, observations[0].GuidanceKind)
	require.Equal(t, req.Title, observations[0].Title)
}

func TestGuidanceBrokerRequestTimeoutFail(t *testing.T) {
	broker := newTestBroker(5 * time.Millisecond)
	req := testRequest()
	req.TimeoutBehavior = GuidanceTimeoutFail

	decision, err := broker.Request(context.Background(), req)
	require.Error(t, err)
	require.Nil(t, decision)
}

func TestGuidanceBrokerRequestContextCancelled(t *testing.T) {
	broker := newTestBroker(50 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	decision, err := broker.Request(ctx, testRequest())
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, decision)
	require.Empty(t, broker.PendingRequests())
}

func TestGuidanceBrokerConcurrentRequests(t *testing.T) {
	broker := newTestBroker(100 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	results := make(chan string, 2)
	for i, choiceID := range []string{"proceed", "skip"} {
		wg.Add(1)
		go func(i int, choiceID string) {
			defer wg.Done()
			req := testRequest()
			req.Title = req.Title + string(rune('a'+i))
			decision, err := broker.Request(ctx, req)
			require.NoError(t, err)
			results <- decision.ChoiceID
		}(i, choiceID)
	}

	require.Eventually(t, func() bool { return len(broker.PendingRequests()) == 2 }, time.Second, 5*time.Millisecond)
	pending := broker.PendingRequests()
	require.Len(t, pending, 2)
	require.NoError(t, broker.Resolve(GuidanceDecision{RequestID: pending[0].ID, ChoiceID: "proceed"}))
	require.NoError(t, broker.Resolve(GuidanceDecision{RequestID: pending[1].ID, ChoiceID: "skip"}))

	wg.Wait()
	close(results)
	choices := make([]string, 0, 2)
	for choice := range results {
		choices = append(choices, choice)
	}
	require.ElementsMatch(t, []string{"proceed", "skip"}, choices)
}

func TestGuidanceBrokerSubmitAsyncAndResolve(t *testing.T) {
	broker := newTestBroker(50 * time.Millisecond)
	events, cancel := broker.Subscribe(4)
	defer cancel()

	requestID, err := broker.SubmitAsync(testRequest())
	require.NoError(t, err)
	require.Len(t, broker.PendingRequests(), 1)

	event := <-events
	require.Equal(t, GuidanceEventRequested, event.Type)
	require.Equal(t, requestID, event.Request.ID)

	require.NoError(t, broker.Resolve(GuidanceDecision{RequestID: requestID, ChoiceID: "skip"}))
	resolved := <-events
	require.Equal(t, GuidanceEventResolved, resolved.Type)
	require.Equal(t, "skip", resolved.Decision.ChoiceID)
}

func TestGuidanceBrokerSubscribeUnsubscribeStopsDelivery(t *testing.T) {
	broker := newTestBroker(50 * time.Millisecond)
	events, cancel := broker.Subscribe(4)

	requestID, err := broker.SubmitAsync(testRequest())
	require.NoError(t, err)
	<-events
	cancel()

	require.NoError(t, broker.Resolve(GuidanceDecision{RequestID: requestID, ChoiceID: "proceed"}))
	_, ok := <-events
	require.False(t, ok)
}

func TestGuidanceBrokerEmitResolutionBroadcasts(t *testing.T) {
	broker := newTestBroker(50 * time.Millisecond)
	events, cancel := broker.Subscribe(2)
	defer cancel()

	broker.EmitResolution("obs-1", "external")

	select {
	case event := <-events:
		require.Equal(t, GuidanceEventResolved, event.Type)
		require.NotNil(t, event.Request)
		require.Equal(t, "obs-1", event.Request.ID)
		require.NotNil(t, event.Decision)
		require.Equal(t, "external", event.Decision.DecidedBy)
	case <-time.After(time.Second):
		t.Fatal("expected resolution event")
	}
}

func TestGuidanceBrokerEmitResolutionNoSubscribers(t *testing.T) {
	broker := newTestBroker(50 * time.Millisecond)
	broker.EmitResolution("obs-1", "external")
}

func TestDeferralPlanAddObservationAndIsEmpty(t *testing.T) {
	dp := &DeferralPlan{ID: "dp-1"}
	require.True(t, dp.IsEmpty())

	dp.AddObservation(EngineeringObservation{
		ID:           "obs-1",
		Source:       "step-1",
		GuidanceKind: GuidanceConfidence,
		Title:        "Low confidence",
		Description:  "Confidence dropped",
	})

	require.False(t, dp.IsEmpty())
	pending := dp.PendingObservations()
	require.Len(t, pending, 1)
	require.Equal(t, "obs-1", pending[0].ID)
}

func TestDeferralPlanResolveObservation(t *testing.T) {
	dp := &DeferralPlan{ID: "dp-1"}
	dp.AddObservation(EngineeringObservation{
		ID:           "obs-1",
		Source:       "step-1",
		GuidanceKind: GuidanceConfidence,
		Title:        "Low confidence",
		Description:  "Confidence dropped",
	})

	dp.ResolveObservation("obs-1")
	require.Empty(t, dp.PendingObservations())
}

func TestDeferralPlanConcurrentAddObservation(t *testing.T) {
	dp := &DeferralPlan{ID: "dp-1"}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			dp.AddObservation(EngineeringObservation{
				Source:       "capability",
				GuidanceKind: GuidanceAmbiguity,
				Title:        "Ambiguous request",
				Description:  "Need clarification",
				BlastRadius:  i,
			})
		}(i)
	}
	wg.Wait()
	require.Len(t, dp.PendingObservations(), 32)
}

func TestDefaultDeferralPolicy(t *testing.T) {
	policy := DefaultDeferralPolicy()
	require.NotZero(t, policy.MaxBlastRadiusForDefer)
	require.Contains(t, policy.DeferrableKinds, GuidanceAmbiguity)
	require.NotContains(t, policy.DeferrableKinds, GuidanceRecovery)
}

func newTestBroker(timeout time.Duration) *GuidanceBroker {
	broker := NewGuidanceBroker(timeout)
	return broker
}

func testRequest() GuidanceRequest {
	return GuidanceRequest{
		Kind:        GuidanceConfidence,
		Title:       "Low confidence on step",
		Description: "Confidence dropped below threshold",
		Choices: []GuidanceChoice{
			{ID: "proceed", Label: "Proceed", IsDefault: true},
			{ID: "skip", Label: "Skip"},
		},
		Context: map[string]any{"confidence": 0.2},
	}
}
