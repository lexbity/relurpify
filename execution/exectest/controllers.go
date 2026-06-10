package exectest

import (
	"context"

	"codeburg.org/lexbit/relurpify/execution/session"
)

// FakeSecurityController implements session.SecurityController for tests.
// Records all calls and returns configurable results.
type FakeSecurityController struct {
	PolicySummaryFunc   func(context.Context) (session.PolicySummary, error)
	RequestApprovalFunc func(context.Context, session.ApprovalRequest) (session.ApprovalDecision, error)
	Calls               []string // "PolicySummary" or "RequestApproval"
}

func (c *FakeSecurityController) PolicySummary(ctx context.Context) (session.PolicySummary, error) {
	c.Calls = append(c.Calls, "PolicySummary")
	if c.PolicySummaryFunc != nil {
		return c.PolicySummaryFunc(ctx)
	}
	return session.PolicySummary{DefaultPolicy: "default-deny", AgentID: "test-agent"}, nil
}

func (c *FakeSecurityController) RequestApproval(ctx context.Context, req session.ApprovalRequest) (session.ApprovalDecision, error) {
	c.Calls = append(c.Calls, "RequestApproval")
	if c.RequestApprovalFunc != nil {
		return c.RequestApprovalFunc(ctx, req)
	}
	return session.ApprovalDecision{Approved: true, Reason: "auto-approved by fake"}, nil
}

var _ session.SecurityController = (*FakeSecurityController)(nil)

// FakeKnowledgeController implements session.KnowledgeController for tests.
type FakeKnowledgeController struct {
	IngestFunc func(context.Context, session.IngestRequest) (session.IngestResult, error)
	QueryFunc  func(context.Context, session.QueryRequest) (session.QueryResult, error)
	Calls      []string
}

func (c *FakeKnowledgeController) Ingest(ctx context.Context, req session.IngestRequest) (session.IngestResult, error) {
	c.Calls = append(c.Calls, "Ingest")
	if c.IngestFunc != nil {
		return c.IngestFunc(ctx, req)
	}
	return session.IngestResult{ChunksIngested: 1}, nil
}

func (c *FakeKnowledgeController) Query(ctx context.Context, req session.QueryRequest) (session.QueryResult, error) {
	c.Calls = append(c.Calls, "Query")
	if c.QueryFunc != nil {
		return c.QueryFunc(ctx, req)
	}
	return session.QueryResult{}, nil
}

var _ session.KnowledgeController = (*FakeKnowledgeController)(nil)

// FakeCapabilityController implements session.CapabilityController for tests.
type FakeCapabilityController struct {
	ListFunc   func(context.Context) ([]session.CapabilitySummary, error)
	InvokeFunc func(context.Context, session.CapabilityInvokeRequest) (session.CapabilityInvokeResult, error)
	Calls      []string
}

func (c *FakeCapabilityController) List(ctx context.Context) ([]session.CapabilitySummary, error) {
	c.Calls = append(c.Calls, "List")
	if c.ListFunc != nil {
		return c.ListFunc(ctx)
	}
	return []session.CapabilitySummary{
		{ID: "cap:test", Name: "Test Capability", Enabled: true},
	}, nil
}

func (c *FakeCapabilityController) Invoke(ctx context.Context, req session.CapabilityInvokeRequest) (session.CapabilityInvokeResult, error) {
	c.Calls = append(c.Calls, "Invoke")
	if c.InvokeFunc != nil {
		return c.InvokeFunc(ctx, req)
	}
	return session.CapabilityInvokeResult{Output: map[string]any{"result": "ok"}}, nil
}

var _ session.CapabilityController = (*FakeCapabilityController)(nil)

// FakeNamedAgentController implements session.NamedAgentController for tests.
type FakeNamedAgentController struct {
	CatalogFunc func(context.Context) ([]session.NamedAgentSummary, error)
	OpenFunc    func(context.Context, session.NamedAgentOpenRequest) (session.NamedAgentSession, error)
	Calls       []string
}

func (c *FakeNamedAgentController) Catalog(ctx context.Context) ([]session.NamedAgentSummary, error) {
	c.Calls = append(c.Calls, "Catalog")
	if c.CatalogFunc != nil {
		return c.CatalogFunc(ctx)
	}
	return []session.NamedAgentSummary{
		{Name: "euclo", Description: "Coding agent"},
	}, nil
}

func (c *FakeNamedAgentController) Open(ctx context.Context, req session.NamedAgentOpenRequest) (session.NamedAgentSession, error) {
	c.Calls = append(c.Calls, "Open")
	if c.OpenFunc != nil {
		return c.OpenFunc(ctx, req)
	}
	return session.NamedAgentSession{ID: req.Name + "-session"}, nil
}

var _ session.NamedAgentController = (*FakeNamedAgentController)(nil)
