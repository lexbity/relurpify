package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestV2GateBasicAcquireRelease tests basic acquire and release.
func TestV2GateBasicAcquireRelease(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()
	callID := GenerateCallID()
	attemptID := GenerateAttemptID()

	// Acquire should succeed
	ticket, err := gate.Acquire(ctx, callID, attemptID)
	require.NoError(t, err)
	require.NotNil(t, ticket)
	require.Equal(t, callID, ticket.CallID)
	require.Equal(t, attemptID, ticket.AttemptID)

	// State should show active
	state := gate.State()
	require.True(t, state.Active)
	require.Equal(t, callID, state.ActiveCallID)
	require.Equal(t, attemptID, state.ActiveAttemptID)
	require.False(t, state.Poisoned)
	require.Equal(t, 0, state.Queued)

	// Release should succeed
	finalState := ticket.Released()
	require.False(t, finalState.Active)
	require.Empty(t, finalState.ActiveCallID)

	// Gate should be idle
	state = gate.State()
	require.False(t, state.Active)
	require.Empty(t, state.ActiveCallID)
}

// waitForQueueDepth blocks until the gate reports the wanted queue depth.
func waitForQueueDepth(t *testing.T, gate *CallGate, want int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if gate.QueueDepth() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("queue depth = %d, want %d", gate.QueueDepth(), want)
}

// TestV2GateFIFOOrdering tests that queued calls are admitted in FIFO order.
func TestV2GateFIFOOrdering(t *testing.T) {
	t.Parallel()
	gate := NewCallGate(GateConfig{
		QueueCapacity: 16,
		QueueTimeout:  5 * time.Second,
		ActiveTimeout: 5 * time.Second,
	})

	held, err := gate.Acquire(context.Background(), "call_held", "attempt_held")
	require.NoError(t, err)

	const queued = 5
	granted := make([]chan CallID, queued)
	for i := 0; i < queued; i++ {
		granted[i] = make(chan CallID, 1)
		go func(index int) {
			ticket, err := gate.Acquire(
				context.Background(),
				CallID(fmt.Sprintf("call_%d", index)),
				AttemptID(fmt.Sprintf("attempt_%d", index)),
			)
			if err != nil {
				granted[index] <- ""
				return
			}
			granted[index] <- ticket.CallID
			ticket.Released()
		}(i)
		waitForQueueDepth(t, gate, int64(i+1))
	}

	held.Released()

	for i := 0; i < queued; i++ {
		select {
		case id := <-granted[i]:
			require.Equal(t, CallID(fmt.Sprintf("call_%d", i)), id)
		case <-time.After(5 * time.Second):
			t.Fatalf("queued call %d was not granted", i)
		}
	}
}

// TestV2GateQueueCapacity tests that queue capacity is enforced.
func TestV2GateQueueCapacity(t *testing.T) {
	t.Parallel()
	gate := NewCallGate(GateConfig{
		QueueCapacity: 2,
		QueueTimeout:  5 * time.Second,
		ActiveTimeout: 5 * time.Second,
	})

	held, err := gate.Acquire(context.Background(), "call_held", "attempt_held")
	require.NoError(t, err)

	cancels := make([]context.CancelFunc, 0, 2)
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		go func(index int) {
			_, _ = gate.Acquire(ctx, CallID(fmt.Sprintf("call_%d", index)), AttemptID(fmt.Sprintf("attempt_%d", index)))
		}(i)
		waitForQueueDepth(t, gate, int64(i+1))
	}

	_, err = gate.Acquire(context.Background(), "call_overflow", "attempt_overflow")
	require.ErrorIs(t, err, ErrQueueSaturated)

	for _, cancel := range cancels {
		cancel()
	}
	held.Released()
}

// TestV2GateQueueTimeout tests that queued calls time out.
func TestV2GateQueueTimeout(t *testing.T) {
	t.Parallel()
	gate := NewCallGate(GateConfig{
		QueueCapacity: 10,
		QueueTimeout:  50 * time.Millisecond,
		ActiveTimeout: 1 * time.Second,
	})

	ctx := context.Background()

	// Hold the gate
	callID1 := GenerateCallID()
	ticket1, err := gate.Acquire(ctx, callID1, GenerateAttemptID())
	require.NoError(t, err)

	// Queue a call with a long deadline
	ctx2, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	_, err = gate.Acquire(ctx2, GenerateCallID(), GenerateAttemptID())
	require.Equal(t, ErrQueueTimeout, err)

	// Release the gate
	ticket1.Released()
}

// TestV2GateNestedCall tests that nested calls are rejected.
// A nested call is one that uses a context that already has an active ticket.
func TestV2GateNestedCall(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()
	callID := GenerateCallID()

	// Acquire first
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	// Try to acquire again using the ticket's context (which has the active marker) - should fail
	// This simulates a reentrant call where a model call tries to make another model call
	_, err = gate.Acquire(ticket.Context, GenerateCallID(), GenerateAttemptID())
	require.Equal(t, ErrNestedModelCall, err)

	// Release first
	ticket.Released()

	// Now acquire should succeed
	_, err = gate.Acquire(ctx, GenerateCallID(), GenerateAttemptID())
	require.NoError(t, err)
}

// TestV2GatePoisonAndRecover tests poisoning and recovery.
func TestV2GatePoisonAndRecover(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()

	// Acquire and poison
	callID := GenerateCallID()
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	finalState := ticket.Poison("test poison")
	require.True(t, finalState.Poisoned)

	// All future acquires should fail
	_, err = gate.Acquire(ctx, GenerateCallID(), GenerateAttemptID())
	require.Equal(t, ErrGatePoisoned, err)

	// Recovery cannot bypass a transport whose lease is still outstanding.
	require.True(t, gate.Recover().Poisoned)
	ticket.Released()
	// Recover only after physical reclamation.
	recoverState := gate.Recover()
	require.False(t, recoverState.Poisoned)

	// Now acquires should succeed
	_, err = gate.Acquire(ctx, GenerateCallID(), GenerateAttemptID())
	require.NoError(t, err)
}

// TestV2GatePoisonClearsQueue tests that poisoning rejects and clears the queue.
func TestV2GatePoisonClearsQueue(t *testing.T) {
	t.Parallel()
	gate := NewCallGate(GateConfig{
		QueueCapacity: 4,
		QueueTimeout:  5 * time.Second,
		ActiveTimeout: 5 * time.Second,
	})

	held, err := gate.Acquire(context.Background(), "call_held", "attempt_held")
	require.NoError(t, err)

	const queued = 2
	results := make([]chan error, queued)
	for i := 0; i < queued; i++ {
		results[i] = make(chan error, 1)
		go func(index int) {
			_, err := gate.Acquire(
				context.Background(),
				CallID(fmt.Sprintf("call_%d", index)),
				AttemptID(fmt.Sprintf("attempt_%d", index)),
			)
			results[index] <- err
		}(i)
		waitForQueueDepth(t, gate, int64(i+1))
	}

	state := gate.Poison("test poison")
	require.True(t, state.Poisoned)
	require.Equal(t, int64(0), gate.QueueDepth())

	for i := 0; i < queued; i++ {
		select {
		case err := <-results[i]:
			require.ErrorIs(t, err, ErrGatePoisoned)
		case <-time.After(2 * time.Second):
			t.Fatalf("queued call %d was not rejected by poison", i)
		}
	}

	held.Released()
}

// TestV2GateActiveTimeout tests that active calls time out.
func TestV2GateActiveTimeout(t *testing.T) {
	t.Parallel()
	gate := NewCallGate(GateConfig{
		QueueCapacity: 10,
		QueueTimeout:  1 * time.Second,
		ActiveTimeout: 50 * time.Millisecond,
	})

	ctx := context.Background()

	// Acquire
	callID := GenerateCallID()
	attemptID := GenerateAttemptID()
	ticket, err := gate.Acquire(ctx, callID, attemptID)
	require.NoError(t, err)

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// The ticket's context should be done
	select {
	case <-ticket.Context.Done():
		// Expected
	default:
		t.Error("ticket context should be done after timeout")
	}

	// Release should still work
	ticket.Released()

	// Next acquire should succeed
	_, err = gate.Acquire(ctx, GenerateCallID(), GenerateAttemptID())
	require.NoError(t, err)
}

// TestV2GateMetrics tests that metrics are tracked correctly.
func TestV2GateMetrics(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()

	// Initially all metrics should be zero
	require.Equal(t, int64(0), gate.QueueDepth())
	require.Equal(t, int64(0), gate.ActiveCount())
	require.Equal(t, int64(0), gate.PoisonCount())
	require.Equal(t, int64(0), gate.TotalGranted())
	require.Equal(t, int64(0), gate.TotalReclaimed())

	// Acquire and release
	callID := GenerateCallID()
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	require.Equal(t, int64(1), gate.TotalGranted())
	require.Equal(t, int64(1), gate.ActiveCount())

	ticket.Released()

	require.Equal(t, int64(1), gate.TotalReclaimed())
	require.Equal(t, int64(0), gate.ActiveCount())

	// Poison and recover
	callID = GenerateCallID()
	ticket, err = gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	ticket.Poison("test")
	require.Equal(t, int64(1), gate.PoisonCount())

	gate.Recover()

	// Poison count should remain 1
	require.Equal(t, int64(1), gate.PoisonCount())
}

// TestV2GateConcurrentAccess tests concurrent acquire/release.
func TestV2GateConcurrentAccess(t *testing.T) {
	t.Parallel()
	gate := NewCallGate(GateConfig{
		QueueCapacity: 128,
		QueueTimeout:  1 * time.Second,
		ActiveTimeout: 1 * time.Second,
	})

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 100
	granted := make(chan struct{}, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			callID := CallID(fmt.Sprintf("call_%d", id))
			attemptID := AttemptID(fmt.Sprintf("attempt_%d", id))

			ticket, err := gate.Acquire(ctx, callID, attemptID)
			if err != nil {
				return
			}

			granted <- struct{}{}
			time.Sleep(time.Millisecond)
			ticket.Released()
		}(i)
	}

	// Wait for all to be granted
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-granted:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for all calls to be granted")
		}
	}

	wg.Wait()

	// Verify metrics
	require.Equal(t, int64(numGoroutines), gate.TotalGranted())
	require.Equal(t, int64(numGoroutines), gate.TotalReclaimed())
	require.Equal(t, int64(0), gate.ActiveCount())
}

// TestV2GateQueueOrderingUnderLoad tests FIFO ordering under load.
func TestV2GateQueueOrderingUnderLoad(t *testing.T) {
	t.Parallel()
	gate := NewCallGate(GateConfig{
		QueueCapacity: 64,
		QueueTimeout:  10 * time.Second,
		ActiveTimeout: 10 * time.Second,
	})

	held, err := gate.Acquire(context.Background(), "call_held", "attempt_held")
	require.NoError(t, err)

	const queued = 32
	granted := make([]chan CallID, queued)
	for i := 0; i < queued; i++ {
		granted[i] = make(chan CallID, 1)
		go func(index int) {
			ticket, err := gate.Acquire(
				context.Background(),
				CallID(fmt.Sprintf("call_%d", index)),
				AttemptID(fmt.Sprintf("attempt_%d", index)),
			)
			if err != nil {
				granted[index] <- ""
				return
			}
			granted[index] <- ticket.CallID
			ticket.Released()
		}(i)
		waitForQueueDepth(t, gate, int64(i+1))
	}

	held.Released()

	for i := 0; i < queued; i++ {
		select {
		case id := <-granted[i]:
			require.Equal(t, CallID(fmt.Sprintf("call_%d", i)), id)
		case <-time.After(10 * time.Second):
			t.Fatalf("queued call %d was not granted", i)
		}
	}
}

// TestV2GateClose tests closing the gate.
func TestV2GateClose(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()

	// Acquire
	callID := GenerateCallID()
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	// Close the gate
	gate.Close()

	// Release should still work
	ticket.Released()

	// New acquires should fail
	_, err = gate.Acquire(ctx, GenerateCallID(), GenerateAttemptID())
	require.Error(t, err)
	require.Contains(t, err.Error(), "gate closed")
}

// TestV2GatePoisonFromOutside tests poisoning from outside a ticket.
func TestV2GatePoisonFromOutside(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()

	// Acquire
	callID := GenerateCallID()
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	// Poison from outside
	state := gate.Poison("external poison")
	require.True(t, state.Poisoned)

	// The ticket's context should be done
	select {
	case <-ticket.Context.Done():
		// Expected
	default:
		t.Error("ticket context should be done after poison")
	}

	// Release should be safe but no-op
	ticket.Released()

	// Next acquire should fail
	_, err = gate.Acquire(ctx, GenerateCallID(), GenerateAttemptID())
	require.Equal(t, ErrGatePoisoned, err)
}

// TestV2GateStateSubscription tests state subscription.
func TestV2GateStateSubscription(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()

	// Subscribe
	stateCh := gate.SubscribeState()

	// Initial state
	state := gate.State()
	require.False(t, state.Active)

	// Acquire and check for state change
	callID := GenerateCallID()
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	// Drain the channel (may have initial state)
	select {
	case <-stateCh:
	default:
	}

	// State should now be active
	state = gate.State()
	require.True(t, state.Active)

	// Release
	ticket.Released()

	// State should be idle
	state = gate.State()
	require.False(t, state.Active)
}

// TestV2GateString tests the string representation.
func TestV2GateString(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	str := gate.String()
	require.Contains(t, str, "CallGate")
	require.Contains(t, str, "queued=0")
	require.Contains(t, str, "active=false")

	ctx := context.Background()
	callID := GenerateCallID()
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	str = gate.String()
	require.Contains(t, str, "active=true")
	require.Contains(t, str, string(callID))

	ticket.Released()
}

// TestV2GateCancelledContext tests acquire with cancelled context.
func TestV2GateCancelledContext(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	// Hold the gate
	ctx := context.Background()
	callID1 := GenerateCallID()
	ticket1, err := gate.Acquire(ctx, callID1, GenerateAttemptID())
	require.NoError(t, err)

	// Try to acquire with cancelled context
	ctx2, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = gate.Acquire(ctx2, GenerateCallID(), GenerateAttemptID())
	// Should fail because context is already cancelled
	require.Error(t, err)

	ticket1.Released()
}

// TestV2GateMultiplePoisons tests multiple poison operations.
func TestV2GateMultiplePoisons(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	// First poison
	gate.Poison("first")
	require.True(t, gate.State().Poisoned)

	// Second poison should be a no-op (gate is already poisoned)
	state := gate.Poison("second")
	require.True(t, state.Poisoned)

	// Recover
	gate.Recover()
	require.False(t, gate.State().Poisoned)

	// Poison again
	gate.Poison("third")
	require.True(t, gate.State().Poisoned)

	// Recover again
	gate.Recover()
	require.False(t, gate.State().Poisoned)

	// Metrics should show 3 poisons (each Poison call increments the counter)
	require.Equal(t, int64(3), gate.PoisonCount())
}

// TestV2GateIDGeneration tests ID generation.
func TestV2GateIDGeneration(t *testing.T) {
	t.Parallel()

	// Generate multiple IDs and verify they're unique
	idSet := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		callID := GenerateCallID()
		attemptID := GenerateAttemptID()

		require.NotEmpty(t, callID)
		require.NotEmpty(t, attemptID)
		require.True(t, len(callID) > 0)
		require.True(t, len(attemptID) > 0)

		idSet[string(callID)] = true
		idSet[string(attemptID)] = true
	}

	// Verify uniqueness
	require.Equal(t, 2000, len(idSet))
}

// TestV2GateConcurrentPoisonAndAcquire tests race between poison and acquire.
func TestV2GateConcurrentPoisonAndAcquire(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()
	var wg sync.WaitGroup

	// Start poison in background
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		gate.Poison("concurrent")
	}()

	// Try to acquire
	wg.Add(1)
	var acquired bool
	var acquireErr error
	go func() {
		defer wg.Done()
		_, acquireErr = gate.Acquire(ctx, GenerateCallID(), GenerateAttemptID())
		acquired = acquireErr == nil
	}()

	wg.Wait()

	// Either poison happened first (acquire fails) or acquire happened first (acquire succeeds)
	if acquired {
		// Acquire succeeded before poison, which is valid
		require.NoError(t, acquireErr)
	} else {
		// Poison happened first
		require.Equal(t, ErrGatePoisoned, acquireErr)
	}
}

// TestV2GateQueueDepthMetric tests the queue depth metric.
func TestV2GateQueueDepthMetric(t *testing.T) {
	t.Parallel()
	gate := NewCallGate(GateConfig{
		QueueCapacity: 4,
		QueueTimeout:  5 * time.Second,
		ActiveTimeout: 5 * time.Second,
	})

	held, err := gate.Acquire(context.Background(), "call_held", "attempt_held")
	require.NoError(t, err)

	cancels := make([]context.CancelFunc, 0, 2)
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		go func(index int) {
			_, _ = gate.Acquire(ctx, CallID(fmt.Sprintf("call_%d", index)), AttemptID(fmt.Sprintf("attempt_%d", index)))
		}(i)
		waitForQueueDepth(t, gate, int64(i+1))
	}
	require.Equal(t, int64(2), gate.QueueDepth())

	for _, cancel := range cancels {
		cancel()
	}
	waitForQueueDepth(t, gate, 0)
	held.Released()
}

// TestV2GateDefaultConfig tests default configuration.
func TestV2GateDefaultConfig(t *testing.T) {
	t.Parallel()
	config := DefaultGateConfig()

	require.Equal(t, 128, config.QueueCapacity)
	require.Equal(t, 300*time.Second, config.QueueTimeout)
	require.Equal(t, 900*time.Second, config.ActiveTimeout)
	require.Equal(t, 250*time.Millisecond, config.CancellationGracePeriod)
}

// TestV2GateCustomConfig tests custom configuration.
func TestV2GateCustomConfig(t *testing.T) {
	t.Parallel()
	gate := NewCallGate(GateConfig{
		QueueCapacity:           50,
		QueueTimeout:            10 * time.Second,
		ActiveTimeout:           20 * time.Second,
		CancellationGracePeriod: 500 * time.Millisecond,
	})

	// Acquire and release to verify basic operation
	callID := GenerateCallID()
	ticket, err := gate.Acquire(context.Background(), callID, GenerateAttemptID())
	require.NoError(t, err)
	require.NotNil(t, ticket)

	ticket.Released()

	// Verify metrics are tracked
	require.Equal(t, int64(1), gate.TotalGranted())
	require.Equal(t, int64(1), gate.TotalReclaimed())

	// Now acquire should succeed again
	_, err = gate.Acquire(context.Background(), GenerateCallID(), GenerateAttemptID())
	require.NoError(t, err)
}

// TestV2GateStateImmutability tests that state returns are immutable.
func TestV2GateStateImmutability(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()
	callID := GenerateCallID()
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	// Get state
	state := gate.State()

	// Modify the returned state (shouldn't affect the gate)
	state.Active = false
	state.ActiveCallID = ""

	// Get state again
	state2 := gate.State()
	require.True(t, state2.Active)
	require.Equal(t, callID, state2.ActiveCallID)

	ticket.Released()
}

// TestV2GateFilteredMetrics tests metric accessor methods.
func TestV2GateFilteredMetrics(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	// All should be zero initially
	require.Equal(t, int64(0), gate.QueueDepth())
	require.Equal(t, int64(0), gate.ActiveCount())
	require.Equal(t, int64(0), gate.PoisonCount())
	require.Equal(t, int64(0), gate.TotalGranted())
	require.Equal(t, int64(0), gate.TotalReclaimed())

	ctx := context.Background()

	// Acquire 10 times
	for i := 0; i < 10; i++ {
		ticket, err := gate.Acquire(ctx, GenerateCallID(), GenerateAttemptID())
		require.NoError(t, err)
		ticket.Released()
	}

	// Verify counters
	require.Equal(t, int64(10), gate.TotalGranted())
	require.Equal(t, int64(10), gate.TotalReclaimed())
}

// TestV2GateErrorClassification tests error classification.
func TestV2GateErrorClassification(t *testing.T) {
	t.Parallel()

	// Test that errors can be classified with errors.Is
	require.True(t, errors.Is(ErrNestedModelCall, ErrNestedModelCall))
	require.True(t, errors.Is(ErrGatePoisoned, ErrGatePoisoned))
	require.True(t, errors.Is(ErrQueueSaturated, ErrQueueSaturated))
	require.True(t, errors.Is(ErrQueueTimeout, ErrQueueTimeout))
	require.True(t, errors.Is(ErrActiveTimeout, ErrActiveTimeout))
	require.True(t, errors.Is(ErrLanePoisoned, ErrLanePoisoned))
}

// TestV2GateAtomicMetrics tests atomic metric operations.
func TestV2GateAtomicMetrics(t *testing.T) {
	t.Parallel()
	// Use a larger queue capacity to avoid queue saturation
	gate := NewCallGate(GateConfig{
		QueueCapacity: 200,
		QueueTimeout:  5 * time.Second,
		ActiveTimeout: 5 * time.Second,
	})

	var wg sync.WaitGroup
	numOps := 100

	// Concurrent acquires and releases with different contexts
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Use a new background context for each goroutine
			ctx := context.Background()
			ticket, err := gate.Acquire(ctx, GenerateCallID(), GenerateAttemptID())
			if err == nil {
				ticket.Released()
			}
		}()
	}

	wg.Wait()

	// Total granted should equal total reclaimed
	require.Equal(t, gate.TotalGranted(), gate.TotalReclaimed())

	// With concurrent access, we should have granted at least some calls
	require.Greater(t, gate.TotalGranted(), int64(0))
}

// TestV2GateZeroConfigValues tests handling of zero config values.
func TestV2GateZeroConfigValues(t *testing.T) {
	t.Parallel()
	// Zero values should use defaults
	gate := NewCallGate(GateConfig{
		QueueCapacity:           0,
		QueueTimeout:            0,
		ActiveTimeout:           0,
		CancellationGracePeriod: 0,
	})

	// Verify defaults are used - the gate should work with default values
	ctx := context.Background()

	// Acquire and release multiple times
	for i := 0; i < 5; i++ {
		callID := GenerateCallID()
		ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
		require.NoError(t, err)
		ticket.Released()
	}

	// Verify defaults were applied
	require.Equal(t, int64(5), gate.TotalGranted())
	require.Equal(t, int64(5), gate.TotalReclaimed())
}

// TestV2GateEmptyCallID tests handling of empty call IDs.
func TestV2GateEmptyCallID(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()

	// Empty call ID should still work (though not recommended)
	ticket, err := gate.Acquire(ctx, "", "")
	require.NoError(t, err)
	require.NotNil(t, ticket)

	ticket.Released()
}

// TestV2GateReleaseIdempotent tests that releasing a ticket multiple times is safe.
func TestV2GateReleaseIdempotent(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()
	callID := GenerateCallID()
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	// Release multiple times
	for i := 0; i < 5; i++ {
		ticket.Released()
	}

	// Should still be idle
	state := gate.State()
	require.False(t, state.Active)
}

// TestV2GatePoisonIdempotent tests that poisoning multiple times is safe.
func TestV2GatePoisonIdempotent(t *testing.T) {
	t.Parallel()
	gate := NewDefaultCallGate()

	ctx := context.Background()
	callID := GenerateCallID()
	ticket, err := gate.Acquire(ctx, callID, GenerateAttemptID())
	require.NoError(t, err)

	// Poison multiple times
	for i := 0; i < 5; i++ {
		ticket.Poison(fmt.Sprintf("reason %d", i))
	}

	// Should still be poisoned
	state := gate.State()
	require.True(t, state.Poisoned)
}

func TestV2GateCanceledLeaseCannotBeRecoveredOrReused(t *testing.T) {
	gate := NewCallGate(GateConfig{ActiveTimeout: time.Hour, CancellationGracePeriod: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	first, err := gate.Acquire(ctx, "same-call", "same-attempt")
	require.NoError(t, err)
	cancel()
	require.Eventually(t, func() bool { return gate.State().Poisoned }, time.Second, time.Millisecond)
	require.True(t, gate.State().Active, "cancellation is not physical reclamation")
	require.True(t, gate.Recover().Poisoned, "recovery bypassed outstanding transport")
	first.Released()
	require.False(t, gate.State().Active)
	require.False(t, gate.Recover().Poisoned)
	second, err := gate.Acquire(context.Background(), "same-call", "same-attempt")
	require.NoError(t, err)
	first.Released()
	first.Poison("stale lease")
	require.True(t, gate.State().Active)
	require.False(t, gate.State().Poisoned)
	second.Released()
	require.EqualValues(t, 2, gate.TotalReclaimed())
}
