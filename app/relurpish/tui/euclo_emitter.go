package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// TUIFrameEmitter implements interaction.FrameEmitter by rendering frames
// into the TUI feed and collecting user responses via Bubble Tea messages.
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
		e.program.Send(eucloFrameMsg{message: msg, frame: frame})
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
// or input bar when the user selects an action).
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

// ──────────────────────────────────────────────────────────────
// Bubble Tea messages for euclo interaction
// ──────────────────────────────────────────────────────────────

// eucloFrameMsg is sent to the TUI when an interaction frame should be rendered.
type eucloFrameMsg struct {
	message Message
	frame   interaction.InteractionFrame
}

// eucloResponseMsg is sent when the user responds to an interaction notification.
type eucloResponseMsg struct {
	response interaction.UserResponse
}

// eucloPhaseProgressMsg updates the phase progress indicator.
type eucloPhaseProgressMsg struct {
	mode       string
	phaseIndex int
	phaseCount int
	labels     []interaction.PhaseInfo
}

// ──────────────────────────────────────────────────────────────
// Interaction notification kind
// ──────────────────────────────────────────────────────────────

const NotifKindInteraction NotificationKind = "interaction"

// PushInteraction pushes an interaction notification with the frame's actions.
func (q *NotificationQueue) PushInteraction(frame interaction.InteractionFrame) string {
	id := generateID()
	extra := map[string]string{
		"mode":  frame.Mode,
		"phase": frame.Phase,
		"kind":  string(frame.Kind),
	}

	// Serialize action slots into extra for the notification bar.
	for i, a := range frame.Actions {
		prefix := fmt.Sprintf("action_%d", i)
		extra[prefix+"_id"] = a.ID
		extra[prefix+"_label"] = a.Label
		extra[prefix+"_shortcut"] = a.Shortcut
		extra[prefix+"_kind"] = string(a.Kind)
		if a.Default {
			extra[prefix+"_default"] = "true"
			extra["default_action"] = a.ID
		}
	}
	extra["action_count"] = fmt.Sprintf("%d", len(frame.Actions))

	msg := fmt.Sprintf("[%s/%s] %s", frame.Mode, frame.Phase, frameLabel(frame))

	q.Push(NotificationItem{
		ID:   id,
		Kind: NotifKindInteraction,
		Msg:  msg,
		Extra: extra,
	})
	return id
}

func frameLabel(frame interaction.InteractionFrame) string {
	switch frame.Kind {
	case interaction.FrameProposal:
		return "Review proposal"
	case interaction.FrameQuestion:
		return "Answer question"
	case interaction.FrameCandidates:
		return "Select candidate"
	case interaction.FrameComparison:
		return "Compare options"
	case interaction.FrameDraft:
		return "Review draft"
	case interaction.FrameResult:
		return "Review result"
	case interaction.FrameTransition:
		return "Mode transition"
	case interaction.FrameHelp:
		return "Help"
	default:
		return string(frame.Kind)
	}
}

// ──────────────────────────────────────────────────────────────
// Notification bar extension for interaction notifications
// ──────────────────────────────────────────────────────────────

// ResolveInteractionKey handles a key press for an interaction notification.
// Returns the resolved UserResponse and true if handled.
func ResolveInteractionKey(item NotificationItem, key string) (interaction.UserResponse, bool) {
	countStr := item.Extra["action_count"]
	count, _ := strconv.Atoi(countStr)
	if count == 0 {
		return interaction.UserResponse{}, false
	}

	// Check shortcut keys.
	for i := 0; i < count; i++ {
		prefix := fmt.Sprintf("action_%d", i)
		shortcut := item.Extra[prefix+"_shortcut"]
		if shortcut != "" && strings.EqualFold(key, shortcut) {
			return interaction.UserResponse{
				ActionID: item.Extra[prefix+"_id"],
			}, true
		}
	}

	// Check number keys (1-9).
	if num, err := strconv.Atoi(key); err == nil && num >= 1 && num <= count {
		prefix := fmt.Sprintf("action_%d", num-1)
		return interaction.UserResponse{
			ActionID: item.Extra[prefix+"_id"],
		}, true
	}

	// Enter selects default.
	if key == "enter" {
		if defaultID := item.Extra["default_action"]; defaultID != "" {
			return interaction.UserResponse{ActionID: defaultID}, true
		}
		// If no default, select first.
		if count > 0 {
			return interaction.UserResponse{ActionID: item.Extra["action_0_id"]}, true
		}
	}

	return interaction.UserResponse{}, false
}

// RenderInteractionNotification renders the notification bar for an
// interaction notification.
func RenderInteractionNotification(item NotificationItem) string {
	label := "● " + item.Msg
	rendered := eucloNotifStyle.Render(label)

	countStr := item.Extra["action_count"]
	count, _ := strconv.Atoi(countStr)
	if count == 0 {
		return rendered
	}

	var actions []interaction.ActionSlot
	for i := 0; i < count; i++ {
		prefix := fmt.Sprintf("action_%d", i)
		actions = append(actions, interaction.ActionSlot{
			ID:       item.Extra[prefix+"_id"],
			Label:    item.Extra[prefix+"_label"],
			Shortcut: item.Extra[prefix+"_shortcut"],
			Kind:     interaction.ActionKind(item.Extra[prefix+"_kind"]),
			Default:  item.Extra[prefix+"_default"] == "true",
		})
	}

	return rendered + RenderActionSlots(actions)
}
