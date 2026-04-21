package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/browser"
)

const defaultStartupTimeout = 10 * time.Second

type Config struct {
	ExecutablePath string
	WebSocketURL   string
	RemotePort     int
	Headless       bool
	StartupTimeout time.Duration
	ExtraArgs      []string
	Policy         sandbox.CommandPolicy
}

type Backend struct {
	transport Transport
	process   *exec.Cmd
	userData  string
	httpBase  string
}

func (b *Backend) Capabilities() browser.Capabilities {
	return browser.Capabilities{
		AccessibilityTree: true,
		NetworkIntercept:  true,
		DownloadEvents:    false,
		PopupTracking:     false,
		ArbitraryEval:     true,
	}
}

type listTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type evaluateResponse struct {
	Result struct {
		Type  string `json:"type"`
		Value any    `json:"value"`
	} `json:"result"`
	ExceptionDetails *struct {
		Text string `json:"text"`
	} `json:"exceptionDetails,omitempty"`
}

type currentURLResponse struct {
	Result struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"result"`
}

type outerHTMLResponse struct {
	OuterHTML string `json:"outerHTML"`
}

type documentResponse struct {
	Root struct {
		NodeID int64 `json:"nodeId"`
	} `json:"root"`
}

type screenshotResponse struct {
	Data string `json:"data"`
}

func New(ctx context.Context, cfg Config) (*Backend, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.StartupTimeout <= 0 {
		cfg.StartupTimeout = defaultStartupTimeout
	}
	backend := &Backend{}
	wsURL := strings.TrimSpace(cfg.WebSocketURL)
	if wsURL == "" {
		launched, err := launchChromium(ctx, cfg)
		if err != nil {
			return nil, err
		}
		backend.process = launched.process
		backend.userData = launched.userData
		backend.httpBase = launched.httpBase
		wsURL = launched.wsURL
	}
	transport, err := newWebsocketTransport(ctx, wsURL)
	if err != nil {
		_ = backend.Close()
		return nil, err
	}
	backend.transport = transport
	return backend, nil
}

func (b *Backend) Navigate(ctx context.Context, rawURL string) error {
	_, err := b.transport.Call(ctx, "Page.navigate", map[string]any{"url": rawURL})
	if err != nil {
		return mapCDPError("navigate", err)
	}
	return b.WaitFor(ctx, browser.WaitCondition{Type: browser.WaitForLoad}, 15*time.Second)
}

func (b *Backend) Click(ctx context.Context, selector string) error {
	_, err := b.evaluate(ctx, fmt.Sprintf(`(() => {
		const el = document.querySelector(%s);
		if (!el) throw new Error("no such element");
		el.click();
		return true;
	})()`, jsString(selector)))
	return mapCDPError("click", err)
}

func (b *Backend) Type(ctx context.Context, selector, text string) error {
	_, err := b.evaluate(ctx, fmt.Sprintf(`(() => {
		const el = document.querySelector(%s);
		if (!el) throw new Error("no such element");
		el.focus();
		if ("value" in el) {
			el.value = %s;
		} else {
			el.textContent = %s;
		}
		el.dispatchEvent(new Event("input", { bubbles: true }));
		el.dispatchEvent(new Event("change", { bubbles: true }));
		return true;
	})()`, jsString(selector), jsString(text), jsString(text)))
	return mapCDPError("type", err)
}

func (b *Backend) GetText(ctx context.Context, selector string) (string, error) {
	value, err := b.evaluate(ctx, fmt.Sprintf(`(() => {
		const el = document.querySelector(%s);
		if (!el) throw new Error("no such element");
		return el.innerText ?? el.textContent ?? "";
	})()`, jsString(selector)))
	if err != nil {
		return "", mapCDPError("get_text", err)
	}
	return fmt.Sprint(value), nil
}

func (b *Backend) GetAccessibilityTree(ctx context.Context) (string, error) {
	result, err := b.transport.Call(ctx, "Accessibility.getFullAXTree", nil)
	if err != nil {
		return "", mapCDPError("get_accessibility_tree", err)
	}
	return string(result), nil
}

func (b *Backend) GetHTML(ctx context.Context) (string, error) {
	result, err := b.transport.Call(ctx, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true})
	if err != nil {
		return "", mapCDPError("get_html", err)
	}
	var document documentResponse
	if err := json.Unmarshal(result, &document); err != nil {
		return "", err
	}
	result, err = b.transport.Call(ctx, "DOM.getOuterHTML", map[string]any{"nodeId": document.Root.NodeID})
	if err != nil {
		return "", mapCDPError("get_html", err)
	}
	var html outerHTMLResponse
	if err := json.Unmarshal(result, &html); err != nil {
		return "", err
	}
	return html.OuterHTML, nil
}

func (b *Backend) ExecuteScript(ctx context.Context, script string) (any, error) {
	value, err := b.evaluate(ctx, script)
	if err != nil {
		return nil, mapCDPError("execute_script", err)
	}
	return value, nil
}

func (b *Backend) Screenshot(ctx context.Context) ([]byte, error) {
	result, err := b.transport.Call(ctx, "Page.captureScreenshot", map[string]any{"format": "png"})
	if err != nil {
		return nil, mapCDPError("screenshot", err)
	}
	var payload screenshotResponse
	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(payload.Data)
}

func (b *Backend) WaitFor(ctx context.Context, condition browser.WaitCondition, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		ok, err := b.checkCondition(waitCtx, condition)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return &browser.Error{Code: browser.ErrTimeout, Backend: "cdp", Operation: "wait", Err: waitCtx.Err()}
		case <-ticker.C:
		}
	}
}

func (b *Backend) CurrentURL(ctx context.Context) (string, error) {
	value, err := b.evaluate(ctx, "window.location.href")
	if err != nil {
		return "", mapCDPError("current_url", err)
	}
	return fmt.Sprint(value), nil
}

func (b *Backend) Close() error {
	var errs []error
	if b.transport != nil {
		if err := b.transport.Close(); err != nil {
			errs = append(errs, err)
		}
		b.transport = nil
	}
	if b.process != nil && b.process.Process != nil {
		_ = b.process.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- b.process.Wait() }()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, os.ErrProcessDone) && !strings.Contains(err.Error(), "signal: terminated") {
				errs = append(errs, err)
			}
		case <-time.After(2 * time.Second):
			_ = b.process.Process.Kill()
			_, _ = b.process.Process.Wait()
		}
		b.process = nil
	}
	if b.userData != "" {
		if err := os.RemoveAll(b.userData); err != nil {
			errs = append(errs, err)
		}
		b.userData = ""
	}
	return errors.Join(errs...)
}

func (b *Backend) evaluate(ctx context.Context, expression string) (any, error) {
	result, err := b.transport.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err != nil {
		return nil, err
	}
	var payload evaluateResponse
	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, err
	}
	if payload.ExceptionDetails != nil {
		return nil, errors.New(payload.ExceptionDetails.Text)
	}
	return payload.Result.Value, nil
}

func (b *Backend) checkCondition(ctx context.Context, condition browser.WaitCondition) (bool, error) {
	switch condition.Type {
	case browser.WaitForLoad:
		value, err := b.evaluate(ctx, "document.readyState === 'complete'")
		if err != nil {
			return false, mapCDPError("wait", err)
		}
		ok, _ := value.(bool)
		return ok, nil
	case browser.WaitForSelector:
		value, err := b.evaluate(ctx, fmt.Sprintf("document.querySelector(%s) !== null", jsString(condition.Selector)))
		if err != nil {
			return false, mapCDPError("wait", err)
		}
		ok, _ := value.(bool)
		return ok, nil
	case browser.WaitForSelectorMissing:
		value, err := b.evaluate(ctx, fmt.Sprintf("document.querySelector(%s) === null", jsString(condition.Selector)))
		if err != nil {
			return false, mapCDPError("wait", err)
		}
		ok, _ := value.(bool)
		return ok, nil
	case browser.WaitForText:
		value, err := b.evaluate(ctx, fmt.Sprintf(`(() => {
			const el = document.querySelector(%s);
			return !!el && (el.innerText || el.textContent || "").includes(%s);
		})()`, jsString(condition.Selector), jsString(condition.Text)))
		if err != nil {
			return false, mapCDPError("wait", err)
		}
		ok, _ := value.(bool)
		return ok, nil
	case browser.WaitForURLContains:
		value, err := b.evaluate(ctx, fmt.Sprintf("window.location.href.includes(%s)", jsString(condition.URLContains)))
		if err != nil {
			return false, mapCDPError("wait", err)
		}
		ok, _ := value.(bool)
		return ok, nil
	default:
		return false, &browser.Error{Code: browser.ErrUnsupportedOperation, Backend: "cdp", Operation: "wait", Err: fmt.Errorf("unsupported wait type %q", condition.Type)}
	}
}

func mapCDPError(operation string, err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "no such element"):
		return &browser.Error{Code: browser.ErrNoSuchElement, Backend: "cdp", Operation: operation, Err: err}
	case strings.Contains(message, "not interactable"):
		return &browser.Error{Code: browser.ErrElementNotInteractable, Backend: "cdp", Operation: operation, Err: err}
	case strings.Contains(message, "context deadline exceeded"):
		return &browser.Error{Code: browser.ErrTimeout, Backend: "cdp", Operation: operation, Err: err}
	case strings.Contains(message, "cannot call") || strings.Contains(message, "exception"):
		return &browser.Error{Code: browser.ErrScriptEvaluation, Backend: "cdp", Operation: operation, Err: err}
	case strings.Contains(message, "closed"):
		return &browser.Error{Code: browser.ErrBackendDisconnected, Backend: "cdp", Operation: operation, Err: err}
	default:
		return &browser.Error{Code: browser.ErrUnknownOperation, Backend: "cdp", Operation: operation, Err: err}
	}
}

func jsString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

type launchedBrowser struct {
	process  *exec.Cmd
	userData string
	httpBase string
	wsURL    string
}

func launchChromium(ctx context.Context, cfg Config) (*launchedBrowser, error) {
	executable := strings.TrimSpace(cfg.ExecutablePath)
	if executable == "" {
		for _, candidate := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
			path, err := exec.LookPath(candidate)
			if err == nil {
				executable = path
				break
			}
		}
	}
	if executable == "" {
		return nil, fmt.Errorf("chromium executable not found")
	}
	port := cfg.RemotePort
	if port == 0 {
		var err error
		port, err = freePort()
		if err != nil {
			return nil, err
		}
	}
	userDataDir, err := os.MkdirTemp("", "relurpify-cdp-*")
	if err != nil {
		return nil, err
	}
	headless := cfg.Headless
	if !headless {
		headless = true
	}
	args := []string{
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=" + strconv.Itoa(port),
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-extensions",
		"--disable-sync",
		"--mute-audio",
		"about:blank",
	}
	if headless {
		args = append(args, "--headless=new", "--disable-gpu")
	}
	if os.Geteuid() == 0 {
		args = append(args, "--no-sandbox")
	}
	args = append(args, cfg.ExtraArgs...)

	if cfg.Policy != nil {
		if err := cfg.Policy.AllowCommand(ctx, sandbox.CommandRequest{
			Args: append([]string{executable}, args...),
		}); err != nil {
			_ = os.RemoveAll(userDataDir)
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(userDataDir)
		return nil, err
	}
	httpBase := fmt.Sprintf("http://127.0.0.1:%d", port)
	wsURL, err := waitForDebugger(ctx, httpBase, cfg.StartupTimeout)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(userDataDir)
		return nil, err
	}
	return &launchedBrowser{
		process:  cmd,
		userData: userDataDir,
		httpBase: httpBase,
		wsURL:    wsURL,
	}, nil
}

func waitForDebugger(ctx context.Context, httpBase string, timeout time.Duration) (string, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		wsURL, err := pageWebSocketURL(waitCtx, httpBase)
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

func pageWebSocketURL(ctx context.Context, httpBase string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpBase+"/json/list", nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var targets []listTarget
	if err := json.NewDecoder(response.Body).Decode(&targets); err != nil {
		return "", err
	}
	for _, target := range targets {
		if target.Type == "page" && target.WebSocketDebuggerURL != "" {
			return target.WebSocketDebuggerURL, nil
		}
	}
	return "", errors.New("no page target available")
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
