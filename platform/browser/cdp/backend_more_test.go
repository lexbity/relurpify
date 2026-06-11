package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/platform/browser"
)

func TestCapabilitiesAndHelpers(t *testing.T) {
	backend := &Backend{}
	require.Equal(t, browser.Capabilities{
		AccessibilityTree: true,
		NetworkIntercept:  true,
		DownloadEvents:    false,
		PopupTracking:     false,
		ArbitraryEval:     true,
	}, backend.Capabilities())

	cases := []struct {
		name string
		err  error
		code browser.ErrorCode
	}{
		{name: "no such element", err: errors.New("no such element"), code: browser.ErrNoSuchElement},
		{name: "not interactable", err: errors.New("element not interactable"), code: browser.ErrElementNotInteractable},
		{name: "deadline", err: context.DeadlineExceeded, code: browser.ErrTimeout},
		{name: "exception", err: errors.New("cannot call function"), code: browser.ErrScriptEvaluation},
		{name: "closed", err: errors.New("transport closed"), code: browser.ErrBackendDisconnected},
		{name: "default", err: errors.New("boom"), code: browser.ErrUnknownOperation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mapCDPError("op", tc.err)
			require.True(t, browser.IsErrorCode(err, tc.code))
		})
	}
}

func TestBackendMethodFlowWithFakeTransport(t *testing.T) {
	transport := &fakeTransport{
		callFunc: func(method string, params map[string]any) (json.RawMessage, error) {
			switch method {
			case "Page.navigate":
				return json.RawMessage(`{"frameId":"frame-1"}`), nil
			case "Runtime.evaluate":
				expr, _ := params["expression"].(string)
				switch {
				case strings.Contains(expr, "document.readyState"):
					return json.RawMessage(`{"result":{"type":"boolean","value":true}}`), nil
				case strings.Contains(expr, "innerText ?? el.textContent ??"):
					return json.RawMessage(`{"result":{"type":"string","value":"hello"}}`), nil
				case strings.Contains(expr, "JSON.stringify"):
					return json.RawMessage(`{"result":{"type":"string","value":"{\"role\":\"document\"}"}}`), nil
				case strings.Contains(expr, "document.documentElement.outerHTML"):
					return json.RawMessage(`{"result":{"type":"string","value":"<html><body>ok</body></html>"}}`), nil
				case strings.Contains(expr, "querySelector(") && strings.Contains(expr, "!== null"):
					return json.RawMessage(`{"result":{"type":"boolean","value":true}}`), nil
				case strings.Contains(expr, "querySelector(") && strings.Contains(expr, "=== null"):
					return json.RawMessage(`{"result":{"type":"boolean","value":true}}`), nil
				case strings.Contains(expr, "includes("):
					return json.RawMessage(`{"result":{"type":"boolean","value":true}}`), nil
				case strings.Contains(expr, "window.location.href.includes"):
					return json.RawMessage(`{"result":{"type":"boolean","value":true}}`), nil
				case strings.Contains(expr, "window.location.href"):
					return json.RawMessage(`{"result":{"type":"string","value":"https://example.com/page"}}`), nil
				default:
					return json.RawMessage(`{"result":{"type":"object","value":{"ok":true}}}`), nil
				}
			case "DOM.getDocument":
				return json.RawMessage(`{"root":{"nodeId":42}}`), nil
			case "DOM.getOuterHTML":
				return json.RawMessage(`{"outerHTML":"<html><body>ok</body></html>"}`), nil
			case "Page.captureScreenshot":
				return json.RawMessage(`{"data":"` + base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47}) + `"}`), nil
			case "Accessibility.getFullAXTree":
				return json.RawMessage(`{"nodes":[{"role":"document"}]}`), nil
			default:
				return json.RawMessage(`{}`), nil
			}
		},
	}
	backend := &Backend{transport: transport}

	require.NoError(t, backend.Navigate(context.Background(), "https://example.com"))
	require.NoError(t, backend.Click(context.Background(), "#submit"))
	require.NoError(t, backend.Type(context.Background(), "#name", "lex"))

	text, err := backend.GetText(context.Background(), "#result")
	require.NoError(t, err)
	require.Equal(t, "hello", text)

	ax, err := backend.GetAccessibilityTree(context.Background())
	require.NoError(t, err)
	require.Contains(t, ax, "document")

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

	currentURL, err := backend.CurrentURL(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://example.com/page", currentURL)

	require.Error(t, backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForNetworkIdle}, time.Millisecond))
}

func TestTransportCallReadLoopAndClose(t *testing.T) {
	wsURL, shutdown := newCDPWebSocketServer(t)
	defer shutdown()

	transport, err := newWebsocketTransport(context.Background(), wsURL)
	require.NoError(t, err)
	defer func() { _ = transport.Close() }()

	result, err := transport.Call(context.Background(), "Runtime.evaluate", map[string]any{"expression": "1+1"})
	require.NoError(t, err)
	require.Contains(t, string(result), "ok")

	_, err = transport.Call(context.Background(), "force.error", map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cdp error")

	transport.pending[99] = make(chan callResult, 1)
	transport.removePending(99)
	_, ok := transport.pending[99]
	require.False(t, ok)
	require.NoError(t, transport.Close())
	require.NoError(t, transport.Close())
}

func TestLaunchChromiumPolicyAndPageTargetErrors(t *testing.T) {
	deny := sandbox.CommandPolicyFunc(func(context.Context, ports.CommandRequest) error {
		return errors.New("denied")
	})
	_, err := launchChromium(context.Background(), Config{
		ExecutablePath: writeSleepScript(t),
		StartupTimeout: time.Millisecond,
		Policy:         deny,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "denied")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]listTarget{{ID: "worker-1", Type: "worker"}})
	}))
	defer server.Close()
	_, err = pageWebSocketURL(context.Background(), server.URL)
	assert.Error(t, err)
}

func TestLaunchChromiumTimeoutBranch(t *testing.T) {
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := portListener.Addr().(*net.TCPAddr).Port
	_ = portListener.Close()

	_, err = launchChromium(context.Background(), Config{
		ExecutablePath: writeSleepScript(t),
		RemotePort:     port,
		StartupTimeout: time.Millisecond,
	})
	require.Error(t, err)
}

func TestTransportCloseDrainsPending(t *testing.T) {
	wsURL, shutdown := newCDPWebSocketServer(t)
	defer shutdown()

	transport, err := newWebsocketTransport(context.Background(), wsURL)
	require.NoError(t, err)
	pending := make(chan callResult, 1)
	transport.pending[1] = pending
	require.NoError(t, transport.Close())
	select {
	case res := <-pending:
		require.Error(t, res.err)
	case <-time.After(time.Second):
		t.Fatal("expected close to fail pending call")
	}
	require.NoError(t, transport.Close())
}

func TestTransportCloseHandlesPendingAndIdempotence(t *testing.T) {
	wsURL, shutdown := newCDPWebSocketServer(t)
	defer shutdown()

	transport, err := newWebsocketTransport(context.Background(), wsURL)
	require.NoError(t, err)
	pending := make(chan callResult, 1)
	transport.pending[7] = pending
	require.NoError(t, transport.Close())
	select {
	case res := <-pending:
		require.Error(t, res.err)
	case <-time.After(time.Second):
		t.Fatal("expected pending call to be failed on close")
	}
	require.NoError(t, transport.Close())
}

func TestLaunchChromiumAndClose(t *testing.T) {
	wsURL, shutdownWS := newCDPWebSocketServer(t)
	defer shutdownWS()

	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := portListener.Addr().(*net.TCPAddr).Port
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/list":
			if err := json.NewEncoder(w).Encode([]listTarget{{
				ID:                   "page-1",
				Type:                 "page",
				WebSocketDebuggerURL: wsURL,
			}}); err != nil {
				panic(err)
			}
		case "/status":
			if err := json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"ready": true}}); err != nil {
				panic(err)
			}
		default:
			http.NotFound(w, r)
		}
	})}
	go func() {
		_ = httpServer.Serve(portListener)
	}()
	defer func() { _ = httpServer.Close() }()

	script := writeSleepScript(t)
	launched, err := launchChromium(context.Background(), Config{
		ExecutablePath: script,
		RemotePort:     port,
		StartupTimeout: time.Second,
		Headless:       true,
	})
	assert.NoError(t, err)

	backend := &Backend{
		transport: &fakeTransport{callFunc: func(method string, params map[string]any) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		}},
		process:  launched.process,
		userData: launched.userData,
	}
	assert.NoError(t, backend.Close(context.Background()))
}

func TestWaitForDebuggerAndPageWebSocketURL(t *testing.T) {
	wsURL, shutdownWS := newCDPWebSocketServer(t)
	defer shutdownWS()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/list" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]listTarget{{ID: "page-1", Type: "page", WebSocketDebuggerURL: wsURL}})
	}))
	defer server.Close()

	got, err := pageWebSocketURL(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, wsURL, got)

	got, err = waitForDebugger(context.Background(), server.URL, time.Second)
	require.NoError(t, err)
	assert.Equal(t, wsURL, got)
}

func newCDPWebSocketServer(t *testing.T) (string, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		assert.NoError(t, err)
		go func() {
			defer func() { _ = conn.Close() }()
			for {
				var req requestEnvelope
				if err := conn.ReadJSON(&req); err != nil {
					return
				}
				switch req.Method {
				case "Runtime.evaluate":
					expr, _ := req.Params["expression"].(string)
					switch {
					case strings.Contains(expr, "document.readyState"):
						assert.NoError(t, conn.WriteJSON(responseEnvelope{ID: req.ID, Result: json.RawMessage(`{"result":{"type":"boolean","value":true}}`)}))
					case strings.Contains(expr, "querySelector(") && strings.Contains(expr, "!== null"):
						assert.NoError(t, conn.WriteJSON(responseEnvelope{ID: req.ID, Result: json.RawMessage(`{"result":{"type":"boolean","value":true}}`)}))
					case strings.Contains(expr, "querySelector(") && strings.Contains(expr, "=== null"):
						assert.NoError(t, conn.WriteJSON(responseEnvelope{ID: req.ID, Result: json.RawMessage(`{"result":{"type":"boolean","value":true}}`)}))
					case strings.Contains(expr, "includes("):
						assert.NoError(t, conn.WriteJSON(responseEnvelope{ID: req.ID, Result: json.RawMessage(`{"result":{"type":"boolean","value":true}}`)}))
					case strings.Contains(expr, "window.location.href"):
						assert.NoError(t, conn.WriteJSON(responseEnvelope{ID: req.ID, Result: json.RawMessage(`{"result":{"type":"string","value":"https://example.com/page"}}`)}))
					default:
						assert.NoError(t, conn.WriteJSON(responseEnvelope{ID: req.ID, Result: json.RawMessage(`{"result":{"type":"object","value":{"ok":true}}}`)}))
					}
				case "force.error":
					assert.NoError(t, conn.WriteJSON(responseEnvelope{ID: req.ID, Error: &responseError{Code: 42, Message: "boom"}}))
				default:
					assert.NoError(t, conn.WriteJSON(responseEnvelope{ID: req.ID, Error: &responseError{Code: 0, Message: req.Method}}))
				}
			}
		}()
	}))
	return "ws" + strings.TrimPrefix(server.URL, "http"), server.Close
}

func writeSleepScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/sleep.sh"
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nsleep 60\n"), 0o755))
	return path
}
