package modes

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// Mock emitter for testing
type mockEmitter struct {
	frames   []interaction.InteractionFrame
	response interaction.UserResponse
	err      error
}

func (m *mockEmitter) Emit(ctx context.Context, frame interaction.InteractionFrame) error {
	m.frames = append(m.frames, frame)
	return m.err
}

func (m *mockEmitter) AwaitResponse(ctx context.Context) (interaction.UserResponse, error) {
	return m.response, m.err
}

func TestChatIntentPhase(t *testing.T) {
	ctx := context.Background()
	
	emitter := &mockEmitter{
		response: interaction.UserResponse{ActionID: "confirm"},
	}
	
	phase := &ChatIntentPhase{}
	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"instruction": "How does this work?",
		},
		Mode:       "chat",
		Phase:      "intent",
		PhaseIndex: 0,
		PhaseCount: 3,
	}
	
	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	// Check outcome
	if !outcome.Advance {
		t.Error("Expected Advance to be true")
	}
	if outcome.StateUpdates["intent.response"] != "confirm" {
		t.Error("State should contain intent.response")
	}
	if outcome.StateUpdates["chat.sub_mode"] != "ask" {
		t.Error("chat.sub_mode should be 'ask' for question")
	}
	
	// Check frame was emitted
	if len(emitter.frames) == 0 {
		t.Error("Expected frame to be emitted")
	}
}

func TestChatIntentPhaseImplement(t *testing.T) {
	ctx := context.Background()
	
	emitter := &mockEmitter{
		response: interaction.UserResponse{ActionID: "implement"},
	}
	
	phase := &ChatIntentPhase{}
	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"instruction": "Implement a new feature",
		},
		Mode:  "chat",
		Phase: "intent",
	}
	
	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	if outcome.StateUpdates["chat.sub_mode"] != "implement" {
		t.Error("chat.sub_mode should be 'implement' for implement action")
	}
}

func TestChatIntentPhaseClarify(t *testing.T) {
	ctx := context.Background()
	
	emitter := &mockEmitter{
		response: interaction.UserResponse{
			ActionID: "clarify",
			Text:     "Can you explain differently?",
		},
	}
	
	phase := &ChatIntentPhase{}
	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"instruction": "Original question",
		},
		Mode:  "chat",
		Phase: "intent",
	}
	
	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	if outcome.StateUpdates["instruction"] != "Can you explain differently?" {
		t.Error("Instruction should be updated with clarify text")
	}
}

func TestChatPresentPhase(t *testing.T) {
	ctx := context.Background()
	
	emitter := &mockEmitter{
		response: interaction.UserResponse{ActionID: "done"},
	}
	
	phase := &ChatPresentPhase{
		RunAnalysis: func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.SummaryContent, error) {
			return interaction.SummaryContent{
				Description: "Test analysis result",
			}, nil
		},
	}
	
	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"instruction": "Test question",
		},
		Mode:       "chat",
		Phase:      "present",
		PhaseIndex: 1,
		PhaseCount: 3,
	}
	
	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	if !outcome.Advance {
		t.Error("Expected Advance to be true")
	}
	if outcome.StateUpdates["present.answered"] != true {
		t.Error("present.answered should be true")
	}
	
	// Check frames: status frame then result frame
	if len(emitter.frames) < 2 {
		t.Error("Expected at least 2 frames (status and result)")
	}
}

func TestChatPresentPhaseFollowUp(t *testing.T) {
	ctx := context.Background()
	
	emitter := &mockEmitter{
		response: interaction.UserResponse{
			ActionID: "follow_up",
			Text:     "Can you elaborate?",
		},
	}
	
	phase := &ChatPresentPhase{
		RunAnalysis: func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.SummaryContent, error) {
			return interaction.SummaryContent{
				Description: "Test analysis",
			}, nil
		},
	}
	
	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State:   map[string]any{},
		Mode:    "chat",
		Phase:   "present",
	}
	
	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	if outcome.StateUpdates["present.follow_up"] != "Can you elaborate?" {
		t.Error("Should record follow-up text")
	}
	if outcome.StateUpdates["present.answered"] != nil {
		t.Error("present.answered should not be set for follow-up")
	}
}

func TestChatPresentPhaseImplement(t *testing.T) {
	ctx := context.Background()
	
	emitter := &mockEmitter{
		response: interaction.UserResponse{ActionID: "implement"},
	}
	
	phase := &ChatPresentPhase{
		RunAnalysis: func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.SummaryContent, error) {
			return interaction.SummaryContent{}, nil
		},
	}
	
	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State:   map[string]any{},
		Mode:    "chat",
		Phase:   "present",
	}
	
	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	if outcome.StateUpdates["chat.propose_transition"] != "code" {
		t.Error("Should propose transition to code mode")
	}
}

func TestChatReflectPhase(t *testing.T) {
	ctx := context.Background()
	
	emitter := &mockEmitter{
		response: interaction.UserResponse{ActionID: "done"},
	}
	
	phase := &ChatReflectPhase{}
	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"present.summary": interaction.SummaryContent{
				Description: "Test summary",
			},
		},
		Mode:  "chat",
		Phase: "reflect",
	}
	
	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	if !outcome.Advance {
		t.Error("Expected Advance to be true")
	}
	if outcome.StateUpdates["reflect.response"] != "done" {
		t.Error("Should record response")
	}
}

func TestClassifyChatSubMode(t *testing.T) {
	testCases := []struct {
		instruction string
		expected    string
	}{
		{"How does this work?", "ask"},
		{"Can you explain?", "ask"},
		{"Implement a feature", "implement"},
		{"Write a function", "implement"},
		{"Create a new file", "implement"},
		{"Build a system", "implement"},
		{"Add a test", "implement"},
		{"Fix the bug", "implement"},
		{"Change the color", "implement"},
		{"Update the API", "implement"},
		{"Refactor this code", "implement"},
	}
	
	for _, tc := range testCases {
		result := classifyChatSubMode(tc.instruction)
		if result != tc.expected {
			t.Errorf("classifyChatSubMode(%q) = %q, want %q", tc.instruction, result, tc.expected)
		}
	}
}
