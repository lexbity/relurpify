package browser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
	platformbrowser "github.com/lexcodex/relurpify/platform/browser"
	"github.com/stretchr/testify/require"
)

type approvingHITL struct {
	requests []fauthorization.PermissionRequest
}

func (a *approvingHITL) RequestPermission(_ context.Context, req fauthorization.PermissionRequest) (*fauthorization.PermissionGrant, error) {
	a.requests = append(a.requests, req)
	return &fauthorization.PermissionGrant{
		ID:         req.ID,
		Permission: req.Permission,
		Scope:      req.Scope,
		GrantedAt:  time.Now().UTC(),
	}, nil
}

type serviceStubBackend struct {
	currentURL string
	text       string
	title      string
	closed     int
}

func (s *serviceStubBackend) Navigate(_ context.Context, url string) error {
	s.currentURL = url
	return nil
}
func (s *serviceStubBackend) Click(context.Context, string) error        { return nil }
func (s *serviceStubBackend) Type(context.Context, string, string) error { return nil }
func (s *serviceStubBackend) GetText(context.Context, string) (string, error) {
	return s.text, nil
}
func (s *serviceStubBackend) GetAccessibilityTree(context.Context) (string, error) {
	return `{"role":"document"}`, nil
}
func (s *serviceStubBackend) GetHTML(context.Context) (string, error) { return "<html></html>", nil }
func (s *serviceStubBackend) ExecuteScript(_ context.Context, script string) (any, error) {
	switch {
	case script == "document.title || ''":
		return s.title, nil
	case script == "window.location.href":
		return s.currentURL, nil
	case strings.Contains(script, "document.links.length"):
		return map[string]any{
			"links":   1,
			"forms":   0,
			"inputs":  0,
			"buttons": 0,
		}, nil
	default:
		return map[string]any{"ok": true}, nil
	}
}
func (s *serviceStubBackend) Screenshot(context.Context) ([]byte, error) {
	return []byte{0x89, 0x50, 0x4e, 0x47}, nil
}
func (s *serviceStubBackend) WaitFor(context.Context, platformbrowser.WaitCondition, time.Duration) error {
	return nil
}
func (s *serviceStubBackend) CurrentURL(context.Context) (string, error) { return s.currentURL, nil }
func (s *serviceStubBackend) Close() error {
	s.closed++
	return nil
}

type failingOnceBackend struct {
	delegate *serviceStubBackend
	failOnce bool
	closed   int
}

func (f *failingOnceBackend) Navigate(ctx context.Context, url string) error {
	if f.failOnce {
		f.failOnce = false
		return &platformbrowser.Error{Code: platformbrowser.ErrBackendDisconnected, Backend: "test", Operation: "navigate"}
	}
	return f.delegate.Navigate(ctx, url)
}
func (f *failingOnceBackend) Click(ctx context.Context, selector string) error {
	return f.delegate.Click(ctx, selector)
}
func (f *failingOnceBackend) Type(ctx context.Context, selector, text string) error {
	return f.delegate.Type(ctx, selector, text)
}
func (f *failingOnceBackend) GetText(ctx context.Context, selector string) (string, error) {
	return f.delegate.GetText(ctx, selector)
}
func (f *failingOnceBackend) GetAccessibilityTree(ctx context.Context) (string, error) {
	return f.delegate.GetAccessibilityTree(ctx)
}
func (f *failingOnceBackend) GetHTML(ctx context.Context) (string, error) {
	return f.delegate.GetHTML(ctx)
}
func (f *failingOnceBackend) ExecuteScript(ctx context.Context, script string) (any, error) {
	return f.delegate.ExecuteScript(ctx, script)
}
func (f *failingOnceBackend) Screenshot(ctx context.Context) ([]byte, error) {
	return f.delegate.Screenshot(ctx)
}
func (f *failingOnceBackend) WaitFor(ctx context.Context, condition platformbrowser.WaitCondition, timeout time.Duration) error {
	return f.delegate.WaitFor(ctx, condition, timeout)
}
func (f *failingOnceBackend) CurrentURL(ctx context.Context) (string, error) {
	return f.delegate.CurrentURL(ctx)
}
func (f *failingOnceBackend) Close() error {
	f.closed++
	return f.delegate.Close()
}

func TestBrowserService_StartRegistersCapabilityAndOpensSession(t *testing.T) {
	registry := capability.NewRegistry()
	workspace := t.TempDir()
	backend := &serviceStubBackend{text: "ready", title: "Ready"}
	svc := New(BrowserServiceConfig{
		WorkspaceRoot: workspace,
		FileScope:     sandbox.NewFileScopePolicy(workspace, nil),
		Registry:      registry,
		Registration:  &fauthorization.AgentRegistration{ID: "agent-browser"},
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:    core.AgentModePrimary,
			Model:   core.AgentModelConfig{Provider: "test", Name: "test"},
			Browser: &core.AgentBrowserSpec{Enabled: true},
		},
		SessionFactory: func(ctx context.Context, cfg browserSessionConfig) (*platformbrowser.Session, error) {
			return platformbrowser.NewSession(platformbrowser.SessionConfig{
				Backend:     backend,
				BackendName: cfg.backendName,
				AgentID:     cfg.agentID,
			})
		},
	})

	requireNoError(t, svc.Start(context.Background()))
	if !registry.HasCapability("browser") {
		t.Fatal("expected browser capability to be registered")
	}

	state := core.NewContext()
	state.Set("task.id", "task-1")
	result, err := registry.InvokeCapability(context.Background(), state, "browser", map[string]interface{}{
		"action": "open",
	})
	requireNoError(t, err)
	sessionID, _ := result.Data["session_id"].(string)
	if sessionID == "" {
		t.Fatal("expected session id")
	}
	if got := state.GetString(browserDefaultSessionKey); got != sessionID {
		t.Fatalf("expected default session %q, got %q", sessionID, got)
	}

	_, err = registry.InvokeCapability(context.Background(), state, "browser", map[string]interface{}{
		"action": "close",
	})
	requireNoError(t, err)
	if backend.closed != 1 {
		t.Fatalf("expected backend to close once, got %d", backend.closed)
	}
}

func TestBrowserService_StopClosesTrackedSessions(t *testing.T) {
	registry := capability.NewRegistry()
	workspace := t.TempDir()
	backend := &serviceStubBackend{text: "ready", title: "Ready"}
	svc := New(BrowserServiceConfig{
		WorkspaceRoot: workspace,
		FileScope:     sandbox.NewFileScopePolicy(workspace, nil),
		Registry:      registry,
		Registration:  &fauthorization.AgentRegistration{ID: "agent-browser"},
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:    core.AgentModePrimary,
			Model:   core.AgentModelConfig{Provider: "test", Name: "test"},
			Browser: &core.AgentBrowserSpec{Enabled: true},
		},
		SessionFactory: func(ctx context.Context, cfg browserSessionConfig) (*platformbrowser.Session, error) {
			return platformbrowser.NewSession(platformbrowser.SessionConfig{
				Backend:     backend,
				BackendName: cfg.backendName,
				AgentID:     cfg.agentID,
			})
		},
	})

	requireNoError(t, svc.Start(context.Background()))
	state := core.NewContext()
	state.Set("task.id", "task-1")
	_, err := registry.InvokeCapability(context.Background(), state, "browser", map[string]interface{}{
		"action": "open",
	})
	requireNoError(t, err)

	requireNoError(t, svc.Stop())
	if backend.closed == 0 {
		t.Fatal("expected backend to be closed during stop")
	}
}

func TestBrowserService_StartCreatesScopedPathRoots(t *testing.T) {
	registry := capability.NewRegistry()
	workspace := t.TempDir()
	svc := New(BrowserServiceConfig{
		WorkspaceRoot: workspace,
		FileScope:     sandbox.NewFileScopePolicy(workspace, nil),
		Registry:      registry,
		Registration:  &fauthorization.AgentRegistration{ID: "agent-browser"},
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:    core.AgentModePrimary,
			Model:   core.AgentModelConfig{Provider: "test", Name: "test"},
			Browser: &core.AgentBrowserSpec{Enabled: true},
		},
	})

	requireNoError(t, svc.Start(context.Background()))
	for label, path := range svc.paths.roots() {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s to exist at %s: %v", label, path, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", label)
		}
	}
}

func TestBrowserService_StartRejectsOutOfScopeRoots(t *testing.T) {
	registry := capability.NewRegistry()
	workspace := t.TempDir()
	scopeRoot := filepath.Join(t.TempDir(), "other-workspace")
	svc := New(BrowserServiceConfig{
		WorkspaceRoot: workspace,
		FileScope:     sandbox.NewFileScopePolicy(scopeRoot, nil),
		Registry:      registry,
		Registration:  &fauthorization.AgentRegistration{ID: "agent-browser"},
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:    core.AgentModePrimary,
			Model:   core.AgentModelConfig{Provider: "test", Name: "test"},
			Browser: &core.AgentBrowserSpec{Enabled: true},
		},
	})

	if err := svc.Start(context.Background()); err == nil {
		t.Fatal("expected start to fail for out-of-scope browser roots")
	}
}

func TestBrowserService_NavigateUsesNetworkPermission(t *testing.T) {
	registry := capability.NewRegistry()
	workspace := t.TempDir()
	backend := &serviceStubBackend{text: "ready", title: "Ready"}
	pm := newTestPermissionManager(t, &core.PermissionSet{
		Network: []core.NetworkPermission{
			{Direction: "egress", Protocol: "https", Host: "example.com", Port: 443},
		},
	}, &approvingHITL{})
	svc := New(BrowserServiceConfig{
		WorkspaceRoot:     workspace,
		FileScope:         sandbox.NewFileScopePolicy(workspace, nil),
		Registry:          registry,
		PermissionManager: pm,
		Registration:      &fauthorization.AgentRegistration{ID: "agent-browser"},
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:    core.AgentModePrimary,
			Model:   core.AgentModelConfig{Provider: "test", Name: "test"},
			Browser: &core.AgentBrowserSpec{Enabled: true},
		},
		SessionFactory: func(ctx context.Context, cfg browserSessionConfig) (*platformbrowser.Session, error) {
			return platformbrowser.NewSession(platformbrowser.SessionConfig{
				Backend:     backend,
				BackendName: cfg.backendName,
				AgentID:     cfg.agentID,
			})
		},
	})

	requireNoError(t, svc.Start(context.Background()))
	state := core.NewContext()
	state.Set("task.id", "task-1")
	_, err := registry.InvokeCapability(context.Background(), state, "browser", map[string]interface{}{
		"action": "open",
	})
	requireNoError(t, err)

	_, err = registry.InvokeCapability(context.Background(), state, "browser", map[string]interface{}{
		"action": "navigate",
		"url":    "https://example.com/docs",
	})
	requireNoError(t, err)
	if backend.currentURL != "https://example.com/docs" {
		t.Fatalf("expected navigation to update url, got %q", backend.currentURL)
	}
}

func TestBrowserService_ExecuteJSUsesHighRiskApproval(t *testing.T) {
	registry := capability.NewRegistry()
	workspace := t.TempDir()
	backend := &serviceStubBackend{text: "ready", title: "Ready"}
	hitl := &approvingHITL{}
	pm := newTestPermissionManager(t, &core.PermissionSet{
		Network: []core.NetworkPermission{
			{Direction: "egress", Protocol: "https", Host: "example.com", Port: 443},
		},
	}, hitl)
	svc := New(BrowserServiceConfig{
		WorkspaceRoot:     workspace,
		FileScope:         sandbox.NewFileScopePolicy(workspace, nil),
		Registry:          registry,
		PermissionManager: pm,
		Registration:      &fauthorization.AgentRegistration{ID: "agent-browser"},
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:    core.AgentModePrimary,
			Model:   core.AgentModelConfig{Provider: "test", Name: "test"},
			Browser: &core.AgentBrowserSpec{Enabled: true},
		},
		SessionFactory: func(ctx context.Context, cfg browserSessionConfig) (*platformbrowser.Session, error) {
			return platformbrowser.NewSession(platformbrowser.SessionConfig{
				Backend:     backend,
				BackendName: cfg.backendName,
			})
		},
	})

	requireNoError(t, svc.Start(context.Background()))
	state := core.NewContext()
	state.Set("task.id", "task-1")
	_, err := registry.InvokeCapability(context.Background(), state, "browser", map[string]interface{}{
		"action": "open",
	})
	requireNoError(t, err)

	_, err = registry.InvokeCapability(context.Background(), state, "browser", map[string]interface{}{
		"action": "execute_js",
		"script": "document.title || ''",
	})
	requireNoError(t, err)
	if len(hitl.requests) != 1 {
		t.Fatalf("expected one approval request, got %d", len(hitl.requests))
	}
	if got := hitl.requests[0].Permission.Action; got != "browser:execute_js" {
		t.Fatalf("expected execute_js approval action, got %q", got)
	}
	if got := hitl.requests[0].Risk; got != fauthorization.RiskLevelHigh {
		t.Fatalf("expected high risk approval, got %q", got)
	}
}

func TestBrowserService_RecoveryIsVisibleInSnapshot(t *testing.T) {
	registry := capability.NewRegistry()
	workspace := t.TempDir()
	backend1 := &failingOnceBackend{
		delegate: &serviceStubBackend{text: "first", title: "First"},
		failOnce: true,
	}
	backend2 := &serviceStubBackend{text: "second", title: "Second"}
	created := 0
	pm := newTestPermissionManager(t, &core.PermissionSet{
		Network: []core.NetworkPermission{
			{Direction: "egress", Protocol: "https", Host: "example.com", Port: 443},
		},
	}, &approvingHITL{})
	svc := New(BrowserServiceConfig{
		WorkspaceRoot:     workspace,
		FileScope:         sandbox.NewFileScopePolicy(workspace, nil),
		Registry:          registry,
		PermissionManager: pm,
		Registration:      &fauthorization.AgentRegistration{ID: "agent-browser"},
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:    core.AgentModePrimary,
			Model:   core.AgentModelConfig{Provider: "test", Name: "test"},
			Browser: &core.AgentBrowserSpec{Enabled: true},
		},
		SessionFactory: func(ctx context.Context, cfg browserSessionConfig) (*platformbrowser.Session, error) {
			created++
			if created == 1 {
				return platformbrowser.NewSession(platformbrowser.SessionConfig{
					Backend:     backend1,
					BackendName: cfg.backendName,
					AgentID:     cfg.agentID,
				})
			}
			return platformbrowser.NewSession(platformbrowser.SessionConfig{
				Backend:     backend2,
				BackendName: cfg.backendName,
				AgentID:     cfg.agentID,
			})
		},
	})

	requireNoError(t, svc.Start(context.Background()))
	state := core.NewContext()
	state.Set("task.id", "task-1")
	_, err := registry.InvokeCapability(context.Background(), state, "browser", map[string]interface{}{
		"action": "open",
	})
	requireNoError(t, err)

	navigateResult, err := registry.InvokeCapability(context.Background(), state, "browser", map[string]interface{}{
		"action": "navigate",
		"url":    "https://example.com/recover",
	})
	requireNoError(t, err)
	require.Equal(t, "https://example.com/recover", navigateResult.Data["page_state"].(*platformbrowser.PageState).URL)
	require.Equal(t, 1, backend1.closed)

	snapshot, err := svc.Snapshot(context.Background())
	requireNoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, 1, snapshot.ActiveSessions)
	require.Equal(t, 1, snapshot.BackendDistribution["cdp"])
	require.Len(t, snapshot.Sessions, 1)
	require.Equal(t, 1, snapshot.Sessions[0].Recoveries)
	require.Empty(t, snapshot.Sessions[0].LastError)
	require.Equal(t, "https://example.com/recover", snapshot.Sessions[0].LastPage.URL)
}

func TestBrowserService_ServiceSnapshotIncludesPathRoots(t *testing.T) {
	registry := capability.NewRegistry()
	workspace := t.TempDir()
	svc := New(BrowserServiceConfig{
		WorkspaceRoot: workspace,
		FileScope:     sandbox.NewFileScopePolicy(workspace, nil),
		Registry:      registry,
		Registration:  &fauthorization.AgentRegistration{ID: "agent-browser"},
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:    core.AgentModePrimary,
			Model:   core.AgentModelConfig{Provider: "test", Name: "test"},
			Browser: &core.AgentBrowserSpec{Enabled: true},
		},
	})

	requireNoError(t, svc.Start(context.Background()))
	snapshot, err := svc.Snapshot(context.Background())
	requireNoError(t, err)
	require.NotNil(t, snapshot)
	require.Contains(t, snapshot.PathRoots, "launch_root")
	require.Contains(t, snapshot.PathRoots, "service_root")
	require.Contains(t, snapshot.PathRoots, "metadata_root")
}

func newTestPermissionManager(t *testing.T, perms *core.PermissionSet, hitl fauthorization.HITLProvider) *fauthorization.PermissionManager {
	t.Helper()
	pm, err := fauthorization.NewPermissionManager(t.TempDir(), perms, nil, hitl)
	requireNoError(t, err)
	pm.SetDefaultPolicy(core.AgentPermissionAllow)
	return pm
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
