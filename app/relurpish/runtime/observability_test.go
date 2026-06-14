package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/platform/observability"
	"codeburg.org/lexbit/relurpify/telemetry/event"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type recordingEventLog struct {
	events []event.FrameworkEvent
}

func (l *recordingEventLog) Append(_ context.Context, partition string, events []event.FrameworkEvent) ([]uint64, error) {
	seqs := make([]uint64, len(events))
	for i := range events {
		events[i].Seq = uint64(len(l.events) + 1)
		events[i].Partition = partition
		l.events = append(l.events, events[i])
		seqs[i] = events[i].Seq
	}
	return seqs, nil
}

func (l *recordingEventLog) Read(_ context.Context, _ string, _ uint64, _ int, _ bool) ([]event.FrameworkEvent, error) {
	return nil, nil
}

func (l *recordingEventLog) ReadByType(_ context.Context, _ string, _ string, _ uint64, _ int) ([]event.FrameworkEvent, error) {
	return nil, nil
}

func (l *recordingEventLog) LastSeq(_ context.Context, _ string) (uint64, error) {
	return uint64(len(l.events)), nil
}

func (l *recordingEventLog) TakeSnapshot(_ context.Context, _ string, _ uint64, _ []byte) error {
	return nil
}

func (l *recordingEventLog) LoadSnapshot(_ context.Context, _ string) (uint64, []byte, error) {
	return 0, nil, nil
}

func (l *recordingEventLog) Close() error {
	return nil
}

func TestEmitContractResolvedEventRecordsSourceAndFingerprint(t *testing.T) {
	log := &recordingEventLog{}
	fp := [32]byte{1, 2, 3, 4}
	snapshot := &config.DocumentSnapshot{
		Document:    &config.Document{Kind: "AgentManifest", Metadata: config.DocumentMetadata{Name: "euclo"}},
		Fingerprint: fp,
	}

	emitContractResolvedEvent(context.Background(), log, "", "agent-123", AgentLabelEuclo, snapshot)

	if len(log.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(log.events))
	}

	ev := log.events[0]
	if ev.Type != event.EventContractResolved {
		t.Fatalf("event type = %q, want %q", ev.Type, event.EventContractResolved)
	}

	var payload map[string]any
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["contract_source"] != "builtin+split" {
		t.Fatalf("contract_source = %v, want %q", payload["contract_source"], "builtin+split")
	}
	if payload["fingerprint"] == "" {
		t.Fatal("fingerprint is empty")
	}
	if payload["agent_id"] != "agent-123" {
		t.Fatalf("agent_id = %v, want %q", payload["agent_id"], "agent-123")
	}
}

func TestEmitContractResolvedEvent_SkipsNilEventLog(t *testing.T) {
	emitContractResolvedEvent(context.Background(), nil, "", "agent-123", AgentLabelEuclo, &config.DocumentSnapshot{})
	// Should not panic.
}

func TestEmitContractResolvedEvent_SkipsNilSnapshot(t *testing.T) {
	emitContractResolvedEvent(context.Background(), &recordingEventLog{}, "", "agent-123", AgentLabelEuclo, nil)
	// Should not panic.
}

func TestEmitAgentStartupEventRecordsEucloActor(t *testing.T) {
	log := &recordingEventLog{}
	var agent agentgraph.WorkflowExecutor = (*euclo.Agent)(nil)

	emitAgentStartupEvent(context.Background(), log, "", "agent-123", AgentLabelEuclo, agent)

	require.Len(t, log.events, 1)

	ev := log.events[0]
	require.Equal(t, event.EventAgentRunStarted, ev.Type)
	require.Equal(t, observability.Actor{Kind: "agent", ID: "agent-123", Label: AgentLabelEuclo}, ev.Actor)
	require.Equal(t, "local", ev.Partition)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(ev.Payload, &payload))
	require.Equal(t, "agent-123", payload["agent_id"])
	require.Equal(t, AgentLabelEuclo, payload["agent_label"])
	require.Equal(t, "*euclo.Agent", payload["executor_type"])
}
