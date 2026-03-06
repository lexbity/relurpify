package runtime

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/browser"
	"github.com/lexcodex/relurpify/framework/browser/bidi"
	"github.com/lexcodex/relurpify/framework/browser/cdp"
	"github.com/lexcodex/relurpify/framework/browser/webdriver"
	"github.com/lexcodex/relurpify/framework/core"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
)

const (
	browserDefaultSessionKey = "browser.default_session"
	defaultBrowserScope      = "browser.default"
	defaultBrowserBackend    = "cdp"
)

type browserProvider struct {
	sessionFactory func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error)
}

type browserSessionConfig struct {
	backendName string
	manager     *fruntime.PermissionManager
	agentID     string
	maxTokens   int
}

func newBrowserProvider() *browserProvider {
	return &browserProvider{
		sessionFactory: newBrowserSession,
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
	return rt.Tools.Register(&browserTool{
		provider: p,
		runtime:  rt,
	})
}

func (p *browserProvider) Close() error {
	return nil
}

func newBrowserSession(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.backendName)) {
	case "", defaultBrowserBackend:
		backend, err := cdp.New(ctx, cdp.Config{Headless: true})
		if err != nil {
			return nil, err
		}
		maxTokens := cfg.maxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		return browser.NewSession(browser.SessionConfig{
			Backend:           backend,
			BackendName:       defaultBrowserBackend,
			PermissionManager: cfg.manager,
			AgentID:           cfg.agentID,
			Budget:            core.NewContextBudget(maxTokens),
		})
	case "webdriver":
		backend, err := webdriver.New(ctx, webdriver.Config{Headless: true})
		if err != nil {
			return nil, err
		}
		maxTokens := cfg.maxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		return browser.NewSession(browser.SessionConfig{
			Backend:           backend,
			BackendName:       "webdriver",
			PermissionManager: cfg.manager,
			AgentID:           cfg.agentID,
			Budget:            core.NewContextBudget(maxTokens),
		})
	case "bidi":
		backend, err := bidi.New(ctx, bidi.Config{Headless: true})
		if err != nil {
			return nil, err
		}
		maxTokens := cfg.maxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		return browser.NewSession(browser.SessionConfig{
			Backend:           backend,
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
		return success(map[string]interface{}{"session_id": sessionID}), nil
	case "click":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.Click(ctx, fmt.Sprint(args["selector"])); err != nil {
			return nil, err
		}
		return success(map[string]interface{}{"session_id": sessionID}), nil
	case "type":
		session, sessionID, err := t.lookupSession(state, args)
		if err != nil {
			return nil, err
		}
		if err := session.Type(ctx, fmt.Sprint(args["selector"]), fmt.Sprint(args["text"])); err != nil {
			return nil, err
		}
		return success(map[string]interface{}{"session_id": sessionID}), nil
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
		return success(map[string]interface{}{"session_id": sessionID}), nil
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
	return true
}

func (t *browserTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(".", core.FileSystemRead)}
}

func (t *browserTool) open(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	if state == nil || state.Registry() == nil {
		return nil, fmt.Errorf("context registry unavailable")
	}
	session, err := t.provider.sessionFactory(ctx, browserSessionConfig{
		backendName: fmt.Sprint(args["backend"]),
		manager:     t.runtime.Registration.Permissions,
		agentID:     t.runtime.Registration.ID,
		maxTokens:   t.maxTokens(),
	})
	if err != nil {
		return nil, err
	}
	scope := browserTaskScope(state)
	sessionID := state.Registry().RegisterScoped(scope, session)
	state.Set(browserDefaultSessionKey, sessionID)
	return success(map[string]interface{}{"session_id": sessionID, "backend": defaultIfEmpty(fmt.Sprint(args["backend"]), defaultBrowserBackend)}), nil
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
	if state.GetString(browserDefaultSessionKey) == sessionID {
		state.Set(browserDefaultSessionKey, "")
	}
	return success(map[string]interface{}{"session_id": sessionID, "closed": true}), nil
}

func (t *browserTool) lookupSession(state *core.Context, args map[string]interface{}) (*browser.Session, string, error) {
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
	session, ok := raw.(*browser.Session)
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
