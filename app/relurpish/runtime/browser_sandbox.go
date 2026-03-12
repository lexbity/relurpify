package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	sandboxCDPPort        = 9222
	sandboxChromeDrvPort  = 9515
	defaultBrowserTimeout = 15 * time.Second
)

type sandboxedBrowserBackend struct {
	remoteURL       string
	cdpWebSocketURL string
	containerID     string
	runtimeBinary   string
}

func newSandboxedBrowserBackend(ctx context.Context, cfg browserSessionConfig) (*sandboxedBrowserBackend, error) {
	if cfg.runtime == nil || cfg.runtime.Registration == nil || cfg.runtime.Registration.Runtime == nil || cfg.runtime.Registration.Manifest == nil {
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
		containerID, err := runSandboxBrowserContainer(ctx, cfg, hostPort, sandboxCDPPort, []string{
			"chromium",
			"--headless=new",
			"--disable-gpu",
			"--disable-dev-shm-usage",
			"--remote-debugging-address=0.0.0.0",
			"--remote-debugging-port=" + strconv.Itoa(sandboxCDPPort),
			"--user-data-dir=/tmp/relurpify-browser",
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-background-networking",
			"--disable-extensions",
			"--disable-sync",
			"--mute-audio",
			"about:blank",
		})
		if err != nil {
			return nil, err
		}
		wsURL, err := waitForCDPWebSocket(ctx, hostPort)
		if err != nil {
			_ = removeSandboxBrowserContainer(context.Background(), cfg, containerID)
			return nil, err
		}
		return &sandboxedBrowserBackend{
			cdpWebSocketURL: wsURL,
			containerID:     containerID,
			runtimeBinary:   browserContainerRuntime(cfg),
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
		}, nil
	default:
		return nil, fmt.Errorf("unsupported browser backend %q", backendName)
	}
}

func (b *sandboxedBrowserBackend) close() error {
	if b == nil || b.containerID == "" {
		return nil
	}
	cmdCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, b.runtimeBinary, "rm", "-f", b.containerID)
	err := cmd.Run()
	b.containerID = ""
	return err
}

func browserContainerRuntime(cfg browserSessionConfig) string {
	runtimeBinary := cfg.runtime.Registration.Runtime.RunConfig().ContainerRuntime
	if runtimeBinary == "" {
		runtimeBinary = "docker"
	}
	return runtimeBinary
}

func runSandboxBrowserContainer(ctx context.Context, cfg browserSessionConfig, hostPort, containerPort int, command []string) (string, error) {
	rtCfg := cfg.runtime.Registration.Runtime.RunConfig()
	runtimeBinary := browserContainerRuntime(cfg)
	runtimeName := strings.TrimSpace(rtCfg.RunscPath)
	if runtimeName == "" {
		runtimeName = "runsc"
	}
	runtimeName = pathBase(runtimeName)

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
	if user := cfg.runtime.Registration.Manifest.Spec.Security.RunAsUser; user > 0 {
		args = append(args, "-u", strconv.Itoa(user))
	}
	if cfg.runtime.Registration.Manifest.Spec.Security.ReadOnlyRoot {
		args = append(args, "--read-only")
	}
	if cfg.runtime.Registration.Manifest.Spec.Security.NoNewPrivileges {
		args = append(args, "--security-opt", "no-new-privileges")
	}
	if rtCfg.SeccompProfile != "" {
		args = append(args, "--security-opt", "seccomp="+rtCfg.SeccompProfile)
	}
	if rtCfg.NetworkIsolation && len(cfg.runtime.Registration.Runtime.Policy().NetworkRules) == 0 {
		args = append(args, "--network", "none")
	}
	image := strings.TrimSpace(cfg.runtime.Registration.Manifest.Spec.Image)
	if image == "" {
		image = "ghcr.io/relurpify/runtime:latest"
	}
	args = append(args, image)
	args = append(args, command...)

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
	cmd := exec.CommandContext(ctx, browserContainerRuntime(cfg), "rm", "-f", containerID)
	return cmd.Run()
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
	var payload []struct {
		Type                 string `json:"type"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	for _, target := range payload {
		if target.Type == "page" && target.WebSocketDebuggerURL != "" {
			return target.WebSocketDebuggerURL, nil
		}
	}
	return "", fmt.Errorf("cdp websocket not ready")
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func pathBase(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	return parts[len(parts)-1]
}
