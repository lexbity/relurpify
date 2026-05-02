package browser

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	platformbrowser "codeburg.org/lexbit/relurpify/platform/browser"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// BrowserServiceSnapshot captures the workspace-level browser service state.
type BrowserServiceSnapshot struct {
	ServiceID           string
	StartedAt           time.Time
	ActiveSessions      int
	DefaultBackend      string
	Sessions            []BrowserSessionSnapshot
	PathRoots           map[string]string
	BackendDistribution map[string]int
	Health              string
}

// BrowserSessionSnapshot captures one live browser session in a service-safe shape.
type BrowserSessionSnapshot struct {
	SessionID      string
	AgentID        string
	TaskID         string
	WorkflowID     string
	Backend        string
	Transport      string
	CreatedAt      time.Time
	LastActivityAt time.Time
	Recoveries     int
	LastError      string
	LastPage       *platformbrowser.PageState
	PathRoots      map[string]string
}

// Snapshot returns a workspace-level browser service view.
func (s *BrowserService) Snapshot(context.Context) (*BrowserServiceSnapshot, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	handles := make([]*browserSessionHandle, 0, len(s.sessions))
	for _, handle := range s.sessions {
		handles = append(handles, handle)
	}
	startedAt := s.startedAt
	defaultBackend := s.defaultBackend
	paths := s.paths
	s.mu.Unlock()

	sessionSnapshots := make([]BrowserSessionSnapshot, 0, len(handles))
	backendDistribution := make(map[string]int)
	for _, handle := range handles {
		snap := handle.serviceSnapshot()
		sessionSnapshots = append(sessionSnapshots, snap)
		backendDistribution[snap.Backend]++
	}
	sort.Slice(sessionSnapshots, func(i, j int) bool {
		return sessionSnapshots[i].SessionID < sessionSnapshots[j].SessionID
	})
	health := "healthy"
	if len(sessionSnapshots) == 0 {
		health = "idle"
	}
	return &BrowserServiceSnapshot{
		ServiceID:           s.agentID(),
		StartedAt:           startedAt,
		ActiveSessions:      len(sessionSnapshots),
		DefaultBackend:      defaultBackend,
		Sessions:            sessionSnapshots,
		PathRoots:           paths.roots(),
		BackendDistribution: backendDistribution,
		Health:              health,
	}, nil
}

func (s *BrowserService) ListSessions(context.Context) ([]core.ProviderSession, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	handles := make([]*browserSessionHandle, 0, len(s.sessions))
	for _, handle := range s.sessions {
		handles = append(handles, handle)
	}
	s.mu.Unlock()
	out := make([]core.ProviderSession, 0, len(handles))
	for _, handle := range handles {
		out = append(out, handle.providerSession())
	}
	return out, nil
}

func (s *BrowserService) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	s.mu.Lock()
	count := len(s.sessions)
	paths := s.paths
	s.mu.Unlock()
	return core.ProviderHealthSnapshot{
		Status:  "healthy",
		Message: "browser service active",
		Metadata: map[string]interface{}{
			"active_sessions": count,
			"path_roots":      paths.roots(),
		},
	}, nil
}

func (s *BrowserService) SnapshotProvider(ctx context.Context) (*core.ProviderSnapshot, error) {
	sessions, err := s.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	health, err := s.HealthSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &core.ProviderSnapshot{
		ProviderID:     "browser",
		Recoverability: core.RecoverabilityInProcess,
		Descriptor: core.ProviderDescriptor{
			ID:                 "browser",
			Kind:               core.ProviderKindAgentRuntime,
			ActivationScope:    defaultBrowserScope,
			TrustBaseline:      core.TrustClassProviderLocalUntrusted,
			RecoverabilityMode: core.RecoverabilityInProcess,
			SupportsHealth:     true,
			Security: core.ProviderSecurityProfile{
				Origin:                     core.ProviderOriginLocal,
				SafeForDirectInsertion:     false,
				RequiresFrameworkMediation: true,
			},
		},
		Health:        health,
		CapabilityIDs: []string{"tool:browser"},
		Metadata: map[string]any{
			"active_sessions": len(sessions),
			"path_roots":      s.paths.roots(),
			"workspace_root":  s.workspaceRoot,
		},
		CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *BrowserService) SnapshotSessions(context.Context) ([]core.ProviderSessionSnapshot, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	handles := make([]*browserSessionHandle, 0, len(s.sessions))
	for _, handle := range s.sessions {
		handles = append(handles, handle)
	}
	s.mu.Unlock()
	out := make([]core.ProviderSessionSnapshot, 0, len(handles))
	for _, handle := range handles {
		out = append(out, handle.snapshot())
	}
	return out, nil
}

func (h *browserSessionHandle) serviceSnapshot() BrowserSessionSnapshot {
	if h == nil {
		return BrowserSessionSnapshot{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	snap := BrowserSessionSnapshot{
		SessionID:      h.sessionID,
		AgentID:        h.agentID,
		TaskID:         h.taskID,
		WorkflowID:     h.workflowID,
		Backend:        h.backendName,
		Transport:      browserTransportForBackend(h.backendName),
		CreatedAt:      h.createdAt.UTC(),
		LastActivityAt: h.lastSeenAt.UTC(),
		Recoveries:     h.recoveries,
		LastError:      h.lastErr,
		PathRoots:      h.paths.roots(),
	}
	if h.lastPage != nil {
		snap.LastPage = h.lastPage
	}
	return snap
}

func recordBrowserObservation(env *contextdata.Envelope, pageState *platformbrowser.PageState) {
	if env == nil || pageState == nil {
		return
	}
	env.SetWorkingValue(browserLastPageStateKey, pageState, contextdata.MemoryClassTask)
	var snapshots []*platformbrowser.PageState
	if existing, ok := env.GetWorkingValue(browserPageStateListKey); ok {
		if typed, ok := existing.([]*platformbrowser.PageState); ok {
			snapshots = append(snapshots, typed...)
		}
	}
	snapshots = append(snapshots, pageState)
	env.SetWorkingValue(browserPageStateListKey, snapshots, contextdata.MemoryClassTask)
	env.AddInteraction(map[string]interface{}{
		"role": "observation",
		"text": formatBrowserObservation(pageState),
		"metadata": map[string]interface{}{
			"kind": "browser_page_state",
			"url":  pageState.URL,
		},
	})
}

func formatBrowserObservation(pageState *platformbrowser.PageState) string {
	if pageState == nil {
		return "[Browser] unavailable"
	}
	return fmt.Sprintf("[Browser]\nURL: %s\nTitle: %s\nInteractive: %d links, %d forms, %d inputs, %d buttons\nPreview: %q",
		pageState.URL,
		pageState.Title,
		pageState.LinkCount,
		pageState.FormCount,
		pageState.InputCount,
		pageState.ButtonCount,
		pageState.Preview,
	)
}

func withExtraction(sessionID string, extraction *platformbrowser.Extraction, key string) map[string]interface{} {
	return map[string]interface{}{
		"session_id":      sessionID,
		key:               extraction.Content,
		"truncated":       extraction.Truncated,
		"original_tokens": extraction.OriginalTokens,
		"final_tokens":    extraction.FinalTokens,
	}
}

func browserTransportForBackend(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case defaultBrowserBackend:
		return "websocket"
	case "webdriver", "bidi":
		return "http"
	default:
		return "unknown"
	}
}

func success(data map[string]interface{}) *contracts.ToolResult {
	return &contracts.ToolResult{Success: true, Data: data}
}

func emitBrowserTelemetry(telemetry core.Telemetry, eventType core.EventType, agentID, taskID, message string, metadata map[string]interface{}) {
	if telemetry == nil {
		return
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	if agentID != "" {
		metadata["agent_id"] = agentID
	}
	telemetry.Emit(core.Event{
		Type:      eventType,
		TaskID:    taskID,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
}

func browserTaskScope(env *contextdata.Envelope) string {
	if env == nil {
		return defaultBrowserScope
	}
	if taskID := strings.TrimSpace(env.TaskID); taskID != "" {
		return taskID
	}
	if sessionID := strings.TrimSpace(env.SessionID); sessionID != "" {
		return sessionID
	}
	return defaultBrowserScope
}

func browserWorkflowID(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return strings.TrimSpace(env.SessionID)
}

func defaultSessionID(env *contextdata.Envelope, args map[string]interface{}) string {
	sessionID := strings.TrimSpace(fmt.Sprint(args["session_id"]))
	if sessionID != "" && sessionID != "<nil>" {
		return sessionID
	}
	if env == nil {
		return ""
	}
	if current, ok := env.GetWorkingValue(browserDefaultSessionKey); ok {
		return strings.TrimSpace(fmt.Sprint(current))
	}
	return ""
}
