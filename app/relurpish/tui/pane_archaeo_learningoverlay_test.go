package tui

import (
	"context"
	"strings"
	"testing"

	"time"

	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Learning overlay tests  (/learning command)
// ---------------------------------------------------------------------------

// TestLearningOverlay_Render verifies that /learning formats pending learning
// interactions into a system message with title, kind, blocking indicator,
// and evidence list.
func TestLearningOverlay_Render(t *testing.T) {
	interactions := []archaeolearning.Interaction{
		{
			ID:          "learn-1",
			Title:       "refactor error handling",
			Kind:        archaeolearning.InteractionPatternProposal,
			Description: "Consistent error wrapping pattern detected",
			Blocking:    false,
			Evidence: []archaeolearning.EvidenceRef{
				{Kind: "tension", Title: "import-order"},
			},
		},
		{
			ID:       "learn-2",
			Title:    "blocking review required",
			Kind:     archaeolearning.InteractionTensionReview,
			Blocking: true,
		},
	}

	summary := formatPendingLearningSummary(interactions)

	require.Contains(t, summary, "learn-1")
	require.Contains(t, summary, "refactor error handling")
	require.Contains(t, summary, "[non-blocking]")
	require.Contains(t, summary, "pattern_proposal")
	require.Contains(t, summary, "import-order")

	require.Contains(t, summary, "learn-2")
	require.Contains(t, summary, "blocking review required")
	require.Contains(t, summary, "[BLOCKING]")
}

// TestLearningOverlay_EmptyQueue verifies that /learning with no interactions
// produces a "no pending" message.
func TestLearningOverlay_EmptyQueue(t *testing.T) {
	summary := formatPendingLearningSummary(nil)
	require.Contains(t, summary, "No pending")
}

// TestLearningOverlay_BlockingRedirectsHITL verifies that blocking interactions
// are visually distinguished so operators know to use the HITL panel.
func TestLearningOverlay_BlockingRedirectsHITL(t *testing.T) {
	interactions := []archaeolearning.Interaction{
		{
			ID:       "block-1",
			Title:    "must resolve before execution",
			Kind:     archaeolearning.InteractionTensionReview,
			Blocking: true,
		},
	}

	summary := formatPendingLearningSummary(interactions)

	require.Contains(t, summary, "[BLOCKING]")
	// Should also mention the HITL panel in the footer.
	require.Contains(t, summary, "HITL")
}

// TestLearningOverlay_DismissNonBlocking verifies that the summary text
// mentions dismissal/deferral options for non-blocking interactions.
func TestLearningOverlay_DismissNonBlocking(t *testing.T) {
	interactions := []archaeolearning.Interaction{
		{
			ID:       "nb-1",
			Title:    "non-blocking hint",
			Kind:     archaeolearning.InteractionPatternProposal,
			Blocking: false,
		},
	}

	summary := formatPendingLearningSummary(interactions)

	require.Contains(t, summary, "[non-blocking]")
	// Footer should mention dismiss/defer.
	lower := strings.ToLower(summary)
	require.True(t,
		strings.Contains(lower, "dismiss") || strings.Contains(lower, "defer"),
		"summary should mention dismiss or defer for non-blocking interactions",
	)
}

// ---------------------------------------------------------------------------
// Keybind configuration tests
// ---------------------------------------------------------------------------

// TestKeybindConfig_AllNewBinds verifies that all Phase 2–5 keybinds are
// declared in GlobalKeys and have non-empty key assignments.
func TestKeybindConfig_AllNewBinds(t *testing.T) {
	// Verify each binding has keys declared via FullHelp (which lists all groups).
	full := GlobalKeys.FullHelp()
	require.NotEmpty(t, full, "FullHelp should return keybind groups")

	// Count non-empty groups — should include blob/service/explore sections.
	foundGroups := 0
	for _, group := range full {
		if len(group) > 0 {
			foundGroups++
		}
	}
	require.Greater(t, foundGroups, 4, "FullHelp should include blob/service/explore groups")

	// Spot-check: SidebarToggle should have ctrl+] assigned.
	keys := GlobalKeys.SidebarToggle.Keys()
	require.NotEmpty(t, keys, "SidebarToggle should have key assignments")
	require.Contains(t, keys, "ctrl+]")

	// Spot-check: ServiceStop should be 's'.
	svcKeys := GlobalKeys.ServiceStop.Keys()
	require.Contains(t, svcKeys, "s")

	// Spot-check: ExplorePromoteAll should be ctrl+p.
	promoKeys := GlobalKeys.ExplorePromoteAll.Keys()
	require.Contains(t, promoKeys, "ctrl+p")
}

// ---------------------------------------------------------------------------
// Titlebar blob count tests
// ---------------------------------------------------------------------------

// TestTitlebar_BlobCounts_ArchaeoTab verifies that when the archaeo tab is
// active, the title bar renders emoji blob count badges.
func TestTitlebar_BlobCounts_ArchaeoTab(t *testing.T) {
	tb := NewTitleBar(SessionInfo{Agent: "euclo", Model: "qwen"})
	tb.SetWidth(120)
	tb.SetActiveTab(TabArchaeo)
	tb.SetBlobCounts(3, 2, 1)
	tb.SetBlobEmoji(true)

	view := tb.View()
	require.Contains(t, view, "⚡3", "should show tension count with emoji")
	require.Contains(t, view, "🧩2", "should show pattern count with emoji")
	require.Contains(t, view, "💡1", "should show learning count with emoji")
}

// TestTitlebar_BlobCounts_EmojiOff verifies that letter badges appear when
// blob emoji rendering is disabled.
func TestTitlebar_BlobCounts_EmojiOff(t *testing.T) {
	tb := NewTitleBar(SessionInfo{Agent: "euclo", Model: "qwen"})
	tb.SetWidth(120)
	tb.SetActiveTab(TabArchaeo)
	tb.SetBlobCounts(3, 2, 1)
	tb.SetBlobEmoji(false)

	view := tb.View()
	require.Contains(t, view, "T:3", "should show tension count with letter badge")
	require.Contains(t, view, "P:2", "should show pattern count with letter badge")
	require.Contains(t, view, "L:1", "should show learning count with letter badge")
}

// TestTitlebar_NonArchaeoTab verifies that blob counts are NOT shown when a
// non-archaeo tab is active.
func TestTitlebar_NonArchaeoTab(t *testing.T) {
	tb := NewTitleBar(SessionInfo{Agent: "euclo", Model: "qwen"})
	tb.SetWidth(120)
	tb.SetActiveTab(TabChat)
	tb.SetBlobCounts(3, 2, 1)
	tb.SetBlobEmoji(true)

	view := tb.View()
	require.NotContains(t, view, "⚡3", "non-archaeo tab should not show blob emoji counts")
	require.NotContains(t, view, "T:3", "non-archaeo tab should not show blob letter counts")
}

// ---------------------------------------------------------------------------
// Plan history subtab tests
// ---------------------------------------------------------------------------

// TestPlanHistorySubtab_Registration verifies that the history subtab is
// registered in the archaeo tab definition.
func TestPlanHistorySubtab_Registration(t *testing.T) {
	reg := NewTabRegistry()
	registerEucloTabs(reg)

	var archaeoDef TabDefinition
	for _, tab := range reg.All() {
		if tab.ID == TabArchaeo {
			archaeoDef = tab
			break
		}
	}
	ids := make([]SubTabID, len(archaeoDef.SubTabs))
	for i, st := range archaeoDef.SubTabs {
		ids[i] = st.ID
	}
	require.Contains(t, ids, SubTabArchaeoHistory, "history subtab should be registered")
}

// TestPlanHistorySubtab_ListVersions verifies that PlanHistoryUpdatedMsg
// populates the history list and the view renders version entries.
func TestPlanHistorySubtab_ListVersions(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoHistory)

	versions := []PlanVersionInfo{
		{Version: 2, Status: "active", ExplorationRef: "abc12345", StepCount: 4},
		{Version: 1, Status: "superseded", ExplorationRef: "xyz98765", StepCount: 3},
	}
	ap, _ := p.Update(PlanHistoryUpdatedMsg{Versions: versions})
	p = ap.(*ArchaeoPane)

	view := p.View()
	require.Contains(t, view, "v2", "version 2 should be rendered")
	require.Contains(t, view, "[active  ]", "active status badge should appear")
	require.Contains(t, view, "4 steps", "step count should be rendered")
	require.Contains(t, view, "abc12345", "exploration ref should appear")
	require.Contains(t, view, "v1", "version 1 should be rendered")
	require.Contains(t, view, "[supersed]", "superseded badge should appear")
}

// TestPlanHistorySubtab_EmptyList verifies the empty-state message.
func TestPlanHistorySubtab_EmptyList(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoHistory)

	view := p.View()
	require.Contains(t, view, "no plan versions", "empty state should display")
}

// TestPlanHistorySubtab_Navigation verifies j/k navigation updates the cursor.
func TestPlanHistorySubtab_Navigation(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoHistory)

	versions := []PlanVersionInfo{
		{Version: 3, Status: "active"},
		{Version: 2, Status: "superseded"},
		{Version: 1, Status: "archived"},
	}
	ap, _ := p.Update(PlanHistoryUpdatedMsg{Versions: versions})
	p = ap.(*ArchaeoPane)

	require.Equal(t, 0, p.historySel)

	ap, _ = p.Update(keyMsg("j"))
	p = ap.(*ArchaeoPane)
	require.Equal(t, 1, p.historySel)

	ap, _ = p.Update(keyMsg("k"))
	p = ap.(*ArchaeoPane)
	require.Equal(t, 0, p.historySel)
}

// TestPlanHistorySubtab_ActivateVersion verifies that pressing Enter on a
// non-active version dispatches an ActivatePlanVersion command.
func TestPlanHistorySubtab_ActivateVersion(t *testing.T) {
	rt := &phase6HistoryRuntime{}
	p := NewArchaeoPane(rt)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoHistory)

	versions := []PlanVersionInfo{
		{Version: 2, Status: "active"},
		{Version: 1, Status: "superseded"},
	}
	ap, _ := p.Update(PlanHistoryUpdatedMsg{Versions: versions})
	p = ap.(*ArchaeoPane)

	// Navigate to version 1 (superseded).
	ap, _ = p.Update(keyMsg("j"))
	p = ap.(*ArchaeoPane)
	require.Equal(t, 1, p.historySel)

	// Press Enter — should dispatch activate command.
	_, cmd := p.Update(keyMsg("enter"))
	require.NotNil(t, cmd, "entering a non-active version should dispatch a command")

	// Execute the command — should call runtime.ActivatePlanVersion.
	msg := cmd()
	activated, ok := msg.(planVersionActivatedMsg)
	require.True(t, ok, "command should return planVersionActivatedMsg")
	require.Equal(t, 1, activated.version)
	require.True(t, rt.activateCalled, "ActivatePlanVersion should be called on the runtime")
}

// TestPlanHistorySubtab_ActivateActiveVersion verifies that pressing Enter on
// the already-active version is a no-op (no command dispatched).
func TestPlanHistorySubtab_ActivateActiveVersion(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoHistory)

	versions := []PlanVersionInfo{
		{Version: 1, Status: "active"},
	}
	ap, _ := p.Update(PlanHistoryUpdatedMsg{Versions: versions})
	p = ap.(*ArchaeoPane)

	_, cmd := p.Update(keyMsg("enter"))
	require.Nil(t, cmd, "enter on active version should be no-op")
}

// TestPlanHistorySubtab_DiffToggle verifies the 'd' key toggles the diff view
// for the focused version.
func TestPlanHistorySubtab_DiffToggle(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoHistory)

	versions := []PlanVersionInfo{
		{Version: 2, Status: "active", StepCount: 3},
		{Version: 1, Status: "superseded", StepCount: 2},
	}
	ap, _ := p.Update(PlanHistoryUpdatedMsg{Versions: versions})
	p = ap.(*ArchaeoPane)

	// No diff selected yet.
	require.Equal(t, 0, p.historyDiffSel)

	// Press 'd' on version 2.
	ap, _ = p.Update(keyMsg("d"))
	p = ap.(*ArchaeoPane)
	require.Equal(t, 2, p.historyDiffSel, "diff should be set to focused version")

	view := p.View()
	require.Contains(t, view, "← selected", "selected version should be marked in view")

	// Press 'd' again to clear.
	ap, _ = p.Update(keyMsg("d"))
	p = ap.(*ArchaeoPane)
	require.Equal(t, 0, p.historyDiffSel, "pressing d again should clear diff selection")
}

// ---------------------------------------------------------------------------
// countBlobsByKind tests
// ---------------------------------------------------------------------------

// TestCountBlobsByKind verifies the helper correctly tallies each blob kind.
func TestCountBlobsByKind(t *testing.T) {
	blobs := []BlobEntry{
		{Kind: BlobTension},
		{Kind: BlobTension},
		{Kind: BlobPattern},
		{Kind: BlobLearning},
		{Kind: BlobLearning},
		{Kind: BlobLearning},
	}
	tensions, patterns, learning := countBlobsByKind(blobs)
	require.Equal(t, 2, tensions)
	require.Equal(t, 1, patterns)
	require.Equal(t, 3, learning)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// phase6HistoryRuntime is a full RuntimeAdapter stub for plan history tests.
type phase6HistoryRuntime struct {
	activateCalled   bool
	activatedVersion int
}

func (r *phase6HistoryRuntime) ExecuteInstruction(context.Context, string, core.TaskType, map[string]any) (*core.Result, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) ExecuteInstructionStream(context.Context, string, core.TaskType, map[string]any, func(string)) (*core.Result, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) AvailableAgents() []string                      { return nil }
func (r *phase6HistoryRuntime) SwitchAgent(string) error                       { return nil }
func (r *phase6HistoryRuntime) SetInteractionEmitter(interaction.FrameEmitter) {}
func (r *phase6HistoryRuntime) SessionInfo() SessionInfo                       { return SessionInfo{} }
func (r *phase6HistoryRuntime) ResolveContextFiles(context.Context, []string) ContextFileResolution {
	return ContextFileResolution{}
}
func (r *phase6HistoryRuntime) SessionArtifacts() SessionArtifacts { return SessionArtifacts{} }
func (r *phase6HistoryRuntime) InferenceModels(context.Context) ([]string, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) RecordingMode() string                                  { return "off" }
func (r *phase6HistoryRuntime) SetRecordingMode(string) error                          { return nil }
func (r *phase6HistoryRuntime) SaveModel(string) error                                 { return nil }
func (r *phase6HistoryRuntime) ContractSummary() *ContractSummary                      { return nil }
func (r *phase6HistoryRuntime) CapabilityAdmissions() []CapabilityAdmissionInfo        { return nil }
func (r *phase6HistoryRuntime) SaveToolPolicy(string, core.AgentPermissionLevel) error { return nil }
func (r *phase6HistoryRuntime) ListToolsInfo() []ToolInfo                              { return nil }
func (r *phase6HistoryRuntime) ListCapabilities() []CapabilityInfo                     { return nil }
func (r *phase6HistoryRuntime) ListPrompts() []PromptInfo                              { return nil }
func (r *phase6HistoryRuntime) ListResources([]string) []ResourceInfo                  { return nil }
func (r *phase6HistoryRuntime) ListLiveProviders() []LiveProviderInfo                  { return nil }
func (r *phase6HistoryRuntime) ListLiveSessions() []LiveProviderSessionInfo            { return nil }
func (r *phase6HistoryRuntime) ListApprovals() []ApprovalInfo                          { return nil }
func (r *phase6HistoryRuntime) GetCapabilityDetail(string) (*CapabilityDetail, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) GetPromptDetail(string) (*PromptDetail, error) { return nil, nil }
func (r *phase6HistoryRuntime) GetResourceDetail(string) (*ResourceDetail, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) GetLiveProviderDetail(string) (*LiveProviderDetail, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) GetLiveSessionDetail(string) (*LiveProviderSessionDetail, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) GetApprovalDetail(string) (*ApprovalDetail, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) GetClassPolicies() map[string]core.AgentPermissionLevel {
	return nil
}
func (r *phase6HistoryRuntime) SetToolPolicyLive(string, core.AgentPermissionLevel)  {}
func (r *phase6HistoryRuntime) SetClassPolicyLive(string, core.AgentPermissionLevel) {}
func (r *phase6HistoryRuntime) ListWorkflows(int) ([]WorkflowInfo, error)            { return nil, nil }
func (r *phase6HistoryRuntime) GetWorkflow(string) (*WorkflowDetails, error)         { return nil, nil }
func (r *phase6HistoryRuntime) CancelWorkflow(string) error                          { return nil }
func (r *phase6HistoryRuntime) ApproveHITL(string, string, fauthorization.GrantScope, time.Duration) error {
	return nil
}
func (r *phase6HistoryRuntime) DenyHITL(string, string) error { return nil }
func (r *phase6HistoryRuntime) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	return nil, func() {}
}
func (r *phase6HistoryRuntime) PendingHITL() []*fauthorization.PermissionRequest { return nil }
func (r *phase6HistoryRuntime) PendingGuidance() []*guidance.GuidanceRequest     { return nil }
func (r *phase6HistoryRuntime) ResolveGuidance(string, string, string) error     { return nil }
func (r *phase6HistoryRuntime) SubscribeGuidance() (<-chan guidance.GuidanceEvent, func()) {
	return nil, func() {}
}
func (r *phase6HistoryRuntime) PendingDeferrals() []guidance.EngineeringObservation { return nil }
func (r *phase6HistoryRuntime) ResolveDeferral(string) error                        { return nil }
func (r *phase6HistoryRuntime) SubscribeLearning() (<-chan archaeolearning.Event, func()) {
	return nil, func() {}
}
func (r *phase6HistoryRuntime) PendingLearning() []archaeolearning.Interaction { return nil }
func (r *phase6HistoryRuntime) ResolveLearning(string, archaeolearning.ResolveInput) error {
	return nil
}
func (r *phase6HistoryRuntime) InvokeCapability(context.Context, string, map[string]any) (*core.ToolResult, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) Diagnostics() DiagnosticsInfo                 { return DiagnosticsInfo{} }
func (r *phase6HistoryRuntime) ApplyChatPolicy(SubTabID) error               { return nil }
func (r *phase6HistoryRuntime) ListServices() []ServiceInfo                  { return nil }
func (r *phase6HistoryRuntime) StopService(string) error                     { return nil }
func (r *phase6HistoryRuntime) RestartService(context.Context, string) error { return nil }
func (r *phase6HistoryRuntime) RestartAllServices(context.Context) error     { return nil }
func (r *phase6HistoryRuntime) LoadActivePlan(context.Context, string) (*ActivePlanView, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) LoadBlobs(context.Context, string) ([]BlobEntry, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) AddBlobToPlan(context.Context, string, string) error { return nil }
func (r *phase6HistoryRuntime) RemoveBlobFromPlan(context.Context, string, string) error {
	return nil
}
func (r *phase6HistoryRuntime) AddFileToContext(string) error    { return nil }
func (r *phase6HistoryRuntime) DropFileFromContext(string) error { return nil }
func (r *phase6HistoryRuntime) ListPlanVersions(context.Context, string) ([]PlanVersionInfo, error) {
	return nil, nil
}
func (r *phase6HistoryRuntime) ActivatePlanVersion(_ context.Context, _ string, version int) error {
	r.activateCalled = true
	r.activatedVersion = version
	return nil
}
func (r *phase6HistoryRuntime) ExecutePlan(context.Context, string) error { return nil }
func (r *phase6HistoryRuntime) ActiveWorkflowID() string                  { return "" }
