package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/stretchr/testify/require"
)

// capabilityCall records a single InvokeCapability call for assertion.
type capabilityCall struct {
	Name string
	Args map[string]any
}

// recordingAdapter is a RuntimeAdapter stub that records InvokeCapability calls
// and returns preset responses.
type recordingAdapter struct {
	calls    []capabilityCall
	response *core.ToolResult
	err      error
}

func (r *recordingAdapter) InvokeCapability(_ context.Context, name string, args map[string]any) (*core.ToolResult, error) {
	r.calls = append(r.calls, capabilityCall{Name: name, Args: args})
	return r.response, r.err
}

func (r *recordingAdapter) ExecuteInstruction(context.Context, string, core.TaskType, map[string]any) (*core.Result, error) {
	return nil, nil
}
func (r *recordingAdapter) ExecuteInstructionStream(context.Context, string, core.TaskType, map[string]any, func(string)) (*core.Result, error) {
	return nil, nil
}
func (r *recordingAdapter) AvailableAgents() []string { return nil }
func (r *recordingAdapter) SwitchAgent(string) error  { return nil }
func (r *recordingAdapter) SessionInfo() SessionInfo  { return SessionInfo{} }
func (r *recordingAdapter) ResolveContextFiles(context.Context, []string) ContextFileResolution {
	return ContextFileResolution{}
}
func (r *recordingAdapter) SessionArtifacts() SessionArtifacts                     { return SessionArtifacts{} }
func (r *recordingAdapter) OllamaModels(context.Context) ([]string, error)         { return nil, nil }
func (r *recordingAdapter) RecordingMode() string                                  { return "off" }
func (r *recordingAdapter) SetRecordingMode(string) error                          { return nil }
func (r *recordingAdapter) SaveModel(string) error                                 { return nil }
func (r *recordingAdapter) ContractSummary() *ContractSummary                      { return nil }
func (r *recordingAdapter) CapabilityAdmissions() []CapabilityAdmissionInfo        { return nil }
func (r *recordingAdapter) SaveToolPolicy(string, core.AgentPermissionLevel) error { return nil }
func (r *recordingAdapter) ListToolsInfo() []ToolInfo                              { return nil }
func (r *recordingAdapter) ListCapabilities() []CapabilityInfo                     { return nil }
func (r *recordingAdapter) ListPrompts() []PromptInfo                              { return nil }
func (r *recordingAdapter) ListResources([]string) []ResourceInfo                  { return nil }
func (r *recordingAdapter) ListLiveProviders() []LiveProviderInfo                  { return nil }
func (r *recordingAdapter) ListLiveSessions() []LiveProviderSessionInfo            { return nil }
func (r *recordingAdapter) ListApprovals() []ApprovalInfo                          { return nil }
func (r *recordingAdapter) GetCapabilityDetail(string) (*CapabilityDetail, error)  { return nil, nil }
func (r *recordingAdapter) GetPromptDetail(string) (*PromptDetail, error)          { return nil, nil }
func (r *recordingAdapter) GetResourceDetail(string) (*ResourceDetail, error)      { return nil, nil }
func (r *recordingAdapter) GetLiveProviderDetail(string) (*LiveProviderDetail, error) {
	return nil, nil
}
func (r *recordingAdapter) GetLiveSessionDetail(string) (*LiveProviderSessionDetail, error) {
	return nil, nil
}
func (r *recordingAdapter) GetApprovalDetail(string) (*ApprovalDetail, error)      { return nil, nil }
func (r *recordingAdapter) GetClassPolicies() map[string]core.AgentPermissionLevel { return nil }
func (r *recordingAdapter) SetToolPolicyLive(string, core.AgentPermissionLevel)    {}
func (r *recordingAdapter) SetClassPolicyLive(string, core.AgentPermissionLevel)   {}
func (r *recordingAdapter) ListWorkflows(int) ([]WorkflowInfo, error)              { return nil, nil }
func (r *recordingAdapter) GetWorkflow(string) (*WorkflowDetails, error)           { return nil, nil }
func (r *recordingAdapter) CancelWorkflow(string) error                            { return nil }
func (r *recordingAdapter) PendingHITL() []*fauthorization.PermissionRequest       { return nil }
func (r *recordingAdapter) ApproveHITL(string, string, fauthorization.GrantScope, time.Duration) error {
	return nil
}
func (r *recordingAdapter) DenyHITL(string, string) error { return nil }
func (r *recordingAdapter) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	return nil, func() {}
}
func (r *recordingAdapter) PendingGuidance() []*guidance.GuidanceRequest { return nil }
func (r *recordingAdapter) ResolveGuidance(string, string, string) error { return nil }
func (r *recordingAdapter) SubscribeGuidance() (<-chan guidance.GuidanceEvent, func()) {
	return nil, func() {}
}
func (r *recordingAdapter) PendingDeferrals() []guidance.EngineeringObservation { return nil }
func (r *recordingAdapter) ResolveDeferral(string) error                        { return nil }
func (r *recordingAdapter) SubscribeLearning() (<-chan archaeolearning.Event, func()) {
	return nil, func() {}
}
func (r *recordingAdapter) PendingLearning() []archaeolearning.Interaction { return nil }
func (r *recordingAdapter) ResolveLearning(string, archaeolearning.ResolveInput) error {
	return nil
}
func (r *recordingAdapter) SetInteractionEmitter(interaction.FrameEmitter) {}
func (r *recordingAdapter) Diagnostics() DiagnosticsInfo                   { return DiagnosticsInfo{} }
func (r *recordingAdapter) ApplyChatPolicy(SubTabID) error                 { return nil }
func (r *recordingAdapter) ListServices() []ServiceInfo                    { return nil }
func (r *recordingAdapter) StopService(string) error                       { return nil }
func (r *recordingAdapter) RestartService(context.Context, string) error   { return nil }
func (r *recordingAdapter) RestartAllServices(context.Context) error       { return nil }
func (r *recordingAdapter) LoadActivePlan(context.Context, string) (*ActivePlanView, error) {
	return nil, nil
}
func (r *recordingAdapter) LoadBlobs(context.Context, string) ([]BlobEntry, error)   { return nil, nil }
func (r *recordingAdapter) AddBlobToPlan(context.Context, string, string) error      { return nil }
func (r *recordingAdapter) RemoveBlobFromPlan(context.Context, string, string) error { return nil }
func (r *recordingAdapter) AddFileToContext(string) error                            { return nil }
func (r *recordingAdapter) DropFileFromContext(string) error                         { return nil }
func (r *recordingAdapter) ListPlanVersions(context.Context, string) ([]PlanVersionInfo, error) {
	return nil, nil
}
func (r *recordingAdapter) ActivatePlanVersion(context.Context, string, int) error { return nil }

// TestGitStatusInvokesCliGit verifies gitStatusCmd routes through cli_git capability.
func TestGitStatusInvokesCliGit(t *testing.T) {
	rt := &recordingAdapter{
		response: &core.ToolResult{Data: map[string]any{"stdout": " M test.txt\n"}},
	}
	cmd := gitStatusCmd(rt)
	msg := cmd()

	require.IsType(t, gitStatusMsg{}, msg)
	require.Len(t, rt.calls, 1)
	require.Equal(t, "cli_git", rt.calls[0].Name)

	args := rt.calls[0].Args["args"].([]string)
	require.Equal(t, []string{"status", "--porcelain"}, args)

	statusMsg := msg.(gitStatusMsg)
	require.Nil(t, statusMsg.Err)
	require.Equal(t, []string{"test.txt"}, statusMsg.Modified)
}

// TestGitStatusEmpty verifies gitStatusCmd returns no files when output is empty.
func TestGitStatusEmpty(t *testing.T) {
	rt := &recordingAdapter{
		response: &core.ToolResult{Data: map[string]any{"stdout": ""}},
	}
	cmd := gitStatusCmd(rt)
	msg := cmd().(gitStatusMsg)
	require.Nil(t, msg.Err)
	require.Len(t, msg.Modified, 0)
}

// TestGitCommitInvokesCliGit verifies gitCommitCmd routes through cli_git capability.
func TestGitCommitInvokesCliGit(t *testing.T) {
	rt := &recordingAdapter{
		response: &core.ToolResult{Data: map[string]any{"stdout": "[main abc1234] fix: update test file"}},
	}
	cmd := gitCommitCmd(rt, "fix: update test file")
	msg := cmd()

	require.IsType(t, gitCommitMsg{}, msg)
	// Two calls: git add -u, then git commit -m
	require.Len(t, rt.calls, 2)

	require.Equal(t, "cli_git", rt.calls[0].Name)
	addArgs := rt.calls[0].Args["args"].([]string)
	require.Equal(t, []string{"add", "-u"}, addArgs)

	require.Equal(t, "cli_git", rt.calls[1].Name)
	commitArgs := rt.calls[1].Args["args"].([]string)
	require.Equal(t, []string{"commit", "-m", "fix: update test file"}, commitArgs)

	commitMsg := msg.(gitCommitMsg)
	require.Nil(t, commitMsg.Err)
	require.Contains(t, commitMsg.Message, "fix: update test file")
}

// TestGitCommitCapabilityError verifies error propagation from capability.
func TestGitCommitCapabilityError(t *testing.T) {
	rt := &recordingAdapter{err: fmt.Errorf("permission denied")}
	cmd := gitCommitCmd(rt, "fix: something")
	msg := cmd().(gitCommitMsg)
	require.NotNil(t, msg.Err)
	require.Contains(t, msg.Err.Error(), "permission denied")
}

// TestGitStatusMsg verifies git status message handling in model.
func TestGitStatusMsg(t *testing.T) {
	adapter := newMinimalCommitTestAdapter()
	m := newRootModel(adapter)

	msg := gitStatusMsg{
		Modified: []string{"file1.go", "file2.go"},
		Err:      nil,
	}

	updated := m.addSystemMessageForGitStatus(msg)
	messages := updated.chat.Messages()

	require.True(t, len(messages) > 0)
	lastMsg := messages[len(messages)-1]
	require.Equal(t, RoleSystem, lastMsg.Role)
	require.Contains(t, lastMsg.Content.Text, "file1.go")
	require.Contains(t, lastMsg.Content.Text, "file2.go")
}

// TestGitCommitMsgHandling verifies commit message handling in model.
func TestGitCommitMsgHandling(t *testing.T) {
	adapter := newMinimalCommitTestAdapter()
	m := newRootModel(adapter)

	msg := gitCommitMsg{
		Message: "[main abc1234] fix: update file",
		Err:     nil,
	}

	_, _ = m.Update(msg)
	messages := m.chat.Messages()

	require.True(t, len(messages) > 0)
	lastMsg := messages[len(messages)-1]
	require.Equal(t, RoleSystem, lastMsg.Role)
	require.Contains(t, lastMsg.Content.Text, "committed")
}

// TestGitCommitError verifies error handling.
func TestGitCommitError(t *testing.T) {
	adapter := newMinimalCommitTestAdapter()
	m := newRootModel(adapter)

	msg := gitCommitMsg{
		Message: "",
		Err:     fmt.Errorf("permission denied"),
	}

	_, _ = m.Update(msg)
	messages := m.chat.Messages()

	require.True(t, len(messages) > 0)
	lastMsg := messages[len(messages)-1]
	require.Equal(t, RoleSystem, lastMsg.Role)
	require.Contains(t, lastMsg.Content.Text, "Commit failed")
	require.Contains(t, lastMsg.Content.Text, "permission denied")
}

// Helper functions

func newMinimalCommitTestAdapter() RuntimeAdapter {
	return &minimalCommitTestAdapter{}
}

type minimalCommitTestAdapter struct{}

func (m *minimalCommitTestAdapter) ExecuteInstruction(context.Context, string, core.TaskType, map[string]any) (*core.Result, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) ExecuteInstructionStream(context.Context, string, core.TaskType, map[string]any, func(string)) (*core.Result, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) AvailableAgents() []string { return nil }
func (m *minimalCommitTestAdapter) SwitchAgent(string) error  { return nil }
func (m *minimalCommitTestAdapter) SessionInfo() SessionInfo  { return SessionInfo{} }
func (m *minimalCommitTestAdapter) ResolveContextFiles(context.Context, []string) ContextFileResolution {
	return ContextFileResolution{}
}
func (m *minimalCommitTestAdapter) SessionArtifacts() SessionArtifacts              { return SessionArtifacts{} }
func (m *minimalCommitTestAdapter) OllamaModels(context.Context) ([]string, error)  { return nil, nil }
func (m *minimalCommitTestAdapter) RecordingMode() string                           { return "off" }
func (m *minimalCommitTestAdapter) SetRecordingMode(string) error                   { return nil }
func (m *minimalCommitTestAdapter) SaveModel(string) error                          { return nil }
func (m *minimalCommitTestAdapter) ContractSummary() *ContractSummary               { return nil }
func (m *minimalCommitTestAdapter) CapabilityAdmissions() []CapabilityAdmissionInfo { return nil }
func (m *minimalCommitTestAdapter) SaveToolPolicy(string, core.AgentPermissionLevel) error {
	return nil
}
func (m *minimalCommitTestAdapter) ListToolsInfo() []ToolInfo                   { return nil }
func (m *minimalCommitTestAdapter) ListCapabilities() []CapabilityInfo          { return nil }
func (m *minimalCommitTestAdapter) ListPrompts() []PromptInfo                   { return nil }
func (m *minimalCommitTestAdapter) ListResources([]string) []ResourceInfo       { return nil }
func (m *minimalCommitTestAdapter) ListLiveProviders() []LiveProviderInfo       { return nil }
func (m *minimalCommitTestAdapter) ListLiveSessions() []LiveProviderSessionInfo { return nil }
func (m *minimalCommitTestAdapter) ListApprovals() []ApprovalInfo               { return nil }
func (m *minimalCommitTestAdapter) GetCapabilityDetail(string) (*CapabilityDetail, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) GetPromptDetail(string) (*PromptDetail, error) { return nil, nil }
func (m *minimalCommitTestAdapter) GetResourceDetail(string) (*ResourceDetail, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) GetLiveProviderDetail(string) (*LiveProviderDetail, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) GetLiveSessionDetail(string) (*LiveProviderSessionDetail, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) GetApprovalDetail(string) (*ApprovalDetail, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) GetClassPolicies() map[string]core.AgentPermissionLevel {
	return nil
}
func (m *minimalCommitTestAdapter) SetToolPolicyLive(string, core.AgentPermissionLevel)  {}
func (m *minimalCommitTestAdapter) SetClassPolicyLive(string, core.AgentPermissionLevel) {}
func (m *minimalCommitTestAdapter) ListWorkflows(int) ([]WorkflowInfo, error)            { return nil, nil }
func (m *minimalCommitTestAdapter) GetWorkflow(string) (*WorkflowDetails, error)         { return nil, nil }
func (m *minimalCommitTestAdapter) CancelWorkflow(string) error                          { return nil }
func (m *minimalCommitTestAdapter) PendingHITL() []*fauthorization.PermissionRequest     { return nil }
func (m *minimalCommitTestAdapter) ApproveHITL(string, string, fauthorization.GrantScope, time.Duration) error {
	return nil
}
func (m *minimalCommitTestAdapter) DenyHITL(string, string) error { return nil }
func (m *minimalCommitTestAdapter) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	return nil, func() {}
}
func (m *minimalCommitTestAdapter) PendingGuidance() []*guidance.GuidanceRequest { return nil }
func (m *minimalCommitTestAdapter) ResolveGuidance(string, string, string) error { return nil }
func (m *minimalCommitTestAdapter) SubscribeGuidance() (<-chan guidance.GuidanceEvent, func()) {
	return nil, func() {}
}
func (m *minimalCommitTestAdapter) PendingDeferrals() []guidance.EngineeringObservation { return nil }
func (m *minimalCommitTestAdapter) ResolveDeferral(string) error                        { return nil }
func (m *minimalCommitTestAdapter) SubscribeLearning() (<-chan archaeolearning.Event, func()) {
	return nil, func() {}
}
func (m *minimalCommitTestAdapter) PendingLearning() []archaeolearning.Interaction { return nil }
func (m *minimalCommitTestAdapter) ResolveLearning(string, archaeolearning.ResolveInput) error {
	return nil
}
func (m *minimalCommitTestAdapter) InvokeCapability(context.Context, string, map[string]any) (*core.ToolResult, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) SetInteractionEmitter(e interaction.FrameEmitter) {}
func (m *minimalCommitTestAdapter) Diagnostics() DiagnosticsInfo                     { return DiagnosticsInfo{} }
func (m *minimalCommitTestAdapter) ApplyChatPolicy(SubTabID) error                   { return nil }
func (m *minimalCommitTestAdapter) ListServices() []ServiceInfo                      { return nil }
func (m *minimalCommitTestAdapter) StopService(string) error                         { return nil }
func (m *minimalCommitTestAdapter) RestartService(context.Context, string) error     { return nil }
func (m *minimalCommitTestAdapter) RestartAllServices(context.Context) error         { return nil }
func (m *minimalCommitTestAdapter) LoadActivePlan(context.Context, string) (*ActivePlanView, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) LoadBlobs(context.Context, string) ([]BlobEntry, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) AddBlobToPlan(context.Context, string, string) error { return nil }
func (m *minimalCommitTestAdapter) RemoveBlobFromPlan(context.Context, string, string) error {
	return nil
}
func (m *minimalCommitTestAdapter) AddFileToContext(string) error    { return nil }
func (m *minimalCommitTestAdapter) DropFileFromContext(string) error { return nil }
func (m *minimalCommitTestAdapter) ListPlanVersions(context.Context, string) ([]PlanVersionInfo, error) {
	return nil, nil
}
func (m *minimalCommitTestAdapter) ActivatePlanVersion(context.Context, string, int) error {
	return nil
}

// --- gitAutoCommitCmd tests ---

// gitAutoCommitTestAdapter sequences capability responses so each InvokeCapability
// call gets a different preset result (status, diff, add, commit).
type gitAutoCommitTestAdapter struct {
	recordingAdapter
	capabilityResponses []*core.ToolResult
	capabilityCallIdx   int
	execResult          *core.Result
	execErr             error
}

func (a *gitAutoCommitTestAdapter) InvokeCapability(_ context.Context, name string, args map[string]any) (*core.ToolResult, error) {
	a.calls = append(a.calls, capabilityCall{Name: name, Args: args})
	if a.capabilityCallIdx < len(a.capabilityResponses) {
		resp := a.capabilityResponses[a.capabilityCallIdx]
		a.capabilityCallIdx++
		return resp, nil
	}
	a.capabilityCallIdx++
	return nil, nil
}

func (a *gitAutoCommitTestAdapter) ExecuteInstruction(_ context.Context, _ string, _ core.TaskType, _ map[string]any) (*core.Result, error) {
	return a.execResult, a.execErr
}

// TestGitAutoCommitFullFlow verifies the happy path: status → diff → LLM → add → commit.
func TestGitAutoCommitFullFlow(t *testing.T) {
	rt := &gitAutoCommitTestAdapter{
		capabilityResponses: []*core.ToolResult{
			{Data: map[string]any{"stdout": " M main.go\n"}},                     // status
			{Data: map[string]any{"stdout": "main.go | 2 ++\n1 file changed\n"}}, // diff
			{Data: map[string]any{"stdout": ""}},                                 // add
			{Data: map[string]any{"stdout": "[main abc1234] add feature X\n"}},   // commit
		},
		execResult: &core.Result{Data: map[string]any{"final_output": "add feature X"}},
	}

	cmd := gitAutoCommitCmd(rt)
	msg := cmd()

	result, ok := msg.(gitCommitMsg)
	require.True(t, ok)
	require.NoError(t, result.Err)

	// 4 capability calls: status, diff --stat, add -u, commit -m
	require.Len(t, rt.calls, 4)
	require.Equal(t, []string{"status", "--porcelain"}, rt.calls[0].Args["args"].([]string))
	require.Equal(t, []string{"diff", "--stat", "HEAD"}, rt.calls[1].Args["args"].([]string))
	require.Equal(t, []string{"add", "-u"}, rt.calls[2].Args["args"].([]string))
	commitArgs := rt.calls[3].Args["args"].([]string)
	require.Equal(t, "commit", commitArgs[0])
	require.Equal(t, "-m", commitArgs[1])
	require.Equal(t, "add feature X", commitArgs[2])
}

// TestGitAutoCommitNothingToCommit verifies empty status returns gitStatusMsg{Modified: nil}.
func TestGitAutoCommitNothingToCommit(t *testing.T) {
	rt := &gitAutoCommitTestAdapter{
		capabilityResponses: []*core.ToolResult{
			{Data: map[string]any{"stdout": ""}},
		},
	}

	cmd := gitAutoCommitCmd(rt)
	msg := cmd()

	statusMsg, ok := msg.(gitStatusMsg)
	require.True(t, ok)
	require.Nil(t, statusMsg.Err)
	require.Empty(t, statusMsg.Modified)
}

// TestGitAutoCommitLLMError verifies LLM failure propagates as gitCommitMsg{Err: ...}.
func TestGitAutoCommitLLMError(t *testing.T) {
	rt := &gitAutoCommitTestAdapter{
		capabilityResponses: []*core.ToolResult{
			{Data: map[string]any{"stdout": " M main.go\n"}},
			{Data: map[string]any{"stdout": "main.go | 1 +\n"}},
		},
		execErr: fmt.Errorf("model offline"),
	}

	cmd := gitAutoCommitCmd(rt)
	msg := cmd().(gitCommitMsg)
	require.Error(t, msg.Err)
	require.Contains(t, msg.Err.Error(), "model offline")
}

// TestGitAutoCommitTruncatesMultilineMessage verifies only the first line of the
// LLM response is used as the commit message.
func TestGitAutoCommitTruncatesMultilineMessage(t *testing.T) {
	rt := &gitAutoCommitTestAdapter{
		capabilityResponses: []*core.ToolResult{
			{Data: map[string]any{"stdout": " M main.go\n"}},
			{Data: map[string]any{"stdout": "main.go | 1 +\n"}},
			{Data: map[string]any{"stdout": ""}},
			{Data: map[string]any{"stdout": "[main] fix: resolve bug\n"}},
		},
		execResult: &core.Result{Data: map[string]any{"final_output": "fix: resolve bug\n\nThis commit resolves an issue in main.go."}},
	}

	cmd := gitAutoCommitCmd(rt)
	msg := cmd().(gitCommitMsg)
	require.NoError(t, msg.Err)

	commitArgs := rt.calls[3].Args["args"].([]string)
	require.Equal(t, "fix: resolve bug", commitArgs[2], "commit message should be first line only")
}

// --- addSystemMessageForGitStatus is a test helper that mirrors the model's
// gitStatusMsg handling logic for direct unit testing.
func (m *RootModel) addSystemMessageForGitStatus(msg gitStatusMsg) *RootModel {
	if msg.Err != nil {
		m.addSystemMessage(fmt.Sprintf("Error: %v", msg.Err))
		return m
	}
	if len(msg.Modified) == 0 {
		m.addSystemMessage("nothing to commit")
		return m
	}
	filesStr := strings.Join(msg.Modified, "\n")
	m.addSystemMessage(fmt.Sprintf("Modified files:\n%s\n\nUse /commit \"message here\" to commit", filesStr))
	return m
}
