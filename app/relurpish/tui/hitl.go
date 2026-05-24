package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	tea "github.com/charmbracelet/bubbletea"
)

// HITLServiceIface is the interface for the HITL approval service.
type HITLServiceIface interface {
	PendingHITL() []*authorization.PermissionRequest
	ApproveHITL(requestID, approver string, scope authorization.GrantScope, duration time.Duration) error
	DenyHITL(requestID, reason string) error
	SubscribeHITL() (<-chan authorization.HITLEvent, func())
}

type hitlEventMsg struct{ event authorization.HITLEvent }

func listenHITLEvents(ch <-chan authorization.HITLEvent) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return hitlEventMsg{event: ev}
	}
}

type hitlResolvedMsg struct {
	requestID string
	approved  bool
	err       error
}

func approveHITLCmd(svc HITLServiceIface, requestID string, scope authorization.GrantScope) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return hitlResolvedMsg{requestID: requestID, approved: true, err: fmt.Errorf("hitl service unavailable")}
		}
		err := svc.ApproveHITL(requestID, "tui", scope, 0)
		return hitlResolvedMsg{requestID: requestID, approved: true, err: err}
	}
}

func denyHITLCmd(svc HITLServiceIface, requestID string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return hitlResolvedMsg{requestID: requestID, approved: false, err: fmt.Errorf("hitl service unavailable")}
		}
		err := svc.DenyHITL(requestID, "denied in TUI")
		return hitlResolvedMsg{requestID: requestID, approved: false, err: err}
	}
}

func (m *RootModel) trackInteractionFrame(notificationID string, frame interaction.InteractionFrame) {
	if m == nil {
		return
	}
	if m.interactionFrames == nil {
		m.interactionFrames = make(map[string]*interaction.InteractionFrame)
	}
	frameCopy := frame
	if notificationID = strings.TrimSpace(notificationID); notificationID != "" {
		m.interactionFrames[notificationID] = &frameCopy
	}
	if frameID := strings.TrimSpace(frame.ID); frameID != "" {
		m.interactionFrames[frameID] = &frameCopy
	}
}

func (m *RootModel) openInteractionGuidance(notificationID string, frame interaction.InteractionFrame) {
	if m == nil {
		return
	}
	if m.hitlRow != nil && m.hitlRow.Active() {
		return
	}
	question := strings.TrimSpace(frame.Question)
	if question == "" {
		question = frameLabelFromInteraction(frame)
	}

	var slotIDs []string
	var slotNames []string

	if len(frame.Choices) > 0 {
		for _, choice := range frame.Choices {
			slotIDs = append(slotIDs, choice)
			slotNames = append(slotNames, choice)
		}
	} else if len(frame.Slots) > 0 {
		for _, slot := range frame.Slots {
			label := strings.TrimSpace(slot.Label)
			if label == "" {
				label = slot.ID
			}
			slotIDs = append(slotIDs, strings.TrimSpace(slot.ID))
			slotNames = append(slotNames, label)
		}
	}

	if m.hitlRow != nil {
		m.hitlRow.Open(strings.TrimSpace(notificationID), question, slotIDs, slotNames)
	}
}

func (m *RootModel) resolvePendingInteraction(notificationID, choiceID, freetext string) bool {
	if m == nil {
		return false
	}
	if m.interactionFrames == nil {
		m.interactionFrames = make(map[string]*interaction.InteractionFrame)
	}
	requestID := strings.TrimSpace(notificationID)
	if requestID == "" {
		return false
	}
	frame, ok := m.interactionFrames[requestID]
	if !ok || frame == nil {
		return false
	}
	answer := strings.TrimSpace(choiceID)
	if answer == "" {
		answer = strings.TrimSpace(freetext)
	}
	if answer == "" {
		answer = defaultInteractionAnswer(frame)
	}
	extra := map[string]any{
		"notification_id": requestID,
		"frame_id":        strings.TrimSpace(frame.ID),
		"frame_type":      string(frame.Type),
	}
	if strings.TrimSpace(frame.TaskID) != "" {
		extra["task_id"] = strings.TrimSpace(frame.TaskID)
	}
	if strings.TrimSpace(freetext) != "" {
		extra["freetext"] = strings.TrimSpace(freetext)
	}
	frame.SetResponse(answer, extra, "relurpish", time.Now().UTC())
	delete(m.interactionFrames, requestID)
	if frameID := strings.TrimSpace(frame.ID); frameID != "" {
		delete(m.interactionFrames, frameID)
	}
	if m.notifQ != nil {
		m.notifQ.Resolve(requestID)
	}
	m.syncOverlayStack()
	if m.runtime != nil {
		if err := m.runtime.ResolveInteractionFrame(context.Background(), frame.TaskID, frame.ID, answer, freetext); err != nil {
			m.addSystemMessage(fmt.Sprintf("Interaction persistence failed: %v", err))
		}
	}
	if answer != "" {
		m.addSystemMessage(fmt.Sprintf("Resolved %s: %s", frameLabelFromInteraction(*frame), answer))
	} else {
		m.addSystemMessage(fmt.Sprintf("Resolved %s", frameLabelFromInteraction(*frame)))
	}
	return true
}

func defaultInteractionAnswer(frame *interaction.InteractionFrame) string {
	if frame == nil {
		return ""
	}
	if slot := strings.TrimSpace(frame.DefaultChoice); slot != "" {
		return slot
	}
	for _, slot := range frame.Slots {
		if slot.Default {
			if id := strings.TrimSpace(slot.ID); id != "" {
				return id
			}
		}
	}
	if len(frame.Slots) > 0 {
		return strings.TrimSpace(frame.Slots[0].ID)
	}
	if len(frame.Choices) > 0 {
		return strings.TrimSpace(frame.Choices[0])
	}
	return ""
}

func (m *RootModel) deferPendingInteraction(notificationID string) bool {
	if m == nil {
		return false
	}
	requestID := strings.TrimSpace(notificationID)
	if requestID == "" {
		return false
	}
	frame, ok := m.interactionFrames[requestID]
	if !ok || frame == nil {
		return false
	}
	delete(m.interactionFrames, requestID)
	if frameID := strings.TrimSpace(frame.ID); frameID != "" {
		delete(m.interactionFrames, frameID)
	}
	if m.notifQ != nil {
		m.notifQ.Resolve(requestID)
	}
	m.addSystemMessage(fmt.Sprintf("Deferred %s", frameLabelFromInteraction(*frame)))
	m.syncOverlayStack()
	return true
}

func frameLabelFromInteraction(frame interaction.InteractionFrame) string {
	frameType := strings.TrimSpace(string(frame.Type))
	if frameType == "" {
		frameType = strings.TrimSpace(string(frame.Kind))
	}
	if frameType == "" {
		frameType = "interaction"
	}
	return prettyFrameLabel(frameType)
}

func prettyFrameLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Fields(strings.ReplaceAll(value, "_", " "))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

// handleHITLEvent processes HITL events from the subscription.
func (m RootModel) handleHITLEvent(msg hitlEventMsg) (RootModel, tea.Cmd) {
	var pending []*authorization.PermissionRequest
	if m.chat != nil {
		if svc := m.chat.HITLService(); svc != nil {
			pending = svc.PendingHITL()
		}
	}
	switch msg.event.Type {
	case authorization.HITLEventRequested:
		req := msg.event.Request
		if req == nil && len(pending) > 0 {
			req = pending[0]
		}
		if req != nil && m.notifQ != nil {
			m.notifQ.PushHITL(req)
		}
	case authorization.HITLEventResolved, authorization.HITLEventExpired:
		if msg.event.Request != nil && m.notifQ != nil {
			m.notifQ.Resolve(msg.event.Request.ID)
		}
		if msg.event.Type == authorization.HITLEventExpired && msg.event.Request != nil {
			reason := msg.event.Error
			if reason == "" {
				reason = "expired"
			}
			m.addSystemMessage(fmt.Sprintf("Permission %s expired: %s", msg.event.Request.ID, reason))
		}
	}
	return m, listenHITLEvents(m.hitlCh)
}

// handleHITLResolved processes HITL resolution messages.
func (m RootModel) handleHITLResolved(msg hitlResolvedMsg) (RootModel, tea.Cmd) {
	if m.notifQ != nil {
		m.notifQ.Resolve(msg.requestID)
	}
	if msg.err != nil {
		m.addSystemMessage(fmt.Sprintf("HITL %s failed: %v", msg.requestID, msg.err))
	} else if msg.approved {
		m.addSystemMessage(fmt.Sprintf("Approved %s", msg.requestID))
	} else {
		m.addSystemMessage(fmt.Sprintf("Denied %s", msg.requestID))
	}
	return m, listenHITLEvents(m.hitlCh)
}
