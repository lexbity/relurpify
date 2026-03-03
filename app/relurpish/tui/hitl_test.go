package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
)

type fakeHITL struct {
	pending []*runtime.PermissionRequest
	ch      chan runtime.HITLEvent

	approved []string
	denied   []string
}

func newFakeHITL() *fakeHITL {
	return &fakeHITL{
		ch: make(chan runtime.HITLEvent, 16),
	}
}

func (f *fakeHITL) PendingHITL() []*runtime.PermissionRequest {
	out := make([]*runtime.PermissionRequest, len(f.pending))
	copy(out, f.pending)
	return out
}

func (f *fakeHITL) ApproveHITL(requestID, _ string, _ runtime.GrantScope, _ time.Duration) error {
	f.approved = append(f.approved, requestID)
	f.pending = removeRequest(f.pending, requestID)
	f.ch <- runtime.HITLEvent{
		Type:    runtime.HITLEventResolved,
		Request: &runtime.PermissionRequest{ID: requestID},
		Decision: &runtime.PermissionDecision{
			RequestID: requestID,
			Approved:  true,
		},
	}
	return nil
}

func (f *fakeHITL) DenyHITL(requestID, _ string) error {
	f.denied = append(f.denied, requestID)
	f.pending = removeRequest(f.pending, requestID)
	f.ch <- runtime.HITLEvent{
		Type:    runtime.HITLEventResolved,
		Request: &runtime.PermissionRequest{ID: requestID},
		Decision: &runtime.PermissionDecision{
			RequestID: requestID,
			Approved:  false,
			Reason:    "denied",
		},
	}
	return nil
}

func (f *fakeHITL) SubscribeHITL() (<-chan runtime.HITLEvent, func()) {
	return f.ch, func() {}
}

func removeRequest(reqs []*runtime.PermissionRequest, id string) []*runtime.PermissionRequest {
	for i, r := range reqs {
		if r != nil && r.ID == id {
			return append(reqs[:i], reqs[i+1:]...)
		}
	}
	return reqs
}

// TestHITLEventPushesNotification verifies that a HITLEventRequested event
// causes the notification queue to receive a HITL item.
func TestHITLEventPushesNotification(t *testing.T) {
	hitl := newFakeHITL()
	req := &runtime.PermissionRequest{
		ID:            "hitl-1",
		Permission:    core.PermissionDescriptor{Action: "file_matrix:write", Resource: "src/main.rs"},
		Justification: "file permission matrix",
	}
	hitl.pending = []*runtime.PermissionRequest{req}

	notifQ := &NotificationQueue{}
	pane := NewChatPane(nil, &AgentContext{}, &Session{}, notifQ)
	pane.hitlSvc = hitl

	event := hitlEventMsg{event: runtime.HITLEvent{Type: runtime.HITLEventRequested, Request: req}}
	_, _ = pane.Update(event)

	if notifQ.Len() == 0 {
		t.Fatalf("expected notification pushed, got 0")
	}
	n, ok := notifQ.Current()
	if !ok {
		t.Fatalf("expected current notification")
	}
	if n.Kind != NotifKindHITL {
		t.Fatalf("expected HITL kind, got %s", n.Kind)
	}
	if n.ID != "hitl-1" {
		t.Fatalf("expected notification ID hitl-1, got %s", n.ID)
	}
}

// TestNotificationBarApproveKey verifies that pressing "y" on a HITL
// notification emits NotifHITLApproveMsg.
func TestNotificationBarApproveKey(t *testing.T) {
	notifQ := &NotificationQueue{}
	notifQ.Push(NotificationItem{
		ID:   "hitl-2",
		Kind: NotifKindHITL,
		Msg:  "Approve hitl-2?",
	})

	nb := NewNotificationBar(notifQ)
	_, cmd := nb.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatalf("expected approve cmd")
	}
	msg := cmd()
	approveMsg, ok := msg.(NotifHITLApproveMsg)
	if !ok {
		t.Fatalf("expected NotifHITLApproveMsg, got %T", msg)
	}
	if approveMsg.ID != "hitl-2" {
		t.Fatalf("expected ID hitl-2, got %s", approveMsg.ID)
	}
}

// TestNotificationBarDenyKey verifies that pressing "n" on a HITL
// notification emits NotifHITLDenyMsg.
func TestNotificationBarDenyKey(t *testing.T) {
	notifQ := &NotificationQueue{}
	notifQ.Push(NotificationItem{
		ID:   "hitl-3",
		Kind: NotifKindHITL,
		Msg:  "Approve hitl-3?",
	})

	nb := NewNotificationBar(notifQ)
	_, cmd := nb.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd == nil {
		t.Fatalf("expected deny cmd")
	}
	msg := cmd()
	denyMsg, ok := msg.(NotifHITLDenyMsg)
	if !ok {
		t.Fatalf("expected NotifHITLDenyMsg, got %T", msg)
	}
	if denyMsg.ID != "hitl-3" {
		t.Fatalf("expected ID hitl-3, got %s", denyMsg.ID)
	}
}

// TestApproveHITLRootCmd exercises the approve command reaching the fake HITL service.
func TestApproveHITLRootCmd(t *testing.T) {
	hitl := newFakeHITL()
	req := &runtime.PermissionRequest{
		ID:            "hitl-approve",
		Permission:    core.PermissionDescriptor{Action: "bash:exec", Resource: "cargo build"},
		Justification: "build test",
	}
	hitl.pending = []*runtime.PermissionRequest{req}

	cmd := approveHITLRootCmd(hitl, req.ID, runtime.GrantScopeOneTime)
	if cmd == nil {
		t.Fatalf("expected command")
	}
	msg := cmd()
	resolved, ok := msg.(hitlResolvedMsg)
	if !ok {
		t.Fatalf("expected hitlResolvedMsg, got %T", msg)
	}
	if !resolved.approved {
		t.Fatalf("expected approved=true")
	}
	if resolved.err != nil {
		t.Fatalf("expected no error, got %v", resolved.err)
	}
	if len(hitl.approved) != 1 || hitl.approved[0] != req.ID {
		t.Fatalf("expected approved %s, got %v", req.ID, hitl.approved)
	}
}

// TestDenyHITLRootCmd exercises the deny command reaching the fake HITL service.
func TestDenyHITLRootCmd(t *testing.T) {
	hitl := newFakeHITL()
	req := &runtime.PermissionRequest{
		ID:            "hitl-deny",
		Permission:    core.PermissionDescriptor{Action: "file:write", Resource: "config.toml"},
		Justification: "write test",
	}
	hitl.pending = []*runtime.PermissionRequest{req}

	cmd := denyHITLRootCmd(hitl, req.ID)
	if cmd == nil {
		t.Fatalf("expected command")
	}
	msg := cmd()
	resolved, ok := msg.(hitlResolvedMsg)
	if !ok {
		t.Fatalf("expected hitlResolvedMsg, got %T", msg)
	}
	if resolved.approved {
		t.Fatalf("expected approved=false")
	}
	if len(hitl.denied) != 1 || hitl.denied[0] != req.ID {
		t.Fatalf("expected denied %s, got %v", req.ID, hitl.denied)
	}
}
