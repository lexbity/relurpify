package telemetry

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestEventTelemetryEmitsMappedFrameworkEvent(t *testing.T) {
	log, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()

	adapter := EventTelemetry{
		Log:       log,
		Partition: "local",
		Actor:     core.EventActor{Kind: "agent", ID: "coding"},
		Clock: func() time.Time {
			return time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
		},
	}
	adapter.Emit(core.Event{
		Type:      core.EventLLMPrompt,
		TaskID:    "task-1",
		Timestamp: time.Date(2026, 3, 9, 12, 0, 1, 0, time.UTC),
		Metadata:  map[string]interface{}{"kind": "chat"},
	})

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, core.FrameworkEventLLMRequested, events[0].Type)

	var payload core.Event
	require.NoError(t, json.Unmarshal(events[0].Payload, &payload))
	require.Equal(t, core.EventLLMPrompt, payload.Type)
}
