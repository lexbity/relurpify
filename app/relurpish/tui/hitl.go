package tui

import (
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	tea "github.com/charmbracelet/bubbletea"
)

// HITLServiceIface is the interface for the HITL approval service.
// It is exported so that euclotui can reference it in the ChatPaner interface.
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
