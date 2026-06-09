package exectest

import (
	"context"

	"codeburg.org/lexbit/relurpify/execution/session"
	"codeburg.org/lexbit/relurpify/execution/workspace"
)

// FakeWorkspaceService implements session.WorkspaceService with a configurable
// open function. By default OpenWorkspace returns a session with fake controllers.
type FakeWorkspaceService struct {
	OpenFunc  func(context.Context, session.OpenWorkspaceRequest) (*session.WorkspaceSession, error)
	OpenCalls []session.OpenWorkspaceRequest
}

// NewFakeWorkspaceService creates a FakeWorkspaceService whose default
// OpenWorkspace returns a NewFakeSession.
func NewFakeWorkspaceService() *FakeWorkspaceService {
	return &FakeWorkspaceService{
		OpenFunc: func(ctx context.Context, req session.OpenWorkspaceRequest) (*session.WorkspaceSession, error) {
			return NewFakeSession(req), nil
		},
	}
}

func (s *FakeWorkspaceService) OpenWorkspace(ctx context.Context, req session.OpenWorkspaceRequest) (*session.WorkspaceSession, error) {
	s.OpenCalls = append(s.OpenCalls, req)
	if s.OpenFunc != nil {
		return s.OpenFunc(ctx, req)
	}
	return NewFakeSession(req), nil
}

// NewFakeSession builds a *session.WorkspaceSession populated with fake
// controllers. All controllers are initialized to no-op fakes that return
// zero values and record invocation counts.
func NewFakeSession(req session.OpenWorkspaceRequest) *session.WorkspaceSession {
	id, _ := workspace.New(req.WorkspaceRoot)
	sess := &session.WorkspaceSession{
		ID:        req.AgentName + "-session",
		Workspace: id,
		Security:  &FakeSecurityController{},
		Knowledge: &FakeKnowledgeController{},
		Agents:    &FakeNamedAgentController{},
		Tools:     &FakeCapabilityController{},
		Telemetry: &fakeTelemetryView{},
	}
	return sess
}

// fakeTelemetryView is a no-op TelemetryView for tests.
type fakeTelemetryView struct{}

var _ session.TelemetryView = (*fakeTelemetryView)(nil)
