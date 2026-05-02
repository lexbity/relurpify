package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	platformbrowser "codeburg.org/lexbit/relurpify/platform/browser"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

const (
	browserDefaultSessionKey = "browser.default_session"
	browserLastPageStateKey  = "browser.last_page_state"
	browserPageStateListKey  = "browser.page_states"
	defaultBrowserScope      = "browser.default"
	defaultBrowserBackend    = "cdp"
	browserActionOpen        = "open"
	browserActionNavigate    = "navigate"
	browserActionClick       = "click"
	browserActionType        = "type"
	browserActionWait        = "wait"
	browserActionExtract     = "extract"
	browserActionGetText     = "get_text"
	browserActionGetHTML     = "get_html"
	browserActionGetAXTree   = "get_accessibility_tree"
	browserActionCurrentURL  = "current_url"
	browserActionScreenshot  = "screenshot"
	browserActionExecuteJS   = "execute_js"
	browserActionClose       = "close"
)

// BrowserService owns browser capability registration, session tracking, and
// workspace-level snapshotting.
type BrowserService struct {
	mu sync.Mutex

	registry          *capability.Registry
	permissionManager *fauthorization.PermissionManager
	fileScope         *sandbox.FileScopePolicy
	telemetry         core.Telemetry

	workspaceRoot   string
	registration    *fauthorization.AgentRegistration
	agentSpec       *agentspec.AgentRuntimeSpec
	commandPolicy   sandbox.CommandPolicy
	defaultBackend  string
	allowedBackends map[string]struct{}
	paths           browserPaths

	sessionFactory func(context.Context, browserSessionConfig) (*platformbrowser.Session, error)
	sessions       map[string]*browserSessionHandle
	started        bool
	stopped        bool
	startedAt      time.Time
}

// New creates a browser service from the supplied workspace manifest.
func New(cfg BrowserServiceConfig) *BrowserService {
	allowed := make(map[string]struct{}, len(cfg.AllowedBackends))
	for _, backend := range cfg.AllowedBackends {
		if trimmed := strings.TrimSpace(strings.ToLower(backend)); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	if strings.TrimSpace(cfg.DefaultBackend) == "" {
		cfg.DefaultBackend = defaultBrowserBackend
	}
	return &BrowserService{
		registry:          cfg.Registry,
		permissionManager: cfg.PermissionManager,
		fileScope:         cfg.FileScope,
		telemetry:         cfg.Telemetry,
		workspaceRoot:     strings.TrimSpace(cfg.WorkspaceRoot),
		registration:      cfg.Registration,
		agentSpec:         cfg.AgentSpec,
		commandPolicy:     cfg.CommandPolicy,
		defaultBackend:    strings.ToLower(strings.TrimSpace(cfg.DefaultBackend)),
		allowedBackends:   allowed,
		paths:             newBrowserPaths(cfg.WorkspaceRoot),
		sessionFactory:    cfg.SessionFactory,
		sessions:          make(map[string]*browserSessionHandle),
	}
}

// Start registers the browser capability and marks the service as active.
func (s *BrowserService) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registry == nil {
		return fmt.Errorf("browser registry unavailable")
	}
	if strings.TrimSpace(s.workspaceRoot) == "" {
		return fmt.Errorf("browser workspace root unavailable")
	}
	if err := s.ensureBrowserRoots(); err != nil {
		return err
	}
	if !shouldEnableBrowserService(s.agentSpec) {
		s.started = true
		s.stopped = false
		s.startedAt = time.Now().UTC()
		return nil
	}
	if !s.registry.HasCapability("browser") {
		if err := s.registry.RegisterInvocableCapability(&browserCapability{service: s}); err != nil {
			return err
		}
	}
	s.started = true
	s.stopped = false
	s.startedAt = time.Now().UTC()
	return nil
}

// Stop closes any tracked sessions.
func (s *BrowserService) Stop() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	handles := make([]*browserSessionHandle, 0, len(s.sessions))
	for _, handle := range s.sessions {
		handles = append(handles, handle)
	}
	s.sessions = make(map[string]*browserSessionHandle)
	s.stopped = true
	s.mu.Unlock()

	var errs []error
	for _, handle := range handles {
		if err := handle.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *BrowserService) sessionFactoryOrDefault() func(context.Context, browserSessionConfig) (*platformbrowser.Session, error) {
	if s != nil && s.sessionFactory != nil {
		return s.sessionFactory
	}
	return newBrowserSession
}

func (s *BrowserService) backendAllowed(backend string) bool {
	if s == nil {
		return false
	}
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		backend = s.defaultBackend
	}
	if len(s.allowedBackends) == 0 {
		return true
	}
	_, ok := s.allowedBackends[backend]
	return ok
}

func shouldEnableBrowserService(spec *agentspec.AgentRuntimeSpec) bool {
	return spec != nil && spec.Browser != nil && spec.Browser.Enabled
}

func (s *BrowserService) fileScopePolicy() *sandbox.FileScopePolicy {
	if s == nil {
		return nil
	}
	if s.fileScope != nil {
		return s.fileScope
	}
	if strings.TrimSpace(s.workspaceRoot) == "" {
		return nil
	}
	s.fileScope = sandbox.NewFileScopePolicy(s.workspaceRoot, nil)
	return s.fileScope
}

func (s *BrowserService) checkFileScope(action contracts.FileSystemAction, target string) error {
	scope := s.fileScopePolicy()
	if scope == nil {
		return nil
	}
	return scope.Check(action, target)
}

func (s *BrowserService) ensureBrowserRoots() error {
	for label, path := range s.paths.roots() {
		if err := s.ensureBrowserPathRoot(label, path); err != nil {
			return err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create browser %s: %w", label, err)
		}
	}
	return nil
}

func (s *BrowserService) ensureSessionPaths(sessionID string) (browserSessionPaths, error) {
	paths := s.paths.session(sessionID)
	for label, path := range paths.roots() {
		if label == "metadata_file" || label == "log_file" {
			if err := s.checkFileScope(contracts.FileSystemWrite, filepath.Dir(path)); err != nil {
				return browserSessionPaths{}, fmt.Errorf("browser %s out of scope: %w", label, err)
			}
			continue
		}
		if err := s.ensureBrowserPathRoot(label, path); err != nil {
			return browserSessionPaths{}, err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return browserSessionPaths{}, fmt.Errorf("create browser %s: %w", label, err)
		}
	}
	return paths, nil
}

func (s *BrowserService) persistSessionMetadata(handle *browserSessionHandle) {
	if s == nil || handle == nil {
		return
	}
	if strings.TrimSpace(handle.paths.metadataFile) == "" {
		return
	}
	if err := s.checkFileScope(contracts.FileSystemWrite, handle.paths.metadataFile); err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(handle.paths.metadataFile), 0o755); err != nil {
		return
	}
	payload := map[string]any{
		"session":    handle.snapshot(),
		"path_roots": handle.paths.roots(),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(handle.paths.metadataFile, data, 0o644)
}

func canonicalBrowserAction(action string) string {
	return strings.ToLower(strings.TrimSpace(action))
}

func browserPermissionAction(action string) string {
	action = canonicalBrowserAction(action)
	if action == "" {
		return "browser"
	}
	return "browser:" + action
}

func (s *BrowserService) actionPolicy(action string) agentspec.AgentPermissionLevel {
	action = canonicalBrowserAction(action)
	if s == nil || s.agentSpec == nil || s.agentSpec.Browser == nil {
		if action == browserActionExecuteJS {
			return agentspec.AgentPermissionAsk
		}
		return agentspec.AgentPermissionAllow
	}
	if level, ok := s.agentSpec.Browser.Actions[action]; ok && level != "" {
		return level
	}
	if action == browserActionExecuteJS {
		return agentspec.AgentPermissionAsk
	}
	return agentspec.AgentPermissionAllow
}
