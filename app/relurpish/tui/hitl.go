package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/framework/runtime"
)

type hitlService interface {
	PendingHITL() []*runtime.PermissionRequest
	ApproveHITL(requestID, approver string, scope runtime.GrantScope, duration time.Duration) error
	DenyHITL(requestID, reason string) error
	SubscribeHITL() (<-chan runtime.HITLEvent, func())
}

type hitlEventMsg struct{ event runtime.HITLEvent }

func listenHITLEvents(ch <-chan runtime.HITLEvent) tea.Cmd {
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

func approveHITLCmd(svc hitlService, requestID string, scope runtime.GrantScope) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return hitlResolvedMsg{requestID: requestID, approved: true, err: fmt.Errorf("hitl service unavailable")}
		}
		err := svc.ApproveHITL(requestID, "tui", scope, 0)
		return hitlResolvedMsg{requestID: requestID, approved: true, err: err}
	}
}

func denyHITLCmd(svc hitlService, requestID string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return hitlResolvedMsg{requestID: requestID, approved: false, err: fmt.Errorf("hitl service unavailable")}
		}
		err := svc.DenyHITL(requestID, "denied in TUI")
		return hitlResolvedMsg{requestID: requestID, approved: false, err: err}
	}
}
