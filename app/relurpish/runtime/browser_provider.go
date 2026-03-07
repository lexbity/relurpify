package runtime

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/tools/browser"
	"github.com/lexcodex/relurpify/tools/browser/bidi"
	"github.com/lexcodex/relurpify/tools/browser/cdp"
	"github.com/lexcodex/relurpify/tools/browser/webdriver"
)

const (
	browserDefaultSessionKey = "browser.default_session"
	browserLastPageStateKey  = "browser.last_page_state"
	browserPageStateListKey  = "browser.page_states"
	defaultBrowserScope      = "browser.default"
	defaultBrowserBackend    = "cdp"
)

type browserProvider struct {
	sessionFactory func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error)
	telemetry      core.Telemetry

	mu       sync.Mutex
	sessions map[string]*browserSessionHandle
}

type browserSessionConfig struct {
	backendName string
	manager     *fruntime.PermissionManager
	agentID     string
	maxTokens   int
	runtime     *Runtime
}

func newBrowserProvider() *browserProvider {
	return &browserProvider{
		sessionFactory: newBrowserSession,
		sessions:       make(map[string]*browserSessionHandle),
	}
}

// RegisterSkillProviders installs runtime-managed providers required by the active skills.
func RegisterSkillProviders(ctx context.Context, rt *Runtime, skills []string) error {
	if !shouldEnableBrowserProvider(skills) {
		return nil
	}
	return rt.RegisterProvider(ctx, newBrowserProvider())
}

func (p *browserProvider) Initialize(_ context.Context, rt *Runtime) error {
	if rt == nil || rt.Tools == nil {
		return fmt.Errorf("runtime tools unavailable")
	}
	p.telemetry = rt.Telemetry
	return rt.Tools.Register(&browserTool{
		provider: p,
		runtime:  rt,
	})
}

func (p *browserProvider) Close() error {
	p.mu.Lock()
	handles := make([]*browserSessionHandle, 0, len(p.sessions))
	for _, handle := range p.sessions {
		handles = append(handles, handle)
	}
	p.sessions = make(map[string]*browserSessionHandle)
	p.mu.Unlock()

	var errs []error
	for _, handle := range handles {
		if err := handle.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func newBrowserSession(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
	sandboxed, err := newSandboxedBrowserBackend(ctx, cfg)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(cfg.backendName)) {
	case "", defaultBrowserBackend:
		backend, err := cdp.New(ctx, cdp.Config{
			Headless:     true,
			WebSocketURL: sandboxed.cdpWebSocketURL,
		})
		if err != nil {
			_ = sandboxed.close()
			return nil, err
		}
		maxTokens := cfg.maxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		return browser.NewSession(browser.SessionConfig{
			Backend:           wrapManagedBrowserBackend(backend, sandboxed.close),
			BackendName:       defaultBrowserBackend,
			PermissionManager: cfg.manager,
			AgentID:           cfg.agentID,
			Budget:            core.NewContextBudget(maxTokens),
		})
	case "webdriver":
		backend, err := webdriver.New(ctx, webdriver.Config{
			Headless:    true,
			RemoteURL:   sandboxed.remoteURL,
			BrowserArgs: []string{"--disable-dev-shm-usage"},
		})
		if err != nil {
			_ = sandboxed.close()
			return nil, err
		}
		maxTokens := cfg.maxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		return browser.NewSession(browser.SessionConfig{
			Backend:           wrapManagedBrowserBackend(backend, sandboxed.close),
			BackendName:       "webdriver",
			PermissionManager: cfg.manager,
			AgentID:           cfg.agentID,
			Budget:            core.NewContextBudget(maxTokens),
		})
	case "bidi":
		backend, err := bidi.New(ctx, bidi.Config{
			Headless:    true,
			RemoteURL:   sandboxed.remoteURL,
			BrowserArgs: []string{"--disable-dev-shm-usage"},
		})
		if err != nil {
			_ = sandboxed.close()
			return nil, err
		}
		maxTokens := cfg.maxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		return browser.NewSession(browser.SessionConfig{
			Backend:           wrapManagedBrowserBackend(backend, sandboxed.close),
			BackendName:       "bidi",
			PermissionManager: cfg.manager,
			AgentID:           cfg.agentID,
			Budget:            core.NewContextBudget(maxTokens),
		})
	default:
		return nil, &browser.Error{
			Code:      browser.ErrUnsupportedOperation,
			Backend:   strings.ToLower(strings.TrimSpace(cfg.backendName)),
			Operation: "open",
			Err:       fmt.Errorf("unsupported browser backend"),
		}
	}
}

type browserTool struct {
	provider *browserProvider
	runtime  *Runtime
	spec     *core.AgentRuntimeSpec
}

type browserSessionHandle struct {
	mu          sync.Mutex
	session     *browser.Session
	cfg         browserSessionConfig
	factory     func(context.Context, browserSessionConfig) (*browser.Session, error)
	telemetry   core.Telemetry
	taskID      string
	agentID     string
	sessionID   string
	backendName string
	recoveries  int
	closed      bool
}

func (t *browserTool) Name() string { return "browser" }
func (t *browserTool) Description() string {
	return "Controls a browser session via a single action-dispatch tool."
}
func (t *browserTool) Category() string { return "browser" }
func (t *browserTool) Tags() []string   { return []string{core.TagNetwork, "browser", "web"} }

func (t *browserTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "action", Type: "string", Required: true},
		{Name: "session_id", Type: "string", Required: false},
		{Name: "backend", Type: "string", Required: false, Default: defaultBrowserBackend},
		{Name: "url", Type: "string", Required: false},
		{Name: "selector", Type: "string", Required: false},
		{Name: "text", Type: "string", Required: false},
		{Name: "script", Type: "string", Required: false},
		{Name: "timeout_ms", Type: "number", Required: false, Default: 10000},
	}
}

func (t *browserTool) SetAgentSpec(spec *core.AgentRuntimeSpec, _ string) {
	t.spec = spec
}

func (t *browserTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	action := strings.ToLower(strings.TrimSpace(fmt.Sprint(args["action"])))
	if err := t.authorizeAction(ctx, action, state, args); err != nil {
		return nil, err
	}
	switch action {
	case "open":
		return t.open(ctx, state, args)
	case "navigate":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.Navigate(ctx, fmt.Sprint(args["url"])); err != nil {
			return nil, err
		}
		return t.successWithSnapshot(ctx, state, session, sessionID, nil)
	case "click":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.Click(ctx, fmt.Sprint(args["selector"])); err != nil {
			return nil, err
		}
		return t.successWithSnapshot(ctx, state, session, sessionID, nil)
	case "type":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.Type(ctx, fmt.Sprint(args["selector"]), fmt.Sprint(args["text"])); err != nil {
			return nil, err
		}
		return t.successWithSnapshot(ctx, state, session, sessionID, nil)
	case "get_text":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		extraction, err := session.ExtractText(ctx, fmt.Sprint(args["selector"]))
		if err != nil {
			return nil, err
		}
		return success(withExtraction(sessionID, extraction, "text")), nil
	case "extract":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		pageState, err := session.CapturePageState(ctx)
		if err != nil {
			return nil, err
		}
		structured, structuredExtraction, err := session.ExtractStructured(ctx)
		if err != nil {
			return nil, err
		}
		axTree, err := session.ExtractAccessibilityTree(ctx)
		if err != nil {
			return nil, err
		}
		result := withExtraction(sessionID, axTree, "accessibility_tree")
		result["page_state"] = pageState
		result["structured"] = structured
		result["structured_truncated"] = structuredExtraction.Truncated
		result["structured_original_tokens"] = structuredExtraction.OriginalTokens
		result["structured_final_tokens"] = structuredExtraction.FinalTokens
		result["capabilities"] = session.Capabilities()
		recordBrowserObservation(state, pageState)
		return success(result), nil
	case "get_html":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		extraction, err := session.ExtractHTML(ctx)
		if err != nil {
			return nil, err
		}
		return success(withExtraction(sessionID, extraction, "html")), nil
	case "get_accessibility_tree":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		extraction, err := session.ExtractAccessibilityTree(ctx)
		if err != nil {
			return nil, err
		}
		return success(withExtraction(sessionID, extraction, "accessibility_tree")), nil
	case "execute_js":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		result, err := session.ExecuteScript(ctx, fmt.Sprint(args["script"]))
		if err != nil {
			return nil, err
		}
		return success(map[string]interface{}{"session_id": sessionID, "result": result}), nil
	case "screenshot":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		data, err := session.Screenshot(ctx)
		if err != nil {
			return nil, err
		}
		return success(map[string]interface{}{
			"session_id": sessionID,
			"png_base64": base64.StdEncoding.EncodeToString(data),
			"size_bytes": len(data),
		}), nil
	case "wait":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.WaitFor(ctx, waitConditionFromArgs(args), timeoutFromArgs(args)); err != nil {
			return nil, err
		}
		return t.successWithSnapshot(ctx, state, session, sessionID, nil)
	case "current_url":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		currentURL, err := session.CurrentURL(ctx)
		if err != nil {
			return nil, err
		}
		return success(map[string]interface{}{"session_id": sessionID, "url": currentURL}), nil
	case "close":
		return t.close(state, args)
	default:
		return nil, fmt.Errorf("unsupported browser action %q", action)
	}
}

func (t *browserTool) IsAvailable(context.Context, *core.Context) bool {
	if t.spec != nil && t.spec.Browser != nil {
		return t.spec.Browser.Enabled
	}
	return true
}

func (t *browserTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(".", core.FileSystemRead)}
}

func (t *browserTool) open(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	if state == nil || state.Registry() == nil {
		return nil, fmt.Errorf("context registry unavailable")
	}
	backendName := t.resolveBackend(args)
	cfg := browserSessionConfig{
		backendName: backendName,
		manager:     t.runtime.Registration.Permissions,
		agentID:     t.runtime.Registration.ID,
		maxTokens:   t.maxTokens(),
		runtime:     t.runtime,
	}
	session, err := t.provider.sessionFactory(ctx, cfg)
	if err != nil {
		return nil, err
	}
	handle := &browserSessionHandle{
		session:     session,
		cfg:         cfg,
		factory:     t.provider.sessionFactory,
		telemetry:   t.runtime.Telemetry,
		taskID:      browserTaskScope(state),
		agentID:     t.runtime.Registration.ID,
		backendName: backendName,
	}
	scope := browserTaskScope(state)
	sessionID := state.Registry().RegisterScoped(scope, handle)
	handle.sessionID = sessionID
	t.provider.trackSession(sessionID, handle)
	state.Set(browserDefaultSessionKey, sessionID)
	emitBrowserTelemetry(t.runtime.Telemetry, core.EventStateChange, t.runtime.Registration.ID, scope, "browser session opened", map[string]interface{}{
		"browser_event": "session_opened",
		"session_id":    sessionID,
		"backend":       backendName,
	})
	result, err := t.successWithSnapshot(ctx, state, handle, sessionID, map[string]interface{}{
		"backend":      backendName,
		"capabilities": handle.Capabilities(),
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (t *browserTool) close(state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	sessionID := defaultSessionID(state, args)
	if sessionID == "" {
		return nil, fmt.Errorf("browser session not found")
	}
	if state == nil || state.Registry() == nil {
		return nil, fmt.Errorf("context registry unavailable")
	}
	state.Registry().Remove(sessionID)
	t.provider.untrackSession(sessionID)
	if state.GetString(browserDefaultSessionKey) == sessionID {
		state.Set(browserDefaultSessionKey, "")
	}
	emitBrowserTelemetry(t.runtime.Telemetry, core.EventStateChange, t.runtime.Registration.ID, browserTaskScope(state), "browser session closed", map[string]interface{}{
		"browser_event": "session_closed",
		"session_id":    sessionID,
	})
	return success(map[string]interface{}{"session_id": sessionID, "closed": true}), nil
}

func (t *browserTool) lookupSession(state *core.Context, args map[string]interface{}) (*browserSessionHandle, string, error) {
	sessionID := defaultSessionID(state, args)
	if sessionID == "" {
		return nil, "", fmt.Errorf("browser session not found")
	}
	if state == nil {
		return nil, "", fmt.Errorf("context unavailable")
	}
	raw, ok := state.Registry().Lookup(sessionID)
	if !ok {
		return nil, "", fmt.Errorf("browser session %s not found", sessionID)
	}
	session, ok := raw.(*browserSessionHandle)
	if !ok {
		return nil, "", fmt.Errorf("browser session %s has invalid type", sessionID)
	}
	return session, sessionID, nil
}

func (t *browserTool) maxTokens() int {
	if t.spec == nil || t.spec.Context.MaxTokens <= 0 {
		return 8192
	}
	return t.spec.Context.MaxTokens
}

func (t *browserTool) resolveBackend(args map[string]interface{}) string {
	backend := defaultIfEmpty(fmt.Sprint(args["backend"]), "")
	if backend == "" && t.spec != nil && t.spec.Browser != nil {
		backend = defaultIfEmpty(t.spec.Browser.DefaultBackend, "")
	}
	return defaultIfEmpty(backend, defaultBrowserBackend)
}

func (t *browserTool) authorizeAction(ctx context.Context, action string, state *core.Context, args map[string]interface{}) error {
	if action == "" {
		return fmt.Errorf("browser action required")
	}
	if t.spec == nil || t.spec.Browser == nil {
		return nil
	}
	if !t.spec.Browser.Enabled {
		return fmt.Errorf("browser tool disabled by agent spec")
	}
	if action == "open" {
		backend := strings.ToLower(strings.TrimSpace(t.resolveBackend(args)))
		if len(t.spec.Browser.AllowedBackends) > 0 {
			allowed := false
			for _, candidate := range t.spec.Browser.AllowedBackends {
				if strings.EqualFold(strings.TrimSpace(candidate), backend) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("browser backend %s blocked by agent spec", backend)
			}
		}
	}
	level := t.spec.Browser.Actions[action]
	switch level {
	case "", core.AgentPermissionAllow:
		return nil
	case core.AgentPermissionDeny:
		return fmt.Errorf("browser action %s denied by agent spec", action)
	case core.AgentPermissionAsk:
		if t.runtime == nil || t.runtime.Registration == nil || t.runtime.Registration.Permissions == nil {
			return fmt.Errorf("browser action %s requires approval but permission manager missing", action)
		}
		resource := t.runtime.Registration.ID
		if state != nil {
			if sessionID := defaultSessionID(state, args); sessionID != "" {
				resource = sessionID
			}
		}
		return t.runtime.Registration.Permissions.RequireApproval(ctx, t.runtime.Registration.ID, fruntime.PermissionDescriptor{
			Type:         core.PermissionTypeHITL,
			Action:       fmt.Sprintf("browser:%s", action),
			Resource:     resource,
			RequiresHITL: true,
		}, "browser action approval", fruntime.GrantScopeOneTime, fruntime.RiskLevelMedium, 0)
	default:
		return fmt.Errorf("browser action %s has invalid policy %s", action, level)
	}
}

func (t *browserTool) successWithSnapshot(ctx context.Context, state *core.Context, session *browserSessionHandle, sessionID string, data map[string]interface{}) (*core.ToolResult, error) {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["session_id"] = sessionID
	if state == nil || session == nil {
		return success(data), nil
	}
	pageState, err := session.CapturePageState(ctx)
	if err == nil && pageState != nil {
		data["page_state"] = pageState
		recordBrowserObservation(state, pageState)
		emitBrowserTelemetry(t.runtime.Telemetry, core.EventStateChange, t.runtime.Registration.ID, browserTaskScope(state), "browser page snapshot captured", map[string]interface{}{
			"browser_event": "page_snapshot",
			"session_id":    sessionID,
			"url":           pageState.URL,
			"title":         pageState.Title,
			"backend":       session.backendName,
			"recoveries":    session.recoveries,
		})
	}
	return success(data), nil
}

func recordBrowserObservation(state *core.Context, pageState *browser.PageState) {
	if state == nil || pageState == nil {
		return
	}
	state.Set(browserLastPageStateKey, pageState)
	var snapshots []*browser.PageState
	if existing, ok := state.Get(browserPageStateListKey); ok {
		if typed, ok := existing.([]*browser.PageState); ok {
			snapshots = append(snapshots, typed...)
		}
	}
	snapshots = append(snapshots, pageState)
	state.Set(browserPageStateListKey, snapshots)
	state.AddInteraction("observation", formatBrowserObservation(pageState), map[string]interface{}{
		"kind": "browser_page_state",
		"url":  pageState.URL,
	})
}

func formatBrowserObservation(pageState *browser.PageState) string {
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

func withExtraction(sessionID string, extraction *browser.Extraction, key string) map[string]interface{} {
	return map[string]interface{}{
		"session_id":      sessionID,
		key:               extraction.Content,
		"truncated":       extraction.Truncated,
		"original_tokens": extraction.OriginalTokens,
		"final_tokens":    extraction.FinalTokens,
	}
}

func success(data map[string]interface{}) *core.ToolResult {
	return &core.ToolResult{Success: true, Data: data}
}

func waitConditionFromArgs(args map[string]interface{}) browser.WaitCondition {
	switch {
	case strings.TrimSpace(fmt.Sprint(args["selector"])) != "" && strings.TrimSpace(fmt.Sprint(args["text"])) != "":
		return browser.WaitCondition{
			Type:     browser.WaitForText,
			Selector: fmt.Sprint(args["selector"]),
			Text:     fmt.Sprint(args["text"]),
		}
	case strings.TrimSpace(fmt.Sprint(args["selector"])) != "":
		return browser.WaitCondition{
			Type:     browser.WaitForSelector,
			Selector: fmt.Sprint(args["selector"]),
		}
	case strings.TrimSpace(fmt.Sprint(args["url"])) != "":
		return browser.WaitCondition{
			Type:        browser.WaitForURLContains,
			URLContains: fmt.Sprint(args["url"]),
		}
	default:
		return browser.WaitCondition{Type: browser.WaitForLoad}
	}
}

func timeoutFromArgs(args map[string]interface{}) time.Duration {
	raw := strings.TrimSpace(fmt.Sprint(args["timeout_ms"]))
	if raw == "" || raw == "<nil>" {
		return 10 * time.Second
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 10 * time.Second
	}
	return time.Duration(value) * time.Millisecond
}

func browserTaskScope(state *core.Context) string {
	if state == nil {
		return defaultBrowserScope
	}
	if taskID := strings.TrimSpace(state.GetString("task.id")); taskID != "" {
		return taskID
	}
	return defaultBrowserScope
}

func defaultSessionID(state *core.Context, args map[string]interface{}) string {
	sessionID := strings.TrimSpace(fmt.Sprint(args["session_id"]))
	if sessionID != "" && sessionID != "<nil>" {
		return sessionID
	}
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.GetString(browserDefaultSessionKey))
}

func defaultIfEmpty(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<nil>" {
		return fallback
	}
	return value
}

func shouldEnableBrowserProvider(skills []string) bool {
	for _, skill := range skills {
		skill = strings.TrimSpace(strings.ToLower(skill))
		if strings.HasPrefix(skill, "web-") {
			return true
		}
	}
	return false
}

func (p *browserProvider) trackSession(sessionID string, handle *browserSessionHandle) {
	if p == nil || sessionID == "" || handle == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessions[sessionID] = handle
}

func (p *browserProvider) untrackSession(sessionID string) {
	if p == nil || sessionID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, sessionID)
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
	_, err := browserRun(h, ctx, "navigate", func(session *browser.Session) (struct{}, error) {
		return struct{}{}, session.Navigate(ctx, url)
	})
	return err
}

func (h *browserSessionHandle) Click(ctx context.Context, selector string) error {
	_, err := browserRun(h, ctx, "click", func(session *browser.Session) (struct{}, error) {
		return struct{}{}, session.Click(ctx, selector)
	})
	return err
}

func (h *browserSessionHandle) Type(ctx context.Context, selector, text string) error {
	_, err := browserRun(h, ctx, "type", func(session *browser.Session) (struct{}, error) {
		return struct{}{}, session.Type(ctx, selector, text)
	})
	return err
}

func (h *browserSessionHandle) ExtractText(ctx context.Context, selector string) (*browser.Extraction, error) {
	return browserRun(h, ctx, "get_text", func(session *browser.Session) (*browser.Extraction, error) {
		return session.ExtractText(ctx, selector)
	})
}

func (h *browserSessionHandle) ExtractHTML(ctx context.Context) (*browser.Extraction, error) {
	return browserRun(h, ctx, "get_html", func(session *browser.Session) (*browser.Extraction, error) {
		return session.ExtractHTML(ctx)
	})
}

func (h *browserSessionHandle) ExtractAccessibilityTree(ctx context.Context) (*browser.Extraction, error) {
	return browserRun(h, ctx, "get_accessibility_tree", func(session *browser.Session) (*browser.Extraction, error) {
		return session.ExtractAccessibilityTree(ctx)
	})
}

func (h *browserSessionHandle) ExecuteScript(ctx context.Context, script string) (any, error) {
	return browserRun(h, ctx, "execute_js", func(session *browser.Session) (any, error) {
		return session.ExecuteScript(ctx, script)
	})
}

func (h *browserSessionHandle) Screenshot(ctx context.Context) ([]byte, error) {
	return browserRun(h, ctx, "screenshot", func(session *browser.Session) ([]byte, error) {
		return session.Screenshot(ctx)
	})
}

func (h *browserSessionHandle) WaitFor(ctx context.Context, condition browser.WaitCondition, timeout time.Duration) error {
	_, err := browserRun(h, ctx, "wait", func(session *browser.Session) (struct{}, error) {
		return struct{}{}, session.WaitFor(ctx, condition, timeout)
	})
	return err
}

func (h *browserSessionHandle) CurrentURL(ctx context.Context) (string, error) {
	return browserRun(h, ctx, "current_url", func(session *browser.Session) (string, error) {
		return session.CurrentURL(ctx)
	})
}

func (h *browserSessionHandle) CapturePageState(ctx context.Context) (*browser.PageState, error) {
	return browserRun(h, ctx, "page_state", func(session *browser.Session) (*browser.PageState, error) {
		return session.CapturePageState(ctx)
	})
}

func (h *browserSessionHandle) ExtractStructured(ctx context.Context) (*browser.StructuredPageData, *browser.Extraction, error) {
	return browserRun2(h, ctx, "extract_structured", func(session *browser.Session) (*browser.StructuredPageData, *browser.Extraction, error) {
		return session.ExtractStructured(ctx)
	})
}

func (h *browserSessionHandle) Capabilities() browser.Capabilities {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session == nil {
		return browser.Capabilities{}
	}
	return h.session.Capabilities()
}

func browserRun[T any](h *browserSessionHandle, ctx context.Context, operation string, fn func(*browser.Session) (T, error)) (T, error) {
	var zero T
	if h == nil {
		return zero, fmt.Errorf("browser session unavailable")
	}
	session, err := h.currentSession()
	if err != nil {
		return zero, err
	}
	result, err := fn(session)
	if err == nil || !browser.IsErrorCode(err, browser.ErrBackendDisconnected) {
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

func browserRun2[A any, B any](h *browserSessionHandle, ctx context.Context, operation string, fn func(*browser.Session) (A, B, error)) (A, B, error) {
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
	if err == nil || !browser.IsErrorCode(err, browser.ErrBackendDisconnected) {
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

func (h *browserSessionHandle) currentSession() (*browser.Session, error) {
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
	return nil
}

type managedBrowserBackend struct {
	backend browser.Backend
	cleanup func() error
}

func wrapManagedBrowserBackend(backend browser.Backend, cleanup func() error) browser.Backend {
	return &managedBrowserBackend{backend: backend, cleanup: cleanup}
}

func (m *managedBrowserBackend) Navigate(ctx context.Context, url string) error {
	return m.backend.Navigate(ctx, url)
}

func (m *managedBrowserBackend) Click(ctx context.Context, selector string) error {
	return m.backend.Click(ctx, selector)
}

func (m *managedBrowserBackend) Type(ctx context.Context, selector, text string) error {
	return m.backend.Type(ctx, selector, text)
}

func (m *managedBrowserBackend) GetText(ctx context.Context, selector string) (string, error) {
	return m.backend.GetText(ctx, selector)
}

func (m *managedBrowserBackend) GetAccessibilityTree(ctx context.Context) (string, error) {
	return m.backend.GetAccessibilityTree(ctx)
}

func (m *managedBrowserBackend) GetHTML(ctx context.Context) (string, error) {
	return m.backend.GetHTML(ctx)
}

func (m *managedBrowserBackend) ExecuteScript(ctx context.Context, script string) (any, error) {
	return m.backend.ExecuteScript(ctx, script)
}

func (m *managedBrowserBackend) Screenshot(ctx context.Context) ([]byte, error) {
	return m.backend.Screenshot(ctx)
}

func (m *managedBrowserBackend) WaitFor(ctx context.Context, condition browser.WaitCondition, timeout time.Duration) error {
	return m.backend.WaitFor(ctx, condition, timeout)
}

func (m *managedBrowserBackend) CurrentURL(ctx context.Context) (string, error) {
	return m.backend.CurrentURL(ctx)
}

func (m *managedBrowserBackend) Capabilities() browser.Capabilities {
	if reporter, ok := m.backend.(browser.CapabilityReporter); ok {
		return reporter.Capabilities()
	}
	return browser.Capabilities{ArbitraryEval: true}
}

func (m *managedBrowserBackend) Close() error {
	var errs []error
	if m.backend != nil {
		errs = append(errs, m.backend.Close())
	}
	if m.cleanup != nil {
		errs = append(errs, m.cleanup())
	}
	return errors.Join(errs...)
}
