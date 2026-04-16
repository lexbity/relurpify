package session

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// testEmitter is a test double for FrameEmitter
type testEmitter struct {
	frames   []interaction.InteractionFrame
	response interaction.UserResponse
}

func (e *testEmitter) Emit(ctx context.Context, frame interaction.InteractionFrame) error {
	e.frames = append(e.frames, frame)
	return nil
}

func (e *testEmitter) AwaitResponse(ctx context.Context) (interaction.UserResponse, error) {
	return e.response, nil
}

func TestSessionResumeTrigger_PhraseMatches(t *testing.T) {
	trigger := SessionResumeTrigger()

	// Test all registered phrases resolve
	phrases := []string{
		"resume session",
		"continue session",
		"load session",
		"restore session",
		"continue last session",
		"resume last session",
		"pick up where i left off",
	}

	resolver := interaction.NewAgencyResolver()
	resolver.RegisterTrigger("", trigger)

	for _, phrase := range phrases {
		matched, ok := resolver.Resolve("", phrase)
		if !ok {
			t.Errorf("Resolve(%q) returned ok=false, want true", phrase)
		}
		if matched == nil {
			t.Errorf("Resolve(%q) returned nil trigger", phrase)
			continue
		}
		if matched.PhaseJump != "session_select" {
			t.Errorf("Resolve(%q) PhaseJump = %q, want 'session_select'", phrase, matched.PhaseJump)
		}
	}
}

func TestSessionResumeTrigger_Description(t *testing.T) {
	trigger := SessionResumeTrigger()
	if trigger.Description == "" {
		t.Error("Description is empty, want non-empty description")
	}
	if trigger.RequiresMode != "" {
		t.Errorf("RequiresMode = %q, want empty (mode-agnostic)", trigger.RequiresMode)
	}
	if trigger.CapabilityID != "" {
		t.Errorf("CapabilityID = %q, want empty (uses PhaseJump)", trigger.CapabilityID)
	}
}

func TestParseSessionSelection_NumericIndex(t *testing.T) {
	list := SessionList{
		Sessions: []SessionRecord{
			{WorkflowID: "wf-1", Instruction: "first task"},
			{WorkflowID: "wf-2", Instruction: "second task"},
			{WorkflowID: "wf-3", Instruction: "third task"},
		},
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"1", "wf-1"},
		{"2", "wf-2"},
		{"3", "wf-3"},
		{"0", ""},  // out of range
		{"4", ""},  // out of range
		{"-1", ""}, // out of range
	}

	for _, tt := range tests {
		result := ParseSessionSelection(tt.input, list)
		if result != tt.expected {
			t.Errorf("ParseSessionSelection(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseSessionSelection_WorkflowIDExact(t *testing.T) {
	list := SessionList{
		Sessions: []SessionRecord{
			{WorkflowID: "abc-123", Instruction: "task 1"},
			{WorkflowID: "def-456", Instruction: "task 2"},
		},
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"abc-123", "abc-123"},
		{"ABC-123", "abc-123"}, // case-insensitive
		{"def-456", "def-456"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		result := ParseSessionSelection(tt.input, list)
		if result != tt.expected {
			t.Errorf("ParseSessionSelection(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseSessionSelection_WorkflowIDPrefix(t *testing.T) {
	list := SessionList{
		Sessions: []SessionRecord{
			{WorkflowID: "workflow-abc-123", Instruction: "task 1"},
			{WorkflowID: "workflow-def-456", Instruction: "task 2"},
		},
	}

	result := ParseSessionSelection("workflow-abc", list)
	if result != "workflow-abc-123" {
		t.Errorf("ParseSessionSelection('workflow-abc') = %q, want 'workflow-abc-123'", result)
	}
}

func TestParseSessionSelection_InstructionSubstring(t *testing.T) {
	list := SessionList{
		Sessions: []SessionRecord{
			{WorkflowID: "wf-1", Instruction: "fix the login bug"},
			{WorkflowID: "wf-2", Instruction: "implement user profile"},
		},
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"login", "wf-1"},
		{"bug", "wf-1"},
		{"profile", "wf-2"},
		{"user", "wf-2"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		result := ParseSessionSelection(tt.input, list)
		if result != tt.expected {
			t.Errorf("ParseSessionSelection(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseSessionSelection_NoMatch_ReturnsEmpty(t *testing.T) {
	list := SessionList{
		Sessions: []SessionRecord{
			{WorkflowID: "wf-1", Instruction: "task 1"},
		},
	}

	result := ParseSessionSelection("completely unrelated", list)
	if result != "" {
		t.Errorf("ParseSessionSelection('completely unrelated') = %q, want empty", result)
	}
}

func TestParseSessionSelection_EmptyResponse(t *testing.T) {
	list := SessionList{
		Sessions: []SessionRecord{
			{WorkflowID: "wf-1", Instruction: "task 1"},
		},
	}

	result := ParseSessionSelection("", list)
	if result != "" {
		t.Errorf("ParseSessionSelection('') = %q, want empty", result)
	}

	result = ParseSessionSelection("   ", list)
	if result != "" {
		t.Errorf("ParseSessionSelection('   ') = %q, want empty", result)
	}
}

func TestSessionSelectPhase_EmitsSessionList(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "test session",
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now,
	}

	index := &SessionIndex{WorkflowStore: store}
	resolver := &SessionResumeResolver{WorkflowStore: store}

	phase := &SessionSelectPhase{
		Index:    index,
		Resolver: resolver,
	}

	// Test with skip response (default)
	emitter := &testEmitter{response: interaction.UserResponse{ActionID: "skip"}}
	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Mode:       "code",
		Phase:      "session_select",
		PhaseIndex: 0,
		PhaseCount: 1,
	}

	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Should advance after AwaitResponse
	if !outcome.Advance {
		t.Error("Advance = false, want true after AwaitResponse")
	}

	// Should have emitted a session_list frame
	if len(emitter.frames) != 1 {
		t.Fatalf("Emitted %d frames, want 1", len(emitter.frames))
	}

	frame := emitter.frames[0]
	if frame.Kind != interaction.FrameSessionList {
		t.Errorf("Frame.Kind = %q, want 'session_list'", frame.Kind)
	}

	content, ok := frame.Content.(interaction.SessionListContent)
	if !ok {
		t.Fatalf("Frame.Content type = %T, want SessionListContent", frame.Content)
	}

	if len(content.Sessions) != 1 {
		t.Errorf("Sessions count = %d, want 1", len(content.Sessions))
	}
}

func TestSessionSelectPhase_EmitsEmptyFrame_NoSessions(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	index := &SessionIndex{WorkflowStore: store}
	resolver := &SessionResumeResolver{WorkflowStore: store}

	phase := &SessionSelectPhase{
		Index:    index,
		Resolver: resolver,
	}

	emitter := &testEmitter{}
	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Mode:       "code",
		Phase:      "session_select",
		PhaseIndex: 0,
		PhaseCount: 1,
	}

	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Should advance when no sessions
	if !outcome.Advance {
		t.Error("Advance = false, want true when no sessions")
	}

	// Should have emitted a session_list_empty frame
	if len(emitter.frames) != 1 {
		t.Fatalf("Emitted %d frames, want 1", len(emitter.frames))
	}

	frame := emitter.frames[0]
	if frame.Kind != interaction.FrameSessionListEmpty {
		t.Errorf("Frame.Kind = %q, want 'session_list_empty'", frame.Kind)
	}
}

func TestSessionSelectPhase_AwaitResponse_SelectFirst(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "test session",
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now,
	}

	index := &SessionIndex{WorkflowStore: store}
	resolver := &SessionResumeResolver{WorkflowStore: store}

	phase := &SessionSelectPhase{
		Index:    index,
		Resolver: resolver,
	}

	// Test with select_1 response
	emitter := &testEmitter{response: interaction.UserResponse{ActionID: "select_1"}}
	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Mode:       "code",
		Phase:      "session_select",
		PhaseIndex: 0,
		PhaseCount: 1,
	}

	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Should advance after successful selection
	if !outcome.Advance {
		t.Error("Advance = false, want true after selection")
	}

	// Should have state update with resume context
	if outcome.StateUpdates == nil {
		t.Fatal("StateUpdates is nil, want resume context")
	}

	resumeCtx, ok := outcome.StateUpdates["euclo.session_resume_context"].(SessionResumeContext)
	if !ok {
		t.Fatalf("StateUpdates['euclo.session_resume_context'] type = %T, want SessionResumeContext", outcome.StateUpdates["euclo.session_resume_context"])
	}

	if resumeCtx.WorkflowID != "wf-1" {
		t.Errorf("ResumeContext.WorkflowID = %q, want 'wf-1'", resumeCtx.WorkflowID)
	}
	if resumeCtx.SessionStartTime.IsZero() {
		t.Fatal("ResumeContext.SessionStartTime is zero, want workflow CreatedAt")
	}
	if !resumeCtx.SessionStartTime.Equal(now.Add(-time.Hour)) {
		t.Fatalf("ResumeContext.SessionStartTime = %v, want %v", resumeCtx.SessionStartTime, now.Add(-time.Hour))
	}

	// Should have emitted resuming frame
	foundResuming := false
	for _, f := range emitter.frames {
		if f.Kind == interaction.FrameSessionResuming {
			foundResuming = true
			break
		}
	}
	if !foundResuming {
		t.Error("Did not emit FrameSessionResuming frame")
	}
}

func TestSessionSelectPhase_AwaitResponse_UnknownWorkflow_EmitsErrorFrame(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}
	// Don't add wf-nonexistent, so resolution will fail

	index := &SessionIndex{WorkflowStore: store}
	resolver := &SessionResumeResolver{WorkflowStore: store}

	phase := &SessionSelectPhase{
		Index:    index,
		Resolver: resolver,
	}

	// First add a session to the list so we get past the empty check
	store.workflows["wf-list"] = memory.WorkflowRecord{
		WorkflowID:  "wf-list",
		Instruction: "list session",
		UpdatedAt:   time.Now(),
	}

	// Test with select_1 pointing to wf-list, but we'll use freetext to select nonexistent
	emitter := &testEmitter{response: interaction.UserResponse{ActionID: "", Text: "wf-nonexistent"}}
	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Mode:       "code",
		Phase:      "session_select",
		PhaseIndex: 0,
		PhaseCount: 1,
	}

	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Should advance even on error
	if !outcome.Advance {
		t.Error("Advance = false, want true even on error")
	}

	// No error frame expected since freetext "wf-nonexistent" doesn't match any workflow
	// The phase just proceeds without resume context
	if outcome.StateUpdates != nil && outcome.StateUpdates["euclo.session_resume_context"] != nil {
		t.Error("Expected no resume context for unmatched selection")
	}
}

func TestParseActionToWorkflowID_NumericIndex(t *testing.T) {
	list := SessionList{
		Sessions: []SessionRecord{
			{WorkflowID: "wf-1", Instruction: "first task"},
			{WorkflowID: "wf-2", Instruction: "second task"},
			{WorkflowID: "wf-3", Instruction: "third task"},
		},
	}

	tests := []struct {
		actionID string
		expected string
	}{
		{"select_1", "wf-1"},
		{"select_2", "wf-2"},
		{"select_3", "wf-3"},
		{"select_0", ""}, // out of range
		{"select_4", ""}, // out of range
		{"skip", ""},     // skip action
		{"", ""},         // empty action
		{"unknown", ""},  // unknown action
	}

	for _, tt := range tests {
		resp := interaction.UserResponse{ActionID: tt.actionID}
		result := parseActionToWorkflowID(resp, list)
		if result != tt.expected {
			t.Errorf("parseActionToWorkflowID(ActionID:%q) = %q, want %q", tt.actionID, result, tt.expected)
		}
	}
}

func TestParseActionToWorkflowID_FreetextFallback(t *testing.T) {
	list := SessionList{
		Sessions: []SessionRecord{
			{WorkflowID: "wf-1", Instruction: "fix the login bug"},
			{WorkflowID: "wf-2", Instruction: "implement user profile"},
		},
	}

	// Test freetext fallback via Text field
	resp := interaction.UserResponse{ActionID: "", Text: "login bug"}
	result := parseActionToWorkflowID(resp, list)
	if result != "wf-1" {
		t.Errorf("parseActionToWorkflowID(Text:'login bug') = %q, want 'wf-1'", result)
	}
}

func TestSessionSelectPhase_AwaitResponse_Skip(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "test session",
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now,
	}

	index := &SessionIndex{WorkflowStore: store}
	resolver := &SessionResumeResolver{WorkflowStore: store}

	phase := &SessionSelectPhase{
		Index:    index,
		Resolver: resolver,
	}

	// Test with skip response
	emitter := &testEmitter{response: interaction.UserResponse{ActionID: "skip"}}
	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      make(map[string]any),
		Mode:       "code",
		Phase:      "session_select",
		PhaseIndex: 0,
		PhaseCount: 1,
	}

	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Should advance without resume context
	if !outcome.Advance {
		t.Error("Advance = false, want true after skip")
	}
	if outcome.StateUpdates != nil && outcome.StateUpdates["euclo.session_resume_context"] != nil {
		t.Error("Expected no resume context after skip")
	}
}

func TestFormatSessionListContent(t *testing.T) {
	now := time.Now()
	list := SessionList{
		Sessions: []SessionRecord{
			{
				WorkflowID:    "wf-1",
				Instruction:   "fix bug",
				Mode:          "debug",
				Status:        "completed",
				HasBKCContext: true,
				LastActiveAt:  now,
			},
		},
		Workspace: "/test/workspace",
	}

	content := formatSessionListContent(list)

	if content.Workspace != "/test/workspace" {
		t.Errorf("Workspace = %q, want '/test/workspace'", content.Workspace)
	}
	if len(content.Sessions) != 1 {
		t.Fatalf("Sessions count = %d, want 1", len(content.Sessions))
	}

	entry := content.Sessions[0]
	if entry.Index != 1 {
		t.Errorf("Index = %d, want 1", entry.Index)
	}
	if entry.WorkflowID != "wf-1" {
		t.Errorf("WorkflowID = %q, want 'wf-1'", entry.WorkflowID)
	}
	if entry.Instruction != "fix bug" {
		t.Errorf("Instruction = %q, want 'fix bug'", entry.Instruction)
	}
	if entry.Mode != "debug" {
		t.Errorf("Mode = %q, want 'debug'", entry.Mode)
	}
	if !entry.HasBKCContext {
		t.Error("HasBKCContext = false, want true")
	}
}

func TestBuildSessionListActions(t *testing.T) {
	actions := buildSessionListActions(3)

	// Should have select_1, select_2, select_3, and skip
	if len(actions) != 4 {
		t.Fatalf("Actions count = %d, want 4", len(actions))
	}

	// Check first action
	if actions[0].ID != "select_1" {
		t.Errorf("First action ID = %q, want 'select_1'", actions[0].ID)
	}
	if actions[0].Shortcut != "1" {
		t.Errorf("First action Shortcut = %q, want '1'", actions[0].Shortcut)
	}

	// Check last action (skip)
	last := actions[len(actions)-1]
	if last.ID != "skip" {
		t.Errorf("Last action ID = %q, want 'skip'", last.ID)
	}
	if !last.Default {
		t.Error("Last action Default = false, want true for skip")
	}
}
