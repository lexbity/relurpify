package ayenitd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	browsersvc "codeburg.org/lexbit/relurpify/ayenitd/service/browser"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	platformbrowser "codeburg.org/lexbit/relurpify/platform/browser"
	"codeburg.org/lexbit/relurpify/platform/browser/bidi"
	"codeburg.org/lexbit/relurpify/platform/browser/cdp"
	"codeburg.org/lexbit/relurpify/platform/browser/webdriver"
	"codeburg.org/lexbit/relurpify/telemetry"
)

const defaultBrowserTimeout = 15 * time.Second
const (
	sandboxCDPPort       = 9222
	sandboxChromeDrvPort = 9515
)

type sandboxedBrowserBackend struct {
	remoteURL       string
	cdpWebSocketURL string
	containerID     string
	runtimeBinary   string
	launchDir       string
	cfg             browsersvc.BrowserSessionConfig
}

func newBrowserSession(ctx context.Context, cfg browsersvc.BrowserSessionConfig) (*platformbrowser.Session, error) {
	sandboxed, err := newSandboxedBrowserBackend(ctx, cfg)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(cfg.BackendName)) {
	case "", "cdp":
		backend, err := cdp.New(ctx, cdp.Config{
			Headless:     true,
			WebSocketURL: sandboxed.cdpWebSocketURL,
			Policy:       browserLaunchPolicy(cfg),
		})
		if err != nil {
			_ = sandboxed.close()
			return nil, err
		}
		maxTokens := cfg.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		return platformbrowser.NewSession(platformbrowser.SessionConfig{
			Backend:           wrapManagedBrowserBackend(backend, sandboxed.close),
			BackendName:       "cdp",
			PermissionManager: cfg.Manager,
			AgentID:           cfg.AgentID,
			Budget:            newBudgetManager(maxTokens),
		})
	case "webdriver":
		backend, err := webdriver.New(ctx, webdriver.Config{
			Headless:    true,
			RemoteURL:   sandboxed.remoteURL,
			BrowserArgs: []string{"--disable-dev-shm-usage"},
			Policy:      browserLaunchPolicy(cfg),
		})
		if err != nil {
			_ = sandboxed.close()
			return nil, err
		}
		return platformbrowser.NewSession(platformbrowser.SessionConfig{
			Backend:           wrapManagedBrowserBackend(backend, sandboxed.close),
			BackendName:       "webdriver",
			PermissionManager: cfg.Manager,
			AgentID:           cfg.AgentID,
			Budget:            newBudgetManager(8192),
		})
	case "bidi":
		backend, err := bidi.New(ctx, bidi.Config{
			Headless:    true,
			RemoteURL:   sandboxed.remoteURL,
			BrowserArgs: []string{"--disable-dev-shm-usage"},
			Policy:      browserLaunchPolicy(cfg),
		})
		if err != nil {
			_ = sandboxed.close()
			return nil, err
		}
		return platformbrowser.NewSession(platformbrowser.SessionConfig{
			Backend:           wrapManagedBrowserBackend(backend, sandboxed.close),
			BackendName:       "bidi",
			PermissionManager: cfg.Manager,
			AgentID:           cfg.AgentID,
			Budget:            newBudgetManager(8192),
		})
	default:
		return nil, &platformbrowser.Error{
			Code:      platformbrowser.ErrUnsupportedOperation,
			Backend:   strings.ToLower(strings.TrimSpace(cfg.BackendName)),
			Operation: "open",
			Err:       fmt.Errorf("unsupported browser backend"),
		}
	}
}

func newSandboxedBrowserBackend(ctx context.Context, cfg browsersvc.BrowserSessionConfig) (*sandboxedBrowserBackend, error) {
	if cfg.Registration == nil || cfg.Registration.Runtime == nil || cfg.Registration.Manifest == nil {
		return nil, fmt.Errorf("sandboxed browser runtime unavailable")
	}
	backendName := strings.ToLower(strings.TrimSpace(cfg.BackendName))
	if backendName == "" {
		backendName = "cdp"
	}
	switch backendName {
	case "cdp":
		hostPort, err := reservePort()
		if err != nil {
			return nil, err
		}
		launchDirRoot := cfg.LaunchRoot
		launchDir, err := os.MkdirTemp(launchDirRoot, "cdp-")
		if err != nil {
			return nil, fmt.Errorf("create browser launch dir: %w", err)
		}
		containerID, err := runSandboxBrowserContainer(ctx, cfg, hostPort, sandboxCDPPort, []string{
			"chromium",
			"--headless=new",
			"--disable-gpu",
			"--disable-dev-shm-usage",
			"--remote-debugging-address=0.0.0.0",
			"--remote-debugging-port=" + strconv.Itoa(sandboxCDPPort),
			"--user-data-dir=" + launchDir,
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-background-networking",
			"--disable-extensions",
			"--disable-sync",
			"--mute-audio",
			"about:blank",
		})
		if err != nil {
			_ = os.RemoveAll(launchDir)
			return nil, err
		}
		wsURL, err := waitForCDPWebSocket(ctx, hostPort)
		if err != nil {
			_ = os.RemoveAll(launchDir)
			_ = removeSandboxBrowserContainer(context.Background(), cfg, containerID)
			return nil, err
		}
		return &sandboxedBrowserBackend{
			cdpWebSocketURL: wsURL,
			containerID:     containerID,
			runtimeBinary:   browserContainerRuntime(cfg),
			launchDir:       launchDir,
			cfg:             cfg,
		}, nil
	case "webdriver", "bidi":
		hostPort, err := reservePort()
		if err != nil {
			return nil, err
		}
		containerID, err := runSandboxBrowserContainer(ctx, cfg, hostPort, sandboxChromeDrvPort, []string{
			"chromedriver",
			"--port=" + strconv.Itoa(sandboxChromeDrvPort),
			"--allowed-ips=",
			"--allowed-origins=*",
		})
		if err != nil {
			return nil, err
		}
		remoteURL := fmt.Sprintf("http://127.0.0.1:%d", hostPort)
		if err := waitForHTTPReady(ctx, remoteURL+"/status"); err != nil {
			_ = removeSandboxBrowserContainer(context.Background(), cfg, containerID)
			return nil, err
		}
		return &sandboxedBrowserBackend{
			remoteURL:     remoteURL,
			containerID:   containerID,
			runtimeBinary: browserContainerRuntime(cfg),
			cfg:           cfg,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported browser backend %q", backendName)
	}
}

func (b *sandboxedBrowserBackend) close() error {
	if b == nil || b.containerID == "" {
		if b != nil && b.launchDir != "" {
			_ = os.RemoveAll(b.launchDir)
			b.launchDir = ""
		}
		return nil
	}
	cmdCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := allowBrowserCommand(cmdCtx, b.cfg, b.runtimeBinary, []string{"rm", "-f", b.containerID}); err != nil {
		return err
	}
	cmd := exec.CommandContext(cmdCtx, b.runtimeBinary, "rm", "-f", b.containerID)
	err := cmd.Run()
	b.containerID = ""
	if b.launchDir != "" {
		_ = os.RemoveAll(b.launchDir)
		b.launchDir = ""
	}
	return err
}

func browserContainerRuntime(cfg browsersvc.BrowserSessionConfig) string {
	if cfg.Registration != nil && cfg.Registration.Runtime != nil {
		runtimeBinary := cfg.Registration.Runtime.RunConfig().ContainerRuntime
		if runtimeBinary != "" {
			return runtimeBinary
		}
	}
	return "docker"
}

func runSandboxBrowserContainer(ctx context.Context, cfg browsersvc.BrowserSessionConfig, hostPort, containerPort int, command []string) (string, error) {
	rtCfg := cfg.Registration.Runtime.RunConfig()
	runtimeBinary := browserContainerRuntime(cfg)
	runtimeName := strings.TrimSpace(rtCfg.RunscPath)
	if runtimeName == "" {
		runtimeName = "runsc"
	}
	runtimeName = filepath.Base(runtimeName)

	args := []string{
		"run",
		"-d",
		"--rm",
		"--runtime", runtimeName,
		"--add-host", "host.docker.internal:host-gateway",
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", hostPort, containerPort),
		"--tmpfs", "/tmp:exec,mode=1777",
		"--tmpfs", "/var/tmp:exec,mode=1777",
	}
	if user := cfg.Registration.Manifest.Spec.Security.RunAsUser; user > 0 {
		args = append(args, "-u", strconv.Itoa(user))
	}
	if cfg.Registration.Manifest.Spec.Security.ReadOnlyRoot {
		args = append(args, "--read-only")
	}
	if cfg.Registration.Manifest.Spec.Security.NoNewPrivileges {
		args = append(args, "--security-opt", "no-new-privileges")
	}
	if rtCfg.SeccompProfile != "" {
		args = append(args, "--security-opt", "seccomp="+rtCfg.SeccompProfile)
	}
	if rtCfg.NetworkIsolation && len(cfg.Registration.Manifest.Spec.Permissions.Network) == 0 {
		args = append(args, "--network", "none")
	}
	image := strings.TrimSpace(cfg.Registration.Manifest.Spec.Image)
	if image == "" {
		image = "ghcr.io/relurpify/runtime:latest"
	}
	args = append(args, image)
	args = append(args, command...)

	if err := allowBrowserCommand(ctx, cfg, runtimeBinary, args); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, runtimeBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("launch sandbox browser: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func removeSandboxBrowserContainer(ctx context.Context, cfg browsersvc.BrowserSessionConfig, containerID string) error {
	if strings.TrimSpace(containerID) == "" {
		return nil
	}
	if err := allowBrowserCommand(ctx, cfg, browserContainerRuntime(cfg), []string{"rm", "-f", containerID}); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, browserContainerRuntime(cfg), "rm", "-f", containerID)
	return cmd.Run()
}

func allowBrowserCommand(ctx context.Context, cfg browsersvc.BrowserSessionConfig, binary string, args []string) error {
	policy := browserLaunchPolicy(cfg)
	if policy == nil {
		return nil
	}
	return policy.AllowCommand(ctx, ports.CommandRequest{
		Args: append([]string{binary}, args...),
	})
}

func browserLaunchPolicy(cfg browsersvc.BrowserSessionConfig) sandbox.CommandPolicy {
	return cfg.Policy
}

type budgetManagerAdapter struct {
	budget *telemetry.ArtifactBudget
}

func newBudgetManager(maxTokens int) telemetry.BudgetManager {
	return budgetManagerAdapter{budget: telemetry.NewArtifactBudget(maxTokens)}
}

func (b budgetManagerAdapter) Allocate(category string, tokens int, item telemetry.BudgetItem) error {
	if b.budget == nil {
		return fmt.Errorf("budget unavailable")
	}
	var adapted telemetry.BudgetItem
	if item != nil {
		adapted = budgetItemAdapter{item: item}
	}
	return b.budget.Allocate(category, tokens, adapted)
}

func (b budgetManagerAdapter) Free(category string, tokens int, itemID string) {
	if b.budget == nil {
		return
	}
	b.budget.Free(category, tokens, itemID)
}

func (b budgetManagerAdapter) GetRemainingBudget(category string) int {
	if b.budget == nil {
		return 0
	}
	return b.budget.GetRemainingBudget(category)
}

func (b budgetManagerAdapter) ShouldCompress() bool {
	if b.budget == nil {
		return false
	}
	return b.budget.ShouldCompress()
}

func (b budgetManagerAdapter) CanAddTokens(tokens int) bool {
	if b.budget == nil {
		return false
	}
	return b.budget.CanAddTokens(tokens)
}

type budgetItemAdapter struct {
	item telemetry.BudgetItem
}

func (b budgetItemAdapter) GetID() string {
	if b.item == nil {
		return ""
	}
	return b.item.GetID()
}

func (b budgetItemAdapter) GetTokenCount() int {
	if b.item == nil {
		return 0
	}
	return b.item.GetTokenCount()
}

func (b budgetItemAdapter) GetPriority() int {
	if b.item == nil {
		return 0
	}
	return b.item.GetPriority()
}

func (b budgetItemAdapter) CanCompress() bool {
	if b.item == nil {
		return false
	}
	return b.item.CanCompress()
}

func (b budgetItemAdapter) Compress() (telemetry.BudgetItem, error) {
	if b.item == nil {
		return nil, nil
	}
	next, err := b.item.Compress()
	if err != nil || next == nil {
		return nil, err
	}
	return budgetItemAdapter{item: next}, nil
}

func (b budgetItemAdapter) CanEvict() bool {
	if b.item == nil {
		return false
	}
	return b.item.CanEvict()
}

func waitForCDPWebSocket(ctx context.Context, hostPort int) (string, error) {
	baseURL := fmt.Sprintf("http://127.0.0.1:%d/json/list", hostPort)
	waitCtx, cancel := context.WithTimeout(ctx, defaultBrowserTimeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		wsURL, err := fetchCDPWebSocket(waitCtx, baseURL)
		if err == nil && wsURL != "" {
			return wsURL, nil
		}
		select {
		case <-waitCtx.Done():
			return "", waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func waitForHTTPReady(ctx context.Context, target string) error {
	waitCtx, cancel := context.WithTimeout(ctx, defaultBrowserTimeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(waitCtx, http.MethodGet, target, nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode < 500 {
					return nil
				}
			}
		}
		select {
		case <-waitCtx.Done():
			return waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func fetchCDPWebSocket(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var targets []struct {
		Type                 string `json:"type"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return "", err
	}
	for _, target := range targets {
		if target.Type == "page" && target.WebSocketDebuggerURL != "" {
			return target.WebSocketDebuggerURL, nil
		}
	}
	return "", errors.New("no page target available")
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

type managedBrowserBackend struct {
	backend platformbrowser.Backend
	cleanup func() error
}

func wrapManagedBrowserBackend(backend platformbrowser.Backend, cleanup func() error) platformbrowser.Backend {
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

func (m *managedBrowserBackend) WaitFor(ctx context.Context, condition platformbrowser.WaitCondition, timeout time.Duration) error {
	return m.backend.WaitFor(ctx, condition, timeout)
}

func (m *managedBrowserBackend) CurrentURL(ctx context.Context) (string, error) {
	return m.backend.CurrentURL(ctx)
}

func (m *managedBrowserBackend) Capabilities() platformbrowser.Capabilities {
	if reporter, ok := m.backend.(platformbrowser.CapabilityReporter); ok {
		return reporter.Capabilities()
	}
	return platformbrowser.Capabilities{ArbitraryEval: true}
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
