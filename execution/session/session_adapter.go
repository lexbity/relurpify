package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/execution/workspace"
)

// SessionAdapter implements WorkspaceService using OpenWorkspace
// as a backend. It is a transitional adapter;
// as domain extraction progresses the adapter shrinks and is eventually
// replaced by direct execution/session construction.
//
// Constructor inputs (WorkspaceConfig) carry the app-composed provider
// products. OpenWorkspace requests carry the per-session parameters.
type SessionAdapter struct {
	config WorkspaceConfig
}

// NewSessionAdapter creates a SessionAdapter that uses the given config
// to open workspace sessions.
func NewSessionAdapter(cfg WorkspaceConfig) *SessionAdapter {
	return &SessionAdapter{config: cfg}
}

// OpenWorkspace opens a workspace  The provider products (security,
// capability, knowledge, model) were supplied at adapter construction time;
// only session-level parameters come from the request.
func (a *SessionAdapter) OpenWorkspace(ctx context.Context, req OpenWorkspaceRequest) (*WorkspaceSession, error) {
	cfg := a.config
	cfg.Workspace = req.WorkspaceRoot
	cfg.ConfigPath = req.ConfigPath
	cfg.AgentName = req.AgentName

	switch req.Mode {
	case OpenModeEmbeddedAgent:
		cfg.Scope = ScopeEmbeddedAgent
	default:
		cfg.Scope = ScopeFull
	}

	ws, err := OpenWorkspace(ctx, cfg)
	if err != nil {
		return nil, err
	}

	id, err := workspace.New(req.WorkspaceRoot)
	if err != nil {
		ws.Close()
		return nil, err
	}

	sess := &WorkspaceSession{
		ID:        ws.Registration.ID,
		Workspace: id,
		Security:  newSecurityController(ws),
		Knowledge: newKnowledgeController(ws),
		Agents:    newNamedAgentController(ws),
		Tools:     newCapabilityController(ws),
		Telemetry: newTelemetryView(ws),
	}
	sess.SetCloseFn(func(ctx context.Context) error { return ws.Close() })
	return sess, nil
}

// securityController wraps Workspace to provide the SecurityController
// interface. It does not expose governance internals.
type securityController struct {
	ws *Workspace
}

func newSecurityController(ws *Workspace) *securityController {
	return &securityController{ws: ws}
}

func (c *securityController) PolicySummary(ctx context.Context) (PolicySummary, error) {
	if c.ws.Registration == nil || c.ws.Registration.Permissions == nil {
		return PolicySummary{}, ErrSecurityUnavailable
	}
	return PolicySummary{
		DefaultPolicy: c.ws.Registration.Permissions.DefaultPolicy(),
		AgentID:       c.ws.Registration.ID,
	}, nil
}

func (c *securityController) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error) {
	if c.ws.Registration == nil || c.ws.Registration.HITL == nil {
		return ApprovalDecision{}, ErrSecurityUnavailable
	}
	return ApprovalDecision{}, ErrSecurityUnavailable
}

// knowledgeController wraps the workspace environment to provide the
// KnowledgeController interface. It delegates to the knowledge
// services configured in the environment when available.
type knowledgeController struct {
	env agentEnv
}

func newKnowledgeController(ws *Workspace) *knowledgeController {
	return &knowledgeController{env: ws.Environment}
}

func (c *knowledgeController) Ingest(ctx context.Context, req IngestRequest) (IngestResult, error) {
	if c.env.KnowledgeStore == nil || c.env.KnowledgeStore.Graph == nil {
		return IngestResult{}, ErrKnowledgeUnavailable
	}
	saved, err := c.env.KnowledgeStore.Save(knowledge.KnowledgeChunk{
		ID:    knowledge.ChunkID(contentHash(req.Content)),
		Body:  knowledge.ChunkBody{Raw: req.Content},
		SourceOrigin: knowledge.SourceOriginTool,
	})
	if err != nil {
		return IngestResult{}, err
	}
	if saved != nil {
		return IngestResult{ChunksIngested: 1}, nil
	}
	return IngestResult{}, nil
}

func (c *knowledgeController) Query(ctx context.Context, req QueryRequest) (QueryResult, error) {
	if c.env.KnowledgeStore == nil || c.env.KnowledgeStore.Graph == nil {
		return QueryResult{}, ErrKnowledgeUnavailable
	}
	// Simple prefix-based lookup on file path.
	chunks, err := c.env.KnowledgeStore.FindByFilePathPrefix(req.Query)
	if err != nil {
		return QueryResult{}, err
	}
	results := make([]QueryResultItem, 0, len(chunks))
	for _, chunk := range chunks {
		results = append(results, QueryResultItem{
			Content: chunk.Body.Raw,
			Score:   1.0,
			Source:  string(chunk.ID),
		})
	}
	return QueryResult{Results: results}, nil
}

// contentHash produces a stable chunk ID from content text.
func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:16])
}

// capabilityController wraps the workspace environment to provide the
// CapabilityController interface. It uses the registry's
// AllCapabilitySnapshots for listing; invocation is unavailable at this
// level because capability execution requires domain-specific runtime
// infrastructure beyond the registry.
type capabilityController struct {
	env agentEnv
}

func newCapabilityController(ws *Workspace) *capabilityController {
	return &capabilityController{env: ws.Environment}
}

func (c *capabilityController) List(ctx context.Context) ([]CapabilitySummary, error) {
	if c.env.Registry == nil {
		return nil, ErrCapabilityUnavailable
	}
	snapshots := c.env.Registry.AllCapabilitySnapshots()
	summaries := make([]CapabilitySummary, 0, len(snapshots))
	for _, snap := range snapshots {
		summaries = append(summaries, CapabilitySummary{
			ID:          snap.Descriptor.ID,
			Name:        snap.Descriptor.Name,
			Description: snap.Descriptor.Description,
			Enabled:     snap.Exposure != agentspec.CapabilityExposureHidden,
		})
	}
	return summaries, nil
}

func (c *capabilityController) Invoke(ctx context.Context, req CapabilityInvokeRequest) (CapabilityInvokeResult, error) {
	if c.env.Registry == nil {
		return CapabilityInvokeResult{}, ErrCapabilityUnavailable
	}
	// Capability invocation is not available through the session controller
	// because it requires domain-specific runtime infrastructure.
	return CapabilityInvokeResult{}, ErrCapabilityUnavailable
}

// namedAgentController wraps Workspace to provide the NamedAgentController
// interface. In this transitional state, named agent operations are always
// unavailable; real implementations are wired when named/euclo extraction
// is complete.
type namedAgentController struct {
	ws *Workspace
}

func newNamedAgentController(ws *Workspace) *namedAgentController {
	return &namedAgentController{ws: ws}
}

func (c *namedAgentController) Catalog(ctx context.Context) ([]NamedAgentSummary, error) {
	return nil, ErrNamedAgentUnavailable
}

func (c *namedAgentController) Open(ctx context.Context, req NamedAgentOpenRequest) (NamedAgentSession, error) {
	return NamedAgentSession{}, ErrNamedAgentUnavailable
}

// telemetryView wraps the workspace telemetry handle.
type telemetryView struct {
	ws *Workspace
}

func newTelemetryView(ws *Workspace) *telemetryView {
	return &telemetryView{ws: ws}
}

// NewSessionFromWorkspace wraps an existing *Workspace in a *WorkspaceSession.
// WorkspaceRoot is the filesystem root path of the workspace, used to populate
// the session's Workspace.Identity.
// This is a transitional helper for app code that constructs the workspace directly
// and needs controller-based access without restructuring its construction path.
func NewSessionFromWorkspace(ws *Workspace, workspaceRoot string) *WorkspaceSession {
	id, _ := workspace.New(workspaceRoot)
	sess := &WorkspaceSession{
		ID:        ws.Registration.ID,
		Workspace: id,
		Security:  newSecurityController(ws),
		Knowledge: newKnowledgeController(ws),
		Agents:    newNamedAgentController(ws),
		Tools:     newCapabilityController(ws),
		Telemetry: newTelemetryView(ws),
	}
	sess.SetCloseFn(func(ctx context.Context) error { return ws.Close() })
	if ws.ServiceManager != nil {
		sess.SetServiceManager(&serviceManagerAdapter{sm: ws.ServiceManager})
	}
	return sess
}

// serviceManagerAdapter wraps *serviceManager to satisfy ServiceManager.
type serviceManagerAdapter struct {
	sm *serviceManager
}

func (a *serviceManagerAdapter) RegisterService(id string, svc Service) {
	a.sm.Register(id, svc)
}

func (a *serviceManagerAdapter) StartAll(ctx context.Context) error {
	return a.sm.StartAll(ctx)
}

func (a *serviceManagerAdapter) Snapshots() []ServiceSnapshot {
	raw := a.sm.Snapshot()
	out := make([]ServiceSnapshot, len(raw))
	for i, s := range raw {
		out[i] = ServiceSnapshot{
			ID:     s.ID,
			Status: s.Status,
			Source: s.Source,
			Owner:  s.Owner,
			Notes:  s.Notes,
		}
	}
	return out
}

// Ensure *SessionAdapter satisfies WorkspaceService at compile time.
var _ WorkspaceService = (*SessionAdapter)(nil)
