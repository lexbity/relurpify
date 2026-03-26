package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/framework/guidance"
)

type guidanceService interface {
	PendingGuidance() []*guidance.GuidanceRequest
	ResolveGuidance(requestID, choiceID, freetext string) error
	SubscribeGuidance() (<-chan guidance.GuidanceEvent, func())
	PendingDeferrals() []guidance.EngineeringObservation
	ResolveDeferral(observationID string) error
}

type guidanceEventMsg struct{ event guidance.GuidanceEvent }

type guidanceSubscribedMsg struct {
	ch    <-chan guidance.GuidanceEvent
	unsub func()
}

type guidanceResolvedMsg struct {
	requestID string
	err       error
}

type NotifGuidanceResolveMsg struct {
	RequestID string
	ChoiceID  string
	Freetext  string
}

func listenGuidanceEvents(ch <-chan guidance.GuidanceEvent) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return guidanceEventMsg{event: ev}
	}
}

func guidanceRequestCmd(svc guidanceService, requestID, choiceID, freetext string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return guidanceResolvedMsg{requestID: requestID, err: fmt.Errorf("guidance service unavailable")}
		}
		return guidanceResolvedMsg{
			requestID: requestID,
			err:       svc.ResolveGuidance(requestID, choiceID, freetext),
		}
	}
}

func (q *NotificationQueue) PushGuidance(req *guidance.GuidanceRequest) {
	if q == nil || req == nil {
		return
	}
	frame := guidance.GuidanceRequestToFrame(*req)
	q.Push(notificationItemFromFrame(req.ID, NotifKindGuidance, frame, map[string]string{
		"guidance_request_id": req.ID,
	}))
}

func formatPendingGuidanceSummary(pending []*guidance.GuidanceRequest) string {
	if len(pending) == 0 {
		return "No pending guidance requests"
	}
	var b strings.Builder
	b.WriteString("Pending guidance requests:\n")
	for _, req := range pending {
		if req == nil {
			continue
		}
		b.WriteString(fmt.Sprintf(" - %s [%s] %s\n", req.ID, req.Kind, req.Title))
	}
	return b.String()
}

func formatDeferredObservationsSummary(observations []guidance.EngineeringObservation) string {
	if len(observations) == 0 {
		return "No deferred guidance observations"
	}
	var b strings.Builder
	b.WriteString("Deferred guidance observations:\n")
	for _, obs := range observations {
		b.WriteString(fmt.Sprintf(" - %s [%s] %s", obs.ID, obs.GuidanceKind, obs.Title))
		if obs.BlastRadius > 0 {
			b.WriteString(fmt.Sprintf(" blast_radius=%d", obs.BlastRadius))
		}
		b.WriteByte('\n')
		if obs.Description != "" {
			b.WriteString(fmt.Sprintf("   %s\n", obs.Description))
		}
	}
	return b.String()
}
