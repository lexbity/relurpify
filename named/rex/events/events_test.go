package events

import (
	"encoding/json"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/rexkeys"
)

func TestMapAdapterNormalizesTrustedTaskEvent(t *testing.T) {
	adapter := MapAdapter{NameID: "stub", DefaultType: TypeTaskRequested, IngressOrigin: OriginPeer, Partition: "local"}
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
	if event.IdempotencyKey != "idem-1" || event.IngressOrigin != OriginPeer {
		t.Fatalf("unexpected identity metadata: %+v", event)
	}
}

func TestDefaultNormalizerRejectsUntrustedResumeIngress(t *testing.T) {
	_, err := DefaultNormalizer{}.Normalize(CanonicalEvent{
		ID:            "evt-2",
		Type:          TypeWorkflowResume,
		IngressOrigin: OriginExternal,
		Payload:       map[string]any{"workflow_id": "wf-1"},
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
	if event.IngressOrigin != OriginInternal {
		t.Fatalf("unexpected ingress origin: %+v", event)
	}
}

func TestToEnvelopeAndTaskPreserveIngressMetadata(t *testing.T) {
	event, err := DefaultNormalizer{}.Normalize(CanonicalEvent{
		ID:             "evt-3",
		Type:           TypeTaskRequested,
		IngressOrigin:  OriginPeer,
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
	if task.Context[rexkeys.RexEventPartition] != "tenant-a" || task.Context["idempotency_key"] != "idem-3" {
		t.Fatalf("missing ingress metadata: %+v", task.Context)
	}
}

func TestNormalizerStaysTransportAgnostic(t *testing.T) {
	adapter := MapAdapter{NameID: "http-json", DefaultType: TypeTaskRequested, IngressOrigin: OriginPeer}
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

func TestEventHelpersCoverFallbacksAndBranches(t *testing.T) {
	if got := (MapAdapter{}).Name(); got != "map" {
		t.Fatalf("default Name() = %q", got)
	}
	if got := (MapAdapter{NameID: " rex "}).Name(); got != "rex" {
		t.Fatalf("trimmed Name() = %q", got)
	}
	if got := normalizeOrigin(" TRUSTED "); got != OriginPeer {
		t.Fatalf("normalizeOrigin peer = %q", got)
	}
	if got := normalizeOrigin("custom-origin"); got != "custom-origin" {
		t.Fatalf("normalizeOrigin custom = %q", got)
	}
	if got := stringValue(nil); got != "" {
		t.Fatalf("stringValue(nil) = %q", got)
	}
	if got := stringValue(7); got != "7" {
		t.Fatalf("stringValue(7) = %q", got)
	}
	if !boolValue(true) || boolValue("true") {
		t.Fatalf("boolValue branches not covered as expected")
	}
	if got := stringSlice([]string{"a", "b"}); len(got) != 2 || got[1] != "b" {
		t.Fatalf("stringSlice([]string) = %+v", got)
	}
	if got := stringSlice([]any{"a", 9, "", nil}); len(got) != 2 || got[0] != "a" || got[1] != "9" {
		t.Fatalf("stringSlice([]any) = %+v", got)
	}
	now := time.Date(2026, 4, 8, 12, 34, 56, 0, time.UTC)
	if got := timeValue(now); !got.Equal(now) {
		t.Fatalf("timeValue(time.Time) = %v", got)
	}
	if got := timeValue("2026-04-08T12:34:56Z"); got.IsZero() || got.UTC().Format(time.RFC3339) != "2026-04-08T12:34:56Z" {
		t.Fatalf("timeValue(string) = %v", got)
	}
	if got := timeValue("not-a-timestamp"); !got.IsZero() {
		t.Fatalf("timeValue(invalid) = %v", got)
	}
	if got := taskTypeForEvent(TypeWorkflowResume, true); got != core.TaskTypeAnalysis {
		t.Fatalf("taskTypeForEvent resume = %v", got)
	}
	if got := taskTypeForEvent(TypeTaskRequested, true); got != core.TaskTypeCodeModification {
		t.Fatalf("taskTypeForEvent task requested/edit = %v", got)
	}
	if got := taskTypeForEvent(TypeTaskRequested, false); got != core.TaskTypeAnalysis {
		t.Fatalf("taskTypeForEvent task requested/read-only = %v", got)
	}
	if got := taskTypeForEvent("custom.event", true); got != core.TaskTypeCodeGeneration {
		t.Fatalf("taskTypeForEvent default/edit = %v", got)
	}
	if got := taskTypeForEvent("custom.event", false); got != core.TaskTypeAnalysis {
		t.Fatalf("taskTypeForEvent default/read-only = %v", got)
	}
}

func TestDefaultNormalizerRejectsInvalidTrustAndMissingIdentity(t *testing.T) {
	if _, err := (DefaultNormalizer{}).Normalize(CanonicalEvent{ID: "evt", Type: TypeTaskRequested, IngressOrigin: "bad-origin", Payload: map[string]any{}}); err == nil {
		t.Fatalf("expected invalid trust rejection")
	}
	if _, err := (DefaultNormalizer{}).Normalize(CanonicalEvent{IngressOrigin: OriginPeer, Payload: map[string]any{}}); err == nil {
		t.Fatalf("expected missing id rejection")
	}
}

func TestFromFrameworkEventRejectsInvalidJSON(t *testing.T) {
	if _, err := FromFrameworkEvent(core.FrameworkEvent{Seq: 1, Type: core.FrameworkEventAgentRunStarted, Payload: []byte("{not-json}")}); err == nil {
		t.Fatalf("expected invalid JSON rejection")
	}
}

func TestToEnvelopeAndTaskUseFallbacks(t *testing.T) {
	event, err := DefaultNormalizer{}.Normalize(CanonicalEvent{
		ID:            "evt-5",
		Type:          TypeTaskRequested,
		IngressOrigin: OriginPeer,
		Payload: map[string]any{
			"task_id":             "task-5",
			"summary":             "fallback summary",
			"workspace":           "/tmp/ws",
			"mutation_allowed":    true,
			"capability_snapshot": []string{"plan", "execute"},
		},
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	env := ToEnvelope(event)
	if env.Instruction != "fallback summary" {
		t.Fatalf("unexpected instruction fallback: %+v", env)
	}
	if !env.EditPermitted {
		t.Fatalf("expected mutation fallback to permit edits: %+v", env)
	}
	task := ToTask(event)
	if task.Type != core.TaskTypeCodeModification {
		t.Fatalf("unexpected task type: %+v", task)
	}
	if len(task.Context["capability_snapshot"].([]string)) != 2 {
		t.Fatalf("unexpected capability snapshot: %+v", task.Context)
	}
}
