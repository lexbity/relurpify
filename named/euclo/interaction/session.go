package interaction

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime/statebus"
)

// SessionResume holds the information needed to resume an interactive session.
type SessionResume struct {
	Mode            string            `json:"mode"`
	LastPhase       string            `json:"last_phase"`
	CompletedPhases []string          `json:"completed_phases"`
	SkippedPhases   []string          `json:"skipped_phases"`
	PhaseStates     map[string]any    `json:"phase_states"`
	Selections      map[string]string `json:"selections"`
	HasArtifacts    bool              `json:"has_artifacts"`
	ArtifactKinds   []string          `json:"artifact_kinds"`
}

// ExtractSessionResume builds a SessionResume from persisted state.
// Returns nil if no interaction state is found.
func ExtractSessionResume(state *core.Context) *SessionResume {
	if state == nil {
		return nil
	}
	raw, ok := statebus.GetAny(state, "euclo.interaction_state")
	if !ok || raw == nil {
		return nil
	}

	// The stored value may be an InteractionState struct or a map[string]any
	// (after JSON round-trip through persistence).
	switch typed := raw.(type) {
	case InteractionState:
		return sessionResumeFromState(typed, state)
	case map[string]any:
		is := InteractionState{
			PhaseStates: make(map[string]any),
			Selections:  make(map[string]string),
		}
		if mode, ok := typed["mode"].(string); ok {
			is.Mode = mode
		}
		if phase, ok := typed["current_phase"].(string); ok {
			is.CurrentPhase = phase
		}
		if ps, ok := typed["phase_states"].(map[string]any); ok {
			is.PhaseStates = ps
		}
		if sel, ok := typed["selections"].(map[string]any); ok {
			for k, v := range sel {
				if s, ok := v.(string); ok {
					is.Selections[k] = s
				}
			}
		}
		if skipped, ok := typed["skipped_phases"].([]any); ok {
			for _, v := range skipped {
				if s, ok := v.(string); ok {
					is.SkippedPhases = append(is.SkippedPhases, s)
				}
			}
		}
		for _, key := range []string{"phases_executed", "completed_phases", "phases_completed"} {
			if values, ok := typed[key].([]any); ok {
				for _, v := range values {
					if s, ok := v.(string); ok {
						is.PhasesExecuted = append(is.PhasesExecuted, s)
					}
				}
				if len(is.PhasesExecuted) > 0 {
					break
				}
			}
		}
		return sessionResumeFromState(is, state)
	}
	return nil
}

func sessionResumeFromState(is InteractionState, state *core.Context) *SessionResume {
	if is.Mode == "" {
		return nil
	}

	resume := &SessionResume{
		Mode:            is.Mode,
		LastPhase:       is.CurrentPhase,
		CompletedPhases: append([]string{}, is.PhasesExecuted...),
		SkippedPhases:   is.SkippedPhases,
		PhaseStates:     is.PhaseStates,
		Selections:      is.Selections,
	}

	// Collect artifact kinds from state.
	if state != nil {
		if raw, ok := statebus.GetAny(state, "euclo.artifacts"); ok && raw != nil {
			resume.HasArtifacts = true
			// Type may vary ([]Artifact or []any after persistence).
			switch typed := raw.(type) {
			case []euclotypes.Artifact:
				for _, item := range typed {
					if item.Kind != "" {
						resume.ArtifactKinds = append(resume.ArtifactKinds, string(item.Kind))
					}
				}
			case []any:
				for _, item := range typed {
					if m, ok := item.(map[string]any); ok {
						if kind, ok := m["kind"].(string); ok {
							resume.ArtifactKinds = append(resume.ArtifactKinds, kind)
						}
					}
				}
			}
		}
	}

	return resume
}

// BuildResumeFrame creates a FrameQuestion asking the user whether to resume.
func BuildResumeFrame(resume *SessionResume) InteractionFrame {
	question := fmt.Sprintf("Resume %s mode from %s phase?", resume.Mode, resume.LastPhase)

	return InteractionFrame{
		Kind:  FrameSessionResume,
		Mode:  resume.Mode,
		Phase: "resume",
		Content: QuestionContent{
			Question: question,
			Options: []QuestionOption{
				{ID: "resume", Label: "Resume", Description: fmt.Sprintf("Continue from %s", resume.LastPhase)},
				{ID: "restart", Label: "Start over", Description: "Begin from the first phase"},
				{ID: "switch", Label: "Switch mode", Description: "Choose a different mode"},
			},
		},
		Actions: []ActionSlot{
			{ID: "resume", Label: "Resume", Kind: ActionConfirm, Default: true},
			{ID: "restart", Label: "Start over", Kind: ActionConfirm},
			{ID: "switch", Label: "Switch mode", Kind: ActionSelect},
		},
		Metadata: FrameMetadata{
			Timestamp: time.Now(),
		},
	}
}

// ApplySessionResume restores machine state from a SessionResume.
// Completed phases are marked so the machine can skip them.
func ApplySessionResume(machine *PhaseMachine, resume *SessionResume) {
	if machine == nil || resume == nil {
		return
	}
	// Restore phase states.
	for k, v := range resume.PhaseStates {
		machine.State()[k] = v
	}
	// Mark completed phases.
	machine.State()["session.resumed"] = true
	machine.State()["session.last_phase"] = resume.LastPhase
	if len(resume.CompletedPhases) > 0 {
		machine.State()["session.completed_phases"] = resume.CompletedPhases
	}
	if resume.LastPhase != "" {
		_ = machine.JumpToPhase(resume.LastPhase)
	}
}

// HandleResumeResponse processes the user's response to the resume frame.
// Returns the action: "resume", "restart", or "switch".
func HandleResumeResponse(resp UserResponse) string {
	switch resp.ActionID {
	case "resume", "restart", "switch":
		return resp.ActionID
	default:
		return "resume" // default to resume
	}
}

// TelemetryEmitter wraps a FrameEmitter to emit telemetry events
// for each frame and response.
type TelemetryEmitter struct {
	Inner     FrameEmitter
	Telemetry *InteractionTelemetry
	mode      string
}

// NewTelemetryEmitter creates a telemetry-emitting wrapper.
func NewTelemetryEmitter(inner FrameEmitter, telemetry *InteractionTelemetry) *TelemetryEmitter {
	return &TelemetryEmitter{Inner: inner, Telemetry: telemetry}
}

// Emit delegates to inner and records telemetry.
func (e *TelemetryEmitter) Emit(ctx context.Context, frame InteractionFrame) error {
	e.mode = frame.Mode
	if e.Telemetry != nil {
		e.Telemetry.EmitFrame(frame)
	}
	return e.Inner.Emit(ctx, frame)
}

// AwaitResponse delegates to inner and records telemetry with response time.
func (e *TelemetryEmitter) AwaitResponse(ctx context.Context) (UserResponse, error) {
	start := time.Now()
	resp, err := e.Inner.AwaitResponse(ctx)
	if err == nil && e.Telemetry != nil {
		phase := ""
		// Best-effort phase extraction — not critical.
		if noop, ok := e.Inner.(*NoopEmitter); ok && len(noop.Frames) > 0 {
			phase = noop.Frames[len(noop.Frames)-1].Phase
		}
		e.Telemetry.EmitResponse(resp, phase, e.mode, time.Since(start))
	}
	return resp, err
}
