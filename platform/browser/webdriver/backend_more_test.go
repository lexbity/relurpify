package webdriver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/platform/browser"
	"github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

func TestCapabilitiesAndHelpers(t *testing.T) {
	backend := &Backend{}
	require.Equal(t, browser.Capabilities{
		AccessibilityTree: false,
		NetworkIntercept:  false,
		DownloadEvents:    false,
		PopupTracking:     false,
		ArbitraryEval:     true,
	}, backend.Capabilities())

	var nilErr *protocolError
	require.Equal(t, "", nilErr.Error())
	require.True(t, isInvalidSession(&protocolError{code: "invalid session id"}))
	require.False(t, isInvalidSession(&protocolError{code: "unexpected"}))
	require.Equal(t, []string{"a", "b", "c"}, splitRunes("abc"))

	cases := []struct {
		name string
		err  error
		code browser.ErrorCode
	}{
		{name: "no such element", err: &protocolError{code: "no such element", message: "missing"}, code: browser.ErrNoSuchElement},
		{name: "stale", err: &protocolError{code: "stale element reference", message: "stale"}, code: browser.ErrStaleElement},
		{name: "not interactable", err: &protocolError{code: "element not interactable", message: "nope"}, code: browser.ErrElementNotInteractable},
		{name: "click intercepted", err: &protocolError{code: "element click intercepted", message: "nope"}, code: browser.ErrElementNotInteractable},
		{name: "timeout", err: &protocolError{code: "timeout", message: "slow"}, code: browser.ErrTimeout},
		{name: "script timeout", err: &protocolError{code: "script timeout", message: "slow"}, code: browser.ErrTimeout},
		{name: "invalid session", err: &protocolError{code: "invalid session id", message: "gone"}, code: browser.ErrBackendDisconnected},
		{name: "deadline", err: context.DeadlineExceeded, code: browser.ErrTimeout},
		{name: "default", err: errors.New("boom"), code: browser.ErrUnknownOperation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mapWebDriverError("op", tc.err)
			require.True(t, browser.IsErrorCode(err, tc.code))
		})
	}
}

func TestBackendMethodFlowWithRemoteServer(t *testing.T) {
	var currentURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"sessionId": "test"}})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/timeouts":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": nil})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/url":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			currentURL, _ = payload["url"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"value": nil})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/element":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["value"] == "#missing" {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"error": "no such element", "message": "missing"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"element-6066-11e4-a52e-4f735466cecf": "elem-1"}})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/element/elem-1/click":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": nil})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/element/elem-1/value":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": nil})
		case r.Method == http.MethodGet && r.URL.Path == "/session/test/element/elem-1/text":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": "hello"})
		case r.Method == http.MethodGet && r.URL.Path == "/session/test/source":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": "<html><body>ok</body></html>"})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/execute/sync":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			script, _ := payload["script"].(string)
			switch {
			case strings.Contains(script, "document.readyState"):
				_ = json.NewEncoder(w).Encode(map[string]any{"value": true})
			case strings.Contains(script, "querySelector(") && strings.Contains(script, "!== null"):
				_ = json.NewEncoder(w).Encode(map[string]any{"value": true})
			case strings.Contains(script, "querySelector(") && strings.Contains(script, "=== null"):
				_ = json.NewEncoder(w).Encode(map[string]any{"value": true})
			case strings.Contains(script, "includes("):
				_ = json.NewEncoder(w).Encode(map[string]any{"value": true})
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"ok": true}})
			}
		case r.Method == http.MethodGet && r.URL.Path == "/session/test/screenshot":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47})})
		case r.Method == http.MethodGet && r.URL.Path == "/session/test/url":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": currentURL})
		case r.Method == http.MethodDelete && r.URL.Path == "/session/test":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": nil})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := New(context.Background(), Config{RemoteURL: server.URL})
	require.NoError(t, err)

	require.NoError(t, backend.Navigate(context.Background(), "https://example.com"))
	require.NoError(t, backend.Click(context.Background(), "#submit"))
	require.NoError(t, backend.Type(context.Background(), "#name", "lex"))

	text, err := backend.GetText(context.Background(), "#result")
	require.NoError(t, err)
	require.Equal(t, "hello", text)

	ax, err := backend.GetAccessibilityTree(context.Background())
	require.NoError(t, err)
	require.Contains(t, ax, "example.com")

	html, err := backend.GetHTML(context.Background())
	require.NoError(t, err)
	require.Equal(t, "<html><body>ok</body></html>", html)

	value, err := backend.ExecuteScript(context.Background(), "return 1")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ok": true}, value)

	screenshot, err := backend.Screenshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, screenshot)

	require.NoError(t, backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForLoad}, time.Second))
	require.NoError(t, backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForSelector, Selector: "#ready"}, time.Second))
	require.NoError(t, backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForSelectorMissing, Selector: "#missing"}, time.Second))
	require.NoError(t, backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForText, Selector: "#result", Text: "hello"}, time.Second))
	require.NoError(t, backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForURLContains, URLContains: "example.com"}, time.Second))

	current, err := backend.CurrentURL(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://example.com", current)

	require.Error(t, backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForNetworkIdle}, time.Millisecond))

	process := exec.Command("sleep", "5")
	require.NoError(t, process.Start())
	backend.process = process
	backend.userData = t.TempDir()
	require.NoError(t, backend.Close())
	require.NoError(t, backend.Close())
}

func TestBackendErrorAndConditionBranches(t *testing.T) {
	var currentURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"sessionId": "test"}})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/timeouts":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": nil})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/url":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			currentURL, _ = payload["url"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"value": nil})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/element":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			switch payload["value"] {
			case "#empty":
				_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{}})
			case "#missing":
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"error": "no such element", "message": "missing"}})
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"element-6066-11e4-a52e-4f735466cecf": "elem-1"}})
			}
		case r.Method == http.MethodGet && r.URL.Path == "/session/test/url":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": currentURL})
		case r.Method == http.MethodGet && r.URL.Path == "/session/test/element/elem-1/text":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": "hello"})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/execute/sync":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"ok": true}})
		case r.Method == http.MethodDelete && r.URL.Path == "/session/test":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"error": "invalid session id", "message": "gone"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := New(context.Background(), Config{RemoteURL: server.URL})
	require.NoError(t, err)
	require.NoError(t, backend.Navigate(context.Background(), "https://example.com"))
	_, err = backend.GetText(context.Background(), "#missing")
	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrNoSuchElement))

	_, err = backend.GetText(context.Background(), "#empty")
	require.Error(t, err)

	ok, err := backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForLoad})
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForSelector, Selector: "#submit"})
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForSelectorMissing, Selector: "#missing"})
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForText, Selector: "#result", Text: "hello"})
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForURLContains, URLContains: "example.com"})
	require.NoError(t, err)
	require.True(t, ok)

	err = backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForNetworkIdle}, time.Millisecond)
	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrUnsupportedOperation))

	_, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitConditionType("bogus")})
	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrUnsupportedOperation))

	_, err = backend.ExecuteScript(context.Background(), "return 1")
	require.NoError(t, err)

	process := exec.Command("sleep", "5")
	require.NoError(t, process.Start())
	backend.process = process
	backend.userData = t.TempDir()
	require.NoError(t, backend.Close())
	require.NoError(t, backend.Close())
}

func TestCheckConditionFalseBranches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"sessionId": "test"}})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/timeouts":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": nil})
		case r.Method == http.MethodPost && r.URL.Path == "/session/test/element":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"error": "no such element", "message": "missing"}})
		case r.Method == http.MethodGet && r.URL.Path == "/session/test/url":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": ""})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend, err := New(context.Background(), Config{RemoteURL: server.URL})
	require.NoError(t, err)

	ok, err := backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForLoad})
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForSelector, Selector: "#missing"})
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForSelectorMissing, Selector: "#missing"})
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForText, Selector: "#missing", Text: "hello"})
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitForURLContains, URLContains: "example.com"})
	require.NoError(t, err)
	require.False(t, ok)

	_, err = backend.checkCondition(context.Background(), browser.WaitCondition{Type: browser.WaitConditionType("bogus")})
	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrUnsupportedOperation))
}

func TestLaunchChromeDriverPolicyAndTimeoutBranches(t *testing.T) {
	deny := sandbox.CommandPolicyFunc(func(context.Context, sandbox.CommandRequest) error {
		return errors.New("denied")
	})
	_, err := launchChromeDriver(context.Background(), Config{
		DriverPath:     writeSleepScript(t),
		StartupTimeout: time.Millisecond,
		Policy:         deny,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "denied")

	err = waitForDriver(context.Background(), "http://127.0.0.1:1", time.Millisecond)
	require.Error(t, err)
}

func TestLaunchChromeDriverTimeoutBranch(t *testing.T) {
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := portListener.Addr().(*net.TCPAddr).Port
	portListener.Close()

	_, err = launchChromeDriver(context.Background(), Config{
		DriverPath:     writeSleepScript(t),
		RemotePort:     port,
		StartupTimeout: time.Millisecond,
	})
	require.Error(t, err)
}

func TestNewMissingSessionIDAndInvalidSessionClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := New(context.Background(), Config{RemoteURL: server.URL})
	require.Error(t, err)

	closeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/session/test" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"error": "invalid session id", "message": "gone"}})
			return
		}
		http.NotFound(w, r)
	}))
	defer closeServer.Close()

	backend := &Backend{
		client:    closeServer.Client(),
		baseURL:   closeServer.URL,
		sessionID: "test",
	}
	require.NoError(t, backend.Close())

	invalidServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/session/test" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"error": "invalid session id", "message": "gone"}})
			return
		}
		http.NotFound(w, r)
	}))
	defer invalidServer.Close()
	backend = &Backend{
		client:    invalidServer.Client(),
		baseURL:   invalidServer.URL,
		sessionID: "test",
	}
	require.NoError(t, backend.Close())
}

func TestLaunchChromeDriverAndWaitForDriver(t *testing.T) {
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := portListener.Addr().(*net.TCPAddr).Port

	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"ready": true}})
	})}
	go func() {
		_ = httpServer.Serve(portListener)
	}()
	defer httpServer.Close()

	script := writeSleepScript(t)
	launched, err := launchChromeDriver(context.Background(), Config{
		DriverPath:     script,
		RemotePort:     port,
		StartupTimeout: time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:"+strconv.Itoa(port), launched.baseURL)
	require.NoError(t, launched.process.Process.Kill())
	_, _ = launched.process.Process.Wait()
}

func TestWaitForDriverAndFreePort(t *testing.T) {
	port, err := freePort()
	require.NoError(t, err)
	require.NotZero(t, port)

	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	actualPort := portListener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"ready": true}})
	})}
	go func() {
		_ = server.Serve(portListener)
	}()
	defer server.Close()

	err = waitForDriver(context.Background(), "http://127.0.0.1:"+strconv.Itoa(actualPort), time.Second)
	require.NoError(t, err)
}

func writeSleepScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/sleep.sh"
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nsleep 60\n"), 0o755))
	return path
}
