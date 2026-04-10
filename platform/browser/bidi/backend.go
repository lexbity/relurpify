package bidi

import (
	"bytes"
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

	"github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/platform/browser"
)

const defaultStartupTimeout = 10 * time.Second

type Config struct {
	DriverPath     string
	BrowserBinary  string
	RemoteURL      string
	RemotePort     int
	Headless       bool
	StartupTimeout time.Duration
	DriverArgs     []string
	BrowserArgs    []string
	Policy         sandbox.CommandPolicy
}

type Backend struct {
	client       *http.Client
	baseURL      string
	sessionID    string
	contextID    string
	webSocketURL string
	transport    Transport
	loadEvents   <-chan json.RawMessage
	process      *exec.Cmd
	userData     string
}

func (b *Backend) Capabilities() browser.Capabilities {
	return browser.Capabilities{
		AccessibilityTree: false,
		NetworkIntercept:  false,
		DownloadEvents:    false,
		PopupTracking:     false,
		ArbitraryEval:     true,
	}
}

type wireResponse struct {
	Value json.RawMessage `json:"value"`
}

type wireError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type newSessionResponse struct {
	SessionID    string `json:"sessionId"`
	Capabilities struct {
		WebSocketURL string `json:"webSocketUrl"`
	} `json:"capabilities"`
}

type treeResult struct {
	Contexts []struct {
		Context string `json:"context"`
	} `json:"contexts"`
}

type scriptResultEnvelope struct {
	Result struct {
		Type  string `json:"type"`
		Value any    `json:"value"`
	} `json:"result"`
	ExceptionDetails any `json:"exceptionDetails,omitempty"`
}

type screenshotResult struct {
	Data string `json:"data"`
}

func New(ctx context.Context, cfg Config) (*Backend, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.StartupTimeout <= 0 {
		cfg.StartupTimeout = defaultStartupTimeout
	}
	backend := &Backend{
		client: &http.Client{Timeout: 30 * time.Second},
	}
	if strings.TrimSpace(cfg.RemoteURL) == "" {
		launched, err := launchChromeDriver(ctx, cfg)
		if err != nil {
			return nil, err
		}
		backend.baseURL = launched.baseURL
		backend.process = launched.process
	} else {
		backend.baseURL = strings.TrimRight(strings.TrimSpace(cfg.RemoteURL), "/")
	}
	if err := backend.startSession(ctx, cfg); err != nil {
		_ = backend.Close()
		return nil, err
	}
	transport, err := newWebsocketTransport(ctx, backend.webSocketURL)
	if err != nil {
		_ = backend.Close()
		return nil, err
	}
	backend.transport = transport
	backend.loadEvents = transport.Subscribe("browsingContext.load")
	if _, err := transport.Call(ctx, "session.subscribe", map[string]any{
		"events": []string{"browsingContext.load"},
	}); err != nil {
		_ = backend.Close()
		return nil, err
	}
	if err := backend.resolveContext(ctx); err != nil {
		_ = backend.Close()
		return nil, err
	}
	return backend, nil
}

func (b *Backend) Navigate(ctx context.Context, rawURL string) error {
	_, err := b.transport.Call(ctx, "browsingContext.navigate", map[string]any{
		"context": b.contextID,
		"url":     rawURL,
		"wait":    "complete",
	})
	if err != nil {
		return mapBiDiError("navigate", err)
	}
	return nil
}

func (b *Backend) Click(ctx context.Context, selector string) error {
	_, err := b.evaluate(ctx, fmt.Sprintf(`(() => {
		const el = document.querySelector(%s);
		if (!el) throw new Error("no such element");
		el.click();
		return true;
	})()`, jsString(selector)))
	return mapBiDiError("click", err)
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
	return mapBiDiError("type", err)
}

func (b *Backend) GetText(ctx context.Context, selector string) (string, error) {
	value, err := b.evaluate(ctx, fmt.Sprintf(`(() => {
		const el = document.querySelector(%s);
		if (!el) throw new Error("no such element");
		return el.innerText ?? el.textContent ?? "";
	})()`, jsString(selector)))
	if err != nil {
		return "", mapBiDiError("get_text", err)
	}
	return fmt.Sprint(value), nil
}

func (b *Backend) GetAccessibilityTree(ctx context.Context) (string, error) {
	value, err := b.evaluate(ctx, `(() => JSON.stringify({
		role: "document",
		name: document.title || "",
		url: window.location.href
	}))()`)
	if err != nil {
		return "", mapBiDiError("get_accessibility_tree", err)
	}
	return fmt.Sprint(value), nil
}

func (b *Backend) GetHTML(ctx context.Context) (string, error) {
	value, err := b.evaluate(ctx, "document.documentElement.outerHTML")
	if err != nil {
		return "", mapBiDiError("get_html", err)
	}
	return fmt.Sprint(value), nil
}

func (b *Backend) ExecuteScript(ctx context.Context, script string) (any, error) {
	value, err := b.evaluate(ctx, script)
	if err != nil {
		return nil, mapBiDiError("execute_script", err)
	}
	return value, nil
}

func (b *Backend) Screenshot(ctx context.Context) ([]byte, error) {
	result, err := b.transport.Call(ctx, "browsingContext.captureScreenshot", map[string]any{
		"context": b.contextID,
	})
	if err != nil {
		return nil, mapBiDiError("screenshot", err)
	}
	var payload screenshotResult
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
	if condition.Type == browser.WaitForLoad {
		return b.waitForLoad(waitCtx)
	}
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
			return &browser.Error{Code: browser.ErrTimeout, Backend: "bidi", Operation: "wait", Err: waitCtx.Err()}
		case <-ticker.C:
		}
	}
}

func (b *Backend) CurrentURL(ctx context.Context) (string, error) {
	value, err := b.evaluate(ctx, "window.location.href")
	if err != nil {
		return "", mapBiDiError("current_url", err)
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
	if b.sessionID != "" {
		_, err := b.do(context.Background(), http.MethodDelete, "/session/"+b.sessionID, nil)
		if err != nil && !isInvalidSession(err) {
			errs = append(errs, err)
		}
		b.sessionID = ""
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

func (b *Backend) startSession(ctx context.Context, cfg Config) error {
	capabilities := map[string]any{
		"browserName":  "chrome",
		"webSocketUrl": true,
	}
	chromeArgs := []string{}
	if cfg.Headless || !cfg.Headless {
		chromeArgs = append(chromeArgs, "--headless=new", "--disable-gpu")
	}
	if os.Geteuid() == 0 {
		chromeArgs = append(chromeArgs, "--no-sandbox")
	}
	userDataDir := fmt.Sprintf("/tmp/relurpify-bidi-%d", time.Now().UnixNano())
	if strings.TrimSpace(b.baseURL) == "" || b.process != nil {
		var err error
		userDataDir, err = os.MkdirTemp("", "relurpify-bidi-*")
		if err != nil {
			return err
		}
		b.userData = userDataDir
	}
	chromeArgs = append(chromeArgs, "--user-data-dir="+userDataDir)
	chromeArgs = append(chromeArgs, cfg.BrowserArgs...)
	chromeOptions := map[string]any{"args": chromeArgs}
	if strings.TrimSpace(cfg.BrowserBinary) != "" {
		chromeOptions["binary"] = cfg.BrowserBinary
	}
	capabilities["goog:chromeOptions"] = chromeOptions
	payload := map[string]any{
		"capabilities": map[string]any{
			"alwaysMatch": capabilities,
		},
	}
	data, err := b.do(ctx, http.MethodPost, "/session", payload)
	if err != nil {
		return mapBiDiError("open", err)
	}
	var response newSessionResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	if response.SessionID == "" {
		return fmt.Errorf("bidi session id missing")
	}
	if response.Capabilities.WebSocketURL == "" {
		return fmt.Errorf("bidi websocket url missing")
	}
	b.sessionID = response.SessionID
	b.webSocketURL = response.Capabilities.WebSocketURL
	return nil
}

func (b *Backend) resolveContext(ctx context.Context) error {
	result, err := b.transport.Call(ctx, "browsingContext.getTree", map[string]any{})
	if err != nil {
		return err
	}
	var tree treeResult
	if err := json.Unmarshal(result, &tree); err != nil {
		return err
	}
	if len(tree.Contexts) == 0 || tree.Contexts[0].Context == "" {
		return fmt.Errorf("bidi browsing context missing")
	}
	b.contextID = tree.Contexts[0].Context
	return nil
}

func (b *Backend) evaluate(ctx context.Context, expression string) (any, error) {
	result, err := b.transport.Call(ctx, "script.evaluate", map[string]any{
		"expression":           expression,
		"target":               map[string]any{"context": b.contextID},
		"awaitPromise":         true,
		"resultOwnership":      "none",
		"serializationOptions": map[string]any{"maxDomDepth": 0, "maxObjectDepth": 2},
	})
	if err != nil {
		return nil, err
	}
	var payload scriptResultEnvelope
	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, err
	}
	if payload.ExceptionDetails != nil {
		return nil, fmt.Errorf("script evaluation failed")
	}
	return payload.Result.Value, nil
}

func (b *Backend) waitForLoad(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return &browser.Error{Code: browser.ErrTimeout, Backend: "bidi", Operation: "wait", Err: ctx.Err()}
		case raw, ok := <-b.loadEvents:
			if !ok {
				return &browser.Error{Code: browser.ErrBackendDisconnected, Backend: "bidi", Operation: "wait", Err: fmt.Errorf("bidi event stream closed")}
			}
			var event struct {
				Context string `json:"context"`
			}
			if err := json.Unmarshal(raw, &event); err == nil && (event.Context == "" || event.Context == b.contextID) {
				return nil
			}
		}
	}
}

func (b *Backend) checkCondition(ctx context.Context, condition browser.WaitCondition) (bool, error) {
	switch condition.Type {
	case browser.WaitForSelector:
		value, err := b.evaluate(ctx, fmt.Sprintf("document.querySelector(%s) !== null", jsString(condition.Selector)))
		if err != nil {
			return false, mapBiDiError("wait", err)
		}
		ok, _ := value.(bool)
		return ok, nil
	case browser.WaitForSelectorMissing:
		value, err := b.evaluate(ctx, fmt.Sprintf("document.querySelector(%s) === null", jsString(condition.Selector)))
		if err != nil {
			return false, mapBiDiError("wait", err)
		}
		ok, _ := value.(bool)
		return ok, nil
	case browser.WaitForText:
		value, err := b.evaluate(ctx, fmt.Sprintf(`(() => {
			const el = document.querySelector(%s);
			return !!el && (el.innerText || el.textContent || "").includes(%s);
		})()`, jsString(condition.Selector), jsString(condition.Text)))
		if err != nil {
			return false, mapBiDiError("wait", err)
		}
		ok, _ := value.(bool)
		return ok, nil
	case browser.WaitForURLContains:
		currentURL, err := b.CurrentURL(ctx)
		if err != nil {
			return false, err
		}
		return strings.Contains(currentURL, condition.URLContains), nil
	default:
		return false, &browser.Error{Code: browser.ErrUnsupportedOperation, Backend: "bidi", Operation: "wait", Err: fmt.Errorf("unsupported wait type %q", condition.Type)}
	}
}

func (b *Backend) do(ctx context.Context, method, path string, payload map[string]any) (json.RawMessage, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var wire wireResponse
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var werr wireError
		if err := json.Unmarshal(wire.Value, &werr); err != nil {
			return nil, err
		}
		return nil, &protocolError{status: resp.StatusCode, code: werr.Error, message: werr.Message}
	}
	return wire.Value, nil
}

type protocolError struct {
	status  int
	code    string
	message string
}

func (e *protocolError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

func mapBiDiError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var protocolErr *protocolError
	if errors.As(err, &protocolErr) {
		switch protocolErr.code {
		case "no such node", "no such element":
			return &browser.Error{Code: browser.ErrNoSuchElement, Backend: "bidi", Operation: operation, Err: err}
		case "stale element reference":
			return &browser.Error{Code: browser.ErrStaleElement, Backend: "bidi", Operation: operation, Err: err}
		case "timeout":
			return &browser.Error{Code: browser.ErrTimeout, Backend: "bidi", Operation: operation, Err: err}
		case "invalid session id":
			return &browser.Error{Code: browser.ErrBackendDisconnected, Backend: "bidi", Operation: operation, Err: err}
		default:
			return &browser.Error{Code: browser.ErrUnknownOperation, Backend: "bidi", Operation: operation, Err: err}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &browser.Error{Code: browser.ErrTimeout, Backend: "bidi", Operation: operation, Err: err}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "no such element") {
		return &browser.Error{Code: browser.ErrNoSuchElement, Backend: "bidi", Operation: operation, Err: err}
	}
	if strings.Contains(message, "closed") {
		return &browser.Error{Code: browser.ErrBackendDisconnected, Backend: "bidi", Operation: operation, Err: err}
	}
	return &browser.Error{Code: browser.ErrUnknownOperation, Backend: "bidi", Operation: operation, Err: err}
}

func isInvalidSession(err error) bool {
	var protocolErr *protocolError
	return errors.As(err, &protocolErr) && protocolErr.code == "invalid session id"
}

func jsString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

type launchedDriver struct {
	process *exec.Cmd
	baseURL string
}

func launchChromeDriver(ctx context.Context, cfg Config) (*launchedDriver, error) {
	driverPath := strings.TrimSpace(cfg.DriverPath)
	if driverPath == "" {
		var err error
		driverPath, err = exec.LookPath("chromedriver")
		if err != nil {
			return nil, err
		}
	}
	port := cfg.RemotePort
	if port == 0 {
		var err error
		port, err = freePort()
		if err != nil {
			return nil, err
		}
	}
	args := []string{"--port=" + strconv.Itoa(port)}
	args = append(args, cfg.DriverArgs...)
	if cfg.Policy != nil {
		if err := cfg.Policy.AllowCommand(ctx, sandbox.CommandRequest{
			Args: append([]string{driverPath}, args...),
		}); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, driverPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitForDriver(ctx, baseURL, cfg.StartupTimeout); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}
	return &launchedDriver{process: cmd, baseURL: baseURL}, nil
}

func waitForDriver(ctx context.Context, baseURL string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(waitCtx, http.MethodGet, baseURL+"/status", nil)
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

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
