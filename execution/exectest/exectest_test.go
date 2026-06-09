package exectest

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/execution/session"
)

func TestFakeWorkspaceService_RecordsCalls(t *testing.T) {
	svc := NewFakeWorkspaceService()
	req := session.OpenWorkspaceRequest{
		WorkspaceRoot: "/test/workspace",
		AgentName:     "test-agent",
		Mode:          session.OpenModeEmbeddedAgent,
	}
	sess, err := svc.OpenWorkspace(context.Background(), req)
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
	if len(svc.OpenCalls) != 1 {
		t.Errorf("OpenCalls count = %d, want 1", len(svc.OpenCalls))
	}
	if svc.OpenCalls[0].WorkspaceRoot != "/test/workspace" {
		t.Errorf("WorkspaceRoot = %q", svc.OpenCalls[0].WorkspaceRoot)
	}
}

func TestFakeSession_HasControllers(t *testing.T) {
	req := session.OpenWorkspaceRequest{
		WorkspaceRoot: "/ws",
		AgentName:     "test",
	}
	sess := NewFakeSession(req)

	if sess.Security == nil {
		t.Fatal("Security controller is nil")
	}
	if sess.Knowledge == nil {
		t.Fatal("Knowledge controller is nil")
	}
	if sess.Agents == nil {
		t.Fatal("NamedAgent controller is nil")
	}
	if sess.Tools == nil {
		t.Fatal("Capability controller is nil")
	}
	if sess.Telemetry == nil {
		t.Fatal("Telemetry view is nil")
	}
}

func TestFakeSecurityController_DefaultPolicy(t *testing.T) {
	c := &FakeSecurityController{}
	summary, err := c.PolicySummary(context.Background())
	if err != nil {
		t.Fatalf("PolicySummary: %v", err)
	}
	if summary.DefaultPolicy != "default-deny" {
		t.Errorf("DefaultPolicy = %q, want %q", summary.DefaultPolicy, "default-deny")
	}
	if summary.AgentID != "test-agent" {
		t.Errorf("AgentID = %q", summary.AgentID)
	}
	if len(c.Calls) != 1 || c.Calls[0] != "PolicySummary" {
		t.Errorf("Calls = %v", c.Calls)
	}
}

func TestFakeSecurityController_Approval(t *testing.T) {
	c := &FakeSecurityController{}
	decision, err := c.RequestApproval(context.Background(), session.ApprovalRequest{
		Action: "test-action",
	})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if !decision.Approved {
		t.Error("expected approved decision")
	}
}

func TestFakeKnowledgeController_Ingest(t *testing.T) {
	c := &FakeKnowledgeController{}
	result, err := c.Ingest(context.Background(), session.IngestRequest{
		Content: "test content",
		Source:  "test",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.ChunksIngested != 1 {
		t.Errorf("ChunksIngested = %d, want 1", result.ChunksIngested)
	}
}

func TestFakeKnowledgeController_Query(t *testing.T) {
	c := &FakeKnowledgeController{}
	result, err := c.Query(context.Background(), session.QueryRequest{
		Query: "test query",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Results != nil && len(result.Results) != 0 {
		t.Errorf("Results = %v", result.Results)
	}
}

func TestFakeCapabilityController_List(t *testing.T) {
	c := &FakeCapabilityController{}
	list, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("List returned empty")
	}
	if list[0].ID != "cap:test" {
		t.Errorf("first capability ID = %q", list[0].ID)
	}
}

func TestFakeCapabilityController_Invoke(t *testing.T) {
	c := &FakeCapabilityController{}
	result, err := c.Invoke(context.Background(), session.CapabilityInvokeRequest{
		CapabilityID: "cap:test",
		Input:        map[string]interface{}{"key": "value"},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Output["result"] != "ok" {
		t.Errorf("Output = %v", result.Output)
	}
}

func TestFakeNamedAgentController_Catalog(t *testing.T) {
	c := &FakeNamedAgentController{}
	catalog, err := c.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(catalog) == 0 {
		t.Fatal("Catalog returned empty")
	}
	if catalog[0].Name != "euclo" {
		t.Errorf("first agent name = %q", catalog[0].Name)
	}
}

func TestFakeNamedAgentController_Open(t *testing.T) {
	c := &FakeNamedAgentController{}
	sess, err := c.Open(context.Background(), session.NamedAgentOpenRequest{Name: "euclo"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if sess.ID != "euclo-session" {
		t.Errorf("Session ID = %q", sess.ID)
	}
}

func TestFakeWorkspaceService_CallFakeController(t *testing.T) {
	svc := NewFakeWorkspaceService()
	sess, err := svc.OpenWorkspace(context.Background(), session.OpenWorkspaceRequest{
		WorkspaceRoot: "/test",
		AgentName:     "my-agent",
	})
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}

	// Use the fake knowledge controller through the session.
	_, err = sess.Knowledge.Ingest(context.Background(), session.IngestRequest{
		Content: "test content",
	})
	if err != nil {
		t.Errorf("Ingest: %v", err)
	}

	// Use the fake capability controller.
	list, err := sess.Tools.List(context.Background())
	if err != nil {
		t.Errorf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 capability, got %d", len(list))
	}

	// Verify session identity.
	if sess.ID != "my-agent-session" {
		t.Errorf("Session ID = %q", sess.ID)
	}
	if sess.Workspace.Root != "/test" {
		t.Errorf("Workspace root = %q", sess.Workspace.Root)
	}
}

func TestFakeWorkspaceService_CustomOpenFunc(t *testing.T) {
	svc := &FakeWorkspaceService{
		OpenFunc: func(ctx context.Context, req session.OpenWorkspaceRequest) (*session.WorkspaceSession, error) {
			return nil, assertAnError
		},
	}
	_, err := svc.OpenWorkspace(context.Background(), session.OpenWorkspaceRequest{})
	if err == nil {
		t.Fatal("expected error from custom OpenFunc")
	}
}

var assertAnError = &fakeError{msg: "custom error"}

type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }
