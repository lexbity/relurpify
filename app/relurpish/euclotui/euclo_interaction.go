package euclotui

import (
	"fmt"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

const NotifKindInteraction tui.NotificationKind = "interaction"

// PushInteraction pushes an interaction notification with the frame's slots.
func PushInteraction(q *tui.NotificationQueue, frame interaction.InteractionFrame) string {
	id := tui.GenerateID()
	if q != nil {
		q.Push(notificationItemFromFrame(id, tui.NotifKindInteraction, frame, nil))
	}
	return id
}

func notificationItemFromFrame(id string, kind tui.NotificationKind, frame interaction.InteractionFrame, extra map[string]string) tui.NotificationItem {
	itemExtra := serializeFrameSlots(frame)
	for key, value := range extra {
		itemExtra[key] = value
	}
	return tui.NotificationItem{
		ID:    id,
		Kind:  kind,
		Msg:   frameLabel(frame),
		Extra: itemExtra,
	}
}

func serializeFrameSlots(frame interaction.InteractionFrame) map[string]string {
	slots := frame.Slots
	if len(slots) == 0 {
		slots = frame.Actions
	}
	frameType := frame.Type
	if frameType == "" {
		frameType = frame.Kind
	}
	extra := map[string]string{
		"frame_id":   frame.ID,
		"frame_type": string(frameType),
	}
	for i, slot := range slots {
		for _, prefix := range []string{fmt.Sprintf("slot_%d", i), fmt.Sprintf("action_%d", i)} {
			extra[prefix+"_id"] = slot.ID
			extra[prefix+"_label"] = slot.Label
			extra[prefix+"_action"] = slot.Action
			extra[prefix+"_shortcut"] = slot.Shortcut
			extra[prefix+"_kind"] = slot.Kind
			extra[prefix+"_risk"] = slot.Risk
		}
		if slot.Default {
			extra[fmt.Sprintf("slot_%d_default", i)] = "true"
			extra[fmt.Sprintf("action_%d_default", i)] = "true"
			extra["default_slot"] = slot.ID
			extra["default_action"] = slot.ID
		}
	}
	extra["slot_count"] = fmt.Sprintf("%d", len(slots))
	extra["action_count"] = fmt.Sprintf("%d", len(slots))
	return extra
}

func frameLabel(frame interaction.InteractionFrame) string {
	frameType := frame.Type
	if frameType == "" {
		frameType = frame.Kind
	}
	switch frameType {
	case interaction.FrameScopeConfirmation:
		return "scope confirmation"
	case interaction.FrameIntentClarification:
		return "intent clarification"
	case interaction.FrameCandidateSelection:
		return "candidate selection"
	case interaction.FrameRecipeSelection:
		return "recipe selection"
	case interaction.FrameCapabilitySelection:
		return "capability selection"
	case interaction.FrameHITLApproval:
		return "approval required"
	case interaction.FrameSessionResume:
		return "session resume"
	case interaction.FrameBackgroundJobStatus:
		return "background job status"
	case interaction.FrameExecutionSummary:
		return "execution summary"
	case interaction.FrameVerificationEvidence:
		return "verification evidence"
	case interaction.FrameOutcomeFeedback:
		return "outcome feedback"
	default:
		return string(frameType)
	}
}

func notificationAllowsFreetext(item tui.NotificationItem) bool {
	_ = item
	return false
}

// RenderInteractionNotification renders the notification bar for an
// interaction notification.
func RenderInteractionNotification(item tui.NotificationItem) string {
	label := "● " + item.Msg
	rendered := eucloFrameStyle.Render(label)

	countStr := item.Extra["slot_count"]
	count, _ := strconv.Atoi(countStr)
	if count == 0 {
		return rendered
	}

	var actions []interaction.ActionSlot
	for i := 0; i < count; i++ {
		prefix := fmt.Sprintf("slot_%d", i)
		actions = append(actions, interaction.ActionSlot{
			ID:      item.Extra[prefix+"_id"],
			Label:   item.Extra[prefix+"_label"],
			Action:  item.Extra[prefix+"_action"],
			Risk:    item.Extra[prefix+"_risk"],
			Default: item.Extra[prefix+"_default"] == "true",
		})
	}
	return rendered + RenderActionSlots(actions)
}

func RenderActionSlots(actions []interaction.ActionSlot) string {
	if len(actions) == 0 {
		return ""
	}
	var parts []string
	for i, action := range actions {
		label := action.Label
		if label == "" {
			label = action.ID
		}
		if action.Default {
			label = "*" + label
		}
		parts = append(parts, fmt.Sprintf("[%d] %s", i+1, label))
	}
	return " " + strings.Join(parts, " ")
}
