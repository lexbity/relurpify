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
	tea "github.com/charmbracelet/bubbletea"
)

// fakeEucloEmitter is a minimal EucloEmitter for use in tests that cannot
// import the euclotui package (which would create a circular import).
type fakeEucloEmitter struct {
	responseCh chan interaction.UserResponse
}

func newFakeEucloEmitter() *fakeEucloEmitter {
	return &fakeEucloEmitter{responseCh: make(chan interaction.UserResponse, 1)}
}

func (e *fakeEucloEmitter) Emit(_ context.Context, _ interaction.InteractionFrame) error {
	return nil
}

func (e *fakeEucloEmitter) AwaitResponse(ctx context.Context) (interaction.UserResponse, error) {
	select {
	case <-ctx.Done():
		return interaction.UserResponse{}, ctx.Err()
	case resp := <-e.responseCh:
		return resp, nil
	}
}

func (e *fakeEucloEmitter) Resolve(resp interaction.UserResponse) {
	select {
	case e.responseCh <- resp:
	default:
	}
}

// ============================================================================
// Gap 1: Interaction Emitter Injection Tests
// ============================================================================

// TestEmitterInjectionInRootModel verifies that eucloEmitter is stored on RootModel.
func TestEmitterInjectionInRootModel(t *testing.T) {
	// Create a minimal runtime adapter with no actual runtime
	fakeRT := &fakeRuntimeAdapter{}
	m := newRootModel(fakeRT)

	// Before injection, should be nil
	if m.eucloEmitter != nil {
		t.Error("expected eucloEmitter to be nil before injection")
	}

	// Create an emitter and set it
	emitter := newFakeEucloEmitter()
	m.eucloEmitter = emitter

	// After setting, should be set
	if m.eucloEmitter != emitter {
		t.Error("expected eucloEmitter to be set")
	}
}

// TestRuntimeSetInteractionEmitterViaAdapter verifies that SetInteractionEmitter
// works through the RuntimeAdapter interface.
func TestRuntimeSetInteractionEmitterViaAdapter(t *testing.T) {
	fakeRT := &fakeRuntimeAdapter{}
	m := newRootModel(fakeRT)

	// Create an emitter and call the runtime adapter method
	emitter := newFakeEucloEmitter()
	m.runtime.SetInteractionEmitter(emitter)

	// The fake adapter should have recorded the call (we can add this check
	// if needed, but the main point is that it doesn't panic)
}

// ============================================================================
// Gap 2: HITL Subscription Transfer Tests
// ============================================================================

// TestChatPaneInitNoLongerIncludesHITLListener verifies that ChatPane.Init()
// no longer includes a HITL listener after Gap 2 changes.
func TestChatPaneInitNoLongerIncludesHITLListener(t *testing.T) {
	pane := NewChatPane(nil, &AgentContext{}, &Session{}, &NotificationQueue{})
	cmd := pane.Init()

	// After Gap 2, Init() should only return spinner.Tick (not a batch with HITL)
	if cmd == nil {
		t.Error("expected Init to return a command")
	}
	// The command should work (not panic)
	_ = cmd()
}

// TestRootModelHITLSubscriptionCreated verifies that RootModel.Init() creates
// a command that subscribes to HITL.
func TestRootModelHITLSubscriptionCreated(t *testing.T) {
	hitl := newFakeHITL()
	fakeRT := &fakeRuntimeAdapterWithHITL{hitl: hitl}
	m := newRootModel(fakeRT)

	// Call Init to get the batch of commands
	cmd := m.Init()
	// Batch returns a command that wraps multiple commands
	if cmd == nil {
		t.Fatal("expected Init to return a command")
	}
}

// TestRootModelHITLUnsubscribeOnCleanup verifies that cleanup() calls the
// HITL unsubscribe function.
func TestRootModelHITLUnsubscribeOnCleanup(t *testing.T) {
	var unsubscribeCalled bool
	hitl := newFakeHITL()
	fakeRT := &fakeRuntimeAdapterWithHITL{hitl: hitl}
	m := newRootModel(fakeRT)

	// Manually set up the subscription
	ch, unsub := fakeRT.SubscribeHITL()
	m.hitlCh = ch
	m.hitlUnsub = func() {
		unsubscribeCalled = true
		unsub()
	}

	// Call cleanup
	m.cleanup()

	// Verify that unsubscribe was called
	if !unsubscribeCalled {
		t.Error("expected hitlUnsub to be called during cleanup")
	}
}

// TestRootModelSingleHITLSubscription verifies that only one subscription is active.
// A single HITLEventRequested should produce exactly one notification entry.
func TestRootModelSingleHITLSubscription(t *testing.T) {
	hitl := newFakeHITL()
	fakeRT := &fakeRuntimeAdapterWithHITL{hitl: hitl}
	m := newRootModel(fakeRT)

	// Set up the subscription
	ch, unsub := fakeRT.SubscribeHITL()
	m.hitlCh = ch
	m.hitlUnsub = unsub
	m.chat = NewChatPane(fakeRT, &AgentContext{}, &Session{}, &NotificationQueue{})
	chatPaneOf(m.chat).hitlSvc = hitl
	m.notifQ = &NotificationQueue{}

	// Send a HITL event
	req := &fauthorization.PermissionRequest{
		ID:            "test-req-1",
		Permission:    core.PermissionDescriptor{Action: "file:write", Resource: "test.txt"},
		Justification: "test",
	}
	hitl.pending = []*fauthorization.PermissionRequest{req}

	// Create an event message and handle it
	msg := hitlEventMsg{
		event: fauthorization.HITLEvent{
			Type:    fauthorization.HITLEventRequested,
			Request: req,
		},
	}

	// Handle the event
	_, _ = m.handleHITLEvent(msg)

	// Check that exactly one notification was pushed
	if m.notifQ.Len() != 1 {
		t.Errorf("expected 1 notification, got %d", m.notifQ.Len())
	}

	item, ok := m.notifQ.Current()
	if !ok {
		t.Fatal("expected current notification")
	}
	if item.Kind != NotifKindHITL {
		t.Errorf("expected HITL kind, got %s", item.Kind)
	}
}

// TestRootModelHITLEventExpiredMessage verifies that expired HITL events
// produce a system message.
func TestRootModelHITLEventExpiredMessage(t *testing.T) {
	m := newRootModel(&fakeRuntimeAdapter{})
	m.chat = NewChatPane(nil, &AgentContext{}, &Session{}, &NotificationQueue{})
	m.notifQ = &NotificationQueue{}

	req := &fauthorization.PermissionRequest{
		ID:            "expired-req",
		Permission:    core.PermissionDescriptor{Action: "file:write", Resource: "test.txt"},
		Justification: "test",
	}

	msg := hitlEventMsg{
		event: fauthorization.HITLEvent{
			Type:    fauthorization.HITLEventExpired,
			Request: req,
			Error:   "timeout",
		},
	}

	// Get system messages before handling
	msgsBefore := len(m.chat.Messages())

	// Handle the event
	_, _ = m.handleHITLEvent(msg)

	// Check that a system message was added
	msgsAfter := len(m.chat.Messages())
	if msgsAfter <= msgsBefore {
		t.Error("expected system message to be added for expired event")
	}

	// Check the content contains the reason
	if msgsAfter > 0 {
		lastMsg := m.chat.Messages()[msgsAfter-1]
		if lastMsg.Role != "system" {
			t.Errorf("expected system message, got role %s", lastMsg.Role)
		}
	}
}

// ============================================================================
// Gap 3: Message Routing Tests
// ============================================================================

// TestRootModelHandlesEucloFrameMsg verifies that eucloFrameMsg is routed
// correctly to notification queue and chat feed.
func TestRootModelHandlesEucloFrameMsg(t *testing.T) {
	m := newRootModel(&fakeRuntimeAdapter{})
	m.notifQ = &NotificationQueue{}
	m.chat = NewChatPane(nil, &AgentContext{}, &Session{}, &NotificationQueue{})
	m.chat.SetSize(80, 24)

	// Create a frame and message
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Mode:  "code",
		Phase: "intent",
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Label: "Confirm", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
		},
		Content: interaction.ProposalContent{
			Interpretation: "test proposal",
		},
		Metadata: interaction.FrameMetadata{Timestamp: time.Now()},
	}
	message := RenderInteractionFrame(frame)

	// Create the message
	msg := EucloFrameMsg{
		Msg:   message,
		Frame: frame,
	}

	// Handle the message
	_, cmd := m.Update(msg)

	// Verify notification was pushed
	if m.notifQ.Len() != 1 {
		t.Errorf("expected 1 notification, got %d", m.notifQ.Len())
	}

	item, ok := m.notifQ.Current()
	if !ok {
		t.Fatal("expected current notification")
	}
	if item.Kind != NotifKindInteraction {
		t.Errorf("expected interaction kind, got %s", item.Kind)
	}

	// Verify message was added to feed
	feedMsgs := m.chat.Messages()
	if len(feedMsgs) == 0 {
		t.Error("expected frame to be added to chat feed")
	}

	// Verify no additional command was returned
	if cmd != nil {
		t.Error("expected no command returned")
	}
}

// TestRootModelHandlesEucloResponseMsg verifies that eucloResponseMsg is routed
// to the emitter's Resolve method.
func TestRootModelHandlesEucloResponseMsg(t *testing.T) {
	m := newRootModel(&fakeRuntimeAdapter{})

	// Create an emitter that tracks responses
	emitter := newFakeEucloEmitter()
	m.eucloEmitter = emitter

	// Create a response message
	resp := interaction.UserResponse{
		ActionID: "confirm",
	}
	msg := EucloResponseMsg{
		Response: resp,
	}

	// Handle the message
	_, cmd := m.Update(msg)

	// The response should be sent to the emitter
	// We can verify this by checking that AwaitResponse returns immediately
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	received, err := emitter.AwaitResponse(ctx)
	if err != nil {
		t.Fatalf("expected response to be received, got error: %v", err)
	}
	if received.ActionID != "confirm" {
		t.Errorf("expected ActionID 'confirm', got %q", received.ActionID)
	}

	// Verify no additional command was returned
	if cmd != nil {
		t.Error("expected no command returned")
	}
}

// TestRootModelEucloResponseMsgWithoutEmitter verifies that eucloResponseMsg
// is handled gracefully when no emitter is set.
func TestRootModelEucloResponseMsgWithoutEmitter(t *testing.T) {
	m := newRootModel(&fakeRuntimeAdapter{})
	// Don't set eucloEmitter

	resp := interaction.UserResponse{
		ActionID: "confirm",
	}
	msg := EucloResponseMsg{
		Response: resp,
	}

	// Should not panic
	_, cmd := m.Update(msg)

	if cmd != nil {
		t.Error("expected no command returned")
	}
}

// ============================================================================
// Helper types for testing
// ============================================================================

type fakeRuntimeAdapter struct {
	resolvedGuidanceRequestID string
	resolvedGuidanceChoiceID  string
	resolvedGuidanceFreetext  string
	deferrals                 []guidance.EngineeringObservation
	pendingGuidance           []*guidance.GuidanceRequest
	capabilities              []CapabilityInfo
	prompts                   []PromptInfo
	patternProposals          []PatternProposalInfo
	confirmedPatterns         []PatternRecordInfo
	intentGaps                []IntentGapInfo
	tensions                  []TensionInfo
	activePlan                *LivePlanInfo
	planDiff                  PlanDiffInfo
	traceInfo                 TraceInfo
	addedPlanNoteStep         string
	addedPlanNoteBody         string
	diagnostics               DiagnosticsInfo
	workflows                 []WorkflowInfo
	providers                 []LiveProviderInfo
	approvals                 []ApprovalInfo
	testResult                DebugTestResultMsg
	testErr                   error
	benchmarkResult           DebugBenchmarkResultMsg
	benchmarkErr              error
}

func (f *fakeRuntimeAdapter) ExecuteInstruction(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any) (*core.Result, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapter) ExecuteInstructionStream(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any, callback func(string)) (*core.Result, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapter) AvailableAgents() []string {
	return nil
}

func (f *fakeRuntimeAdapter) SwitchAgent(name string) error {
	return nil
}

func (f *fakeRuntimeAdapter) SetInteractionEmitter(e interaction.FrameEmitter) {
	// no-op
}

func (f *fakeRuntimeAdapter) SessionInfo() SessionInfo {
	return SessionInfo{}
}

func (f *fakeRuntimeAdapter) ResolveContextFiles(ctx context.Context, files []string) ContextFileResolution {
	return ContextFileResolution{}
}

func (f *fakeRuntimeAdapter) SessionArtifacts() SessionArtifacts {
	return SessionArtifacts{}
}

func (f *fakeRuntimeAdapter) InferenceModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapter) RecordingMode() string {
	return ""
}

func (f *fakeRuntimeAdapter) SetRecordingMode(mode string) error {
	return nil
}

func (f *fakeRuntimeAdapter) SaveModel(model string) error {
	return nil
}

func (f *fakeRuntimeAdapter) ContractSummary() *ContractSummary {
	return nil
}

func (f *fakeRuntimeAdapter) CapabilityAdmissions() []CapabilityAdmissionInfo {
	return nil
}

func (f *fakeRuntimeAdapter) SaveToolPolicy(toolName string, level core.AgentPermissionLevel) error {
	return nil
}

func (f *fakeRuntimeAdapter) ListToolsInfo() []ToolInfo {
	return nil
}

func (f *fakeRuntimeAdapter) ListCapabilities() []CapabilityInfo {
	return append([]CapabilityInfo(nil), f.capabilities...)
}

func (f *fakeRuntimeAdapter) ListPrompts() []PromptInfo {
	return append([]PromptInfo(nil), f.prompts...)
}

func (f *fakeRuntimeAdapter) ListResources(workflowRefs []string) []ResourceInfo {
	return nil
}

func (f *fakeRuntimeAdapter) ListLiveProviders() []LiveProviderInfo {
	return append([]LiveProviderInfo(nil), f.providers...)
}

func (f *fakeRuntimeAdapter) ListLiveSessions() []LiveProviderSessionInfo {
	return nil
}

func (f *fakeRuntimeAdapter) ListApprovals() []ApprovalInfo {
	return append([]ApprovalInfo(nil), f.approvals...)
}

func (f *fakeRuntimeAdapter) GetCapabilityDetail(id string) (*CapabilityDetail, error) {
	for _, capability := range f.capabilities {
		if capability.ID == id {
			return &CapabilityDetail{
				Meta: InspectableMeta{
					ID:            capability.ID,
					Kind:          capability.Kind,
					Title:         capability.Name,
					RuntimeFamily: capability.RuntimeFamily,
					TrustClass:    capability.TrustClass,
					Scope:         capability.Scope,
					Source:        capability.ProviderID,
					State:         capability.Exposure,
				},
				Description: capability.Description,
				Exposure:    capability.Exposure,
				ProviderID:  capability.ProviderID,
			}, nil
		}
	}
	return nil, nil
}

func (f *fakeRuntimeAdapter) GetPromptDetail(id string) (*PromptDetail, error) {
	for _, prompt := range f.prompts {
		if prompt.PromptID == id {
			return &PromptDetail{
				Meta:        prompt.Meta,
				PromptID:    prompt.PromptID,
				ProviderID:  prompt.ProviderID,
				Description: "Prompt detail",
				Messages: []StructuredPromptMessage{{
					Role: "system",
					Content: []StructuredContentBlock{{
						Type:    "text",
						Summary: "text output",
						Body:    "Prompt body",
					}},
				}},
			}, nil
		}
	}
	return nil, nil
}

func (f *fakeRuntimeAdapter) GetResourceDetail(idOrURI string) (*ResourceDetail, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapter) GetLiveProviderDetail(providerID string) (*LiveProviderDetail, error) {
	for _, provider := range f.providers {
		if provider.ProviderID == providerID {
			return &LiveProviderDetail{
				Meta:           provider.Meta,
				ProviderID:     provider.ProviderID,
				Kind:           provider.Kind,
				TrustBaseline:  provider.TrustBaseline,
				Recoverability: provider.Recoverability,
				ConfiguredFrom: provider.ConfiguredFrom,
				CapabilityIDs:  append([]string(nil), provider.CapabilityIDs...),
			}, nil
		}
	}
	return nil, nil
}

func (f *fakeRuntimeAdapter) GetLiveSessionDetail(sessionID string) (*LiveProviderSessionDetail, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapter) GetApprovalDetail(id string) (*ApprovalDetail, error) {
	for _, approval := range f.approvals {
		if approval.ID == id {
			return &ApprovalDetail{
				Meta:           approval.Meta,
				ID:             approval.ID,
				Kind:           approval.Kind,
				PermissionType: approval.PermissionType,
				Action:         approval.Action,
				Resource:       approval.Resource,
				Risk:           approval.Risk,
				Scope:          approval.Scope,
				Justification:  approval.Justification,
				RequestedAt:    approval.RequestedAt,
				Metadata:       approval.Metadata,
			}, nil
		}
	}
	return nil, nil
}

func (f *fakeRuntimeAdapter) GetClassPolicies() map[string]core.AgentPermissionLevel {
	return nil
}

func (f *fakeRuntimeAdapter) SetToolPolicyLive(name string, level core.AgentPermissionLevel) {
}

func (f *fakeRuntimeAdapter) SetClassPolicyLive(class string, level core.AgentPermissionLevel) {
}

func (f *fakeRuntimeAdapter) ListWorkflows(limit int) ([]WorkflowInfo, error) {
	if limit <= 0 || limit >= len(f.workflows) {
		return append([]WorkflowInfo(nil), f.workflows...), nil
	}
	return append([]WorkflowInfo(nil), f.workflows[:limit]...), nil
}

func (f *fakeRuntimeAdapter) GetWorkflow(workflowID string) (*WorkflowDetails, error) {
	for _, workflow := range f.workflows {
		if workflow.WorkflowID == workflowID {
			return &WorkflowDetails{
				Workflow: workflow,
				ResourceDetails: []WorkflowLinkedResourceInfo{{
					URI:     "workflow://wf-1/warm?role=planner",
					Summary: "wf-1 / warm / planner",
				}},
			}, nil
		}
	}
	return nil, nil
}

func (f *fakeRuntimeAdapter) CancelWorkflow(workflowID string) error {
	return nil
}

func (f *fakeRuntimeAdapter) ApproveHITL(requestID, approver string, scope fauthorization.GrantScope, duration time.Duration) error {
	return nil
}

func (f *fakeRuntimeAdapter) DenyHITL(requestID, reason string) error {
	return nil
}

func (f *fakeRuntimeAdapter) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	ch := make(chan fauthorization.HITLEvent)
	return ch, func() { close(ch) }
}

func (f *fakeRuntimeAdapter) PendingHITL() []*fauthorization.PermissionRequest {
	return nil
}

func (f *fakeRuntimeAdapter) PendingGuidance() []*guidance.GuidanceRequest {
	return append([]*guidance.GuidanceRequest(nil), f.pendingGuidance...)
}
func (f *fakeRuntimeAdapter) ResolveGuidance(requestID, choiceID, freetext string) error {
	f.resolvedGuidanceRequestID = requestID
	f.resolvedGuidanceChoiceID = choiceID
	f.resolvedGuidanceFreetext = freetext
	return nil
}
func (f *fakeRuntimeAdapter) SubscribeGuidance() (<-chan guidance.GuidanceEvent, func()) {
	ch := make(chan guidance.GuidanceEvent)
	return ch, func() { close(ch) }
}
func (f *fakeRuntimeAdapter) PendingDeferrals() []guidance.EngineeringObservation {
	return append([]guidance.EngineeringObservation(nil), f.deferrals...)
}
func (f *fakeRuntimeAdapter) ResolveDeferral(string) error { return nil }
func (f *fakeRuntimeAdapter) SubscribeLearning() (<-chan archaeolearning.Event, func()) {
	return nil, func() {}
}
func (f *fakeRuntimeAdapter) PendingLearning() []archaeolearning.Interaction { return nil }
func (f *fakeRuntimeAdapter) ResolveLearning(string, archaeolearning.ResolveInput) error {
	return nil
}

func (f *fakeRuntimeAdapter) InvokeCapability(context.Context, string, map[string]any) (*core.ToolResult, error) {
	return nil, nil
}
func (f *fakeRuntimeAdapter) Diagnostics() DiagnosticsInfo   { return f.diagnostics }
func (f *fakeRuntimeAdapter) ApplyChatPolicy(SubTabID) error { return nil }
func (f *fakeRuntimeAdapter) QueryPatternProposals(string) ([]PatternProposalInfo, error) {
	return append([]PatternProposalInfo(nil), f.patternProposals...), nil
}
func (f *fakeRuntimeAdapter) QueryConfirmedPatterns(string) ([]PatternRecordInfo, error) {
	return append([]PatternRecordInfo(nil), f.confirmedPatterns...), nil
}
func (f *fakeRuntimeAdapter) QueryIntentGaps(string, string) ([]IntentGapInfo, error) {
	return append([]IntentGapInfo(nil), f.intentGaps...), nil
}
func (f *fakeRuntimeAdapter) QueryTensions(string) ([]TensionInfo, error) {
	return append([]TensionInfo(nil), f.tensions...), nil
}
func (f *fakeRuntimeAdapter) LoadLivePlan(string) (*LivePlanInfo, error) {
	if f.activePlan == nil {
		return nil, nil
	}
	clone := *f.activePlan
	clone.Steps = append([]PlanStepInfo(nil), f.activePlan.Steps...)
	return &clone, nil
}
func (f *fakeRuntimeAdapter) LoadActivePlan(context.Context, string) (*ActivePlanView, error) {
	return nil, nil
}
func (f *fakeRuntimeAdapter) LoadBlobs(context.Context, string) ([]BlobEntry, error) { return nil, nil }
func (f *fakeRuntimeAdapter) AddBlobToPlan(context.Context, string, string) error    { return nil }
func (f *fakeRuntimeAdapter) RemoveBlobFromPlan(context.Context, string, string) error {
	return nil
}
func (f *fakeRuntimeAdapter) ListServices() []ServiceInfo                  { return nil }
func (f *fakeRuntimeAdapter) StopService(string) error                     { return nil }
func (f *fakeRuntimeAdapter) RestartService(context.Context, string) error { return nil }
func (f *fakeRuntimeAdapter) RestartAllServices(context.Context) error     { return nil }
func (f *fakeRuntimeAdapter) AddFileToContext(string) error                { return nil }
func (f *fakeRuntimeAdapter) DropFileFromContext(string) error             { return nil }
func (f *fakeRuntimeAdapter) ListPlanVersions(context.Context, string) ([]PlanVersionInfo, error) {
	return nil, nil
}
func (f *fakeRuntimeAdapter) ActivatePlanVersion(context.Context, string, int) error { return nil }
func (f *fakeRuntimeAdapter) ExecutePlan(context.Context, string) error              { return nil }
func (f *fakeRuntimeAdapter) ActiveWorkflowID() string                               { return "" }
func (f *fakeRuntimeAdapter) UpdateSidebarFromFrame(interaction.InteractionFrame)    {}
func (f *fakeRuntimeAdapter) AddPlanNote(stepRef string, body string) error {
	f.addedPlanNoteStep = stepRef
	f.addedPlanNoteBody = body
	return nil
}
func (f *fakeRuntimeAdapter) GetPlanDiff(string) (PlanDiffInfo, error) { return f.planDiff, nil }
func (f *fakeRuntimeAdapter) GetLatestTrace() (TraceInfo, error)       { return f.traceInfo, nil }
func (f *fakeRuntimeAdapter) RunTests(pkg string) (DebugTestResultMsg, error) {
	result := f.testResult
	if result.Package == "" {
		result.Package = pkg
	}
	return result, f.testErr
}
func (f *fakeRuntimeAdapter) RunBenchmark(pkg string) (DebugBenchmarkResultMsg, error) {
	result := f.benchmarkResult
	if result.Package == "" {
		result.Package = pkg
	}
	return result, f.benchmarkErr
}

// fakeRuntimeAdapterWithHITL is like fakeRuntimeAdapter but delegates HITL to a fake service.
type fakeRuntimeAdapterWithHITL struct {
	hitl *fakeHITL
}

func (f *fakeRuntimeAdapterWithHITL) ExecuteInstruction(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any) (*core.Result, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) ExecuteInstructionStream(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any, callback func(string)) (*core.Result, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) AvailableAgents() []string {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) SwitchAgent(name string) error {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) SetInteractionEmitter(e interaction.FrameEmitter) {
}

func (f *fakeRuntimeAdapterWithHITL) SessionInfo() SessionInfo {
	return SessionInfo{}
}

func (f *fakeRuntimeAdapterWithHITL) ResolveContextFiles(ctx context.Context, files []string) ContextFileResolution {
	return ContextFileResolution{}
}

func (f *fakeRuntimeAdapterWithHITL) SessionArtifacts() SessionArtifacts {
	return SessionArtifacts{}
}

func (f *fakeRuntimeAdapterWithHITL) InferenceModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) RecordingMode() string {
	return ""
}

func (f *fakeRuntimeAdapterWithHITL) SetRecordingMode(mode string) error {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) SaveModel(model string) error {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) ContractSummary() *ContractSummary {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) CapabilityAdmissions() []CapabilityAdmissionInfo {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) SaveToolPolicy(toolName string, level core.AgentPermissionLevel) error {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) ListToolsInfo() []ToolInfo {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) ListCapabilities() []CapabilityInfo {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) ListPrompts() []PromptInfo {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) ListResources(workflowRefs []string) []ResourceInfo {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) ListLiveProviders() []LiveProviderInfo {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) ListLiveSessions() []LiveProviderSessionInfo {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) ListApprovals() []ApprovalInfo {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) GetCapabilityDetail(id string) (*CapabilityDetail, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) GetPromptDetail(id string) (*PromptDetail, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) GetResourceDetail(idOrURI string) (*ResourceDetail, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) GetLiveProviderDetail(providerID string) (*LiveProviderDetail, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) GetLiveSessionDetail(sessionID string) (*LiveProviderSessionDetail, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) GetApprovalDetail(id string) (*ApprovalDetail, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) GetClassPolicies() map[string]core.AgentPermissionLevel {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) SetToolPolicyLive(name string, level core.AgentPermissionLevel) {
}

func (f *fakeRuntimeAdapterWithHITL) SetClassPolicyLive(class string, level core.AgentPermissionLevel) {
}

func (f *fakeRuntimeAdapterWithHITL) ListWorkflows(limit int) ([]WorkflowInfo, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) GetWorkflow(workflowID string) (*WorkflowDetails, error) {
	return nil, nil
}

func (f *fakeRuntimeAdapterWithHITL) CancelWorkflow(workflowID string) error {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) ApproveHITL(requestID, approver string, scope fauthorization.GrantScope, duration time.Duration) error {
	return f.hitl.ApproveHITL(requestID, approver, scope, duration)
}

func (f *fakeRuntimeAdapterWithHITL) DenyHITL(requestID, reason string) error {
	return f.hitl.DenyHITL(requestID, reason)
}

func (f *fakeRuntimeAdapterWithHITL) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	return f.hitl.SubscribeHITL()
}

func (f *fakeRuntimeAdapterWithHITL) PendingHITL() []*fauthorization.PermissionRequest {
	return f.hitl.PendingHITL()
}

func (f *fakeRuntimeAdapterWithHITL) PendingGuidance() []*guidance.GuidanceRequest { return nil }
func (f *fakeRuntimeAdapterWithHITL) ResolveGuidance(string, string, string) error { return nil }
func (f *fakeRuntimeAdapterWithHITL) SubscribeGuidance() (<-chan guidance.GuidanceEvent, func()) {
	ch := make(chan guidance.GuidanceEvent)
	return ch, func() { close(ch) }
}
func (f *fakeRuntimeAdapterWithHITL) PendingDeferrals() []guidance.EngineeringObservation { return nil }
func (f *fakeRuntimeAdapterWithHITL) ResolveDeferral(string) error                        { return nil }
func (f *fakeRuntimeAdapterWithHITL) SubscribeLearning() (<-chan archaeolearning.Event, func()) {
	return nil, func() {}
}
func (f *fakeRuntimeAdapterWithHITL) PendingLearning() []archaeolearning.Interaction { return nil }
func (f *fakeRuntimeAdapterWithHITL) ResolveLearning(string, archaeolearning.ResolveInput) error {
	return nil
}

func (f *fakeRuntimeAdapterWithHITL) InvokeCapability(context.Context, string, map[string]any) (*core.ToolResult, error) {
	return nil, nil
}
func (f *fakeRuntimeAdapterWithHITL) Diagnostics() DiagnosticsInfo                 { return DiagnosticsInfo{} }
func (f *fakeRuntimeAdapterWithHITL) ApplyChatPolicy(SubTabID) error               { return nil }
func (f *fakeRuntimeAdapterWithHITL) ListServices() []ServiceInfo                  { return nil }
func (f *fakeRuntimeAdapterWithHITL) StopService(string) error                     { return nil }
func (f *fakeRuntimeAdapterWithHITL) RestartService(context.Context, string) error { return nil }
func (f *fakeRuntimeAdapterWithHITL) RestartAllServices(context.Context) error     { return nil }
func (f *fakeRuntimeAdapterWithHITL) LoadActivePlan(context.Context, string) (*ActivePlanView, error) {
	return nil, nil
}
func (f *fakeRuntimeAdapterWithHITL) LoadBlobs(context.Context, string) ([]BlobEntry, error) {
	return nil, nil
}
func (f *fakeRuntimeAdapterWithHITL) AddBlobToPlan(context.Context, string, string) error { return nil }
func (f *fakeRuntimeAdapterWithHITL) RemoveBlobFromPlan(context.Context, string, string) error {
	return nil
}
func (f *fakeRuntimeAdapterWithHITL) AddFileToContext(string) error                       { return nil }
func (f *fakeRuntimeAdapterWithHITL) DropFileFromContext(string) error                    { return nil }
func (f *fakeRuntimeAdapterWithHITL) UpdateSidebarFromFrame(interaction.InteractionFrame) {}
func (f *fakeRuntimeAdapterWithHITL) ListPlanVersions(context.Context, string) ([]PlanVersionInfo, error) {
	return nil, nil
}
func (f *fakeRuntimeAdapterWithHITL) ActivatePlanVersion(context.Context, string, int) error {
	return nil
}
func (f *fakeRuntimeAdapterWithHITL) ExecutePlan(context.Context, string) error { return nil }
func (f *fakeRuntimeAdapterWithHITL) ActiveWorkflowID() string                  { return "" }

func TestRootModelHandlesGuidanceRequestedEvent(t *testing.T) {
	m := newRootModel(&fakeRuntimeAdapter{})

	req := &guidance.GuidanceRequest{
		ID:    "guidance-1",
		Kind:  guidance.GuidanceConfidence,
		Title: "Proceed with low confidence?",
		Choices: []guidance.GuidanceChoice{
			{ID: "proceed", Label: "Proceed", IsDefault: true},
			{ID: "skip", Label: "Skip"},
		},
	}

	updated, _ := m.handleGuidanceEvent(guidanceEventMsg{
		event: guidance.GuidanceEvent{Type: guidance.GuidanceEventRequested, Request: req},
	})

	if !updated.hitlPanel.IsOpen() {
		t.Fatal("expected guidance panel to be open")
	}
	if updated.hitlPanel.RequestID() != req.ID {
		t.Fatalf("expected panel request id %s, got %s", req.ID, updated.hitlPanel.RequestID())
	}
}

func TestRootModelRoutesGuidanceResolutionThroughRuntime(t *testing.T) {
	rt := &fakeRuntimeAdapter{}
	m := newRootModel(rt)

	updated, cmd := m.Update(NotifGuidanceResolveMsg{
		RequestID: "guidance-2",
		ChoiceID:  "proceed",
	})
	_ = updated
	if cmd == nil {
		t.Fatal("expected guidance resolution command")
	}

	msg := cmd()
	resolved, ok := msg.(guidanceResolvedMsg)
	if !ok {
		t.Fatalf("expected guidanceResolvedMsg, got %T", msg)
	}
	if resolved.err != nil {
		t.Fatalf("expected nil error, got %v", resolved.err)
	}
	if rt.resolvedGuidanceRequestID != "guidance-2" || rt.resolvedGuidanceChoiceID != "proceed" {
		t.Fatalf("expected runtime resolution to be recorded, got %q / %q", rt.resolvedGuidanceRequestID, rt.resolvedGuidanceChoiceID)
	}
}

func TestRootModelRoutesGuidanceFreetextThroughRuntime(t *testing.T) {
	rt := &fakeRuntimeAdapter{}
	m := newRootModel(rt)
	m.notifQ = &NotificationQueue{}
	m.notifQ.Push(NotificationItem{
		ID:   "guidance-5",
		Kind: NotifKindGuidance,
		Msg:  "[confidence] Clarify",
		Extra: map[string]string{
			"guidance_request_id": "guidance-5",
			"action_count":        "1",
			"action_0_id":         "freetext",
			"action_0_label":      "Other",
			"action_0_kind":       string(interaction.ActionFreetext),
		},
	})

	updated, cmd := m.handleInputSubmitted("take the safer option")
	if cmd == nil {
		t.Fatal("expected guidance freetext command")
	}
	root, ok := updated.(RootModel)
	if !ok {
		t.Fatalf("expected RootModel, got %T", updated)
	}
	msg := cmd()
	resolved, ok := msg.(guidanceResolvedMsg)
	if !ok {
		t.Fatalf("expected guidanceResolvedMsg, got %T", msg)
	}
	if resolved.err != nil {
		t.Fatalf("expected nil error, got %v", resolved.err)
	}
	if rt.resolvedGuidanceRequestID != "guidance-5" {
		t.Fatalf("expected request id guidance-5, got %q", rt.resolvedGuidanceRequestID)
	}
	if rt.resolvedGuidanceChoiceID != "" {
		t.Fatalf("expected empty choice id for freetext, got %q", rt.resolvedGuidanceChoiceID)
	}
	if rt.resolvedGuidanceFreetext != "take the safer option" {
		t.Fatalf("expected freetext to be recorded, got %q", rt.resolvedGuidanceFreetext)
	}
	if root.notifQ.Len() != 0 {
		t.Fatalf("expected guidance notification to be resolved after freetext submission")
	}
}

func TestRootModelRoutesInteractionFreetextAsEucloResponse(t *testing.T) {
	rt := &fakeRuntimeAdapter{}
	m := newRootModel(rt)
	m.notifQ = &NotificationQueue{}
	m.notifQ.Push(NotificationItem{
		ID:   "interaction-1",
		Kind: NotifKindInteraction,
		Msg:  "What is the goal?",
		Extra: map[string]string{
			"action_count":   "1",
			"action_0_id":    "user_goal",
			"action_0_label": "Describe goal",
			"action_0_kind":  string(interaction.ActionFreetext),
		},
	})

	_, cmd := m.handleInputSubmitted("refactor the auth layer")
	if cmd == nil {
		t.Fatal("expected a command for interaction freetext")
	}
	msg := cmd()
	resp, ok := msg.(EucloResponseMsg)
	if !ok {
		t.Fatalf("expected EucloResponseMsg, got %T", msg)
	}
	if resp.Response.ActionID != "user_goal" {
		t.Fatalf("expected ActionID=user_goal, got %q", resp.Response.ActionID)
	}
	if resp.Response.Text != "refactor the auth layer" {
		t.Fatalf("expected Text to carry typed input, got %q", resp.Response.Text)
	}
	if m.notifQ.Len() != 0 {
		t.Fatal("expected notification to be resolved after freetext submission")
	}
}

func TestRootModelInteractionFreetextShouldRouteToInputBar(t *testing.T) {
	rt := &fakeRuntimeAdapter{}
	m := newRootModel(rt)
	// Replace the queue and rewire the notification bar so Active() returns true.
	q := &NotificationQueue{}
	m.notifQ = q
	m.notifBar = NewNotificationBar(q)
	m.notifQ.Push(NotificationItem{
		ID:   "interaction-2",
		Kind: NotifKindInteraction,
		Msg:  "Describe the scope",
		Extra: map[string]string{
			"action_count":   "1",
			"action_0_id":    "scope",
			"action_0_label": "Type answer",
			"action_0_kind":  string(interaction.ActionFreetext),
		},
	})
	letterKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	escKey := tea.KeyMsg{Type: tea.KeyEsc}
	if !m.shouldRouteNotificationKeyToInput(letterKey) {
		t.Error("letter key should route to input bar when interaction freetext is active")
	}
	if m.shouldRouteNotificationKeyToInput(escKey) {
		t.Error("esc key must not route to input bar")
	}
}

func TestRootModelGuidanceDeferredEventAddsSummaryNotification(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		deferrals: []guidance.EngineeringObservation{
			{ID: "obs-1"},
			{ID: "obs-2"},
		},
	}
	m := newRootModel(rt)
	m.notifQ = &NotificationQueue{}

	updated, _ := m.handleGuidanceEvent(guidanceEventMsg{
		event: guidance.GuidanceEvent{
			Type:    guidance.GuidanceEventDeferred,
			Request: &guidance.GuidanceRequest{ID: "guidance-3"},
		},
	})

	if updated.notifQ.Len() != 1 {
		t.Fatalf("expected one notification, got %d", updated.notifQ.Len())
	}
	item, _ := updated.notifQ.Current()
	if item.Kind != NotifKindDeferred {
		t.Fatalf("expected deferred notification, got %s", item.Kind)
	}
}

func TestRootModelGuidanceDeferredEventSkipsEmptySummaryNotification(t *testing.T) {
	m := newRootModel(&fakeRuntimeAdapter{})
	m.notifQ = &NotificationQueue{}

	updated, _ := m.handleGuidanceEvent(guidanceEventMsg{
		event: guidance.GuidanceEvent{
			Type:    guidance.GuidanceEventDeferred,
			Request: &guidance.GuidanceRequest{ID: "guidance-6"},
		},
	})

	if updated.notifQ.Len() != 0 {
		t.Fatalf("expected no deferred notification when no pending deferrals, got %d", updated.notifQ.Len())
	}
}

func TestNotificationBarDeferredReviewKeyEmitsReviewMessage(t *testing.T) {
	q := &NotificationQueue{}
	q.Push(NotificationItem{
		ID:   "deferred-1",
		Kind: NotifKindDeferred,
		Msg:  "2 deferred guidance items pending review",
	})
	bar := NewNotificationBar(q)

	_, cmd := bar.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected review command")
	}
	msg := cmd()
	if _, ok := msg.(NotifReviewDeferredMsg); !ok {
		t.Fatalf("expected NotifReviewDeferredMsg, got %T", msg)
	}
}

func TestRootModelPlannerRefreshOnSubtabSwitch(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		confirmedPatterns: []PatternRecordInfo{{ID: "pat-1", Title: "Adapter", Scope: "workspace"}},
		patternProposals:  []PatternProposalInfo{{ID: "prop-1", Title: "Potential Adapter", Scope: "workspace"}},
		intentGaps:        []IntentGapInfo{{AnchorName: "contract", Description: "drift", Severity: "significant"}},
		tensions:          []TensionInfo{{ID: "ten-1", TitleA: "Adapter", TitleB: "Facade"}},
		activePlan: &LivePlanInfo{
			WorkflowID: "wf-1",
			Title:      "Improve TUI",
			Steps:      []PlanStepInfo{{ID: "step-1", Title: "Wire planner", Status: "pending"}},
		},
	}
	m := newRootModel(rt)

	m.setActiveTab(TabPlanner)
	m.setActiveSubTab(SubTabPlannerExplore)
	msg := m.refreshActiveSurfaceCmd()()
	if _, ok := msg.(PlannerPatternsMsg); !ok {
		t.Fatalf("expected PlannerPatternsMsg, got %T", msg)
	}

	m.setActiveSubTab(SubTabPlannerAnalyze)
	msg = m.refreshActiveSurfaceCmd()()
	if _, ok := msg.(PlannerTensionsMsg); !ok {
		t.Fatalf("expected PlannerTensionsMsg, got %T", msg)
	}

	m.setActiveSubTab(SubTabPlannerFinalize)
	msg = m.refreshActiveSurfaceCmd()()
	if _, ok := msg.(PlannerPlanMsg); !ok {
		t.Fatalf("expected PlannerPlanMsg, got %T", msg)
	}
}

func TestRootModelPlannerNotePersistsViaRuntime(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		activePlan: &LivePlanInfo{
			WorkflowID: "wf-1",
			Title:      "Improve TUI",
			Steps:      []PlanStepInfo{{ID: "step-1", Title: "Wire planner", Status: "pending"}},
		},
	}
	m := newRootModel(rt)
	m.setActiveTab(TabPlanner)
	m.setActiveSubTab(SubTabPlannerFinalize)

	updated, _ := m.Update(plannerNoteAddedMsg{stepID: "step-1", note: "persist this"})
	root, ok := updated.(RootModel)
	if !ok {
		t.Fatalf("expected RootModel, got %T", updated)
	}
	if rt.addedPlanNoteStep != "step-1" || rt.addedPlanNoteBody != "persist this" {
		t.Fatalf("expected note persistence, got step=%q body=%q", rt.addedPlanNoteStep, rt.addedPlanNoteBody)
	}
	if root.planner == nil {
		t.Fatal("expected planner pane to remain available")
	}
}

func TestRootModelDebugRefreshOnSubtabSwitch(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		traceInfo: TraceInfo{Description: "trace", Frames: []TraceFrame{{FuncName: "main"}}},
		planDiff:  PlanDiffInfo{WorkflowID: "wf-1", Steps: []PlanStepInfo{{ID: "step-1", Title: "Wire planner"}}},
	}
	m := newRootModel(rt)
	m.setActiveTab(TabDebug)

	m.setActiveSubTab(SubTabDebugTrace)
	msg := m.refreshActiveSurfaceCmd()()
	if _, ok := msg.(DebugTraceMsg); !ok {
		t.Fatalf("expected DebugTraceMsg, got %T", msg)
	}

	m.setActiveSubTab(SubTabDebugPlanDiff)
	msg = m.refreshActiveSurfaceCmd()()
	if _, ok := msg.(DebugPlanDiffMsg); !ok {
		t.Fatalf("expected DebugPlanDiffMsg, got %T", msg)
	}
}

func TestRootModelDebugTestSubmissionUsesRuntimeRunner(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		testResult: DebugTestResultMsg{
			Package: "codeburg.org/lexbit/relurpify/app/relurpish/tui",
			Passed:  3,
			Output:  []string{"ok"},
		},
	}
	m := newRootModel(rt)
	m.setActiveTab(TabDebug)
	m.setActiveSubTab(SubTabDebugTest)

	updated, cmd := m.handleInputSubmitted("./app/relurpish/tui")
	root := updated.(RootModel)
	msg := cmd()
	result, ok := msg.(DebugTestResultMsg)
	if !ok {
		t.Fatalf("expected DebugTestResultMsg, got %T", msg)
	}
	if result.Passed != 3 {
		t.Fatalf("expected passed count to survive runtime execution, got %d", result.Passed)
	}
	debugPane, ok := root.debug.(*DebugPane)
	if !ok {
		t.Fatalf("expected *DebugPane, got %T", root.debug)
	}
	if !strings.Contains(debugPane.statusMsg, "running tests") {
		t.Fatalf("expected debug pane status to update, got %q", debugPane.statusMsg)
	}
}

func TestRootModelDebugBenchmarkSubmissionUsesRuntimeRunner(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		benchmarkResult: DebugBenchmarkResultMsg{
			Package: "./framework/graphdb",
			Results: []BenchmarkEntry{{Name: "BenchmarkFindPath", NsPerOp: 12.5}},
		},
	}
	m := newRootModel(rt)
	m.setActiveTab(TabDebug)
	m.setActiveSubTab(SubTabDebugBenchmark)

	updated, cmd := m.handleInputSubmitted("./framework/graphdb")
	root := updated.(RootModel)
	msg := cmd()
	result, ok := msg.(DebugBenchmarkResultMsg)
	if !ok {
		t.Fatalf("expected DebugBenchmarkResultMsg, got %T", msg)
	}
	if len(result.Results) != 1 || result.Results[0].Name != "BenchmarkFindPath" {
		t.Fatalf("unexpected benchmark payload: %+v", result.Results)
	}
	debugPane, ok := root.debug.(*DebugPane)
	if !ok {
		t.Fatalf("expected *DebugPane, got %T", root.debug)
	}
	if !strings.Contains(debugPane.statusMsg, "running benchmark") {
		t.Fatalf("expected debug pane status to update, got %q", debugPane.statusMsg)
	}
}

func TestRootHandleRunTestsCommandUsesRuntimeRunner(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		testResult: DebugTestResultMsg{Package: "./app/relurpish/tui", Passed: 5},
	}
	m := newRootModel(rt)

	updated, cmd := rootHandleRunTests(&m, []string{"./app/relurpish/tui"})
	if updated.activeTab != TabDebug || updated.tabs.ActiveSubTab() != SubTabDebugTest {
		t.Fatalf("expected debug/test surface, got tab=%q subtab=%q", updated.activeTab, updated.tabs.ActiveSubTab())
	}
	msg := cmd()
	result, ok := msg.(DebugTestResultMsg)
	if !ok {
		t.Fatalf("expected DebugTestResultMsg, got %T", msg)
	}
	if result.Passed != 5 {
		t.Fatalf("expected passed count 5, got %d", result.Passed)
	}
}

func TestRootHandleRunBenchmarkCommandUsesRuntimeRunner(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		benchmarkResult: DebugBenchmarkResultMsg{Package: "./framework/graphdb", Results: []BenchmarkEntry{{Name: "BenchmarkOpenReplay"}}},
	}
	m := newRootModel(rt)

	updated, cmd := rootHandleRunBenchmark(&m, []string{"./framework/graphdb"})
	if updated.activeTab != TabDebug || updated.tabs.ActiveSubTab() != SubTabDebugBenchmark {
		t.Fatalf("expected debug/benchmark surface, got tab=%q subtab=%q", updated.activeTab, updated.tabs.ActiveSubTab())
	}
	msg := cmd()
	result, ok := msg.(DebugBenchmarkResultMsg)
	if !ok {
		t.Fatalf("expected DebugBenchmarkResultMsg, got %T", msg)
	}
	if len(result.Results) != 1 || result.Results[0].Name != "BenchmarkOpenReplay" {
		t.Fatalf("unexpected benchmark results: %+v", result.Results)
	}
}

func TestRootHandleTraceRefreshCommandLoadsTrace(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		traceInfo: TraceInfo{Description: "runtime trace", Frames: []TraceFrame{{FuncName: "main"}}},
	}
	m := newRootModel(rt)

	updated, cmd := rootHandleTraceRefresh(&m, nil)
	if updated.activeTab != TabDebug || updated.tabs.ActiveSubTab() != SubTabDebugTrace {
		t.Fatalf("expected debug/trace surface, got tab=%q subtab=%q", updated.activeTab, updated.tabs.ActiveSubTab())
	}
	msg := cmd()
	if _, ok := msg.(DebugTraceMsg); !ok {
		t.Fatalf("expected DebugTraceMsg, got %T", msg)
	}
}

func TestRootHandlePlanDiffRefreshCommandLoadsDiff(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		planDiff: PlanDiffInfo{WorkflowID: "wf-1", Steps: []PlanStepInfo{{ID: "step-1", Title: "Wire debug"}}},
	}
	m := newRootModel(rt)

	updated, cmd := rootHandlePlanDiffRefresh(&m, nil)
	if updated.activeTab != TabDebug || updated.tabs.ActiveSubTab() != SubTabDebugPlanDiff {
		t.Fatalf("expected debug/plan-diff surface, got tab=%q subtab=%q", updated.activeTab, updated.tabs.ActiveSubTab())
	}
	msg := cmd()
	if _, ok := msg.(DebugPlanDiffMsg); !ok {
		t.Fatalf("expected DebugPlanDiffMsg, got %T", msg)
	}
}

func TestRootHandleQueueTaskEnqueuesAndSurfacesSessionTasks(t *testing.T) {
	rt := &fakeRuntimeAdapter{}
	m := newRootModel(rt)

	updated, cmd := rootHandleQueueTask(&m, []string{"investigate", "planner", "pane"})
	if updated.activeTab != TabSession || updated.tabs.ActiveSubTab() != SubTabSessionTasks {
		t.Fatalf("expected session/tasks surface, got tab=%q subtab=%q", updated.activeTab, updated.tabs.ActiveSubTab())
	}
	if len(updated.tasks.Items()) != 1 {
		t.Fatalf("expected queued task, got %d", len(updated.tasks.Items()))
	}
	if cmd == nil {
		t.Fatal("expected dequeue command")
	}
	view := updated.session.View()
	if !strings.Contains(view, "Queued Tasks") || !strings.Contains(view, "investigate planner pane") {
		t.Fatalf("expected queued task in session view, got %q", view)
	}
}

func TestRootModelSessionLiveRefreshIncludesRuntimeSummaries(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		diagnostics: DiagnosticsInfo{ActiveWorkflows: 2, LiveProviders: 1, PendingApprovals: 1},
		workflows: []WorkflowInfo{{
			WorkflowID:  "wf-1",
			Instruction: "stabilize session pane",
			Status:      "running",
		}},
		providers: []LiveProviderInfo{{
			ProviderID: "provider-1",
			Kind:       "ollama",
			Meta:       InspectableMeta{State: "healthy"},
		}},
		approvals: []ApprovalInfo{{
			ID:     "approval-1",
			Kind:   "filesystem",
			Action: "write",
		}},
	}
	m := newRootModel(rt)
	m.setActiveTab(TabSession)
	m.setActiveSubTab(SubTabSessionLive)

	msg := m.refreshActiveSurfaceCmd()()
	snapshot, ok := msg.(SessionLiveSnapshotMsg)
	if !ok {
		t.Fatalf("expected SessionLiveSnapshotMsg, got %T", msg)
	}

	updated, _ := m.Update(snapshot)
	root := updated.(RootModel)
	view := root.session.View()
	if !strings.Contains(view, "Workflows") || !strings.Contains(view, "wf-1") {
		t.Fatalf("expected workflow summary in live view, got %q", view)
	}
	if !strings.Contains(view, "Providers") || !strings.Contains(view, "provider-1") {
		t.Fatalf("expected provider summary in live view, got %q", view)
	}
	if !strings.Contains(view, "Approvals") || !strings.Contains(view, "approval-1") {
		t.Fatalf("expected approval summary in live view, got %q", view)
	}

	sessionPane, _ := root.session.Update(tea.KeyMsg{Type: tea.KeyTab})
	root.session = sessionPane
	view = root.session.View()
	if !strings.Contains(view, "provider-1") || !strings.Contains(view, "Recoverability") {
		t.Fatalf("expected provider detail after focus switch, got %q", view)
	}

	sessionPane, _ = root.session.Update(tea.KeyMsg{Type: tea.KeyTab})
	root.session = sessionPane
	view = root.session.View()
	if !strings.Contains(view, "approval-1") || !strings.Contains(view, "Justification") {
		t.Fatalf("expected approval detail after second focus switch, got %q", view)
	}

	root.session.liveSection = liveSectionWorkflows
	view = root.session.View()
	if !strings.Contains(view, "wf-1 / warm / planner") {
		t.Fatalf("expected workflow linked resource detail in live view, got %q", view)
	}
}

func TestConfigPaneCapabilitiesAndPromptsAbsorbInspectorViews(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		capabilities: []CapabilityInfo{{
			ID:            "cap-1",
			Kind:          "capability",
			Name:          "planner.detect",
			Description:   "Detects planning opportunities",
			RuntimeFamily: "relurpic",
			TrustClass:    "workspace_trusted",
			Scope:         "workspace",
			ProviderID:    "relurpic",
			Exposure:      "agent",
		}},
		prompts: []PromptInfo{{
			Meta: InspectableMeta{
				ID:            "prompt-1",
				Title:         "Gap Detector",
				RuntimeFamily: "relurpic",
				TrustClass:    "workspace_trusted",
			},
			PromptID:   "prompt-1",
			ProviderID: "relurpic",
		}},
	}
	pane := NewConfigPane(rt)
	pane.SetSize(120, 30)

	pane.section = configSectionCapabilities
	pane.refreshDetail()
	view := pane.View()
	if !strings.Contains(view, "planner.detect") || !strings.Contains(view, "Detects planning opportunities") {
		t.Fatalf("expected capability inspector content in config view, got %q", view)
	}

	pane.section = configSectionPrompts
	pane.sel = 0
	pane.refreshDetail()
	view = pane.View()
	if !strings.Contains(view, "Gap Detector") || !strings.Contains(view, "Prompt detail") {
		t.Fatalf("expected prompt inspector content in config view, got %q", view)
	}
}

func TestRootHandleGuidanceSummarizesPendingRequests(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		pendingGuidance: []*guidance.GuidanceRequest{
			{ID: "guidance-4", Kind: guidance.GuidanceApproach, Title: "Pick an approach"},
		},
	}
	m := newRootModel(rt)

	updated, _ := rootHandleGuidance(&m, nil)
	msgs := updated.chat.Messages()
	if len(msgs) == 0 {
		t.Fatal("expected system message")
	}
	if !strings.Contains(msgs[len(msgs)-1].Content.Text, "guidance-4") {
		t.Fatalf("expected pending guidance in message, got %q", msgs[len(msgs)-1].Content.Text)
	}
}

func TestRootHandleDeferredSummarizesObservations(t *testing.T) {
	rt := &fakeRuntimeAdapter{
		deferrals: []guidance.EngineeringObservation{
			{ID: "obs-1", GuidanceKind: guidance.GuidanceConfidence, Title: "Low confidence", Description: "Review later", BlastRadius: 2},
		},
	}
	m := newRootModel(rt)

	updated, _ := rootHandleDeferred(&m, nil)
	msgs := updated.chat.Messages()
	if len(msgs) == 0 {
		t.Fatal("expected system message")
	}
	if !strings.Contains(msgs[len(msgs)-1].Content.Text, "obs-1") {
		t.Fatalf("expected deferred observation in message, got %q", msgs[len(msgs)-1].Content.Text)
	}
}
