package browser

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

	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	platformbrowser "codeburg.org/lexbit/relurpify/platform/browser"
	"codeburg.org/lexbit/relurpify/platform/browser/bidi"
	"codeburg.org/lexbit/relurpify/platform/browser/cdp"
	"codeburg.org/lexbit/relurpify/platform/browser/webdriver"
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
	cfg             browserSessionConfig
}

func newBrowserSession(ctx context.Context, cfg browserSessionConfig) (*platformbrowser.Session, error) {
	sandboxed, err := newSandboxedBrowserBackend(ctx, cfg)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(cfg.backendName)) {
	case "", defaultBrowserBackend:
		backend, err := cdp.New(ctx, cdp.Config{
			Headless:     true,
			WebSocketURL: sandboxed.cdpWebSocketURL,
			Policy:       browserLaunchPolicy(cfg),
		})
		if err != nil {
			_ = sandboxed.close()
			return nil, err
		}
		maxTokens := cfg.maxTokens
		if maxTokens <= 0 {
			maxTokens = 8192
		}
		return platformbrowser.NewSession(platformbrowser.SessionConfig{
			Backend:           wrapManagedBrowserBackend(backend, sandboxed.close),
			BackendName:       defaultBrowserBackend,
			PermissionManager: cfg.manager,
			AgentID:           cfg.agentID,
			Budget:            core.NewArtifactBudget(maxTokens),
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
			PermissionManager: cfg.manager,
			AgentID:           cfg.agentID,
			Budget:            core.NewArtifactBudget(8192),
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
			PermissionManager: cfg.manager,
			AgentID:           cfg.agentID,
			Budget:            core.NewArtifactBudget(8192),
		})
	default:
		return nil, &platformbrowser.Error{
			Code:      platformbrowser.ErrUnsupportedOperation,
			Backend:   strings.ToLower(strings.TrimSpace(cfg.backendName)),
			Operation: "open",
			Err:       fmt.Errorf("unsupported browser backend"),
		}
	}
}

func newSandboxedBrowserBackend(ctx context.Context, cfg browserSessionConfig) (*sandboxedBrowserBackend, error) {
	if cfg.registration == nil || cfg.registration.Runtime == nil || cfg.registration.Manifest == nil {
		return nil, fmt.Errorf("sandboxed browser runtime unavailable")
	}
	backendName := strings.ToLower(strings.TrimSpace(cfg.backendName))
	if backendName == "" {
		backendName = defaultBrowserBackend
	}
	switch backendName {
	case defaultBrowserBackend:
		hostPort, err := reservePort()
		if err != nil {
			return nil, err
		}
		if cfg.service == nil {
			return nil, fmt.Errorf("browser service unavailable")
		}
		launchDirRoot := cfg.service.paths.launchRoot
		launchDir, err := os.MkdirTemp(launchDirRoot, defaultBrowserBackend+"-")
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

func browserContainerRuntime(cfg browserSessionConfig) string {
	if cfg.registration != nil && cfg.registration.Runtime != nil {
		runtimeBinary := cfg.registration.Runtime.RunConfig().ContainerRuntime
		if runtimeBinary != "" {
			return runtimeBinary
		}
	}
	return "docker"
}

func runSandboxBrowserContainer(ctx context.Context, cfg browserSessionConfig, hostPort, containerPort int, command []string) (string, error) {
	rtCfg := cfg.registration.Runtime.RunConfig()
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
	if user := cfg.registration.Manifest.Spec.Security.RunAsUser; user > 0 {
		args = append(args, "-u", strconv.Itoa(user))
	}
	if cfg.registration.Manifest.Spec.Security.ReadOnlyRoot {
		args = append(args, "--read-only")
	}
	if cfg.registration.Manifest.Spec.Security.NoNewPrivileges {
		args = append(args, "--security-opt", "no-new-privileges")
	}
	if rtCfg.SeccompProfile != "" {
		args = append(args, "--security-opt", "seccomp="+rtCfg.SeccompProfile)
	}
	if rtCfg.NetworkIsolation && len(cfg.registration.Manifest.Spec.Permissions.Network) == 0 {
		args = append(args, "--network", "none")
	}
	image := strings.TrimSpace(cfg.registration.Manifest.Spec.Image)
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

func removeSandboxBrowserContainer(ctx context.Context, cfg browserSessionConfig, containerID string) error {
	if strings.TrimSpace(containerID) == "" {
		return nil
	}
	if err := allowBrowserCommand(ctx, cfg, browserContainerRuntime(cfg), []string{"rm", "-f", containerID}); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, browserContainerRuntime(cfg), "rm", "-f", containerID)
	return cmd.Run()
}

func allowBrowserCommand(ctx context.Context, cfg browserSessionConfig, binary string, args []string) error {
	policy := browserCommandPolicyFromConfig(cfg)
	if policy == nil {
		return nil
	}
	return policy.AllowCommand(ctx, sandbox.CommandRequest{
		Args: append([]string{binary}, args...),
	})
}

func browserLaunchPolicy(cfg browserSessionConfig) sandbox.CommandPolicy {
	return browserCommandPolicyFromConfig(cfg)
}

func browserCommandPolicyFromConfig(cfg browserSessionConfig) sandbox.CommandPolicy {
	if cfg.service != nil && cfg.service.commandPolicy != nil {
		return cfg.service.commandPolicy
	}
	if cfg.registration == nil || cfg.registration.Permissions == nil {
		return nil
	}
	var spec *core.AgentRuntimeSpec
	if cfg.service != nil {
		spec = cfg.service.agentSpec
	}
	if spec == nil && cfg.registration != nil && cfg.registration.Manifest != nil {
		spec = cfg.registration.Manifest.Spec.Agent
	}
	return fauthorization.NewCommandAuthorizationPolicy(cfg.registration.Permissions, cfg.registration.ID, spec, "browser")
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
