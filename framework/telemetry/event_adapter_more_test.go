package telemetry

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
	"github.com/stretchr/testify/require"
)

type capturedEventLog struct {
	events []core.FrameworkEvent
}

func (c *capturedEventLog) Append(_ context.Context, _ string, events []core.FrameworkEvent) ([]uint64, error) {
	c.events = append(c.events, events...)
	seqs := make([]uint64, len(events))
	for i := range seqs {
		seqs[i] = uint64(i + 1)
	}
	return seqs, nil
}
func (c *capturedEventLog) Read(context.Context, string, uint64, int, bool) ([]core.FrameworkEvent, error) {
	return nil, nil
}
func (c *capturedEventLog) ReadByType(context.Context, string, string, uint64, int) ([]core.FrameworkEvent, error) {
	return nil, nil
}
func (c *capturedEventLog) LastSeq(context.Context, string) (uint64, error) { return 0, nil }
func (c *capturedEventLog) TakeSnapshot(context.Context, string, uint64, []byte) error {
	return nil
}
func (c *capturedEventLog) LoadSnapshot(context.Context, string) (uint64, []byte, error) {
	return 0, nil, nil
}
func (c *capturedEventLog) Close() error { return nil }

var _ event.Log = (*capturedEventLog)(nil)

func TestEventTelemetryDefaultsActorAndPartition(t *testing.T) {
	log := &capturedEventLog{}
	adapter := EventTelemetry{Log: log}

	adapter.Emit(core.Event{
		Type:      core.EventAgentFinish,
		Timestamp: time.Time{},
		Metadata:  map[string]interface{}{"status": "failed"},
	})

	require.Len(t, log.events, 1)
	require.Equal(t, "local", log.events[0].Partition)
	require.Equal(t, core.EventActor{Kind: "system", ID: "relurpify"}, log.events[0].Actor)
	require.Equal(t, core.FrameworkEventAgentRunFailed, log.events[0].Type)

	var payload core.Event
	require.NoError(t, json.Unmarshal(log.events[0].Payload, &payload))
	require.Equal(t, core.EventAgentFinish, payload.Type)
}

func TestEventTelemetryHITLEventMapping(t *testing.T) {
	log := &capturedEventLog{}
	adapter := EventTelemetry{
		Log:   log,
		Actor: core.EventActor{Kind: "agent", ID: "coding"},
		Clock: func() time.Time {
			return time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
		},
	}
	adapter.EmitHITLEvent(authorization.HITLEvent{
		Type: authorization.HITLEventResolved,
	})

	require.Len(t, log.events, 1)
	require.Equal(t, core.FrameworkEventHITLResolved, log.events[0].Type)
	require.Equal(t, "local", log.events[0].Partition)
	require.Equal(t, core.EventActor{Kind: "agent", ID: "coding"}, log.events[0].Actor)
}

func TestMetadataValue(t *testing.T) {
	value, ok := metadataValue(map[string]interface{}{"status": "failed"}, "status")
	require.True(t, ok)
	require.Equal(t, "failed", value)

	value, ok = metadataValue(map[string]interface{}{"status": 1}, "status")
	require.False(t, ok)
	require.Empty(t, value)
}
