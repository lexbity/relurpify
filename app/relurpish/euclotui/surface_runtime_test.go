package euclotui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	capabilityports "codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/execution"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// stubRuntimeAdapter implements tui.RuntimeAdapter with minimal stubs.
// It delegates SubmitTurn to the real runtime for the echo scenario test.
type stubRuntimeAdapter struct {
	submitFunc func(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error)
}

func (s *stubRuntimeAdapter) SubmitTurn(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error) {
	return s.submitFunc(ctx, instruction, taskType, metadata, callback)
}

func (s *stubRuntimeAdapter) ExecuteInstruction(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any) (*execution.Result, error) {
	return s.submitFunc(ctx, instruction, taskType, metadata, nil)
}

func (s *stubRuntimeAdapter) ExecuteInstructionStream(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error) {
	return s.submitFunc(ctx, instruction, taskType, metadata, callback)
}

func (s *stubRuntimeAdapter) AvailableAgents() []string                                                  { return []string{"euclo"} }
func (s *stubRuntimeAdapter) SwitchAgent(name string) error                                               { return nil }
func (s *stubRuntimeAdapter) SessionInfo() tui.SessionInfo                                                { return tui.SessionInfo{} }
func (s *stubRuntimeAdapter) ResolveContextFiles(ctx context.Context, files []string) tui.ContextFileResolution { return tui.ContextFileResolution{Allowed: files} }
func (s *stubRuntimeAdapter) SessionArtifacts() tui.SessionArtifacts                                       { return tui.SessionArtifacts{} }
func (s *stubRuntimeAdapter) InferenceModels(ctx context.Context) ([]string, error)                        { return nil, nil }
func (s *stubRuntimeAdapter) RecordingMode() string                                                        { return "off" }
func (s *stubRuntimeAdapter) SetRecordingMode(string) error                                                { return nil }
func (s *stubRuntimeAdapter) SaveModel(string) error                                                       { return nil }
func (s *stubRuntimeAdapter) ContractSummary() *tui.ContractSummary                                        { return nil }
func (s *stubRuntimeAdapter) CapabilityAdmissions() []tui.CapabilityAdmissionInfo                          { return nil }
func (s *stubRuntimeAdapter) SaveToolPolicy(string, agentspec.AgentPermissionLevel) error                    { return nil }
func (s *stubRuntimeAdapter) LoadSandboxDocument() (*config.Document, error)                                { return nil, nil }
func (s *stubRuntimeAdapter) SaveSandboxDocument(*config.Document) (string, error)                          { return "", nil }
func (s *stubRuntimeAdapter) SandboxBackend() string                                                        { return "" }
func (s *stubRuntimeAdapter) SaveSandboxBackend(string) (string, error)                                    { return "", nil }
func (s *stubRuntimeAdapter) ExecutionMode() config.ExecutionMode                                           { return config.ExecutionModeStaged }
func (s *stubRuntimeAdapter) ListToolsInfo() []tui.ToolInfo                                                 { return nil }
func (s *stubRuntimeAdapter) ListCapabilities() []tui.CapabilityInfo                                       { return nil }
func (s *stubRuntimeAdapter) ListPrompts() []tui.PromptInfo                                                 { return nil }
func (s *stubRuntimeAdapter) ListResources([]string) []tui.ResourceInfo                                     { return nil }
func (s *stubRuntimeAdapter) ListLiveProviders() []tui.LiveProviderInfo                                    { return nil }
func (s *stubRuntimeAdapter) ListLiveSessions() []tui.LiveProviderSessionInfo                              { return nil }
func (s *stubRuntimeAdapter) ListApprovals() []tui.ApprovalInfo                                           { return nil }
func (s *stubRuntimeAdapter) GetCapabilityDetail(string) (*tui.CapabilityDetail, error)                    { return nil, nil }
func (s *stubRuntimeAdapter) GetPromptDetail(string) (*tui.PromptDetail, error)                            { return nil, nil }
func (s *stubRuntimeAdapter) GetResourceDetail(string) (*tui.ResourceDetail, error)                        { return nil, nil }
func (s *stubRuntimeAdapter) GetLiveProviderDetail(string) (*tui.LiveProviderDetail, error)                { return nil, nil }
func (s *stubRuntimeAdapter) GetLiveSessionDetail(string) (*tui.LiveProviderSessionDetail, error)          { return nil, nil }
func (s *stubRuntimeAdapter) GetApprovalDetail(string) (*tui.ApprovalDetail, error)                       { return nil, nil }
func (s *stubRuntimeAdapter) GetClassPolicies() map[string]agentspec.AgentPermissionLevel                    { return nil }
func (s *stubRuntimeAdapter) SetToolPolicyLive(string, agentspec.AgentPermissionLevel)                       {}
func (s *stubRuntimeAdapter) SetClassPolicyLive(string, agentspec.AgentPermissionLevel)                     {}
func (s *stubRuntimeAdapter) ListWorkflows(int) ([]tui.WorkflowInfo, error)                                { return nil, nil }
func (s *stubRuntimeAdapter) GetWorkflow(string) (*tui.WorkflowDetails, error)                             { return nil, nil }
func (s *stubRuntimeAdapter) CancelWorkflow(string) error                                                  { return nil }
func (s *stubRuntimeAdapter) InvokeCapability(context.Context, string, map[string]any) (*capabilityports.ToolResult, error) { return nil, nil }
func (s *stubRuntimeAdapter) Diagnostics() tui.DiagnosticsInfo                                               { return tui.DiagnosticsInfo{} }
func (s *stubRuntimeAdapter) BuildDoctorReport(context.Context) tui.DoctorReport                            { return tui.DoctorReport{} }
func (s *stubRuntimeAdapter) ReloadWorkspace(context.Context, string) error                                 { return nil }
func (s *stubRuntimeAdapter) InitializeWorkspaceFromTemplates(bool) error                                   { return nil }
func (s *stubRuntimeAdapter) ApplyChatPolicy(tui.SubTabID) error                                            { return nil }
func (s *stubRuntimeAdapter) ListServices() []tui.ServiceInfo                                                { return nil }
func (s *stubRuntimeAdapter) StopService(string) error                                                      { return nil }
func (s *stubRuntimeAdapter) RestartService(context.Context, string) error                                  { return nil }
func (s *stubRuntimeAdapter) RestartAllServices(context.Context) error                                      { return nil }
func (s *stubRuntimeAdapter) AddFileToContext(string) error                                                 { return nil }
func (s *stubRuntimeAdapter) DropFileFromContext(string) error                                              { return nil }
func (s *stubRuntimeAdapter) ActiveWorkflowID() string                                                      { return "" }
func (s *stubRuntimeAdapter) ResumeSession(context.Context, string) error                                   { return nil }
func (s *stubRuntimeAdapter) ResolveInteractionFrame(context.Context, string, string, string, string) error { return nil }

// HITL methods
func (s *stubRuntimeAdapter) PendingHITL() []*fauthorization.PermissionRequest { return nil }
func (s *stubRuntimeAdapter) ApproveHITL(string, string, policy.GrantScope, time.Duration) error { return nil }
func (s *stubRuntimeAdapter) DenyHITL(string, string) error { return nil }
func (s *stubRuntimeAdapter) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	ch := make(chan fauthorization.HITLEvent)
	close(ch)
	return ch, func() {}
}

func (s *stubRuntimeAdapter) SubscribeExecutionEvents() (<-chan telemetry.Event, func()) {
	ch := make(chan telemetry.Event)
	close(ch)
	return ch, func() {}
}

// TestEucloChatPaneAcceptsOfflineTurn verifies NewSurface() creates a euclo
// surface whose ChatPane accepts a submitted prompt and wires through to the
// runtime's SubmitTurn path.
func TestEucloChatPaneAcceptsOfflineTurn(t *testing.T) {
	submitCh := make(chan struct{}, 1)
	adapter := &stubRuntimeAdapter{
		submitFunc: func(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error) {
			submitCh <- struct{}{}
			if callback != nil {
				callback("mock token")
			}
			return &execution.Result{NodeID: "test", Success: true}, nil
		},
	}

	surface := NewSurface()
	if surface == nil {
		t.Fatal("NewSurface() returned nil")
	}
	if surface.Name() != "euclo" {
		t.Fatalf("surface name = %q, want euclo", surface.Name())
	}

	ctx := &tui.AgentContext{}
	sess := &tui.Session{}
	chat := surface.NewChat(adapter, ctx, sess, nil)
	if chat == nil {
		t.Fatal("NewChat returned nil")
	}

	cmd := chat.HandleInputSubmit("hello")
	if cmd == nil {
		t.Fatal("HandleInputSubmit returned nil cmd")
	}

	select {
	case <-submitCh:
	case <-time.After(time.Second):
		t.Fatal("SubmitTurn was not called within 1s — ChatPane did not wire through to runtime")
	}
}

func extractChatTextFromMsgs(t *testing.T, cmd tea.Cmd, maxIter int) string {
	t.Helper()
	var parts []string
	for i := 0; i < maxIter && cmd != nil; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		switch v := msg.(type) {
		case tui.StreamTokenMsg:
			parts = append(parts, v.Token)
		case tui.StreamCompleteMsg:
		case tui.ChatSystemMsg:
			parts = append(parts, v.Text)
		}
	}
	return strings.Join(parts, " ")
}
