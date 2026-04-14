package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ──────────────────────────────────────────────────────────────
// Interaction notification kind
// ──────────────────────────────────────────────────────────────

const NotifKindInteraction NotificationKind = "interaction"

// PushInteraction pushes an interaction notification with the frame's actions.
func (q *NotificationQueue) PushInteraction(frame interaction.InteractionFrame) string {
	id := generateID()
	q.Push(notificationItemFromFrame(id, NotifKindInteraction, frame, nil))
	return id
}

func notificationItemFromFrame(id string, kind NotificationKind, frame interaction.InteractionFrame, extra map[string]string) NotificationItem {
	itemExtra := serializeFrameActions(frame)
	for key, value := range extra {
		itemExtra[key] = value
	}
	return NotificationItem{
		ID:    id,
		Kind:  kind,
		Msg:   fmt.Sprintf("[%s/%s] %s", frame.Mode, frame.Phase, frameLabel(frame)),
		Extra: itemExtra,
	}
}

func serializeFrameActions(frame interaction.InteractionFrame) map[string]string {
	extra := map[string]string{
		"mode":  frame.Mode,
		"phase": frame.Phase,
		"kind":  string(frame.Kind),
	}
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
	return extra
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
	case interaction.FrameSessionList:
		return "Resume Session"
	case interaction.FrameSessionListEmpty:
		return "Resume Session"
	case interaction.FrameSessionResuming:
		return "Resuming"
	case interaction.FrameSessionResumeError:
		return "Resume Error"
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

func notificationAllowsFreetext(item NotificationItem) bool {
	count, _ := strconv.Atoi(item.Extra["action_count"])
	for i := 0; i < count; i++ {
		if item.Extra[fmt.Sprintf("action_%d_kind", i)] == string(interaction.ActionFreetext) {
			return true
		}
	}
	return false
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
