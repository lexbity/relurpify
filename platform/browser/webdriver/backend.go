package webdriver

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
}

type Backend struct {
	client    *http.Client
	baseURL   string
	sessionID string
	process   *exec.Cmd
	userData  string
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
	SessionID string `json:"sessionId"`
}

type elementReference struct {
	ElementID string `json:"element-6066-11e4-a52e-4f735466cecf"`
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
		backend.userData = launched.userData
	} else {
		backend.baseURL = strings.TrimRight(strings.TrimSpace(cfg.RemoteURL), "/")
	}
	if err := backend.startSession(ctx, cfg); err != nil {
		_ = backend.Close()
		return nil, err
	}
	return backend, nil
}

func (b *Backend) Navigate(ctx context.Context, rawURL string) error {
	_, err := b.do(ctx, http.MethodPost, "/session/"+b.sessionID+"/url", map[string]any{"url": rawURL})
	if err != nil {
		return mapWebDriverError("navigate", err)
	}
	return b.WaitFor(ctx, browser.WaitCondition{Type: browser.WaitForLoad}, 15*time.Second)
}

func (b *Backend) Click(ctx context.Context, selector string) error {
	elementID, err := b.findElement(ctx, selector)
	if err != nil {
		return err
	}
	_, err = b.do(ctx, http.MethodPost, "/session/"+b.sessionID+"/element/"+elementID+"/click", map[string]any{})
	return mapWebDriverError("click", err)
}

func (b *Backend) Type(ctx context.Context, selector, text string) error {
	elementID, err := b.findElement(ctx, selector)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"text":  text,
		"value": splitRunes(text),
	}
	_, err = b.do(ctx, http.MethodPost, "/session/"+b.sessionID+"/element/"+elementID+"/value", payload)
	return mapWebDriverError("type", err)
}

func (b *Backend) GetText(ctx context.Context, selector string) (string, error) {
	elementID, err := b.findElement(ctx, selector)
	if err != nil {
		return "", err
	}
	data, err := b.do(ctx, http.MethodGet, "/session/"+b.sessionID+"/element/"+elementID+"/text", nil)
	if err != nil {
		return "", mapWebDriverError("get_text", err)
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return "", err
	}
	return text, nil
}

func (b *Backend) GetAccessibilityTree(ctx context.Context) (string, error) {
	currentURL, err := b.CurrentURL(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"role":"document","name":"","url":%s}`, jsString(currentURL)), nil
}

func (b *Backend) GetHTML(ctx context.Context) (string, error) {
	data, err := b.do(ctx, http.MethodGet, "/session/"+b.sessionID+"/source", nil)
	if err != nil {
		return "", mapWebDriverError("get_html", err)
	}
	var html string
	if err := json.Unmarshal(data, &html); err != nil {
		return "", err
	}
	return html, nil
}

func (b *Backend) ExecuteScript(ctx context.Context, script string) (any, error) {
	payload := map[string]any{
		"script": script,
		"args":   []any{},
	}
	data, err := b.do(ctx, http.MethodPost, "/session/"+b.sessionID+"/execute/sync", payload)
	if err != nil {
		return nil, mapWebDriverError("execute_script", err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (b *Backend) Screenshot(ctx context.Context) ([]byte, error) {
	data, err := b.do(ctx, http.MethodGet, "/session/"+b.sessionID+"/screenshot", nil)
	if err != nil {
		return nil, mapWebDriverError("screenshot", err)
	}
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(encoded)
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
			return &browser.Error{Code: browser.ErrTimeout, Backend: "webdriver", Operation: "wait", Err: waitCtx.Err()}
		case <-ticker.C:
		}
	}
}

func (b *Backend) CurrentURL(ctx context.Context) (string, error) {
	data, err := b.do(ctx, http.MethodGet, "/session/"+b.sessionID+"/url", nil)
	if err != nil {
		return "", mapWebDriverError("current_url", err)
	}
	var currentURL string
	if err := json.Unmarshal(data, &currentURL); err != nil {
		return "", err
	}
	return currentURL, nil
}

func (b *Backend) Close() error {
	var errs []error
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
		"browserName": "chrome",
	}
	chromeArgs := []string{}
	if cfg.Headless || !cfg.Headless {
		chromeArgs = append(chromeArgs, "--headless=new", "--disable-gpu")
	}
	if os.Geteuid() == 0 {
		chromeArgs = append(chromeArgs, "--no-sandbox")
	}
	userDataDir := fmt.Sprintf("/tmp/relurpify-webdriver-%d", time.Now().UnixNano())
	if strings.TrimSpace(b.baseURL) == "" || b.process != nil {
		var err error
		userDataDir, err = os.MkdirTemp("", "relurpify-webdriver-*")
		if err != nil {
			return err
		}
		b.userData = userDataDir
	}
	chromeArgs = append(chromeArgs, "--user-data-dir="+userDataDir)
	chromeArgs = append(chromeArgs, cfg.BrowserArgs...)
	chromeOptions := map[string]any{
		"args": chromeArgs,
	}
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
		return mapWebDriverError("open", err)
	}
	var response newSessionResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	if response.SessionID == "" {
		return fmt.Errorf("webdriver session id missing")
	}
	b.sessionID = response.SessionID
	_, err = b.do(ctx, http.MethodPost, "/session/"+b.sessionID+"/timeouts", map[string]any{
		"implicit": 0,
		"pageLoad": 300000,
		"script":   30000,
	})
	return err
}

func (b *Backend) findElement(ctx context.Context, selector string) (string, error) {
	data, err := b.do(ctx, http.MethodPost, "/session/"+b.sessionID+"/element", map[string]any{
		"using": "css selector",
		"value": selector,
	})
	if err != nil {
		return "", mapWebDriverError("find_element", err)
	}
	var element elementReference
	if err := json.Unmarshal(data, &element); err != nil {
		return "", err
	}
	if element.ElementID == "" {
		return "", &browser.Error{Code: browser.ErrNoSuchElement, Backend: "webdriver", Operation: "find_element", Err: fmt.Errorf("element id missing")}
	}
	return element.ElementID, nil
}

func (b *Backend) checkCondition(ctx context.Context, condition browser.WaitCondition) (bool, error) {
	switch condition.Type {
	case browser.WaitForLoad:
		currentURL, err := b.CurrentURL(ctx)
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(currentURL) != "", nil
	case browser.WaitForSelector:
		_, err := b.findElement(ctx, condition.Selector)
		if err == nil {
			return true, nil
		}
		if browser.IsErrorCode(err, browser.ErrNoSuchElement) {
			return false, nil
		}
		return false, err
	case browser.WaitForSelectorMissing:
		_, err := b.findElement(ctx, condition.Selector)
		if err == nil {
			return false, nil
		}
		if browser.IsErrorCode(err, browser.ErrNoSuchElement) {
			return true, nil
		}
		return false, err
	case browser.WaitForText:
		text, err := b.GetText(ctx, condition.Selector)
		if err != nil {
			if browser.IsErrorCode(err, browser.ErrNoSuchElement) {
				return false, nil
			}
			return false, err
		}
		return strings.Contains(text, condition.Text), nil
	case browser.WaitForURLContains:
		currentURL, err := b.CurrentURL(ctx)
		if err != nil {
			return false, err
		}
		return strings.Contains(currentURL, condition.URLContains), nil
	default:
		return false, &browser.Error{Code: browser.ErrUnsupportedOperation, Backend: "webdriver", Operation: "wait", Err: fmt.Errorf("unsupported wait type %q", condition.Type)}
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

func mapWebDriverError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var protocolErr *protocolError
	if errors.As(err, &protocolErr) {
		switch protocolErr.code {
		case "no such element":
			return &browser.Error{Code: browser.ErrNoSuchElement, Backend: "webdriver", Operation: operation, Err: err}
		case "stale element reference":
			return &browser.Error{Code: browser.ErrStaleElement, Backend: "webdriver", Operation: operation, Err: err}
		case "element not interactable", "element click intercepted":
			return &browser.Error{Code: browser.ErrElementNotInteractable, Backend: "webdriver", Operation: operation, Err: err}
		case "script timeout", "timeout":
			return &browser.Error{Code: browser.ErrTimeout, Backend: "webdriver", Operation: operation, Err: err}
		case "invalid session id":
			return &browser.Error{Code: browser.ErrBackendDisconnected, Backend: "webdriver", Operation: operation, Err: err}
		default:
			return &browser.Error{Code: browser.ErrUnknownOperation, Backend: "webdriver", Operation: operation, Err: err}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &browser.Error{Code: browser.ErrTimeout, Backend: "webdriver", Operation: operation, Err: err}
	}
	return &browser.Error{Code: browser.ErrUnknownOperation, Backend: "webdriver", Operation: operation, Err: err}
}

func isInvalidSession(err error) bool {
	var protocolErr *protocolError
	return errors.As(err, &protocolErr) && protocolErr.code == "invalid session id"
}

func jsString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func splitRunes(value string) []string {
	parts := make([]string, 0, len(value))
	for _, r := range value {
		parts = append(parts, string(r))
	}
	return parts
}

type launchedDriver struct {
	process  *exec.Cmd
	baseURL  string
	userData string
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
	return &launchedDriver{
		process: cmd,
		baseURL: baseURL,
	}, nil
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
