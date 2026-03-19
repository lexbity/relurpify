package interaction

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestInteractionRecording_RecordAndRetrieve(t *testing.T) {
	rec := NewInteractionRecording()

	rec.RecordFrame(InteractionFrame{Kind: FrameProposal, Phase: "scope", Mode: "code"})
	rec.RecordResponse(UserResponse{ActionID: "confirm"}, "scope", "code")
	rec.RecordPhaseSkip("clarify", "code", "auto_skip")
	rec.RecordTransition("code", "debug", "verification_failure")

	events := rec.Events()
	if len(events) != 4 {
		t.Fatalf("events: got %d, want 4", len(events))
	}
	if events[0].Type != "frame" {
		t.Errorf("event[0].Type: got %q", events[0].Type)
	}
	if events[1].Type != "response" {
		t.Errorf("event[1].Type: got %q", events[1].Type)
	}
	if events[2].Type != "phase_skip" {
		t.Errorf("event[2].Type: got %q", events[2].Type)
	}
	if events[3].Type != "transition" {
		t.Errorf("event[3].Type: got %q", events[3].Type)
	}
}

func TestInteractionRecording_FrameEvents(t *testing.T) {
	rec := NewInteractionRecording()
	rec.RecordFrame(InteractionFrame{Kind: FrameProposal})
	rec.RecordResponse(UserResponse{}, "", "")
	rec.RecordFrame(InteractionFrame{Kind: FrameQuestion})

	frames := rec.FrameEvents()
	if len(frames) != 2 {
		t.Errorf("frame events: got %d, want 2", len(frames))
	}
}

func TestInteractionRecording_TransitionEvents(t *testing.T) {
	rec := NewInteractionRecording()
	rec.RecordFrame(InteractionFrame{})
	rec.RecordTransition("code", "debug", "test")
	rec.RecordTransition("debug", "code", "completion")

	transitions := rec.TransitionEvents()
	if len(transitions) != 2 {
		t.Errorf("transitions: got %d, want 2", len(transitions))
	}
}

func TestInteractionRecording_MarshalJSON(t *testing.T) {
	rec := NewInteractionRecording()
	rec.RecordFrame(InteractionFrame{Kind: FrameProposal, Phase: "scope"})

	data, err := rec.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Events) != 1 {
		t.Errorf("events: got %d", len(parsed.Events))
	}
}

func TestInteractionRecording_ToStateMap(t *testing.T) {
	rec := NewInteractionRecording()
	rec.RecordFrame(InteractionFrame{Kind: FrameProposal, Phase: "scope", Mode: "code"})
	rec.RecordFrame(InteractionFrame{Kind: FrameQuestion, Phase: "clarify", Mode: "code"})
	rec.RecordTransition("code", "debug", "failure")

	m := rec.ToStateMap()
	if m["event_count"] != 3 {
		t.Errorf("event_count: got %v", m["event_count"])
	}
	frames, ok := m["frames"].([]map[string]any)
	if !ok || len(frames) != 2 {
		t.Errorf("frames: got %v", m["frames"])
	}
	transitions, ok := m["transitions"].([]map[string]any)
	if !ok || len(transitions) != 1 {
		t.Errorf("transitions: got %v", m["transitions"])
	}
}

func TestRecordingEmitter_RecordsFramesAndResponses(t *testing.T) {
	inner := &NoopEmitter{}
	emitter := NewRecordingEmitter(inner)
	ctx := context.Background()

	_ = emitter.Emit(ctx, InteractionFrame{
		Kind:  FrameProposal,
		Phase: "scope",
		Mode:  "code",
		Actions: []ActionSlot{
			{ID: "confirm", Default: true},
		},
	})
	resp, err := emitter.AwaitResponse(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ActionID != "confirm" {
		t.Errorf("response: got %q", resp.ActionID)
	}

	events := emitter.Recording.Events()
	if len(events) != 2 {
		t.Fatalf("events: got %d, want 2", len(events))
	}
	if events[0].Type != "frame" {
		t.Error("expected frame event")
	}
	if events[1].Type != "response" {
		t.Error("expected response event")
	}
	if events[1].Response.ActionID != "confirm" {
		t.Error("response should be confirm")
	}
	if events[1].Phase != "scope" {
		t.Errorf("response phase: got %q, want scope", events[1].Phase)
	}
	records := emitter.Recording.Records()
	if len(records) != 1 {
		t.Fatalf("records: got %d, want 1", len(records))
	}
	if records[0].Kind != "proposal" {
		t.Fatalf("record kind: got %q", records[0].Kind)
	}
	if !strings.Contains(string(records[0].Response), "confirm") {
		t.Fatalf("expected record response to include confirm, got %s", string(records[0].Response))
	}
	if records[0].Duration == "" {
		t.Fatal("expected response duration to be captured")
	}
}

func TestRecordingEmitter_WithTestEmitter(t *testing.T) {
	inner := NewTestFrameEmitter(
		ScriptedResponse{Phase: "scope", ActionID: "edit"},
	)
	emitter := NewRecordingEmitter(inner)
	ctx := context.Background()

	_ = emitter.Emit(ctx, InteractionFrame{Kind: FrameProposal, Phase: "scope", Mode: "code"})
	resp, _ := emitter.AwaitResponse(ctx)

	if resp.ActionID != "edit" {
		t.Errorf("got %q, want edit", resp.ActionID)
	}

	events := emitter.Recording.Events()
	if len(events) != 2 {
		t.Fatalf("events: got %d, want 2", len(events))
	}
}

func TestInteractionRecording_ToJSONLines(t *testing.T) {
	rec := NewInteractionRecording()
	rec.RecordFrame(InteractionFrame{Kind: FrameProposal, Phase: "scope", Mode: "code"})
	rec.RecordResponse(UserResponse{ActionID: "confirm"}, "scope", "code")

	data, err := rec.ToJSONLines()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 jsonl record, got %d", len(lines))
	}
	var record InteractionRecord
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatal(err)
	}
	if record.Kind != "proposal" || record.Phase != "scope" || record.Mode != "code" {
		t.Fatalf("unexpected interaction record: %+v", record)
	}
}

func TestInteractionRecording_RecordPhaseArtifacts(t *testing.T) {
	rec := NewInteractionRecording()
	rec.RecordFrame(InteractionFrame{Kind: FrameSummary, Phase: "commit", Mode: "planning"})
	rec.RecordPhaseArtifacts("commit", "planning",
		[]euclotypes.Artifact{{
			Kind:    euclotypes.ArtifactKindPlan,
			Summary: "rate limit plan",
			Payload: map[string]any{"steps": []string{"add limiter"}},
		}},
		[]euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
	)

	records := rec.Records()
	if len(records) != 1 {
		t.Fatalf("records: got %d, want 1", len(records))
	}
	if len(records[0].ArtifactsProduced) != 1 || records[0].ArtifactsProduced[0] != "euclo.plan" {
		t.Fatalf("unexpected produced artifacts: %+v", records[0].ArtifactsProduced)
	}
	if len(records[0].ArtifactsConsumed) != 1 || records[0].ArtifactsConsumed[0] != "euclo.explore" {
		t.Fatalf("unexpected consumed artifacts: %+v", records[0].ArtifactsConsumed)
	}
	if len(records[0].ProducedArtifacts) != 1 || records[0].ProducedArtifacts[0].Kind != "euclo.plan" {
		t.Fatalf("unexpected produced artifact details: %+v", records[0].ProducedArtifacts)
	}
	if !strings.Contains(string(records[0].ProducedArtifacts[0].Payload), "limiter") {
		t.Fatalf("expected artifact payload to be recorded, got %s", string(records[0].ProducedArtifacts[0].Payload))
	}
}
