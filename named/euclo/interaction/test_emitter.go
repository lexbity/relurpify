package interaction

import (
	"context"
	"fmt"
	"sync"
)

// ScriptedResponse defines a canned response for a specific phase (or any phase).
type ScriptedResponse struct {
	Phase    string // phase ID to match, or "" for any phase
	Kind     string // frame kind to match, or "" for any kind
	ActionID string // action to select
	Text     string // freetext response
}

// TestFrameEmitter is a FrameEmitter for testing that records all frames
// and responds with scripted or auto-default responses.
type TestFrameEmitter struct {
	mu       sync.Mutex
	frames   []InteractionFrame
	script   []ScriptedResponse
	scriptAt int

	// DefaultActionID is returned when no script entry matches.
	// If empty, the frame's default action is used.
	DefaultActionID string

	// AutoSkip when true returns "skip" for any frame with a skip action.
	AutoSkip bool

	// FrameCallback is called for each emitted frame, if set.
	FrameCallback func(frame InteractionFrame)
}

// NewTestFrameEmitter creates a test emitter with an optional script.
func NewTestFrameEmitter(script ...ScriptedResponse) *TestFrameEmitter {
	return &TestFrameEmitter{script: script}
}

// Emit records the frame.
func (e *TestFrameEmitter) Emit(_ context.Context, frame InteractionFrame) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frames = append(e.frames, frame)
	if e.FrameCallback != nil {
		e.FrameCallback(frame)
	}
	return nil
}

// AwaitResponse returns a scripted response if one matches, otherwise the default.
func (e *TestFrameEmitter) AwaitResponse(ctx context.Context) (UserResponse, error) {
	if err := ctx.Err(); err != nil {
		return UserResponse{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.frames) == 0 {
		return UserResponse{}, nil
	}
	last := e.frames[len(e.frames)-1]

	// Try scripted responses.
	if e.scriptAt < len(e.script) {
		entry := e.script[e.scriptAt]
		if matchesScript(entry, last) {
			e.scriptAt++
			return UserResponse{
				ActionID: entry.ActionID,
				Text:     entry.Text,
			}, nil
		}
	}

	// Auto-skip.
	if e.AutoSkip {
		if action := last.ActionByID("skip"); action != nil {
			return UserResponse{ActionID: "skip"}, nil
		}
	}

	// Explicit default action ID.
	if e.DefaultActionID != "" {
		return UserResponse{ActionID: e.DefaultActionID}, nil
	}

	// Frame's default action.
	if def := last.DefaultAction(); def != nil {
		return UserResponse{ActionID: def.ID}, nil
	}
	if len(last.Actions) > 0 {
		return UserResponse{ActionID: last.Actions[0].ID}, nil
	}
	return UserResponse{}, nil
}

// Frames returns all recorded frames.
func (e *TestFrameEmitter) Frames() []InteractionFrame {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]InteractionFrame, len(e.frames))
	copy(out, e.frames)
	return out
}

// FrameCount returns the number of recorded frames.
func (e *TestFrameEmitter) FrameCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.frames)
}

// FramesOfKind returns frames matching the given kind.
func (e *TestFrameEmitter) FramesOfKind(kind FrameKind) []InteractionFrame {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []InteractionFrame
	for _, f := range e.frames {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

// FramesByPhase returns frames emitted during the given phase.
func (e *TestFrameEmitter) FramesByPhase(phase string) []InteractionFrame {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []InteractionFrame
	for _, f := range e.frames {
		if f.Phase == phase {
			out = append(out, f)
		}
	}
	return out
}

// Reset clears all recorded frames and resets the script position.
func (e *TestFrameEmitter) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frames = e.frames[:0]
	e.scriptAt = 0
}

// AssertFrameCount fails the test if frame count doesn't match.
func (e *TestFrameEmitter) AssertFrameCount(expected int) error {
	got := e.FrameCount()
	if got != expected {
		return fmt.Errorf("frame count: got %d, want %d", got, expected)
	}
	return nil
}

// AssertHasFrameKind fails if no frame of the given kind was emitted.
func (e *TestFrameEmitter) AssertHasFrameKind(kind FrameKind) error {
	if len(e.FramesOfKind(kind)) == 0 {
		return fmt.Errorf("expected frame kind %q, none emitted", kind)
	}
	return nil
}

// AssertNoFrameKind fails if any frame of the given kind was emitted.
func (e *TestFrameEmitter) AssertNoFrameKind(kind FrameKind) error {
	if len(e.FramesOfKind(kind)) > 0 {
		return fmt.Errorf("expected no frame kind %q, found %d", kind, len(e.FramesOfKind(kind)))
	}
	return nil
}

func matchesScript(entry ScriptedResponse, frame InteractionFrame) bool {
	if entry.Phase != "" && entry.Phase != frame.Phase {
		return false
	}
	if entry.Kind != "" && entry.Kind != string(frame.Kind) {
		return false
	}
	return true
}
