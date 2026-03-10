package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type fakeTasksRuntimeAdapter struct {
	workflows    []WorkflowInfo
	workflowByID map[string]*WorkflowDetails
	capabilities []CapabilityInfo
	prompts      []PromptInfo
	resources    []ResourceInfo
	providers    []LiveProviderInfo
	sessions     []LiveProviderSessionInfo
	approvals    []ApprovalInfo
}

func (f *fakeTasksRuntimeAdapter) ExecuteInstruction(context.Context, string, core.TaskType, map[string]any) (*core.Result, error) {
	return nil, nil
}
func (f *fakeTasksRuntimeAdapter) ExecuteInstructionStream(context.Context, string, core.TaskType, map[string]any, func(string)) (*core.Result, error) {
	return nil, nil
}
func (f *fakeTasksRuntimeAdapter) AvailableAgents() []string { return nil }
func (f *fakeTasksRuntimeAdapter) SwitchAgent(string) error  { return nil }
func (f *fakeTasksRuntimeAdapter) SessionInfo() SessionInfo  { return SessionInfo{} }
func (f *fakeTasksRuntimeAdapter) ResolveContextFiles(context.Context, []string) ContextFileResolution {
	return ContextFileResolution{}
}
func (f *fakeTasksRuntimeAdapter) SessionArtifacts() SessionArtifacts             { return SessionArtifacts{} }
func (f *fakeTasksRuntimeAdapter) OllamaModels(context.Context) ([]string, error) { return nil, nil }
func (f *fakeTasksRuntimeAdapter) RecordingMode() string                          { return "off" }
func (f *fakeTasksRuntimeAdapter) SetRecordingMode(string) error                  { return nil }
func (f *fakeTasksRuntimeAdapter) SaveModel(string) error                         { return nil }
func (f *fakeTasksRuntimeAdapter) SaveToolPolicy(string, core.AgentPermissionLevel) error {
	return nil
}
func (f *fakeTasksRuntimeAdapter) ListToolsInfo() []ToolInfo { return nil }
func (f *fakeTasksRuntimeAdapter) ListCapabilities() []CapabilityInfo {
	return append([]CapabilityInfo(nil), f.capabilities...)
}
func (f *fakeTasksRuntimeAdapter) ListPrompts() []PromptInfo {
	return append([]PromptInfo(nil), f.prompts...)
}
func (f *fakeTasksRuntimeAdapter) ListResources(workflowRefs []string) []ResourceInfo {
	if len(workflowRefs) == 0 {
		return append([]ResourceInfo(nil), f.resources...)
	}
	filtered := make([]ResourceInfo, 0, len(workflowRefs))
	for _, resource := range f.resources {
		for _, ref := range workflowRefs {
			if resource.ResourceID == ref {
				filtered = append(filtered, resource)
			}
		}
	}
	return filtered
}
func (f *fakeTasksRuntimeAdapter) ListLiveProviders() []LiveProviderInfo {
	return append([]LiveProviderInfo(nil), f.providers...)
}
func (f *fakeTasksRuntimeAdapter) ListLiveSessions() []LiveProviderSessionInfo {
	return append([]LiveProviderSessionInfo(nil), f.sessions...)
}
func (f *fakeTasksRuntimeAdapter) ListApprovals() []ApprovalInfo {
	return append([]ApprovalInfo(nil), f.approvals...)
}
func (f *fakeTasksRuntimeAdapter) GetCapabilityDetail(id string) (*CapabilityDetail, error) {
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
				Category:    capability.Category,
				Exposure:    capability.Exposure,
				Callable:    capability.Callable,
				ProviderID:  capability.ProviderID,
			}, nil
		}
	}
	return nil, nil
}
func (f *fakeTasksRuntimeAdapter) GetLiveProviderDetail(providerID string) (*LiveProviderDetail, error) {
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
func (f *fakeTasksRuntimeAdapter) GetPromptDetail(id string) (*PromptDetail, error) {
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
func (f *fakeTasksRuntimeAdapter) GetResourceDetail(id string) (*ResourceDetail, error) {
	for _, resource := range f.resources {
		if resource.ResourceID == id {
			return &ResourceDetail{
				Meta:             resource.Meta,
				ResourceID:       resource.ResourceID,
				ProviderID:       resource.ProviderID,
				WorkflowResource: resource.WorkflowResource,
				WorkflowURI:      resource.WorkflowURI,
				Description:      "Resource detail",
				Contents: []StructuredContentBlock{{
					Type:    "structured",
					Summary: "structured output",
					Body:    `{"hello":"world"}`,
				}},
			}, nil
		}
	}
	return nil, nil
}
func (f *fakeTasksRuntimeAdapter) GetLiveSessionDetail(sessionID string) (*LiveProviderSessionDetail, error) {
	for _, session := range f.sessions {
		if session.SessionID == sessionID {
			return &LiveProviderSessionDetail{
				Meta:            session.Meta,
				SessionID:       session.SessionID,
				ProviderID:      session.ProviderID,
				WorkflowID:      session.WorkflowID,
				TaskID:          session.TaskID,
				Recoverability:  session.Recoverability,
				CapabilityIDs:   append([]string(nil), session.CapabilityIDs...),
				LastActivityAt:  session.LastActivityAt,
				MetadataSummary: append([]string(nil), session.MetadataSummary...),
			}, nil
		}
	}
	return nil, nil
}
func (f *fakeTasksRuntimeAdapter) GetApprovalDetail(id string) (*ApprovalDetail, error) {
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
func (f *fakeTasksRuntimeAdapter) GetClassPolicies() map[string]core.AgentPermissionLevel {
	return nil
}
func (f *fakeTasksRuntimeAdapter) SetToolPolicyLive(string, core.AgentPermissionLevel)  {}
func (f *fakeTasksRuntimeAdapter) SetClassPolicyLive(string, core.AgentPermissionLevel) {}
func (f *fakeTasksRuntimeAdapter) ListWorkflows(int) ([]WorkflowInfo, error) {
	return append([]WorkflowInfo(nil), f.workflows...), nil
}
func (f *fakeTasksRuntimeAdapter) GetWorkflow(workflowID string) (*WorkflowDetails, error) {
	if f.workflowByID == nil {
		return nil, nil
	}
	return f.workflowByID[workflowID], nil
}
func (f *fakeTasksRuntimeAdapter) CancelWorkflow(string) error                      { return nil }
func (f *fakeTasksRuntimeAdapter) PendingHITL() []*fauthorization.PermissionRequest { return nil }
func (f *fakeTasksRuntimeAdapter) ApproveHITL(string, string, fauthorization.GrantScope, time.Duration) error {
	return nil
}
func (f *fakeTasksRuntimeAdapter) DenyHITL(string, string) error { return nil }
func (f *fakeTasksRuntimeAdapter) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	return nil, func() {}
}

func TestTasksPaneWorkflowInspector(t *testing.T) {
	adapter := &fakeTasksRuntimeAdapter{
		workflows: []WorkflowInfo{{
			WorkflowID:  "wf-1",
			Status:      "running",
			Instruction: "Inspect workflow",
		}},
		workflowByID: map[string]*WorkflowDetails{
			"wf-1": {
				Workflow: WorkflowInfo{
					WorkflowID:  "wf-1",
					Status:      "running",
					Instruction: "Inspect workflow",
				},
				Delegations:       []WorkflowDelegationInfo{{DelegationID: "delegation-1"}},
				WorkflowArtifacts: []WorkflowArtifactInfo{{ArtifactID: "artifact-1"}},
				Providers:         []WorkflowProviderInfo{{ProviderID: "provider-1"}},
				ProviderSessions:  []WorkflowProviderSessionInfo{{SessionID: "session-1"}},
				LinkedResources:   []string{"workflow://wf-1/warm?role=planner"},
				ResourceDetails:   []WorkflowLinkedResourceInfo{{URI: "workflow://wf-1/warm?role=planner", Summary: "wf-1 / warm / planner"}},
			},
		},
	}
	pane := NewTasksPane(adapter, nil)
	pane.SetSize(100, 30)

	updated, _ := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	view := updated.View()

	require.Equal(t, taskInspectWorkflows, updated.inspectKind)
	require.Contains(t, view, "Tasks Inspector: Workflows")
	require.Contains(t, view, "wf-1")
	require.Contains(t, view, "Delegations: 1")
	require.Contains(t, view, "Linked resources: workflow://wf-1/warm?role=planner")
	require.Contains(t, view, "Press [r] to browse linked workflow resources")
}

func TestTasksPaneApprovalInspector(t *testing.T) {
	adapter := &fakeTasksRuntimeAdapter{
		approvals: []ApprovalInfo{{
			Meta: InspectableMeta{
				ID:         "approval-1",
				Kind:       "provider_operation",
				Title:      "provider:remote-mcp:activate",
				Source:     "remote-mcp",
				State:      "pending",
				CapturedAt: "2026-03-08T12:00:00Z",
			},
			ID:             "approval-1",
			Kind:           "provider_operation",
			PermissionType: "capability",
			Action:         "provider:remote-mcp:activate",
			Resource:       "remote-mcp",
			Risk:           "medium",
			Scope:          "session",
			Justification:  "activate provider remote-mcp",
			RequestedAt:    time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC),
			Metadata: map[string]string{
				"provider_id": "remote-mcp",
			},
		}},
	}
	pane := NewTasksPane(adapter, nil)
	pane.SetSize(100, 30)

	updated, _ := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	view := updated.View()

	require.Equal(t, taskInspectApprovals, updated.inspectKind)
	require.Contains(t, view, "Tasks Inspector: Approvals")
	require.Contains(t, view, "provider_operation")
	require.Contains(t, view, "provider:remote-mcp:activate")
	require.Contains(t, view, "Justification: activate provider remote-mcp")
}

func TestTasksPanePromptAndResourceInspectors(t *testing.T) {
	adapter := &fakeTasksRuntimeAdapter{
		prompts: []PromptInfo{{
			Meta:       InspectableMeta{ID: "prompt:summary", Kind: "prompt", Title: "summary.prompt", RuntimeFamily: "provider"},
			PromptID:   "prompt:summary",
			ProviderID: "remote-mcp",
		}},
		resources: []ResourceInfo{{
			Meta:             InspectableMeta{ID: "workflow://wf-1/warm?role=planner", Kind: "workflow-resource", Title: "wf-1 / warm / planner", Source: "workflow"},
			ResourceID:       "workflow://wf-1/warm?role=planner",
			WorkflowResource: true,
			WorkflowURI:      "workflow://wf-1/warm?role=planner",
		}},
	}
	pane := NewTasksPane(adapter, nil)
	pane.SetSize(100, 30)

	updated, _ := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	promptView := updated.View()
	require.Equal(t, taskInspectPrompts, updated.inspectKind)
	require.Contains(t, promptView, "Tasks Inspector: Prompts")
	require.Contains(t, promptView, "summary.prompt")
	require.Contains(t, promptView, "Prompt body")

	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	resourceView := updated.View()
	require.Equal(t, taskInspectResources, updated.inspectKind)
	require.Contains(t, resourceView, "Tasks Inspector: Resources")
	require.Contains(t, resourceView, "workflow://wf-1/warm?role=planner")
	require.Contains(t, resourceView, `"hello":"world"`)
}
