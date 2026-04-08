package tui

import (
	"context"
	"testing"
	"time"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/stretchr/testify/require"
)

// TestKeybindingsUndo verifies ctrl+z reverts to previous snapshot.
func TestKeybindingsUndo(t *testing.T) {
	adapter := newMinimalTestAdapter()
	m := newRootModel(adapter)

	msg1 := Message{
		ID:   "msg-1",
		Role: RoleUser,
		Content: MessageContent{
			Text: "Hello",
		},
		Timestamp: time.Now(),
	}
	m.chat.AppendMessage(msg1)

	// Snapshot after first message
	m.chat.PushUndoSnapshot(m.chat.Messages())

	msg2 := Message{
		ID:   "msg-2",
		Role: RoleAgent,
		Content: MessageContent{
			Text: "Hi there",
		},
		Timestamp: time.Now(),
	}
	m.chat.AppendMessage(msg2)

	// Initially 2 messages
	require.Len(t, m.chat.Messages(), 2)

	// Undo should revert to snapshot (1 message)
	updated, _ := m.handleGlobalKey("ctrl+z")
	m = updated.(RootModel)

	// Now should have 1 message
	require.Len(t, m.chat.Messages(), 1)
	require.Equal(t, "msg-1", m.chat.Messages()[0].ID)
}

// TestKeybindingsRedo verifies ctrl+y restores the next snapshot.
func TestKeybindingsRedo(t *testing.T) {
	adapter := newMinimalTestAdapter()
	m := newRootModel(adapter)

	msg1 := Message{
		ID:   "msg-1",
		Role: RoleUser,
		Content: MessageContent{
			Text: "Hello",
		},
		Timestamp: time.Now(),
	}
	m.chat.AppendMessage(msg1)

	// Snapshot and add second message
	m.chat.PushUndoSnapshot(m.chat.Messages())

	msg2 := Message{
		ID:   "msg-2",
		Role: RoleAgent,
		Content: MessageContent{
			Text: "Hi there",
		},
		Timestamp: time.Now(),
	}
	m.chat.AppendMessage(msg2)

	// Undo to go back to 1 message
	updated, _ := m.handleGlobalKey("ctrl+z")
	m = updated.(RootModel)
	require.Len(t, m.chat.Messages(), 1)

	// Redo should restore to 2 messages
	updated, _ = m.handleGlobalKey("ctrl+y")
	m = updated.(RootModel)

	require.Len(t, m.chat.Messages(), 2)
	require.Equal(t, "msg-2", m.chat.Messages()[1].ID)
}

// TestKeybindingsScrollUp verifies ctrl+u scrolls the feed up.
func TestKeybindingsScrollUp(t *testing.T) {
	adapter := newMinimalTestAdapter()
	m := newRootModel(adapter)
	m.activeTab = TabChat
	m.chat.SetSize(80, 20)

	// Add multiple messages
	for i := 1; i <= 30; i++ {
		m.chat.AppendMessage(Message{
			ID:        "msg-" + string(rune(i)),
			Role:      RoleUser,
			Content:   MessageContent{Text: "Message " + string(rune(i))},
			Timestamp: time.Now(),
		})
	}

	// Scroll to bottom
	chatPaneOf(m.chat).feed.vp.GotoBottom()
	initialOffset := chatPaneOf(m.chat).feed.vp.YOffset

	// Scroll up
	updated, _ := m.handleGlobalKey("ctrl+u")
	m = updated.(RootModel)

	// Should have scrolled up (smaller offset)
	require.Less(t, chatPaneOf(m.chat).feed.vp.YOffset, initialOffset)
}

// TestKeybindingsScrollDown verifies pagedown scrolls the feed down.
func TestKeybindingsScrollDown(t *testing.T) {
	adapter := newMinimalTestAdapter()
	m := newRootModel(adapter)
	m.activeTab = TabChat
	m.chat.SetSize(80, 20)

	// Add multiple messages
	for i := 1; i <= 30; i++ {
		m.chat.AppendMessage(Message{
			ID:        "msg-" + string(rune(i)),
			Role:      RoleUser,
			Content:   MessageContent{Text: "Message " + string(rune(i))},
			Timestamp: time.Now(),
		})
	}

	// Start at top
	chatPaneOf(m.chat).feed.vp.GotoTop()
	initialOffset := chatPaneOf(m.chat).feed.vp.YOffset

	// Scroll down
	updated, _ := m.handleGlobalKey("pagedown")
	m = updated.(RootModel)

	// Should have scrolled down (larger offset)
	require.Greater(t, chatPaneOf(m.chat).feed.vp.YOffset, initialOffset)
}

// TestKeybindingsPageUp verifies pageup scrolls the feed up by page.
func TestKeybindingsPageUp(t *testing.T) {
	adapter := newMinimalTestAdapter()
	m := newRootModel(adapter)
	m.activeTab = TabChat
	m.chat.SetSize(80, 20)

	// Add multiple messages to enable paging
	for i := 1; i <= 50; i++ {
		m.chat.AppendMessage(Message{
			ID:        "msg-" + string(rune(i)),
			Role:      RoleUser,
			Content:   MessageContent{Text: "Message " + string(rune(i))},
			Timestamp: time.Now(),
		})
	}

	// Scroll to bottom
	chatPaneOf(m.chat).feed.vp.GotoBottom()
	initialOffset := chatPaneOf(m.chat).feed.vp.YOffset

	// Page up
	updated, _ := m.handleGlobalKey("pageup")
	m = updated.(RootModel)

	// Should have scrolled up
	require.Less(t, chatPaneOf(m.chat).feed.vp.YOffset, initialOffset)
}

// TestKeybindingsFilePicker verifies @ enters file picker mode.
func TestKeybindingsFilePicker(t *testing.T) {
	adapter := newMinimalTestAdapter()
	m := newRootModel(adapter)

	// Enable file picker mode
	updated, _ := m.handleGlobalKey("@")
	m = updated.(RootModel)

	// Input should have file picker placeholder
	require.Contains(t, m.inputBar.input.Placeholder, "select files")
	// Input should start with @
	require.Equal(t, "@", m.inputBar.Value())
}

// TestKeybindingsCompact verifies ctrl+k toggles compact mode.
func TestKeybindingsCompact(t *testing.T) {
	adapter := newMinimalTestAdapter()
	m := newRootModel(adapter)

	initialMode := chatPaneOf(m.chat).expandTarget
	require.Equal(t, "thinking", initialMode)

	// Toggle once
	updated, _ := m.handleGlobalKey("ctrl+k")
	m = updated.(RootModel)
	require.Equal(t, "plan", chatPaneOf(m.chat).expandTarget)

	// Toggle again
	updated, _ = m.handleGlobalKey("ctrl+k")
	m = updated.(RootModel)
	require.Equal(t, "all", chatPaneOf(m.chat).expandTarget)

	// Toggle back to thinking
	updated, _ = m.handleGlobalKey("ctrl+k")
	m = updated.(RootModel)
	require.Equal(t, "thinking", chatPaneOf(m.chat).expandTarget)
}

// TestFeedScrollMethods verifies Feed scroll methods work correctly.
func TestFeedScrollMethods(t *testing.T) {
	feed := NewFeed()
	feed.SetSize(80, 20)

	// Add messages
	for i := 1; i <= 30; i++ {
		feed.AppendMessage(Message{
			ID:        "msg-" + string(rune(i)),
			Role:      RoleUser,
			Content:   MessageContent{Text: "Message " + string(rune(i))},
			Timestamp: time.Now(),
		})
	}

	// Test ScrollUp
	feed.vp.GotoBottom()
	offsetBefore := feed.vp.YOffset
	feed.ScrollUp()
	require.Less(t, feed.vp.YOffset, offsetBefore)

	// Test ScrollDown
	feed.vp.GotoTop()
	offsetBefore = feed.vp.YOffset
	feed.ScrollDown()
	require.Greater(t, feed.vp.YOffset, offsetBefore)

	// Test PageUp
	feed.vp.GotoBottom()
	offsetBefore = feed.vp.YOffset
	feed.PageUp()
	require.Less(t, feed.vp.YOffset, offsetBefore)

	// Test PageDown
	feed.vp.GotoTop()
	offsetBefore = feed.vp.YOffset
	feed.PageDown()
	require.Greater(t, feed.vp.YOffset, offsetBefore)
}

// TestChatPaneToggleCompact verifies compact mode cycling.
func TestChatPaneToggleCompact(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)

	require.Equal(t, "thinking", pane.expandTarget)

	pane.ToggleCompact()
	require.Equal(t, "plan", pane.expandTarget)

	pane.ToggleCompact()
	require.Equal(t, "all", pane.expandTarget)

	pane.ToggleCompact()
	require.Equal(t, "thinking", pane.expandTarget)
}

// TestKeybindingsScrollNoOpWhenWrongTab verifies scroll only works in chat tab.
func TestKeybindingsScrollNoOpWhenWrongTab(t *testing.T) {
	adapter := newMinimalTestAdapter()
	m := newRootModel(adapter)
	m.activeTab = TabSession

	// Add messages to chat
	m.chat.SetSize(80, 20)
	for i := 1; i <= 10; i++ {
		m.chat.AppendMessage(Message{
			ID:        "msg-" + string(rune(i)),
			Role:      RoleUser,
			Content:   MessageContent{Text: "Message " + string(rune(i))},
			Timestamp: time.Now(),
		})
	}

	chatPaneOf(m.chat).feed.vp.GotoTop()
	initialOffset := chatPaneOf(m.chat).feed.vp.YOffset

	// Try to scroll while on tasks tab
	updated, _ := m.handleGlobalKey("pagedown")
	m = updated.(RootModel)

	// Offset should not change (scroll didn't apply)
	require.Equal(t, initialOffset, chatPaneOf(m.chat).feed.vp.YOffset)
}

// newMinimalTestAdapter creates a test runtime adapter for keybinding tests.
func newMinimalTestAdapter() RuntimeAdapter {
	return &minimalKeybindingTestAdapter{}
}

type minimalKeybindingTestAdapter struct{}

func (m *minimalKeybindingTestAdapter) ExecuteInstruction(context.Context, string, core.TaskType, map[string]any) (*core.Result, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) ExecuteInstructionStream(context.Context, string, core.TaskType, map[string]any, func(string)) (*core.Result, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) AvailableAgents() []string { return nil }
func (m *minimalKeybindingTestAdapter) SwitchAgent(string) error  { return nil }
func (m *minimalKeybindingTestAdapter) SessionInfo() SessionInfo  { return SessionInfo{} }
func (m *minimalKeybindingTestAdapter) ResolveContextFiles(context.Context, []string) ContextFileResolution {
	return ContextFileResolution{}
}
func (m *minimalKeybindingTestAdapter) SessionArtifacts() SessionArtifacts { return SessionArtifacts{} }
func (m *minimalKeybindingTestAdapter) OllamaModels(context.Context) ([]string, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) RecordingMode() string                           { return "off" }
func (m *minimalKeybindingTestAdapter) SetRecordingMode(string) error                   { return nil }
func (m *minimalKeybindingTestAdapter) SaveModel(string) error                          { return nil }
func (m *minimalKeybindingTestAdapter) ContractSummary() *ContractSummary               { return nil }
func (m *minimalKeybindingTestAdapter) CapabilityAdmissions() []CapabilityAdmissionInfo { return nil }
func (m *minimalKeybindingTestAdapter) SaveToolPolicy(string, core.AgentPermissionLevel) error {
	return nil
}
func (m *minimalKeybindingTestAdapter) ListToolsInfo() []ToolInfo                   { return nil }
func (m *minimalKeybindingTestAdapter) ListCapabilities() []CapabilityInfo          { return nil }
func (m *minimalKeybindingTestAdapter) ListPrompts() []PromptInfo                   { return nil }
func (m *minimalKeybindingTestAdapter) ListResources([]string) []ResourceInfo       { return nil }
func (m *minimalKeybindingTestAdapter) ListLiveProviders() []LiveProviderInfo       { return nil }
func (m *minimalKeybindingTestAdapter) ListLiveSessions() []LiveProviderSessionInfo { return nil }
func (m *minimalKeybindingTestAdapter) ListApprovals() []ApprovalInfo               { return nil }
func (m *minimalKeybindingTestAdapter) GetCapabilityDetail(string) (*CapabilityDetail, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) GetPromptDetail(string) (*PromptDetail, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) GetResourceDetail(string) (*ResourceDetail, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) GetLiveProviderDetail(string) (*LiveProviderDetail, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) GetLiveSessionDetail(string) (*LiveProviderSessionDetail, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) GetApprovalDetail(string) (*ApprovalDetail, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) GetClassPolicies() map[string]core.AgentPermissionLevel {
	return nil
}
func (m *minimalKeybindingTestAdapter) SetToolPolicyLive(string, core.AgentPermissionLevel)  {}
func (m *minimalKeybindingTestAdapter) SetClassPolicyLive(string, core.AgentPermissionLevel) {}
func (m *minimalKeybindingTestAdapter) ListWorkflows(int) ([]WorkflowInfo, error)            { return nil, nil }
func (m *minimalKeybindingTestAdapter) GetWorkflow(string) (*WorkflowDetails, error)         { return nil, nil }
func (m *minimalKeybindingTestAdapter) CancelWorkflow(string) error                          { return nil }
func (m *minimalKeybindingTestAdapter) PendingHITL() []*fauthorization.PermissionRequest     { return nil }
func (m *minimalKeybindingTestAdapter) ApproveHITL(string, string, fauthorization.GrantScope, time.Duration) error {
	return nil
}
func (m *minimalKeybindingTestAdapter) DenyHITL(string, string) error { return nil }
func (m *minimalKeybindingTestAdapter) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	return nil, func() {}
}
func (m *minimalKeybindingTestAdapter) PendingGuidance() []*guidance.GuidanceRequest { return nil }
func (m *minimalKeybindingTestAdapter) ResolveGuidance(string, string, string) error { return nil }
func (m *minimalKeybindingTestAdapter) SubscribeGuidance() (<-chan guidance.GuidanceEvent, func()) {
	return nil, func() {}
}
func (m *minimalKeybindingTestAdapter) PendingDeferrals() []guidance.EngineeringObservation {
	return nil
}
func (m *minimalKeybindingTestAdapter) ResolveDeferral(string) error { return nil }
func (m *minimalKeybindingTestAdapter) SubscribeLearning() (<-chan archaeolearning.Event, func()) {
	return nil, func() {}
}
func (m *minimalKeybindingTestAdapter) PendingLearning() []archaeolearning.Interaction { return nil }
func (m *minimalKeybindingTestAdapter) ResolveLearning(string, archaeolearning.ResolveInput) error {
	return nil
}
func (m *minimalKeybindingTestAdapter) InvokeCapability(context.Context, string, map[string]any) (*core.ToolResult, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) SetInteractionEmitter(e interaction.FrameEmitter) {}
func (m *minimalKeybindingTestAdapter) Diagnostics() DiagnosticsInfo                     { return DiagnosticsInfo{} }
func (m *minimalKeybindingTestAdapter) ApplyChatPolicy(SubTabID) error                   { return nil }
func (m *minimalKeybindingTestAdapter) ListServices() []ServiceInfo                      { return nil }
func (m *minimalKeybindingTestAdapter) StopService(string) error                         { return nil }
func (m *minimalKeybindingTestAdapter) RestartService(context.Context, string) error     { return nil }
func (m *minimalKeybindingTestAdapter) RestartAllServices(context.Context) error         { return nil }
func (m *minimalKeybindingTestAdapter) LoadActivePlan(context.Context, string) (*ActivePlanView, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) LoadBlobs(context.Context, string) ([]BlobEntry, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) AddBlobToPlan(context.Context, string, string) error {
	return nil
}
func (m *minimalKeybindingTestAdapter) RemoveBlobFromPlan(context.Context, string, string) error {
	return nil
}
func (m *minimalKeybindingTestAdapter) AddFileToContext(string) error    { return nil }
func (m *minimalKeybindingTestAdapter) DropFileFromContext(string) error { return nil }
func (m *minimalKeybindingTestAdapter) ListPlanVersions(context.Context, string) ([]PlanVersionInfo, error) {
	return nil, nil
}
func (m *minimalKeybindingTestAdapter) ActivatePlanVersion(context.Context, string, int) error {
	return nil
}
