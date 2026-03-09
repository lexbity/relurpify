package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/tools/browser"
	"github.com/stretchr/testify/require"
)

type stubBrowserBackend struct {
	currentURL string
	text       string
	title      string
	closed     int
}

func (s *stubBrowserBackend) Navigate(_ context.Context, url string) error {
	s.currentURL = url
	return nil
}
func (s *stubBrowserBackend) Click(context.Context, string) error             { return nil }
func (s *stubBrowserBackend) Type(context.Context, string, string) error      { return nil }
func (s *stubBrowserBackend) GetText(context.Context, string) (string, error) { return s.text, nil }
func (s *stubBrowserBackend) GetAccessibilityTree(context.Context) (string, error) {
	return "{\"role\":\"document\"}", nil
}
func (s *stubBrowserBackend) GetHTML(context.Context) (string, error) { return "<html></html>", nil }
func (s *stubBrowserBackend) ExecuteScript(_ context.Context, script string) (any, error) {
	switch {
	case script == "document.title || ''":
		return s.title, nil
	case script == "window.location.href":
		return s.currentURL, nil
	case strings.Contains(script, "headings: Array.from"):
		return map[string]any{
			"url":      s.currentURL,
			"title":    s.title,
			"headings": []any{"Heading 1", "Heading 2"},
			"links": []any{
				map[string]any{"text": "Home", "href": "https://example.com/"},
				map[string]any{"text": "Docs", "href": "https://example.com/docs"},
			},
			"inputs": []any{
				map[string]any{"name": "q", "type": "search", "placeholder": "Search"},
				map[string]any{"name": "email", "type": "email", "placeholder": "Email"},
				map[string]any{"name": "notes", "type": "textarea", "placeholder": "Notes"},
			},
			"buttons": []any{"Submit", "Cancel"},
			"code":    []any{"fmt.Println(\"hello\")"},
		}, nil
	case script != "":
		return map[string]any{"links": 2, "forms": 1, "inputs": 3, "buttons": 1}, nil
	default:
		return map[string]any{"ok": true}, nil
	}
}
func (s *stubBrowserBackend) Screenshot(context.Context) ([]byte, error) {
	return []byte{0x89, 0x50, 0x4e, 0x47}, nil
}
func (s *stubBrowserBackend) WaitFor(context.Context, browser.WaitCondition, time.Duration) error {
	return nil
}
func (s *stubBrowserBackend) CurrentURL(context.Context) (string, error) { return s.currentURL, nil }
func (s *stubBrowserBackend) Close() error {
	s.closed++
	return nil
}

type flakyBrowserBackend struct {
	delegate *stubBrowserBackend
	failOnce bool
	closed   int
}

func (f *flakyBrowserBackend) Navigate(ctx context.Context, url string) error {
	if f.failOnce {
		f.failOnce = false
		return &browser.Error{Code: browser.ErrBackendDisconnected, Backend: "test", Operation: "navigate"}
	}
	return f.delegate.Navigate(ctx, url)
}
func (f *flakyBrowserBackend) Click(ctx context.Context, selector string) error {
	return f.delegate.Click(ctx, selector)
}
func (f *flakyBrowserBackend) Type(ctx context.Context, selector, text string) error {
	return f.delegate.Type(ctx, selector, text)
}
func (f *flakyBrowserBackend) GetText(ctx context.Context, selector string) (string, error) {
	return f.delegate.GetText(ctx, selector)
}
func (f *flakyBrowserBackend) GetAccessibilityTree(ctx context.Context) (string, error) {
	return f.delegate.GetAccessibilityTree(ctx)
}
func (f *flakyBrowserBackend) GetHTML(ctx context.Context) (string, error) {
	return f.delegate.GetHTML(ctx)
}
func (f *flakyBrowserBackend) ExecuteScript(ctx context.Context, script string) (any, error) {
	return f.delegate.ExecuteScript(ctx, script)
}
func (f *flakyBrowserBackend) Screenshot(ctx context.Context) ([]byte, error) {
	return f.delegate.Screenshot(ctx)
}
func (f *flakyBrowserBackend) WaitFor(ctx context.Context, condition browser.WaitCondition, timeout time.Duration) error {
	return f.delegate.WaitFor(ctx, condition, timeout)
}
func (f *flakyBrowserBackend) CurrentURL(ctx context.Context) (string, error) {
	return f.delegate.CurrentURL(ctx)
}
func (f *flakyBrowserBackend) Close() error {
	f.closed++
	return f.delegate.Close()
}

type recordingTelemetry struct {
	events []core.Event
}

func (r *recordingTelemetry) Emit(event core.Event) {
	r.events = append(r.events, event)
}

func TestBrowserProviderRegistersBrowserTool(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools: registry,
	}

	err := newBrowserProvider().Initialize(context.Background(), rt)

	require.NoError(t, err)
	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	require.Equal(t, "browser", tool.Name())
}

func TestBrowserToolOpenUsesScopedDefaultSession(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	backend := &stubBrowserBackend{text: "ready", title: "Ready"}
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		return browser.NewSession(browser.SessionConfig{
			Backend:     backend,
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	state := core.NewContext()
	state.Set("task.id", "task-123")

	openResult, err := tool.Execute(context.Background(), state, map[string]interface{}{"action": "open"})
	require.NoError(t, err)
	sessionID := openResult.Data["session_id"].(string)
	require.NotEmpty(t, sessionID)
	require.Equal(t, sessionID, state.GetString(browserDefaultSessionKey))
	require.NotNil(t, openResult.Data["page_state"])
	_, ok = openResult.Data["capabilities"].(browser.Capabilities)
	require.True(t, ok)

	textResult, err := tool.Execute(context.Background(), state, map[string]interface{}{
		"action":   "get_text",
		"selector": "#result",
	})
	require.NoError(t, err)
	require.Equal(t, "ready", textResult.Data["text"])

	closeResult, err := tool.Execute(context.Background(), state, map[string]interface{}{
		"action": "close",
	})
	require.NoError(t, err)
	require.Equal(t, true, closeResult.Data["closed"])
	require.Equal(t, 1, backend.closed)
	require.Empty(t, state.GetString(browserDefaultSessionKey))
}

func TestBrowserToolUsesAgentBrowserDefaultBackend(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	var seenBackend string
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		seenBackend = cfg.backendName
		return browser.NewSession(browser.SessionConfig{
			Backend:     &stubBrowserBackend{text: "ok", title: "Ready"},
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	registry.UseAgentSpec("agent-browser", &core.AgentRuntimeSpec{
		Mode: core.AgentModePrimary,
		Model: core.AgentModelConfig{
			Provider: "test",
			Name:     "test",
		},
		Browser: &core.AgentBrowserSpec{
			Enabled:        true,
			DefaultBackend: "bidi",
		},
	})
	tool, ok = registry.GetModelTool("browser")
	require.True(t, ok)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"action": "open"})

	require.NoError(t, err)
	require.Equal(t, "bidi", seenBackend)
}

func TestBrowserToolRejectsDisallowedBackend(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		return browser.NewSession(browser.SessionConfig{
			Backend:     &stubBrowserBackend{text: "ok", title: "Ready"},
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	registry.UseAgentSpec("agent-browser", &core.AgentRuntimeSpec{
		Mode: core.AgentModePrimary,
		Model: core.AgentModelConfig{
			Provider: "test",
			Name:     "test",
		},
		Browser: &core.AgentBrowserSpec{
			Enabled:         true,
			AllowedBackends: []string{"cdp"},
		},
	})
	tool, ok = registry.GetModelTool("browser")
	require.True(t, ok)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"action":  "open",
		"backend": "webdriver",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "blocked by agent spec")
}

func TestBrowserToolRejectsDeniedExecuteJS(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		return browser.NewSession(browser.SessionConfig{
			Backend:     &stubBrowserBackend{text: "ok", title: "Ready"},
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	registry.UseAgentSpec("agent-browser", &core.AgentRuntimeSpec{
		Mode: core.AgentModePrimary,
		Model: core.AgentModelConfig{
			Provider: "test",
			Name:     "test",
		},
		Browser: &core.AgentBrowserSpec{
			Enabled: true,
			Actions: map[string]core.AgentPermissionLevel{
				"execute_js": core.AgentPermissionDeny,
			},
		},
	})
	tool, ok = registry.GetModelTool("browser")
	require.True(t, ok)

	state := core.NewContext()
	openResult, err := tool.Execute(context.Background(), state, map[string]interface{}{"action": "open"})
	require.NoError(t, err)

	_, err = tool.Execute(context.Background(), state, map[string]interface{}{
		"action":     "execute_js",
		"session_id": openResult.Data["session_id"],
		"script":     "return 1",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "denied by agent spec")
}

func TestBrowserToolNavigateRecordsPageObservation(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	backend := &stubBrowserBackend{text: "ready", title: "Example"}
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		return browser.NewSession(browser.SessionConfig{
			Backend:     backend,
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	state := core.NewContext()

	openResult, err := tool.Execute(context.Background(), state, map[string]interface{}{"action": "open"})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), state, map[string]interface{}{
		"action":     "navigate",
		"session_id": openResult.Data["session_id"],
		"url":        "https://example.com/docs",
	})

	require.NoError(t, err)
	pageState, ok := result.Data["page_state"].(*browser.PageState)
	require.True(t, ok)
	require.Equal(t, "https://example.com/docs", pageState.URL)
	stored, ok := state.Get(browserLastPageStateKey)
	require.True(t, ok)
	require.Equal(t, pageState, stored)
	history := state.History()
	require.NotEmpty(t, history)
	require.Equal(t, "observation", history[len(history)-1].Role)
	require.Contains(t, history[len(history)-1].Content, "https://example.com/docs")
}

func TestBrowserToolExtractReturnsStructuredPageData(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	backend := &stubBrowserBackend{
		text:       "Hello from the docs page",
		title:      "Docs",
		currentURL: "https://example.com/docs",
	}
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		return browser.NewSession(browser.SessionConfig{
			Backend:     backend,
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	state := core.NewContext()
	openResult, err := tool.Execute(context.Background(), state, map[string]interface{}{"action": "open"})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), state, map[string]interface{}{
		"action":     "extract",
		"session_id": openResult.Data["session_id"],
	})

	require.NoError(t, err)
	structured, ok := result.Data["structured"].(*browser.StructuredPageData)
	require.True(t, ok)
	require.Equal(t, "Docs", structured.Title)
	require.Equal(t, "https://example.com/docs", structured.URL)
	require.Equal(t, 2, len(structured.Links))
	require.Equal(t, 3, len(structured.Inputs))
	require.Equal(t, false, result.Data["structured_truncated"])
	_, ok = result.Data["capabilities"].(browser.Capabilities)
	require.True(t, ok)
}

func TestShouldEnableBrowserProvider(t *testing.T) {
	require.True(t, shouldEnableBrowserProvider(&core.AgentRuntimeSpec{
		Browser: &core.AgentBrowserSpec{Enabled: true},
	}))
	require.False(t, shouldEnableBrowserProvider(&core.AgentRuntimeSpec{
		Browser: &core.AgentBrowserSpec{Enabled: false},
	}))
	require.False(t, shouldEnableBrowserProvider(nil))
}

func TestRegisterBuiltinProvidersUsesExplicitBrowserSpec(t *testing.T) {
	rt := &Runtime{
		Tools: capability.NewRegistry(),
		AgentSpec: &core.AgentRuntimeSpec{
			Browser: &core.AgentBrowserSpec{Enabled: true},
		},
	}
	require.NoError(t, RegisterBuiltinProviders(context.Background(), rt))
	require.Len(t, rt.registeredProviders(), 1)
}

func TestBrowserToolOpenPassesBackendSelection(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	var seenBackend string
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		seenBackend = cfg.backendName
		return browser.NewSession(browser.SessionConfig{
			Backend:     &stubBrowserBackend{text: "ok"},
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"action":  "open",
		"backend": "webdriver",
	})

	require.NoError(t, err)
	require.Equal(t, "webdriver", seenBackend)
}

func TestBrowserToolOpenSupportsBiDiSelection(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	var seenBackend string
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		seenBackend = cfg.backendName
		return browser.NewSession(browser.SessionConfig{
			Backend:     &stubBrowserBackend{text: "ok"},
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"action":  "open",
		"backend": "bidi",
	})

	require.NoError(t, err)
	require.Equal(t, "bidi", seenBackend)
}

func TestBrowserToolParallelSessionsStayIsolatedAndScopeCleanupClosesThem(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	backends := []*stubBrowserBackend{
		{text: "first"},
		{text: "second"},
	}
	index := 0
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		session, err := browser.NewSession(browser.SessionConfig{
			Backend:     backends[index],
			BackendName: cfg.backendName,
		})
		index++
		return session, err
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	state := core.NewContext()
	state.Set("task.id", "task-parallel")

	firstOpen, err := tool.Execute(context.Background(), state, map[string]interface{}{"action": "open"})
	require.NoError(t, err)
	firstSessionID := firstOpen.Data["session_id"].(string)

	secondOpen, err := tool.Execute(context.Background(), state, map[string]interface{}{"action": "open"})
	require.NoError(t, err)
	secondSessionID := secondOpen.Data["session_id"].(string)

	require.NotEqual(t, firstSessionID, secondSessionID)

	firstText, err := tool.Execute(context.Background(), state, map[string]interface{}{
		"action":     "get_text",
		"session_id": firstSessionID,
		"selector":   "#result",
	})
	require.NoError(t, err)
	require.Equal(t, "first", firstText.Data["text"])

	secondText, err := tool.Execute(context.Background(), state, map[string]interface{}{
		"action":     "get_text",
		"session_id": secondSessionID,
		"selector":   "#result",
	})
	require.NoError(t, err)
	require.Equal(t, "second", secondText.Data["text"])

	state.ClearHandleScope("task-parallel")
	require.Equal(t, 1, backends[0].closed)
	require.Equal(t, 1, backends[1].closed)
}

func TestBrowserToolRecoversAfterBackendDisconnect(t *testing.T) {
	registry := capability.NewRegistry()
	telemetry := &recordingTelemetry{}
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
		Telemetry:    telemetry,
	}
	provider := newBrowserProvider()
	created := 0
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		created++
		backend := &stubBrowserBackend{text: "ok", title: "Recovered"}
		if created == 1 {
			return browser.NewSession(browser.SessionConfig{
				Backend:     &flakyBrowserBackend{delegate: backend, failOnce: true},
				BackendName: cfg.backendName,
			})
		}
		return browser.NewSession(browser.SessionConfig{
			Backend:     backend,
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	state := core.NewContext()
	openResult, err := tool.Execute(context.Background(), state, map[string]interface{}{"action": "open"})
	require.NoError(t, err)

	_, err = tool.Execute(context.Background(), state, map[string]interface{}{
		"action":     "navigate",
		"session_id": openResult.Data["session_id"],
		"url":        "https://example.com/recovered",
	})

	require.NoError(t, err)
	require.Equal(t, 2, created)
	foundRecovery := false
	for _, event := range telemetry.events {
		if event.Metadata["browser_event"] == "session_recovered" {
			foundRecovery = true
			break
		}
	}
	require.True(t, foundRecovery)

	snapshots, err := provider.SnapshotSessions(context.Background())
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, "browser", snapshots[0].Session.ProviderID)
	require.Equal(t, 1, snapshots[0].Session.Metadata["recoveries"])
	stateData, ok := snapshots[0].State.(map[string]any)
	require.True(t, ok)
	require.Equal(t, 1, stateData["recoveries"])
}

func TestBrowserProviderCloseClosesTrackedSessions(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	backend := &stubBrowserBackend{text: "ready", title: "Ready"}
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		return browser.NewSession(browser.SessionConfig{
			Backend:     backend,
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"action": "open"})
	require.NoError(t, err)
	require.Len(t, provider.sessions, 1)

	require.NoError(t, provider.Close())
	require.Equal(t, 1, backend.closed)
	require.Empty(t, provider.sessions)
}

func TestBrowserProviderCloseSessionClosesTrackedSession(t *testing.T) {
	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	backend := &stubBrowserBackend{text: "ready", title: "Ready"}
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		return browser.NewSession(browser.SessionConfig{
			Backend:     backend,
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	openResult, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"action": "open"})
	require.NoError(t, err)

	require.NoError(t, provider.CloseSession(context.Background(), openResult.Data["session_id"].(string)))
	require.Equal(t, 1, backend.closed)
	require.Empty(t, provider.sessions)
}

func TestBrowserToolOpenHonorsSessionSafetyBudgets(t *testing.T) {
	registry := capability.NewRegistry()
	registry.UseAgentSpec("agent-browser", &core.AgentRuntimeSpec{
		RuntimeSafety: &core.RuntimeSafetySpec{
			MaxSubprocessesPerSession: 1,
			MaxNetworkRequestsSession: 1,
		},
	})
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	backends := []*stubBrowserBackend{
		{text: "ready", title: "Ready 1"},
		{text: "ready", title: "Ready 2"},
	}
	index := 0
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		session, err := browser.NewSession(browser.SessionConfig{
			Backend:     backends[index],
			BackendName: cfg.backendName,
		})
		index++
		return session, err
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.GetModelTool("browser")
	require.True(t, ok)
	state := core.NewContext()

	openResult, err := tool.Execute(context.Background(), state, map[string]interface{}{"action": "open"})
	require.NoError(t, err)
	require.NotEmpty(t, openResult.Data["session_id"])

	_, err = tool.Execute(context.Background(), state, map[string]interface{}{
		"action":     "navigate",
		"session_id": openResult.Data["session_id"],
		"url":        "https://example.com",
	})
	require.ErrorContains(t, err, "network request budget exceeded")
}
