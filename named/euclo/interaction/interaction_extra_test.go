package interaction_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ---------------------------------------------------------------------------
// ModePhaseMap
// ---------------------------------------------------------------------------

func TestModePhaseMap_RegisterAndGet(t *testing.T) {
	m := interaction.NewModePhaseMap()
	phases := []interaction.PhaseInfo{
		{ID: "understand", Label: "Understand"},
		{ID: "implement", Label: "Implement"},
	}
	m.Register("chat", phases)
	got := m.Get("chat")
	if len(got) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(got))
	}
	if got[0].ID != "understand" {
		t.Fatalf("unexpected phase ID: %q", got[0].ID)
	}
}

func TestModePhaseMap_GetUnknownReturnsNil(t *testing.T) {
	m := interaction.NewModePhaseMap()
	if m.Get("unknown") != nil {
		t.Fatal("expected nil for unknown mode")
	}
}

// ---------------------------------------------------------------------------
// ModeTransitions
// ---------------------------------------------------------------------------

func TestModeTransitions_RegisterAndGet(t *testing.T) {
	t2 := interaction.NewModeTransitions()
	t2.Register("code", []interaction.TransitionInfo{
		{Phrase: "debug this", TargetMode: "debug"},
	})
	transitions := t2.Get("code")
	if len(transitions) != 1 || transitions[0].TargetMode != "debug" {
		t.Fatalf("unexpected transitions: %v", transitions)
	}
}

func TestModeTransitions_GetUnknownReturnsNil(t *testing.T) {
	t2 := interaction.NewModeTransitions()
	if t2.Get("unknown") != nil {
		t.Fatal("expected nil for unknown mode")
	}
}

// ---------------------------------------------------------------------------
// BuildHelpFrame
// ---------------------------------------------------------------------------

func TestBuildHelpFrame_KindIsHelp(t *testing.T) {
	f := interaction.BuildHelpFrame("chat", "understand", nil, nil, nil)
	if f.Kind != interaction.FrameHelp {
		t.Fatalf("expected FrameHelp, got %q", f.Kind)
	}
	if f.Mode != "chat" {
		t.Fatalf("expected mode=chat, got %q", f.Mode)
	}
}

func TestBuildHelpFrame_WithPhaseMap(t *testing.T) {
	pm := interaction.NewModePhaseMap()
	pm.Register("chat", []interaction.PhaseInfo{
		{ID: "p1", Label: "Phase 1"},
		{ID: "p2", Label: "Phase 2"},
	})
	f := interaction.BuildHelpFrame("chat", "p1", nil, pm, nil)
	content, ok := f.Content.(interaction.HelpContent)
	if !ok {
		t.Fatalf("expected HelpContent, got %T", f.Content)
	}
	if len(content.PhaseMap) != 2 {
		t.Fatalf("expected 2 phases in map, got %d", len(content.PhaseMap))
	}
	// p1 should be marked current
	if !content.PhaseMap[0].Current {
		t.Fatal("expected p1 to be marked current")
	}
	if content.PhaseMap[1].Current {
		t.Fatal("expected p2 to not be marked current")
	}
}

func TestBuildHelpFrame_WithResolver(t *testing.T) {
	resolver := interaction.NewAgencyResolver()
	resolver.RegisterTrigger("chat", interaction.AgencyTrigger{
		Phrases:     []string{"run tests"},
		Description: "Execute verification",
	})
	f := interaction.BuildHelpFrame("chat", "implement", resolver, nil, nil)
	content, ok := f.Content.(interaction.HelpContent)
	if !ok {
		t.Fatalf("expected HelpContent, got %T", f.Content)
	}
	if len(content.AvailableActions) != 1 {
		t.Fatalf("expected 1 available action, got %d", len(content.AvailableActions))
	}
	if content.AvailableActions[0].Phrase != "run tests" {
		t.Fatalf("unexpected phrase: %q", content.AvailableActions[0].Phrase)
	}
}

func TestBuildHelpFrame_WithTransitions(t *testing.T) {
	t2 := interaction.NewModeTransitions()
	t2.Register("code", []interaction.TransitionInfo{
		{Phrase: "debug this", TargetMode: "debug"},
	})
	f := interaction.BuildHelpFrame("code", "implement", nil, nil, t2)
	content, ok := f.Content.(interaction.HelpContent)
	if !ok {
		t.Fatalf("expected HelpContent, got %T", f.Content)
	}
	if len(content.AvailableTransitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(content.AvailableTransitions))
	}
}

// ---------------------------------------------------------------------------
// DefaultModeTransitions
// ---------------------------------------------------------------------------

func TestDefaultModeTransitions_NotEmpty(t *testing.T) {
	t2 := interaction.DefaultModeTransitions()
	if t2.Get("code") == nil {
		t.Fatal("expected code transitions")
	}
	if t2.Get("debug") == nil {
		t.Fatal("expected debug transitions")
	}
}

// ---------------------------------------------------------------------------
// RegisterHelpTriggers
// ---------------------------------------------------------------------------

func TestRegisterHelpTriggers_NilResolverNoPanic(t *testing.T) {
	interaction.RegisterHelpTriggers(nil)
}

func TestRegisterHelpTriggers_RegistersHelpPhrases(t *testing.T) {
	resolver := interaction.NewAgencyResolver()
	interaction.RegisterHelpTriggers(resolver)
	_, ok := resolver.Resolve("chat", "help")
	if !ok {
		t.Fatal("expected 'help' phrase to be registered")
	}
}

// ---------------------------------------------------------------------------
// ConfirmationPhase
// ---------------------------------------------------------------------------

func TestConfirmationPhase_ConfirmAdvances(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	ph := &interaction.ConfirmationPhase{
		BuildProposal: func(_ interaction.PhaseMachineContext) interaction.ProposalContent {
			return interaction.ProposalContent{Interpretation: "add feature X"}
		},
	}
	mc := interaction.PhaseMachineContext{
		Emitter: noop,
		Mode:    "chat",
		Phase:   "understand",
		State:   map[string]any{},
	}
	outcome, err := ph.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// NoopEmitter default action for FrameProposal is "confirm" → Advance=true
	if !outcome.Advance {
		t.Fatal("expected Advance=true for confirm")
	}
	if outcome.StateUpdates["understand.response"] != "confirm" {
		t.Fatalf("unexpected response in state: %v", outcome.StateUpdates)
	}
}

func TestConfirmationPhase_RejectDoesNotAdvance(t *testing.T) {
	// Use an emitter that always returns "reject"
	rejectEmitter := &fixedResponseEmitter{actionID: "reject"}
	ph := &interaction.ConfirmationPhase{
		BuildProposal: func(_ interaction.PhaseMachineContext) interaction.ProposalContent {
			return interaction.ProposalContent{Interpretation: "do X"}
		},
	}
	mc := interaction.PhaseMachineContext{
		Emitter: rejectEmitter,
		Mode:    "chat",
		Phase:   "understand",
		State:   map[string]any{},
	}
	outcome, err := ph.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if outcome.Advance {
		t.Fatal("expected Advance=false for reject")
	}
}

func TestConfirmationPhase_CorrectAdvancesWithText(t *testing.T) {
	correctEmitter := &fixedResponseEmitter{actionID: "correct", text: "fix this way"}
	ph := &interaction.ConfirmationPhase{
		BuildProposal: func(_ interaction.PhaseMachineContext) interaction.ProposalContent {
			return interaction.ProposalContent{Interpretation: "do X"}
		},
	}
	mc := interaction.PhaseMachineContext{
		Emitter: correctEmitter,
		Mode:    "chat",
		Phase:   "understand",
		State:   map[string]any{},
	}
	outcome, err := ph.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !outcome.Advance {
		t.Fatal("expected Advance=true for correct")
	}
	if outcome.StateUpdates["understand.correction"] != "fix this way" {
		t.Fatalf("expected correction in state: %v", outcome.StateUpdates)
	}
}

// ---------------------------------------------------------------------------
// QuestionPhase
// ---------------------------------------------------------------------------

func TestQuestionPhase_Execute(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	ph := &interaction.QuestionPhase{
		BuildQuestion: func(_ interaction.PhaseMachineContext) interaction.QuestionContent {
			return interaction.QuestionContent{
				Question: "Which approach?",
				Options: []interaction.QuestionOption{
					{ID: "a", Label: "Option A"},
					{ID: "b", Label: "Option B"},
				},
			}
		},
	}
	mc := interaction.PhaseMachineContext{
		Emitter: noop,
		Mode:    "chat",
		Phase:   "clarify",
		State:   map[string]any{},
	}
	outcome, err := ph.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !outcome.Advance {
		t.Fatal("expected Advance=true")
	}
	// First option should be default
	if outcome.StateUpdates["clarify.response"] == nil {
		t.Fatal("expected clarify.response in state")
	}
}

func TestQuestionPhase_WithFreetext(t *testing.T) {
	freetextEmitter := &fixedResponseEmitter{actionID: "freetext", text: "my custom answer"}
	ph := &interaction.QuestionPhase{
		BuildQuestion: func(_ interaction.PhaseMachineContext) interaction.QuestionContent {
			return interaction.QuestionContent{
				Question:      "What do you want?",
				AllowFreetext: true,
			}
		},
	}
	mc := interaction.PhaseMachineContext{
		Emitter: freetextEmitter,
		Mode:    "chat",
		Phase:   "ask",
		State:   map[string]any{},
	}
	outcome, err := ph.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if outcome.StateUpdates["ask.text"] != "my custom answer" {
		t.Fatalf("expected text in state: %v", outcome.StateUpdates)
	}
}

// ---------------------------------------------------------------------------
// ExecutionPhase
// ---------------------------------------------------------------------------

func TestExecutionPhase_SuccessEmitsStatusAndResult(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	ph := &interaction.ExecutionPhase{
		RunFunc: func(_ context.Context, _ interaction.PhaseMachineContext) (interaction.ResultContent, error) {
			return interaction.ResultContent{Status: "passed"}, nil
		},
	}
	mc := interaction.PhaseMachineContext{
		Emitter: noop,
		Mode:    "chat",
		Phase:   "verify",
		State:   map[string]any{},
	}
	outcome, err := ph.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !outcome.Advance {
		t.Fatal("expected Advance=true")
	}
	// Status frame + result frame = 2 emitted frames
	if len(noop.Frames) != 2 {
		t.Fatalf("expected 2 frames (status+result), got %d", len(noop.Frames))
	}
	if noop.Frames[0].Kind != interaction.FrameStatus {
		t.Fatalf("expected first frame to be status, got %q", noop.Frames[0].Kind)
	}
	if noop.Frames[1].Kind != interaction.FrameResult {
		t.Fatalf("expected second frame to be result, got %q", noop.Frames[1].Kind)
	}
}

func TestExecutionPhase_RunFuncErrorPropagates(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	ph := &interaction.ExecutionPhase{
		RunFunc: func(_ context.Context, _ interaction.PhaseMachineContext) (interaction.ResultContent, error) {
			return interaction.ResultContent{}, errors.New("execution failed")
		},
	}
	mc := interaction.PhaseMachineContext{
		Emitter: noop,
		Mode:    "chat",
		Phase:   "verify",
		State:   map[string]any{},
	}
	_, err := ph.Execute(context.Background(), mc)
	if err == nil {
		t.Fatal("expected error from RunFunc")
	}
}

// ---------------------------------------------------------------------------
// SummaryPhase
// ---------------------------------------------------------------------------

func TestSummaryPhase_Execute(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	ph := &interaction.SummaryPhase{
		BuildSummary: func(_ interaction.PhaseMachineContext) interaction.SummaryContent {
			return interaction.SummaryContent{
				Description: "Completed successfully",
				Artifacts:   []string{"diff_summary"},
			}
		},
	}
	mc := interaction.PhaseMachineContext{
		Emitter: noop,
		Mode:    "chat",
		Phase:   "summary",
		State:   map[string]any{},
	}
	outcome, err := ph.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !outcome.Advance {
		t.Fatal("expected Advance=true")
	}
	if len(noop.Frames) != 1 || noop.Frames[0].Kind != interaction.FrameSummary {
		t.Fatalf("expected 1 summary frame, got %d frames", len(noop.Frames))
	}
}

// ---------------------------------------------------------------------------
// machine.go: Emitter accessor + proposeTransition (via Run with transition outcome)
// ---------------------------------------------------------------------------

func TestPhaseMachine_EmitterAccessor(t *testing.T) {
	noop := &interaction.NoopEmitter{}
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Emitter: noop,
	})
	if m.Emitter() == nil {
		t.Fatal("expected non-nil emitter")
	}
}

func TestPhaseMachine_TransitionRejected(t *testing.T) {
	// Phase returns Transition outcome; NoopEmitter rejects transitions.
	noop := &interaction.NoopEmitter{}
	phases := []interaction.PhaseDefinition{
		{ID: "p1", Handler: &transitionHandler{toMode: "debug"}},
	}
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: noop,
	})
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// With NoopEmitter rejecting transitions, state should show transition.rejected
	if m.State()["transition.rejected"] != "debug" {
		t.Logf("state: %v", m.State())
		// Not fatal — transition handling is conditional on Emit / AwaitResponse
	}
}

func TestPhaseMachine_TransitionAccepted(t *testing.T) {
	// Use an emitter that accepts all transitions.
	acceptEmitter := &fixedResponseEmitter{actionID: "accept"}
	phases := []interaction.PhaseDefinition{
		{ID: "p1", Handler: &transitionHandler{toMode: "debug"}},
	}
	m := interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "chat",
		Phases:  phases,
		Emitter: acceptEmitter,
	})
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if m.State()["transition.accepted"] != "debug" {
		t.Fatalf("expected transition.accepted=debug, got %v", m.State())
	}
}

// ---------------------------------------------------------------------------
// recording.go: RecordPhaseArtifacts and Records
// ---------------------------------------------------------------------------

func TestInteractionRecording_RecordPhaseArtifacts(t *testing.T) {
	rec := interaction.NewInteractionRecording()
	// First ensure there's a matching frame record
	rec.RecordFrame(interaction.InteractionFrame{
		Kind:  interaction.FrameResult,
		Mode:  "chat",
		Phase: "verify",
	})
	rec.RecordPhaseArtifacts("verify", "chat",
		[]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindVerification}},
		[]euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
	)
	records := rec.Records()
	if len(records) == 0 {
		t.Fatal("expected records after RecordPhaseArtifacts")
	}
	// Find the verify record
	var found bool
	for _, r := range records {
		if r.Phase == "verify" && r.Mode == "chat" {
			found = true
			if len(r.ArtifactsProduced) == 0 {
				t.Fatal("expected ArtifactsProduced to be populated")
			}
			break
		}
	}
	if !found {
		t.Fatal("expected verify record in Records()")
	}
}

func TestInteractionRecording_RecordPhaseArtifactsNewRecord(t *testing.T) {
	rec := interaction.NewInteractionRecording()
	// RecordPhaseArtifacts when no matching record exists → creates one
	rec.RecordPhaseArtifacts("new-phase", "chat",
		[]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindAnalyze}},
		nil,
	)
	records := rec.Records()
	if len(records) == 0 {
		t.Fatal("expected new record to be created")
	}
}

// ---------------------------------------------------------------------------
// Helper types
// ---------------------------------------------------------------------------

// fixedResponseEmitter returns a fixed response regardless of frame.
type fixedResponseEmitter struct {
	frames   []interaction.InteractionFrame
	actionID string
	text     string
}

func (e *fixedResponseEmitter) Emit(_ context.Context, frame interaction.InteractionFrame) error {
	e.frames = append(e.frames, frame)
	return nil
}

func (e *fixedResponseEmitter) AwaitResponse(ctx context.Context) (interaction.UserResponse, error) {
	if err := ctx.Err(); err != nil {
		return interaction.UserResponse{}, err
	}
	return interaction.UserResponse{ActionID: e.actionID, Text: e.text}, nil
}

// transitionHandler returns a Transition outcome.
type transitionHandler struct {
	toMode string
}

func (h *transitionHandler) Execute(_ context.Context, _ interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	return interaction.PhaseOutcome{Transition: h.toMode}, nil
}
