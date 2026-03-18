package interaction

import "context"

// FrameEmitter is the boundary between Euclo and the UX layer.
// Euclo calls Emit to send a frame and AwaitResponse to block for user input.
type FrameEmitter interface {
	Emit(ctx context.Context, frame InteractionFrame) error
	AwaitResponse(ctx context.Context) (UserResponse, error)
}

// UserResponse carries the user's response to an interaction frame.
type UserResponse struct {
	ActionID   string   `json:"action_id"`
	Text       string   `json:"text,omitempty"`
	Selections []string `json:"selections,omitempty"`
}

// NoopEmitter auto-selects default actions and advances through phases
// without user input. Used for batch/non-interactive execution and testing.
type NoopEmitter struct {
	// Frames records all emitted frames for test inspection.
	Frames []InteractionFrame
}

// Emit records the frame and returns immediately.
func (e *NoopEmitter) Emit(_ context.Context, frame InteractionFrame) error {
	e.Frames = append(e.Frames, frame)
	return nil
}

// AwaitResponse returns a frame-kind-aware response for non-interactive execution.
// Smart defaults per kind:
//   - Proposal → confirm (auto-approve)
//   - Question → select first option
//   - Candidates → select recommended candidate
//   - Draft → accept as-is
//   - Transition → reject (stay in current mode)
//   - Result/Status/Summary/Help → advance
func (e *NoopEmitter) AwaitResponse(ctx context.Context) (UserResponse, error) {
	if err := ctx.Err(); err != nil {
		return UserResponse{}, err
	}
	if len(e.Frames) == 0 {
		return UserResponse{}, nil
	}
	last := e.Frames[len(e.Frames)-1]

	// Explicit default action always wins.
	if def := last.DefaultAction(); def != nil {
		return UserResponse{ActionID: def.ID}, nil
	}

	// Frame-kind-specific smart defaults when no explicit default is set.
	switch last.Kind {
	case FrameTransition:
		// Non-interactive: reject transitions to preserve straight-line execution.
		if a := last.ActionByID("reject"); a != nil {
			return UserResponse{ActionID: "reject"}, nil
		}
	case FrameCandidates:
		// Select recommended candidate if available.
		if content, ok := last.Content.(CandidatesContent); ok && content.RecommendedID != "" {
			return UserResponse{ActionID: content.RecommendedID}, nil
		}
	case FrameProposal:
		// Auto-confirm proposals.
		if a := last.ActionByID("confirm"); a != nil {
			return UserResponse{ActionID: "confirm"}, nil
		}
	case FrameDraft:
		// Accept draft as-is.
		if a := last.ActionByID("accept"); a != nil {
			return UserResponse{ActionID: "accept"}, nil
		}
	case FrameResult, FrameStatus, FrameSummary, FrameHelp:
		// Informational frames — advance.
		if a := last.ActionByID("continue"); a != nil {
			return UserResponse{ActionID: "continue"}, nil
		}
	}

	// Fallback: first action.
	if len(last.Actions) > 0 {
		return UserResponse{ActionID: last.Actions[0].ID}, nil
	}
	return UserResponse{}, nil
}

// Reset clears recorded frames.
func (e *NoopEmitter) Reset() {
	e.Frames = e.Frames[:0]
}
