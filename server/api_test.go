package server

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/persistence"
	"github.com/stretchr/testify/assert"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

type stubAgent struct{}

type fakeInspector struct {
	capabilities []CapabilityResource
	prompts      []PromptResource
	providers    []ProviderResource
	resources    []ReadableResource
	sessions     []SessionResource
	approvals    []ApprovalResource
}

func (stubAgent) Initialize(config *core.Config) error { return nil }
func (stubAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	state.Set("handled", true)
	return &core.Result{NodeID: "stub", Success: true, Data: map[string]interface{}{"ok": true}}, nil
}
func (stubAgent) Capabilities() []core.Capability { return nil }
func (stubAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	return graph.NewGraph(), nil
}

func (f fakeInspector) ListCapabilities(context.Context) ([]CapabilityResource, error) {
	return append([]CapabilityResource(nil), f.capabilities...), nil
}
func (f fakeInspector) GetCapability(_ context.Context, id string) (*CapabilityResource, error) {
	for _, resource := range f.capabilities {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, io.EOF
}
func (f fakeInspector) ListProviders(context.Context) ([]ProviderResource, error) {
	return append([]ProviderResource(nil), f.providers...), nil
}
func (f fakeInspector) ListPrompts(context.Context) ([]PromptResource, error) {
	return append([]PromptResource(nil), f.prompts...), nil
}
func (f fakeInspector) GetPrompt(_ context.Context, id string) (*PromptResource, error) {
	for _, resource := range f.prompts {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, io.EOF
}
func (f fakeInspector) GetProvider(_ context.Context, id string) (*ProviderResource, error) {
	for _, resource := range f.providers {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, io.EOF
}
func (f fakeInspector) ListResources(context.Context) ([]ReadableResource, error) {
	return append([]ReadableResource(nil), f.resources...), nil
}
func (f fakeInspector) GetResource(_ context.Context, id string) (*ReadableResource, error) {
	for _, resource := range f.resources {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, io.EOF
}
func (f fakeInspector) GetWorkflowResource(_ context.Context, uri string) (*ReadableResource, error) {
	for _, resource := range f.resources {
		if resource.Resource.WorkflowURI == uri {
			copy := resource
			return &copy, nil
		}
	}
	return nil, io.EOF
}
func (f fakeInspector) ListSessions(context.Context) ([]SessionResource, error) {
	return append([]SessionResource(nil), f.sessions...), nil
}
func (f fakeInspector) GetSession(_ context.Context, id string) (*SessionResource, error) {
	for _, resource := range f.sessions {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, io.EOF
}
func (f fakeInspector) ListApprovals(context.Context) ([]ApprovalResource, error) {
	return append([]ApprovalResource(nil), f.approvals...), nil
}
func (f fakeInspector) GetApproval(_ context.Context, id string) (*ApprovalResource, error) {
	for _, resource := range f.approvals {
		if resource.Meta.ID == id {
			copy := resource
			return &copy, nil
		}
	}
	return nil, io.EOF
}

func TestAPIServerHandleTask(t *testing.T) {
	api := &APIServer{
		Agent:   stubAgent{},
		Context: core.NewContext(),
		Logger:  log.New(io.Discard, "", 0),
	}
	reqBody, _ := json.Marshal(TaskRequest{
		Instruction: "test",
		Type:        core.TaskTypeAnalysis,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/task", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	api.handleTask(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp TaskResponse
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "stub", resp.Result.NodeID)
}

func TestAPIServerListsAndInspectsWorkflows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workflow_state.db")
	store, err := persistence.NewSQLiteWorkflowStateStore(dbPath)
	assert.NoError(t, err)
	defer store.Close()
	assert.NoError(t, store.CreateWorkflow(context.Background(), persistence.WorkflowRecord{
		WorkflowID:  "wf-api",
		TaskID:      "wf-api",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Inspect workflow",
		Status:      persistence.WorkflowRunStatusRunning,
	}))

	api := &APIServer{
		Agent:             stubAgent{},
		Context:           core.NewContext(),
		Logger:            log.New(io.Discard, "", 0),
		WorkflowStatePath: dbPath,
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/workflows", nil)
	listRec := httptest.NewRecorder()
	api.handleWorkflows(listRec, listReq)
	assert.Equal(t, http.StatusOK, listRec.Code)

	inspectReq := httptest.NewRequest(http.MethodGet, "/api/workflows/wf-api", nil)
	inspectRec := httptest.NewRecorder()
	api.handleWorkflowByID(inspectRec, inspectReq)
	assert.Equal(t, http.StatusOK, inspectRec.Code)
}

func TestAPIServerInspectionEndpoints(t *testing.T) {
	api := &APIServer{
		Agent:   stubAgent{},
		Context: core.NewContext(),
		Logger:  log.New(io.Discard, "", 0),
		Inspector: fakeInspector{
			capabilities: []CapabilityResource{{
				Meta:       InspectableMeta{ID: "relurpic:planner.plan", Kind: "tool", Title: "planner.plan"},
				Capability: CapabilityPayload{Exposure: "callable", Callable: true},
			}},
			prompts: []PromptResource{{
				Meta:   InspectableMeta{ID: "prompt:summary", Kind: "prompt", Title: "summary.prompt"},
				Prompt: PromptPayload{PromptID: "prompt:summary", Description: "summary prompt"},
			}},
			providers: []ProviderResource{{
				Meta:     InspectableMeta{ID: "remote-mcp", Kind: "mcp-client", Title: "remote-mcp"},
				Provider: ProviderPayload{ProviderID: "remote-mcp"},
			}},
			resources: []ReadableResource{{
				Meta:     InspectableMeta{ID: "resource:docs", Kind: "resource", Title: "docs"},
				Resource: ReadableResourcePayload{ResourceID: "resource:docs", Description: "docs resource"},
			}, {
				Meta:     InspectableMeta{ID: "workflow://wf-api/warm?run=run-1&role=planner", Kind: "workflow-resource", Title: "wf-api warm planner"},
				Resource: ReadableResourcePayload{ResourceID: "workflow://wf-api/warm?run=run-1&role=planner", WorkflowResource: true, WorkflowURI: "workflow://wf-api/warm?run=run-1&role=planner"},
			}},
			sessions: []SessionResource{{
				Meta:    InspectableMeta{ID: "remote-mcp:primary", Kind: "session", Title: "remote-mcp:primary"},
				Session: SessionPayload{SessionID: "remote-mcp:primary", ProviderID: "remote-mcp"},
			}},
			approvals: []ApprovalResource{{
				Meta:     InspectableMeta{ID: "approval-1", Kind: "provider_operation", Title: "provider:remote-mcp:activate"},
				Approval: ApprovalPayload{ID: "approval-1", Kind: "provider_operation", Action: "provider:remote-mcp:activate"},
			}},
		},
	}

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/api/capabilities", want: "planner.plan"},
		{path: "/api/capabilities/relurpic:planner.plan", want: "planner.plan"},
		{path: "/api/prompts", want: "summary.prompt"},
		{path: "/api/prompts/prompt:summary", want: "summary prompt"},
		{path: "/api/providers", want: "remote-mcp"},
		{path: "/api/providers/remote-mcp", want: "remote-mcp"},
		{path: "/api/resources", want: "resource:docs"},
		{path: "/api/resources/resource:docs", want: "docs resource"},
		{path: "/api/workflow-resources/read?uri=" + url.QueryEscape("workflow://wf-api/warm?run=run-1&role=planner"), want: "workflow-resource"},
		{path: "/api/sessions", want: "remote-mcp:primary"},
		{path: "/api/sessions/remote-mcp:primary", want: "remote-mcp:primary"},
		{path: "/api/approvals", want: "approval-1"},
		{path: "/api/approvals/approval-1", want: "approval-1"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		api.newHTTPServer("").Handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, tc.path)
		assert.Contains(t, rec.Body.String(), tc.want, tc.path)
	}
}

func TestAPIServerWorkflowSubresourceEndpoints(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workflow_state.db")
	store, err := persistence.NewSQLiteWorkflowStateStore(dbPath)
	assert.NoError(t, err)
	defer store.Close()
	ctx := context.Background()
	assert.NoError(t, store.CreateWorkflow(ctx, persistence.WorkflowRecord{
		WorkflowID:  "wf-api",
		TaskID:      "wf-api",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Inspect workflow",
		Status:      persistence.WorkflowRunStatusRunning,
	}))
	assert.NoError(t, store.CreateRun(ctx, persistence.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-api",
		Status:     persistence.WorkflowRunStatusRunning,
	}))
	assert.NoError(t, store.UpsertDelegation(ctx, persistence.WorkflowDelegationRecord{
		DelegationID:   "delegation-1",
		WorkflowID:     "wf-api",
		RunID:          "run-1",
		TaskID:         "wf-api",
		State:          core.DelegationStateSucceeded,
		TrustClass:     core.TrustClassBuiltinTrusted,
		Recoverability: core.RecoverabilityInProcess,
		Request: core.DelegationRequest{
			ID:                 "delegation-1",
			TargetCapabilityID: "relurpic:planner.plan",
			ResourceRefs:       []string{"workflow://wf-api/warm?run=run-1&role=planner"},
		},
		Result: &core.DelegationResult{
			DelegationID: "delegation-1",
			State:        core.DelegationStateSucceeded,
			Success:      true,
			Insertion:    core.InsertionDecision{Action: core.InsertionActionSummarized},
		},
	}))
	assert.NoError(t, store.UpsertWorkflowArtifact(ctx, persistence.WorkflowArtifactRecord{
		ArtifactID:    "artifact-1",
		WorkflowID:    "wf-api",
		RunID:         "run-1",
		Kind:          "delegation_result",
		ContentType:   "application/json",
		SummaryText:   "delegation summary",
		StorageKind:   persistence.ArtifactStorageInline,
		InlineRawText: `{"summary":"delegated"}`,
	}))
	assert.NoError(t, store.ReplaceProviderSnapshots(ctx, "wf-api", "run-1", []persistence.WorkflowProviderSnapshotRecord{{
		SnapshotID:     "provider-1",
		WorkflowID:     "wf-api",
		RunID:          "run-1",
		ProviderID:     "delegation-runtime",
		Recoverability: core.RecoverabilityInProcess,
		Descriptor:     core.ProviderDescriptor{ID: "delegation-runtime", Kind: core.ProviderKindAgentRuntime},
		Health:         core.ProviderHealthSnapshot{Status: "ok"},
	}}))
	assert.NoError(t, store.ReplaceProviderSessionSnapshots(ctx, "wf-api", "run-1", []persistence.WorkflowProviderSessionSnapshotRecord{{
		SnapshotID: "session-1",
		WorkflowID: "wf-api",
		RunID:      "run-1",
		Session: core.ProviderSession{
			ID:         "session-1",
			ProviderID: "delegation-runtime",
			Health:     "running",
		},
	}}))

	api := &APIServer{
		Agent:             stubAgent{},
		Context:           core.NewContext(),
		Logger:            log.New(io.Discard, "", 0),
		WorkflowStatePath: dbPath,
	}

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/api/workflows/wf-api/delegations", want: "delegation-1"},
		{path: "/api/workflows/wf-api/artifacts", want: "artifact-1"},
		{path: "/api/workflows/wf-api/providers", want: "delegation-runtime"},
		{path: "/api/workflows/wf-api/sessions", want: "session-1"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		api.newHTTPServer("").Handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, tc.path)
		assert.Contains(t, rec.Body.String(), tc.want, tc.path)
	}
}
