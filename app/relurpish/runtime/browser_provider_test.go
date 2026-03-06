package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"github.com/lexcodex/relurpify/tools/browser"
	"github.com/stretchr/testify/require"
)

type stubBrowserBackend struct {
	currentURL string
	text       string
	closed     int
}

func (s *stubBrowserBackend) Navigate(context.Context, string) error          { return nil }
func (s *stubBrowserBackend) Click(context.Context, string) error             { return nil }
func (s *stubBrowserBackend) Type(context.Context, string, string) error      { return nil }
func (s *stubBrowserBackend) GetText(context.Context, string) (string, error) { return s.text, nil }
func (s *stubBrowserBackend) GetAccessibilityTree(context.Context) (string, error) {
	return "{\"role\":\"document\"}", nil
}
func (s *stubBrowserBackend) GetHTML(context.Context) (string, error) { return "<html></html>", nil }
func (s *stubBrowserBackend) ExecuteScript(context.Context, string) (any, error) {
	return map[string]any{"ok": true}, nil
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

func TestBrowserProviderRegistersBrowserTool(t *testing.T) {
	registry := toolsys.NewToolRegistry()
	rt := &Runtime{
		Tools: registry,
	}

	err := newBrowserProvider().Initialize(context.Background(), rt)

	require.NoError(t, err)
	tool, ok := registry.Get("browser")
	require.True(t, ok)
	require.Equal(t, "browser", tool.Name())
}

func TestBrowserToolOpenUsesScopedDefaultSession(t *testing.T) {
	registry := toolsys.NewToolRegistry()
	rt := &Runtime{
		Tools:        registry,
		Context:      core.NewContext(),
		Registration: &fruntime.AgentRegistration{ID: "agent-browser"},
	}
	provider := newBrowserProvider()
	backend := &stubBrowserBackend{text: "ready"}
	provider.sessionFactory = func(ctx context.Context, cfg browserSessionConfig) (*browser.Session, error) {
		return browser.NewSession(browser.SessionConfig{
			Backend:     backend,
			BackendName: cfg.backendName,
		})
	}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	tool, ok := registry.Get("browser")
	require.True(t, ok)
	state := core.NewContext()
	state.Set("task.id", "task-123")

	openResult, err := tool.Execute(context.Background(), state, map[string]interface{}{"action": "open"})
	require.NoError(t, err)
	sessionID := openResult.Data["session_id"].(string)
	require.NotEmpty(t, sessionID)
	require.Equal(t, sessionID, state.GetString(browserDefaultSessionKey))

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

func TestShouldEnableBrowserProvider(t *testing.T) {
	require.True(t, shouldEnableBrowserProvider([]string{"coding", "web-testing"}))
	require.True(t, shouldEnableBrowserProvider([]string{"web-research"}))
	require.False(t, shouldEnableBrowserProvider([]string{"coding", "system"}))
}

func TestBrowserToolOpenPassesBackendSelection(t *testing.T) {
	registry := toolsys.NewToolRegistry()
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

	tool, ok := registry.Get("browser")
	require.True(t, ok)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"action":  "open",
		"backend": "webdriver",
	})

	require.NoError(t, err)
	require.Equal(t, "webdriver", seenBackend)
}

func TestBrowserToolOpenSupportsBiDiSelection(t *testing.T) {
	registry := toolsys.NewToolRegistry()
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

	tool, ok := registry.Get("browser")
	require.True(t, ok)

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"action":  "open",
		"backend": "bidi",
	})

	require.NoError(t, err)
	require.Equal(t, "bidi", seenBackend)
}

func TestBrowserToolParallelSessionsStayIsolatedAndScopeCleanupClosesThem(t *testing.T) {
	registry := toolsys.NewToolRegistry()
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

	tool, ok := registry.Get("browser")
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
