package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

type fakeHITL struct {
	pending []*authorization.PermissionRequest
	ch      chan authorization.HITLEvent

	approved []string
	denied   []string
}

func newFakeHITL() *fakeHITL {
	return &fakeHITL{
		ch: make(chan authorization.HITLEvent, 16),
	}
}

func (f *fakeHITL) PendingHITL() []*authorization.PermissionRequest {
	out := make([]*authorization.PermissionRequest, len(f.pending))
	copy(out, f.pending)
	return out
}

func (f *fakeHITL) ApproveHITL(requestID, _ string, _ authorization.GrantScope, _ time.Duration) error {
	f.approved = append(f.approved, requestID)
	f.pending = removeRequest(f.pending, requestID)
	f.ch <- authorization.HITLEvent{
		Type:    authorization.HITLEventResolved,
		Request: &authorization.PermissionRequest{ID: requestID},
		Decision: &authorization.PermissionDecision{
			RequestID: requestID,
			Approved:  true,
		},
	}
	return nil
}

func (f *fakeHITL) DenyHITL(requestID, _ string) error {
	f.denied = append(f.denied, requestID)
	f.pending = removeRequest(f.pending, requestID)
	f.ch <- authorization.HITLEvent{
		Type:    authorization.HITLEventResolved,
		Request: &authorization.PermissionRequest{ID: requestID},
		Decision: &authorization.PermissionDecision{
			RequestID: requestID,
			Approved:  false,
			Reason:    "denied",
		},
	}
	return nil
}

func (f *fakeHITL) SubscribeHITL() (<-chan authorization.HITLEvent, func()) {
	return f.ch, func() {}
}

func removeRequest(reqs []*authorization.PermissionRequest, id string) []*authorization.PermissionRequest {
	for i, r := range reqs {
		if r != nil && r.ID == id {
			return append(reqs[:i], reqs[i+1:]...)
		}
	}
	return reqs
}

// minimalHITLRuntimeAdapter implements RuntimeAdapter for HITL tests.
type minimalHITLRuntimeAdapter struct {
	hitlSvc hitlService
}

func (m *minimalHITLRuntimeAdapter) SubscribeHITL() (<-chan authorization.HITLEvent, func()) {
	return m.hitlSvc.SubscribeHITL()
}
func (m *minimalHITLRuntimeAdapter) PendingHITL() []*authorization.PermissionRequest {
	return m.hitlSvc.PendingHITL()
}
func (m *minimalHITLRuntimeAdapter) ApproveHITL(requestID, approver string, scope authorization.GrantScope, duration time.Duration) error {
	return m.hitlSvc.ApproveHITL(requestID, approver, scope, duration)
}
func (m *minimalHITLRuntimeAdapter) DenyHITL(requestID, reason string) error {
	return m.hitlSvc.DenyHITL(requestID, reason)
}
func (m *minimalHITLRuntimeAdapter) PendingGuidance() []*guidance.GuidanceRequest { return nil }
func (m *minimalHITLRuntimeAdapter) ResolveGuidance(string, string, string) error { return nil }
func (m *minimalHITLRuntimeAdapter) SubscribeGuidance() (<-chan guidance.GuidanceEvent, func()) {
	return nil, func() {}
}
func (m *minimalHITLRuntimeAdapter) PendingDeferrals() []guidance.EngineeringObservation { return nil }
func (m *minimalHITLRuntimeAdapter) ResolveDeferral(string) error                        { return nil }
func (m *minimalHITLRuntimeAdapter) SetInteractionEmitter(e interaction.FrameEmitter) {
	// no-op
}

// Stubs for other RuntimeAdapter methods
func (m *minimalHITLRuntimeAdapter) ExecuteInstruction(context.Context, string, core.TaskType, map[string]any) (*core.Result, error) {
	return nil, nil
}
func (m *minimalHITLRuntimeAdapter) ExecuteInstructionStream(context.Context, string, core.TaskType, map[string]any, func(string)) (*core.Result, error) {
	return nil, nil
}
func (m *minimalHITLRuntimeAdapter) AvailableAgents() []string { return nil }
func (m *minimalHITLRuntimeAdapter) SwitchAgent(string) error  { return nil }
func (m *minimalHITLRuntimeAdapter) SessionInfo() SessionInfo  { return SessionInfo{} }
func (m *minimalHITLRuntimeAdapter) ResolveContextFiles(context.Context, []string) ContextFileResolution {
	return ContextFileResolution{}
}
func (m *minimalHITLRuntimeAdapter) SessionArtifacts() SessionArtifacts              { return SessionArtifacts{} }
func (m *minimalHITLRuntimeAdapter) OllamaModels(context.Context) ([]string, error)  { return nil, nil }
func (m *minimalHITLRuntimeAdapter) RecordingMode() string                           { return "off" }
func (m *minimalHITLRuntimeAdapter) SetRecordingMode(string) error                   { return nil }
func (m *minimalHITLRuntimeAdapter) SaveModel(string) error                          { return nil }
func (m *minimalHITLRuntimeAdapter) ContractSummary() *ContractSummary               { return nil }
func (m *minimalHITLRuntimeAdapter) CapabilityAdmissions() []CapabilityAdmissionInfo { return nil }
func (m *minimalHITLRuntimeAdapter) SaveToolPolicy(string, core.AgentPermissionLevel) error {
	return nil
}
func (m *minimalHITLRuntimeAdapter) ListToolsInfo() []ToolInfo                   { return nil }
func (m *minimalHITLRuntimeAdapter) ListCapabilities() []CapabilityInfo          { return nil }
func (m *minimalHITLRuntimeAdapter) ListPrompts() []PromptInfo                   { return nil }
func (m *minimalHITLRuntimeAdapter) ListResources([]string) []ResourceInfo       { return nil }
func (m *minimalHITLRuntimeAdapter) ListLiveProviders() []LiveProviderInfo       { return nil }
func (m *minimalHITLRuntimeAdapter) ListLiveSessions() []LiveProviderSessionInfo { return nil }
func (m *minimalHITLRuntimeAdapter) ListApprovals() []ApprovalInfo               { return nil }
func (m *minimalHITLRuntimeAdapter) GetCapabilityDetail(string) (*CapabilityDetail, error) {
	return nil, nil
}
func (m *minimalHITLRuntimeAdapter) GetPromptDetail(string) (*PromptDetail, error) { return nil, nil }
func (m *minimalHITLRuntimeAdapter) GetResourceDetail(string) (*ResourceDetail, error) {
	return nil, nil
}
func (m *minimalHITLRuntimeAdapter) GetLiveProviderDetail(string) (*LiveProviderDetail, error) {
	return nil, nil
}
func (m *minimalHITLRuntimeAdapter) GetLiveSessionDetail(string) (*LiveProviderSessionDetail, error) {
	return nil, nil
}
func (m *minimalHITLRuntimeAdapter) GetApprovalDetail(string) (*ApprovalDetail, error) {
	return nil, nil
}
func (m *minimalHITLRuntimeAdapter) GetClassPolicies() map[string]core.AgentPermissionLevel {
	return nil
}
func (m *minimalHITLRuntimeAdapter) SetToolPolicyLive(string, core.AgentPermissionLevel)  {}
func (m *minimalHITLRuntimeAdapter) SetClassPolicyLive(string, core.AgentPermissionLevel) {}
func (m *minimalHITLRuntimeAdapter) ListWorkflows(int) ([]WorkflowInfo, error)            { return nil, nil }
func (m *minimalHITLRuntimeAdapter) GetWorkflow(string) (*WorkflowDetails, error)         { return nil, nil }
func (m *minimalHITLRuntimeAdapter) CancelWorkflow(string) error                          { return nil }
func (m *minimalHITLRuntimeAdapter) InvokeCapability(context.Context, string, map[string]any) (*core.ToolResult, error) {
	return nil, nil
}
func (m *minimalHITLRuntimeAdapter) Diagnostics() DiagnosticsInfo   { return DiagnosticsInfo{} }
func (m *minimalHITLRuntimeAdapter) ApplyChatPolicy(SubTabID) error { return nil }

// TestHITLEventPushesNotification verifies that a HITLEventRequested event
// causes the notification queue to receive a HITL item via RootModel (after Gap 2).
func TestHITLEventPushesNotification(t *testing.T) {
	hitl := newFakeHITL()
	req := &authorization.PermissionRequest{
		ID:            "hitl-1",
		Permission:    core.PermissionDescriptor{Action: "file_matrix:write", Resource: "src/main.rs"},
		Justification: "file permission matrix",
	}
	hitl.pending = []*authorization.PermissionRequest{req}

	notifQ := &NotificationQueue{}
	chat := NewChatPane(nil, &AgentContext{}, &Session{}, notifQ)
	chat.hitlSvc = hitl

	// Create a RootModel and set up HITL
	adapter := &minimalHITLRuntimeAdapter{hitlSvc: hitl}
	m := newRootModel(adapter)
	m.notifQ = notifQ
	m.chat = chat

	event := hitlEventMsg{event: authorization.HITLEvent{Type: authorization.HITLEventRequested, Request: req}}
	_, _ = m.handleHITLEvent(event)

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
	req := &authorization.PermissionRequest{
		ID:            "hitl-approve",
		Permission:    core.PermissionDescriptor{Action: "command:exec", Resource: "cargo build"},
		Justification: "build test",
	}
	hitl.pending = []*authorization.PermissionRequest{req}

	cmd := approveHITLRootCmd(hitl, req.ID, authorization.GrantScopeOneTime)
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
	req := &authorization.PermissionRequest{
		ID:            "hitl-deny",
		Permission:    core.PermissionDescriptor{Action: "file:write", Resource: "config.toml"},
		Justification: "write test",
	}
	hitl.pending = []*authorization.PermissionRequest{req}

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
