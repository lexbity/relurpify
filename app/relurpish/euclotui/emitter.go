package euclotui

import (
	"context"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	tea "github.com/charmbracelet/bubbletea"
)

// TUIFrameEmitter implements interaction.FrameEmitter by rendering frames into
// the TUI feed and collecting user responses via Bubble Tea messages.
type TUIFrameEmitter struct {
	program    *tea.Program
	responseCh chan interaction.UserResponse
	frames     []interaction.InteractionFrame
}

// NewTUIFrameEmitter creates a FrameEmitter wired to the given tea.Program.
func NewTUIFrameEmitter(program *tea.Program) *TUIFrameEmitter {
	return &TUIFrameEmitter{
		program:    program,
		responseCh: make(chan interaction.UserResponse, 1),
	}
}

// Emit renders the interaction frame into the TUI feed and pushes an
// interaction notification with the frame's action slots.
func (e *TUIFrameEmitter) Emit(_ context.Context, frame interaction.InteractionFrame) error {
	e.frames = append(e.frames, frame)

	// Render frame into a feed message.
	msg := RenderInteractionFrame(frame)
	if e.program != nil {
		e.program.Send(tui.EucloFrameMsg{Msg: msg, Frame: frame})
	}
	return nil
}

// AwaitResponse blocks until the user responds to the current interaction
// notification. Returns the selected action or freetext input.
func (e *TUIFrameEmitter) AwaitResponse(ctx context.Context) (interaction.UserResponse, error) {
	select {
	case <-ctx.Done():
		return interaction.UserResponse{}, ctx.Err()
	case resp := <-e.responseCh:
		return resp, nil
	}
}

// Resolve sends a user response to the emitter (called by the notification bar
// or input bar when the user selects an action). Implements tui.EucloEmitter.
func (e *TUIFrameEmitter) Resolve(resp interaction.UserResponse) {
	select {
	case e.responseCh <- resp:
	default:
		// Drop if no one is waiting — shouldn't happen in normal flow.
	}
}

// Frames returns the recorded frames (useful for testing).
func (e *TUIFrameEmitter) Frames() []interaction.InteractionFrame {
	return e.frames
}
