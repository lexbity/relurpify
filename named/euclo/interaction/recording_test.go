package interaction

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

func TestNewInteractionRecording(t *testing.T) {
	recording := NewInteractionRecording()
	if recording == nil {
		t.Fatal("NewInteractionRecording returned nil")
	}

	// Initial recording should have no events
	events := recording.Events()
	if len(events) != 0 {
		t.Errorf("New recording should have 0 events, got %d", len(events))
	}
}

func TestRecordFrame(t *testing.T) {
	recording := NewInteractionRecording()

	// Define FrameKind constants if they don't exist
	frameKind := FrameKind("proposal")

	frame := InteractionFrame{
		Kind:    frameKind,
		Mode:    "chat",
		Phase:   "intent",
		Content: "Test content",
		Metadata: FrameMetadata{
			Timestamp: time.Now(),
		},
	}

	recording.RecordFrame(frame)

	events := recording.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != "frame" {
		t.Errorf("Expected event type 'frame', got %s", event.Type)
	}
	if event.Frame == nil {
		t.Fatal("Event.Frame should not be nil")
	}
	if event.Frame.Kind != frame.Kind {
		t.Errorf("Expected frame kind %s, got %s", frame.Kind, event.Frame.Kind)
	}
}

func TestRecordResponse(t *testing.T) {
	recording := NewInteractionRecording()

	// First record a frame
	frame := InteractionFrame{
		Kind:  FrameProposal,
		Mode:  "chat",
		Phase: "intent",
	}
	recording.RecordFrame(frame)

	// Then record a response
	resp := UserResponse{
		ActionID: "confirm",
		Text:     "yes",
	}
	recording.RecordResponse(resp, "intent", "chat")

	events := recording.Events()
	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	// Check response event
	responseEvent := events[1]
	if responseEvent.Type != "response" {
		t.Errorf("Expected event type 'response', got %s", responseEvent.Type)
	}
	if responseEvent.Response == nil {
		t.Fatal("Event.Response should not be nil")
	}
	if responseEvent.Response.ActionID != resp.ActionID {
		t.Errorf("Expected ActionID %s, got %s", resp.ActionID, responseEvent.Response.ActionID)
	}
}

func TestRecordPhaseSkip(t *testing.T) {
	recording := NewInteractionRecording()

	recording.RecordPhaseSkip("planning", "code", "no artifacts")

	events := recording.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != "phase_skip" {
		t.Errorf("Expected event type 'phase_skip', got %s", event.Type)
	}
	if event.Phase != "planning" {
		t.Errorf("Expected phase 'planning', got %s", event.Phase)
	}
	if event.Detail != "no artifacts" {
		t.Errorf("Expected detail 'no artifacts', got %s", event.Detail)
	}
}

func TestRecordTransition(t *testing.T) {
	recording := NewInteractionRecording()

	recording.RecordTransition("chat", "code", "user request")

	events := recording.Events()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != "transition" {
		t.Errorf("Expected event type 'transition', got %s", event.Type)
	}
	if event.Mode != "chat" {
		t.Errorf("Expected mode 'chat', got %s", event.Mode)
	}
	if event.Detail != "code:user request" {
		t.Errorf("Expected detail 'code:user request', got %s", event.Detail)
	}
}

func TestRecordPhaseArtifacts(t *testing.T) {
	recording := NewInteractionRecording()

	produced := []euclotypes.Artifact{
		{
			ID:      "artifact1",
			Kind:    euclotypes.ArtifactKindExplore,
			Summary: "Test artifact",
		},
	}
	consumed := []euclotypes.ArtifactKind{
		euclotypes.ArtifactKindPlan,
		euclotypes.ArtifactKindExplore,
	}

	recording.RecordPhaseArtifacts("plan", "code", produced, consumed)

	// This adds to the full records, not events
	records := recording.Records()
	if len(records) == 0 {
		t.Error("RecordPhaseArtifacts should create a record")
	}
}

func TestRecordingEmitter(t *testing.T) {
	// Create a mock inner emitter
	mockEmitter := &mockFrameEmitter{}

	recordingEmitter := NewRecordingEmitter(mockEmitter)
	if recordingEmitter == nil {
		t.Fatal("NewRecordingEmitter returned nil")
	}
	if recordingEmitter.Recording == nil {
		t.Fatal("RecordingEmitter should have a Recording")
	}

	// Test Emit
	frame := InteractionFrame{
		Kind:  FrameStatus,
		Mode:  "chat",
		Phase: "test",
	}

	ctx := context.Background()
	err := recordingEmitter.Emit(ctx, frame)
	if err != nil {
		t.Errorf("Emit failed: %v", err)
	}

	// Check recording has the frame
	events := recordingEmitter.Recording.Events()
	if len(events) != 1 {
		t.Errorf("Expected 1 recorded event, got %d", len(events))
	}

	// Test AwaitResponse
	mockEmitter.response = UserResponse{ActionID: "test"}
	resp, err := recordingEmitter.AwaitResponse(ctx)
	if err != nil {
		t.Errorf("AwaitResponse failed: %v", err)
	}
	if resp.ActionID != "test" {
		t.Errorf("Expected ActionID 'test', got %s", resp.ActionID)
	}

	// Check response was recorded
	events = recordingEmitter.Recording.Events()
	if len(events) != 2 {
		t.Errorf("Expected 2 recorded events, got %d", len(events))
	}
}

func TestMarshalJSON(t *testing.T) {
	recording := NewInteractionRecording()

	// Add some events
	recording.RecordFrame(InteractionFrame{
		Kind:  FrameProposal,
		Mode:  "chat",
		Phase: "test",
	})

	data, err := recording.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Check structure
	if _, ok := parsed["events"]; !ok {
		t.Error("JSON should have 'events' field")
	}
	if _, ok := parsed["records"]; !ok {
		t.Error("JSON should have 'records' field")
	}
}

// Mock frame emitter for testing
type mockFrameEmitter struct {
	response UserResponse
	err      error
}

func (m *mockFrameEmitter) Emit(ctx context.Context, frame InteractionFrame) error {
	return m.err
}

func (m *mockFrameEmitter) AwaitResponse(ctx context.Context) (UserResponse, error) {
	return m.response, m.err
}
