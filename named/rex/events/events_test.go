package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestMapAdapterNormalizesTrustedTaskEvent(t *testing.T) {
	adapter := MapAdapter{NameID: "stub", DefaultType: TypeTaskRequested, TrustClass: TrustTrusted, Partition: "local"}
	event, err := adapter.Normalize(map[string]any{
		"event_id":        "evt-1",
		"task_id":         "task-1",
		"instruction":     "review current runtime state",
		"workspace":       "/tmp/ws",
		"idempotency_key": "idem-1",
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event.Type != TypeTaskRequested || event.Partition != "local" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.IdempotencyKey != "idem-1" || event.TrustClass != TrustTrusted {
		t.Fatalf("unexpected identity metadata: %+v", event)
	}
}

func TestDefaultNormalizerRejectsUntrustedResumeIngress(t *testing.T) {
	_, err := DefaultNormalizer{}.Normalize(CanonicalEvent{
		ID:         "evt-2",
		Type:       TypeWorkflowResume,
		TrustClass: TrustUntrusted,
		Payload:    map[string]any{"workflow_id": "wf-1"},
	})
	if err == nil {
		t.Fatalf("expected rejection for untrusted resume ingress")
	}
}

func TestFromFrameworkEventPreservesActorPartitionAndIdempotency(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"instruction": "resume managed work"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	event, err := FromFrameworkEvent(core.FrameworkEvent{
		Seq:            42,
		Timestamp:      time.Date(2026, 3, 20, 9, 0, 0, 0, time.UTC),
		Type:           core.FrameworkEventAgentRunStarted,
		Payload:        payload,
		Actor:          core.EventActor{ID: "nexus", TenantID: "tenant-1"},
		IdempotencyKey: "idem-42",
		Partition:      "tenant-1",
	})
	if err != nil {
		t.Fatalf("FromFrameworkEvent: %v", err)
	}
	if event.ActorID != "nexus" || event.Partition != "tenant-1" || event.IdempotencyKey != "idem-42" {
		t.Fatalf("unexpected canonical event: %+v", event)
	}
	if event.TrustClass != TrustInternal {
		t.Fatalf("unexpected trust class: %+v", event)
	}
}

func TestToEnvelopeAndTaskPreserveIngressMetadata(t *testing.T) {
	event, err := DefaultNormalizer{}.Normalize(CanonicalEvent{
		ID:             "evt-3",
		Type:           TypeTaskRequested,
		TrustClass:     TrustTrusted,
		Source:         "nexus-runtime",
		Partition:      "tenant-a",
		IdempotencyKey: "idem-3",
		Payload: map[string]any{
			"task_id":             "task-3",
			"instruction":         "implement the requested patch",
			"workspace":           "/tmp/ws",
			"workflow_id":         "wf-3",
			"run_id":              "run-3",
			"edit_permitted":      true,
			"capability_snapshot": []any{"plan", "execute"},
			"mode_hint":           "mutation",
		},
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	env := ToEnvelope(event)
	task := ToTask(event)
	if env.WorkflowID != "wf-3" || env.RunID != "run-3" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if task.ID != "task-3" || task.Type != core.TaskTypeCodeModification {
		t.Fatalf("unexpected task: %+v", task)
	}
	if task.Context["event_partition"] != "tenant-a" || task.Context["idempotency_key"] != "idem-3" {
		t.Fatalf("missing ingress metadata: %+v", task.Context)
	}
}

func TestNormalizerStaysTransportAgnostic(t *testing.T) {
	adapter := MapAdapter{NameID: "http-json", DefaultType: TypeTaskRequested, TrustClass: TrustTrusted}
	event, err := adapter.Normalize(map[string]any{
		"event_id":    "evt-4",
		"type":        TypeTaskRequested,
		"instruction": "review transport independence",
		"transport":   "http",
		"headers":     map[string]any{"x-custom": "1"},
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event.Payload["transport"] != "http" {
		t.Fatalf("expected payload preservation, got %+v", event.Payload)
	}
	if _, ok := event.Payload["headers"].(map[string]any); !ok {
		t.Fatalf("expected transport framing in opaque payload: %+v", event.Payload)
	}
}
