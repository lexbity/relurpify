package browser

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	platformbrowser "codeburg.org/lexbit/relurpify/platform/browser"
)

type browserSessionHandle struct {
	mu          sync.Mutex
	session     *platformbrowser.Session
	cfg         browserSessionConfig
	paths       browserSessionPaths
	factory     func(context.Context, browserSessionConfig) (*platformbrowser.Session, error)
	telemetry   core.Telemetry
	taskID      string
	workflowID  string
	agentID     string
	sessionID   string
	backendName string
	recoveries  int
	createdAt   time.Time
	lastSeenAt  time.Time
	lastPage    *platformbrowser.PageState
	lastErr     string
	closed      bool
	service     *BrowserService
}

func (s *BrowserService) open(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.ToolResult, error) {
	backendName := s.resolveBackend(args)
	cfg := browserSessionConfig{
		backendName:  backendName,
		manager:      s.permissionManager,
		agentID:      s.agentID(),
		maxTokens:    s.maxTokens(),
		registration: s.registration,
		service:      s,
	}
	factory := s.sessionFactoryOrDefault()
	session, err := factory(ctx, cfg)
	if err != nil {
		return nil, err
	}
	handle := &browserSessionHandle{
		session:     session,
		cfg:         cfg,
		factory:     factory,
		telemetry:   s.telemetry,
		taskID:      browserTaskScope(env),
		workflowID:  browserWorkflowID(env),
		agentID:     s.agentID(),
		backendName: backendName,
		createdAt:   time.Now().UTC(),
		lastSeenAt:  time.Now().UTC(),
		service:     s,
	}
	sessionID := s.nextSessionID()
	handle.sessionID = sessionID
	sessionPaths, err := s.ensureSessionPaths(sessionID)
	if err != nil {
		_ = handle.Close()
		return nil, err
	}
	handle.paths = sessionPaths
	handle.cfg.paths = sessionPaths
	s.trackSession(sessionID, handle)
	if err := s.recordSessionActivity(sessionID, "open"); err != nil {
		_ = handle.Close()
		s.untrackSession(sessionID)
		return nil, err
	}
	handle.noteActivity()
	if env != nil {
		env.SetWorkingValue(browserDefaultSessionKey, sessionID, contextdata.MemoryClassTask)
	}
	scope := browserTaskScope(env)
	emitBrowserTelemetry(s.telemetry, core.EventStateChange, s.agentID(), scope, "browser session opened", map[string]interface{}{
		"browser_event":  "session_opened",
		"browser_action": browserPermissionAction(browserActionOpen),
		"session_id":     sessionID,
		"backend":        backendName,
	})
	s.persistSessionMetadata(handle)
	result, err := s.successWithSnapshot(ctx, env, handle, sessionID, map[string]interface{}{
		"backend":      backendName,
		"capabilities": handle.Capabilities(),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *BrowserService) close(env *contextdata.Envelope, args map[string]interface{}) (*core.ToolResult, error) {
	sessionID := defaultSessionID(env, args)
	if sessionID == "" {
		return nil, fmt.Errorf("browser session not found")
	}
	handle := s.sessionHandle(sessionID)
	s.untrackSession(sessionID)
	if env != nil {
		if current, ok := env.GetWorkingValue(browserDefaultSessionKey); ok {
			if strings.TrimSpace(fmt.Sprint(current)) == sessionID {
				env.SetWorkingValue(browserDefaultSessionKey, "", contextdata.MemoryClassTask)
			}
		}
	}
	emitBrowserTelemetry(s.telemetry, core.EventStateChange, s.agentID(), browserTaskScope(env), "browser session closed", map[string]interface{}{
		"browser_event":  "session_closed",
		"browser_action": browserPermissionAction(browserActionClose),
		"session_id":     sessionID,
	})
	s.persistSessionMetadata(handle)
	return success(map[string]interface{}{"session_id": sessionID, "closed": true}), nil
}

func (s *BrowserService) lookupSession(env *contextdata.Envelope, args map[string]interface{}) (*browserSessionHandle, string, error) {
	sessionID := defaultSessionID(env, args)
	if sessionID == "" {
		return nil, "", fmt.Errorf("browser session not found")
	}
	raw := s.sessionHandle(sessionID)
	if raw == nil {
		return nil, "", fmt.Errorf("browser session %s not found", sessionID)
	}
	if err := s.recordSessionActivity(sessionID, strings.ToLower(strings.TrimSpace(fmt.Sprint(args["action"])))); err != nil {
		return nil, "", err
	}
	raw.noteActivity()
	return raw, sessionID, nil
}

func (s *BrowserService) sessionHandle(sessionID string) *browserSessionHandle {
	if s == nil || sessionID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[sessionID]
}

func (s *BrowserService) resolveBackend(args map[string]interface{}) string {
	backend := defaultIfEmpty(fmt.Sprint(args["backend"]), "")
	if backend == "" && s.agentSpec != nil && s.agentSpec.Browser != nil {
		backend = defaultIfEmpty(s.agentSpec.Browser.DefaultBackend, "")
	}
	if backend == "" {
		backend = s.defaultBackend
	}
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		return defaultBrowserBackend
	}
	if !s.backendAllowed(backend) {
		return defaultBrowserBackend
	}
	return backend
}

func (s *BrowserService) maxTokens() int {
	if s == nil || s.agentSpec == nil || s.agentSpec.ArtifactWindow.MaxTokens <= 0 {
		return 8192
	}
	return s.agentSpec.ArtifactWindow.MaxTokens
}

func (s *BrowserService) agentID() string {
	if s == nil || s.registration == nil {
		return ""
	}
	return s.registration.ID
}

func (s *BrowserService) recordSessionActivity(sessionID, action string) error {
	if s == nil || s.registry == nil || sessionID == "" {
		return nil
	}
	switch action {
	case "", "close":
		return nil
	case "open":
		if err := s.registry.RecordSessionSubprocess(sessionID, 1); err != nil {
			return err
		}
		return s.registry.RecordSessionNetworkRequest(sessionID, 1)
	default:
		return s.registry.RecordSessionNetworkRequest(sessionID, 1)
	}
}

func (s *BrowserService) trackSession(sessionID string, handle *browserSessionHandle) {
	if s == nil || sessionID == "" || handle == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = handle
}

func (s *BrowserService) nextSessionID() string {
	return fmt.Sprintf("browser-%d", time.Now().UTC().UnixNano())
}

func (s *BrowserService) untrackSession(sessionID string) {
	if s == nil || sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (h *browserSessionHandle) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	session := h.session
	h.session = nil
	h.mu.Unlock()
	if session == nil {
		return nil
	}
	return session.Close()
}

func (h *browserSessionHandle) Navigate(ctx context.Context, url string) error {
	_, err := browserRun(h, ctx, "navigate", func(session *platformbrowser.Session) (struct{}, error) {
		return struct{}{}, session.Navigate(ctx, url)
	})
	return err
}

func (h *browserSessionHandle) Click(ctx context.Context, selector string) error {
	_, err := browserRun(h, ctx, "click", func(session *platformbrowser.Session) (struct{}, error) {
		return struct{}{}, session.Click(ctx, selector)
	})
	return err
}

func (h *browserSessionHandle) Type(ctx context.Context, selector, text string) error {
	_, err := browserRun(h, ctx, "type", func(session *platformbrowser.Session) (struct{}, error) {
		return struct{}{}, session.Type(ctx, selector, text)
	})
	return err
}

func (h *browserSessionHandle) ExtractText(ctx context.Context, selector string) (*platformbrowser.Extraction, error) {
	return browserRun(h, ctx, "get_text", func(session *platformbrowser.Session) (*platformbrowser.Extraction, error) {
		return session.ExtractText(ctx, selector)
	})
}

func (h *browserSessionHandle) ExtractHTML(ctx context.Context) (*platformbrowser.Extraction, error) {
	return browserRun(h, ctx, "get_html", func(session *platformbrowser.Session) (*platformbrowser.Extraction, error) {
		return session.ExtractHTML(ctx)
	})
}

func (h *browserSessionHandle) ExtractAccessibilityTree(ctx context.Context) (*platformbrowser.Extraction, error) {
	return browserRun(h, ctx, "get_accessibility_tree", func(session *platformbrowser.Session) (*platformbrowser.Extraction, error) {
		return session.ExtractAccessibilityTree(ctx)
	})
}

func (h *browserSessionHandle) ExecuteScript(ctx context.Context, script string) (any, error) {
	return browserRun(h, ctx, "execute_js", func(session *platformbrowser.Session) (any, error) {
		return session.ExecuteScript(ctx, script)
	})
}

func (h *browserSessionHandle) Screenshot(ctx context.Context) ([]byte, error) {
	return browserRun(h, ctx, "screenshot", func(session *platformbrowser.Session) ([]byte, error) {
		return session.Screenshot(ctx)
	})
}

func (h *browserSessionHandle) WaitFor(ctx context.Context, condition platformbrowser.WaitCondition, timeout time.Duration) error {
	_, err := browserRun(h, ctx, "wait", func(session *platformbrowser.Session) (struct{}, error) {
		return struct{}{}, session.WaitFor(ctx, condition, timeout)
	})
	return err
}

func (h *browserSessionHandle) CurrentURL(ctx context.Context) (string, error) {
	return browserRun(h, ctx, "current_url", func(session *platformbrowser.Session) (string, error) {
		return session.CurrentURL(ctx)
	})
}

func (h *browserSessionHandle) CapturePageState(ctx context.Context) (*platformbrowser.PageState, error) {
	return browserRun(h, ctx, "page_state", func(session *platformbrowser.Session) (*platformbrowser.PageState, error) {
		return session.CapturePageState(ctx)
	})
}

func (h *browserSessionHandle) ExtractStructured(ctx context.Context) (*platformbrowser.StructuredPageData, *platformbrowser.Extraction, error) {
	return browserRun2(h, ctx, "extract_structured", func(session *platformbrowser.Session) (*platformbrowser.StructuredPageData, *platformbrowser.Extraction, error) {
		return session.ExtractStructured(ctx)
	})
}

func (h *browserSessionHandle) Capabilities() platformbrowser.Capabilities {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session == nil {
		return platformbrowser.Capabilities{}
	}
	return h.session.Capabilities()
}

func browserRun[T any](h *browserSessionHandle, ctx context.Context, operation string, fn func(*platformbrowser.Session) (T, error)) (T, error) {
	var zero T
	if h == nil {
		return zero, fmt.Errorf("browser session unavailable")
	}
	session, err := h.currentSession()
	if err != nil {
		return zero, err
	}
	result, err := fn(session)
	if err == nil || !platformbrowser.IsErrorCode(err, platformbrowser.ErrBackendDisconnected) {
		return result, err
	}
	if recoverErr := h.recover(ctx, operation, err); recoverErr != nil {
		return zero, recoverErr
	}
	session, err = h.currentSession()
	if err != nil {
		return zero, err
	}
	return fn(session)
}

func browserRun2[A any, B any](h *browserSessionHandle, ctx context.Context, operation string, fn func(*platformbrowser.Session) (A, B, error)) (A, B, error) {
	var zeroA A
	var zeroB B
	if h == nil {
		return zeroA, zeroB, fmt.Errorf("browser session unavailable")
	}
	session, err := h.currentSession()
	if err != nil {
		return zeroA, zeroB, err
	}
	first, second, err := fn(session)
	if err == nil || !platformbrowser.IsErrorCode(err, platformbrowser.ErrBackendDisconnected) {
		return first, second, err
	}
	if recoverErr := h.recover(ctx, operation, err); recoverErr != nil {
		return zeroA, zeroB, recoverErr
	}
	session, err = h.currentSession()
	if err != nil {
		return zeroA, zeroB, err
	}
	return fn(session)
}

func (h *browserSessionHandle) currentSession() (*platformbrowser.Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, fmt.Errorf("browser session closed")
	}
	if h.session == nil {
		return nil, fmt.Errorf("browser session unavailable")
	}
	return h.session, nil
}

func (h *browserSessionHandle) noteActivity() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastSeenAt = time.Now().UTC()
}

func (h *browserSessionHandle) notePageState(page *platformbrowser.PageState) {
	if h == nil || page == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastPage = page
	h.lastSeenAt = time.Now().UTC()
}

func (h *browserSessionHandle) providerSession() core.ProviderSession {
	if h == nil {
		return core.ProviderSession{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	session := core.ProviderSession{
		ID:             h.sessionID,
		ProviderID:     "browser",
		CapabilityIDs:  []string{"tool:browser"},
		WorkflowID:     h.workflowID,
		TaskID:         h.taskID,
		TrustClass:     core.TrustClassProviderLocalUntrusted,
		Recoverability: core.RecoverabilityInProcess,
		CreatedAt:      h.createdAt.UTC().Format(time.RFC3339Nano),
		LastActivityAt: h.lastSeenAt.UTC().Format(time.RFC3339Nano),
		Health:         "active",
		Metadata: map[string]interface{}{
			"backend":    h.backendName,
			"recoveries": h.recoveries,
			"path_roots": h.paths.roots(),
		},
	}
	if h.lastPage != nil {
		session.Metadata["last_url"] = h.lastPage.URL
		session.Metadata["last_title"] = h.lastPage.Title
	}
	if h.lastErr != "" {
		session.Metadata["last_recovery_error"] = h.lastErr
	}
	return session
}

func (h *browserSessionHandle) snapshot() core.ProviderSessionSnapshot {
	session := h.providerSession()
	var state any
	h.mu.Lock()
	if h.lastPage != nil {
		state = map[string]any{
			"page_state": h.lastPage,
			"backend":    h.backendName,
			"recoveries": h.recoveries,
		}
	}
	lastErr := h.lastErr
	h.mu.Unlock()
	return core.ProviderSessionSnapshot{
		Session:         session,
		State:           state,
		CapturedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		LastRecoveryErr: lastErr,
	}
}

func (h *browserSessionHandle) recover(ctx context.Context, operation string, cause error) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return fmt.Errorf("browser session closed")
	}
	old := h.session
	h.session = nil
	h.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
	newSession, err := h.factory(ctx, h.cfg)
	if err != nil {
		h.mu.Lock()
		h.lastErr = err.Error()
		h.mu.Unlock()
		emitBrowserTelemetry(h.telemetry, core.EventStateChange, h.agentID, h.taskID, "browser session recovery failed", map[string]interface{}{
			"browser_event": "session_recovery_failed",
			"session_id":    h.sessionID,
			"backend":       h.backendName,
			"operation":     operation,
			"cause":         cause.Error(),
			"error":         err.Error(),
		})
		return err
	}

	h.mu.Lock()
	h.session = newSession
	h.recoveries++
	h.lastErr = ""
	h.lastSeenAt = time.Now().UTC()
	recoveries := h.recoveries
	h.mu.Unlock()

	emitBrowserTelemetry(h.telemetry, core.EventStateChange, h.agentID, h.taskID, "browser session recovered", map[string]interface{}{
		"browser_event": "session_recovered",
		"session_id":    h.sessionID,
		"backend":       h.backendName,
		"operation":     operation,
		"recoveries":    recoveries,
		"cause":         cause.Error(),
	})
	if h.service != nil {
		h.service.persistSessionMetadata(h)
	}
	return nil
}

func (s *BrowserService) successWithSnapshot(ctx context.Context, env *contextdata.Envelope, session *browserSessionHandle, sessionID string, data map[string]interface{}) (*core.ToolResult, error) {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["session_id"] = sessionID
	if env == nil || session == nil {
		return success(data), nil
	}
	pageState, err := session.CapturePageState(ctx)
	if err == nil && pageState != nil {
		data["page_state"] = pageState
		session.notePageState(pageState)
		recordBrowserObservation(env, pageState)
		emitBrowserTelemetry(s.telemetry, core.EventStateChange, s.agentID(), browserTaskScope(env), "browser page snapshot captured", map[string]interface{}{
			"browser_event": "page_snapshot",
			"session_id":    sessionID,
			"url":           pageState.URL,
			"title":         pageState.Title,
			"backend":       session.backendName,
			"recoveries":    session.recoveries,
		})
	}
	s.persistSessionMetadata(session)
	return success(data), nil
}

func defaultIfEmpty(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<nil>" {
		return fallback
	}
	return value
}
