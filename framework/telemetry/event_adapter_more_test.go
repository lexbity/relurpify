package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
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

	value, ok = metadataValue(nil, "status")
	require.False(t, ok)
	require.Empty(t, value)

	value, ok = metadataValue(map[string]interface{}{}, "status")
	require.False(t, ok)
	require.Empty(t, value)
}

type capturedTelemetrySink struct {
	events []core.Event
}

func (c *capturedTelemetrySink) Emit(event core.Event) {
	c.events = append(c.events, event)
}

func TestMultiplexTelemetryEmit(t *testing.T) {
	left := &capturedTelemetrySink{}
	right := &capturedTelemetrySink{}

	event := core.Event{Type: core.EventNodeStart, NodeID: "node-1"}
	MultiplexTelemetry{Sinks: []core.Telemetry{left, right}}.Emit(event)

	require.Len(t, left.events, 1)
	require.Len(t, right.events, 1)
	require.Equal(t, event, left.events[0])
	require.Equal(t, event, right.events[0])
}

func TestJSONFileTelemetryLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.jsonl")

	telemetry, err := NewJSONFileTelemetry(path)
	require.NoError(t, err)

	event := core.Event{
		Type:      core.EventNodeFinish,
		NodeID:    "node-1",
		TaskID:    "task-1",
		Message:   "done",
		Timestamp: time.Date(2026, 4, 8, 12, 30, 0, 0, time.UTC),
		Metadata:  map[string]interface{}{"status": "ok"},
	}
	telemetry.Emit(event)
	require.NoError(t, telemetry.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	require.Len(t, lines, 1)

	var got core.Event
	require.NoError(t, json.Unmarshal(lines[0], &got))
	require.Equal(t, event.Type, got.Type)
	require.Equal(t, event.NodeID, got.NodeID)
	require.Equal(t, event.TaskID, got.TaskID)
	require.Equal(t, event.Message, got.Message)
	require.Equal(t, event.Timestamp, got.Timestamp)
	require.Equal(t, event.Metadata, got.Metadata)

	zero := &JSONFileTelemetry{}
	zero.Emit(event)
	require.NoError(t, zero.Close())
}

func TestJSONFileTelemetryNewError(t *testing.T) {
	_, err := NewJSONFileTelemetry(filepath.Join(t.TempDir(), "missing", "telemetry.jsonl"))
	require.Error(t, err)
}

func TestLoggerTelemetryEmitAndSignals(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	telemetry := LoggerTelemetry{Logger: logger}

	telemetry.Emit(core.Event{
		Type:     core.EventGraphStart,
		NodeID:   "node-7",
		TaskID:   "task-9",
		Message:  "hello",
		Metadata: map[string]interface{}{"key": "value"},
	})
	telemetry.OnContextCompression("task-7", core.CompressionStats{
		TotalInteractionsCompressed: 3,
		TotalTokensSaved:            120,
		CompressionEvents:           2,
		CurrentHistorySize:          4,
		CompressedChunks:            1,
	})
	telemetry.OnContextPruning("task-8", 2, 44)
	telemetry.OnBudgetExceeded("task-9", 900, 100)
	telemetry.OnCheckpointCreated("task-10", "checkpoint-1", "node-2")
	telemetry.OnCheckpointRestored("task-11", "checkpoint-2")
	telemetry.OnGraphResume("task-12", "checkpoint-3", "node-3")

	output := buf.String()
	require.Contains(t, output, "[graph_start] node=node-7 task=task-9 meta=map[key:value] msg=hello")
	require.Contains(t, output, "[context_compression] task=task-7 stats={TotalInteractionsCompressed:3 TotalTokensSaved:120 CompressionEvents:2 CurrentHistorySize:4 CompressedChunks:1}")
	require.Contains(t, output, "[context_pruning] task=task-8 removed=2 tokens=44")
	require.Contains(t, output, "[budget_exceeded] task=task-9 attempted=900 available=100")
	require.Contains(t, output, "[checkpoint_created] task=task-10 checkpoint=checkpoint-1 node=node-2")
	require.Contains(t, output, "[checkpoint_restored] task=task-11 checkpoint=checkpoint-2")
	require.Contains(t, output, "[graph_resume] task=task-12 checkpoint=checkpoint-3 node=node-3")
}

func TestLoggerTelemetryDefaultsToStandardLogger(t *testing.T) {
	var buf bytes.Buffer
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(originalFlags)
	})

	telemetry := LoggerTelemetry{}
	telemetry.Emit(core.Event{Type: core.EventGraphFinish, NodeID: "node-1", TaskID: "task-1"})
	telemetry.OnContextCompression("task-2", core.CompressionStats{TotalTokensSaved: 10})
	telemetry.OnContextPruning("task-3", 1, 2)
	telemetry.OnBudgetExceeded("task-4", 3, 4)
	telemetry.OnCheckpointCreated("task-5", "checkpoint-5", "node-5")
	telemetry.OnCheckpointRestored("task-6", "checkpoint-6")
	telemetry.OnGraphResume("task-7", "checkpoint-7", "node-7")

	require.NotEmpty(t, buf.String())
	require.Contains(t, buf.String(), "[graph_finish] node=node-1 task=task-1 meta=map[] msg=")
}

func TestEventTelemetryMappingAndErrorPaths(t *testing.T) {
	log := &capturedEventLog{}
	clock := func() time.Time {
		return time.Date(2026, 4, 8, 15, 0, 0, 0, time.UTC)
	}
	adapter := EventTelemetry{
		Log:       log,
		Partition: "custom",
		Actor:     core.EventActor{Kind: "agent", ID: "codex"},
		Clock:     clock,
	}

	cases := []struct {
		name string
		ev   core.Event
		want string
	}{
		{name: "agent start", ev: core.Event{Type: core.EventAgentStart}, want: core.FrameworkEventAgentRunStarted},
		{name: "agent finish completed", ev: core.Event{Type: core.EventAgentFinish}, want: core.FrameworkEventAgentRunCompleted},
		{name: "agent finish failed", ev: core.Event{Type: core.EventAgentFinish, Metadata: map[string]interface{}{"status": "failed"}}, want: core.FrameworkEventAgentRunFailed},
		{name: "llm prompt", ev: core.Event{Type: core.EventLLMPrompt}, want: core.FrameworkEventLLMRequested},
		{name: "llm response", ev: core.Event{Type: core.EventLLMResponse}, want: core.FrameworkEventLLMResponded},
		{name: "capability call", ev: core.Event{Type: core.EventCapabilityCall}, want: core.FrameworkEventCapabilityInvoked},
		{name: "tool call", ev: core.Event{Type: core.EventToolCall}, want: core.FrameworkEventCapabilityInvoked},
		{name: "capability result", ev: core.Event{Type: core.EventCapabilityResult}, want: core.FrameworkEventCapabilityResult},
		{name: "tool result", ev: core.Event{Type: core.EventToolResult}, want: core.FrameworkEventCapabilityResult},
		{name: "default mapping", ev: core.Event{Type: core.EventStateChange}, want: "telemetry.state_change.v1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := len(log.events)
			ev := tc.ev
			ev.Timestamp = time.Time{}
			adapter.Emit(ev)
			require.Len(t, log.events, before+1)
			got := log.events[len(log.events)-1]
			require.Equal(t, tc.want, got.Type)
			require.Equal(t, clock().UTC(), got.Timestamp)
			require.Equal(t, "custom", got.Partition)
			require.Equal(t, core.EventActor{Kind: "agent", ID: "codex"}, got.Actor)
		})
	}

	adapter.EmitHITLEvent(authorization.HITLEvent{Type: authorization.HITLEventRequested})
	require.Len(t, log.events, len(cases)+1)
	require.Equal(t, core.FrameworkEventHITLRequested, log.events[len(log.events)-1].Type)

	adapter.EmitHITLEvent(authorization.HITLEvent{Type: authorization.HITLEventResolved})
	require.Len(t, log.events, len(cases)+2)
	require.Equal(t, core.FrameworkEventHITLResolved, log.events[len(log.events)-1].Type)

	adapter.EmitHITLEvent(authorization.HITLEvent{Type: authorization.HITLEventExpired})
	require.Len(t, log.events, len(cases)+3)
	require.Equal(t, core.FrameworkEventHITLResolved, log.events[len(log.events)-1].Type)

	before := len(log.events)
	adapter.Emit(core.Event{Type: core.EventNodeError, Metadata: map[string]interface{}{"bad": make(chan int)}})
	require.Len(t, log.events, before)

	nilAdapter := EventTelemetry{}
	nilAdapter.Emit(core.Event{Type: core.EventNodeStart})
	nilAdapter.EmitHITLEvent(authorization.HITLEvent{Type: authorization.HITLEventRequested})
}

func TestEventTelemetryNilHITLEventLog(t *testing.T) {
	EventTelemetry{}.EmitHITLEvent(authorization.HITLEvent{Type: authorization.HITLEventRequested})
}
