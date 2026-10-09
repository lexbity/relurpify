package knowledge

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// recordingBusTelemetry captures bus-level telemetry events.
type recordingBusTelemetry struct {
	mu     sync.Mutex
	counts map[telemetry.EventType]int
}

func newRecordingBusTelemetry() *recordingBusTelemetry {
	return &recordingBusTelemetry{counts: make(map[telemetry.EventType]int)}
}

func (r *recordingBusTelemetry) Emit(ev telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[ev.Type]++
}

func (r *recordingBusTelemetry) count(kind telemetry.EventType) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[kind]
}

// TestEventBusFastSubscriberNoDrops proves the fast path never blocks and never
// accounts a drop while a subscriber keeps up.
func TestEventBusFastSubscriberNoDrops(t *testing.T) {
	bus := &EventBus{}
	ch, unsub := bus.Subscribe(16)
	defer unsub()

	for i := 0; i < 10; i++ {
		bus.Publish(Event{Kind: EventChunkStaled, Payload: ChunkStaledPayload{Reason: "fast"}})
	}
	for i := 0; i < 10; i++ {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("event not delivered")
		}
	}
	require.Equal(t, uint64(0), bus.DroppedTotal())
}

// TestEventBusSlowSubscriberAccountsDrops proves a full buffer is waited on
// briefly and then dropped-and-counted, with bounded per-event latency.
func TestEventBusSlowSubscriberAccountsDrops(t *testing.T) {
	bus := &EventBus{}
	_, unsub := bus.Subscribe(1)
	defer unsub()

	const events = 100
	start := time.Now()
	for i := 0; i < events; i++ {
		bus.Publish(Event{Kind: EventChunkStaled, Payload: ChunkStaledPayload{Reason: "slow"}})
	}
	elapsed := time.Since(start)

	dropped := bus.DroppedTotal()
	require.GreaterOrEqual(t, dropped, uint64(events-1))
	require.LessOrEqual(t, dropped, uint64(events))
	require.LessOrEqual(t, elapsed, 12*time.Millisecond*time.Duration(events),
		"publish must bound its wait per subscriber")
}

// TestEventBusDropTelemetry proves every dropped event is surfaced as
// knowledge.event_dropped telemetry.
func TestEventBusDropTelemetry(t *testing.T) {
	tel := newRecordingBusTelemetry()
	bus := &EventBus{}
	bus.SetTelemetry(tel)
	_, unsub := bus.Subscribe(1)
	defer unsub()

	for i := 0; i < 5; i++ {
		bus.Publish(Event{Kind: EventChunkStaled})
	}
	require.Equal(t, int(bus.DroppedTotal()), tel.count(telemetry.EventEventDropped))
	require.Greater(t, tel.count(telemetry.EventEventDropped), 0)
}

// TestEventBusNilSafe proves nil-receiver calls are inert.
func TestEventBusNilSafe(t *testing.T) {
	var bus *EventBus
	bus.Publish(Event{Kind: EventChunkStaled})
	require.Equal(t, uint64(0), bus.DroppedTotal())
	bus.SetTelemetry(nil)
}
