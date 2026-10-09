package knowledge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestKnowledgeHealthLatchesDegraded proves the health aggregate latches the
// invalidation-degraded and store-degraded signals off the composition bus.
func TestKnowledgeHealthLatchesDegraded(t *testing.T) {
	bus := &EventBus{}
	health := NewKnowledgeHealth(bus)
	t.Cleanup(health.Close)

	degraded, reason := health.Degraded()
	require.False(t, degraded)
	require.Empty(t, reason)

	bus.EmitInvalidationDegraded(InvalidationDegradedPayload{FailureCount: 5, Error: "store unavailable"})
	require.Eventually(t, func() bool {
		ok, _ := health.Degraded()
		return ok
	}, 2*time.Second, time.Millisecond)
	_, reason = health.Degraded()
	require.Contains(t, reason, "invalidation degraded")

	bus.EmitStoreDegraded(StoreDegradedPayload{Error: "engine wedged"})
	require.Eventually(t, func() bool {
		_, r := health.Degraded()
		return r == "store degraded: engine wedged"
	}, 2*time.Second, time.Millisecond)
}

// TestKnowledgeHealthNilBusIsHealthy proves a nil-bus aggregator is valid and
// never reports degraded.
func TestKnowledgeHealthNilBusIsHealthy(t *testing.T) {
	var health *KnowledgeHealth
	degraded, reason := health.Degraded()
	require.False(t, degraded)
	require.Empty(t, reason)
	health.Close() // nil-safe

	health = NewKnowledgeHealth(nil)
	t.Cleanup(health.Close)
	degraded, reason = health.Degraded()
	require.False(t, degraded)
	require.Empty(t, reason)
}

// TestAssertSameBus proves the composition-root one-bus contract.
func TestAssertSameBus(t *testing.T) {
	bus := &EventBus{}
	require.NoError(t, AssertSameBus(bus, bus))
	require.NoError(t, AssertSameBus(nil, nil))
	require.Error(t, AssertSameBus(bus, &EventBus{}))
	require.Error(t, AssertSameBus(bus, nil))
}
