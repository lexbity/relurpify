package bidi

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
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/platform/browser"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/gorilla/websocket"
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
	require.Equal(t, "\"hello\"", jsString("hello"))

	cases := []struct {
		name string
		err  error
		code browser.ErrorCode
	}{
		{name: "no such node", err: &protocolError{code: "no such node", message: "missing"}, code: browser.ErrNoSuchElement},
		{name: "stale", err: &protocolError{code: "stale element reference", message: "stale"}, code: browser.ErrStaleElement},
		{name: "timeout", err: &protocolError{code: "timeout", message: "slow"}, code: browser.ErrTimeout},
		{name: "invalid session", err: &protocolError{code: "invalid session id", message: "gone"}, code: browser.ErrBackendDisconnected},
		{name: "deadline", err: context.DeadlineExceeded, code: browser.ErrTimeout},
		{name: "closed", err: errors.New("transport closed"), code: browser.ErrBackendDisconnected},
		{name: "default", err: errors.New("boom"), code: browser.ErrUnknownOperation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mapBiDiError("op", tc.err)
			require.True(t, browser.IsErrorCode(err, tc.code))
		})
	}
}

func TestBackendMethodFlowWithRemoteServers(t *testing.T) {
	wsURL, shutdown := newBidiTestWebSocketServer(t)
	defer shutdown()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": map[string]any{
					"sessionId": "session-1",
					"capabilities": map[string]any{
						"webSocketUrl": wsURL,
					},
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/session/session-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": nil,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	backend, err := New(context.Background(), Config{RemoteURL: api.URL})
	require.NoError(t, err)

	require.NoError(t, backend.Navigate(context.Background(), "https://example.com"))
	require.NoError(t, backend.Click(context.Background(), "#submit"))
	require.NoError(t, backend.Type(context.Background(), "#name", "lex"))

	text, err := backend.GetText(context.Background(), "#result")
	require.NoError(t, err)
	require.Equal(t, "hello", text)

	ax, err := backend.GetAccessibilityTree(context.Background())
	require.NoError(t, err)
	require.Contains(t, ax, `"role":"document"`)

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

	closeProc := exec.Command("sleep", "5")
	require.NoError(t, closeProc.Start())
	backend.process = closeProc
	backend.userData = t.TempDir()
	require.NoError(t, backend.Close())
	require.NoError(t, backend.Close())
}

func TestTransportCallReadLoopAndClose(t *testing.T) {
	wsURL, shutdown := newBidiTestWebSocketServer(t)
	defer shutdown()

	transport, err := newWebsocketTransport(context.Background(), wsURL)
	require.NoError(t, err)
	defer transport.Close()

	sub := transport.Subscribe("browsingContext.load")
	other := transport.Subscribe("custom.event")
	pendingCh := make(chan callResult, 1)
	transport.pending[99] = pendingCh
	removeCh := make(chan callResult, 1)
	transport.pending[100] = removeCh
	_, err = transport.Call(context.Background(), "session.subscribe", map[string]any{"events": []string{"browsingContext.load"}})
	require.NoError(t, err)
	result, err := transport.Call(context.Background(), "browsingContext.getTree", map[string]any{})
	require.NoError(t, err)
	require.Contains(t, string(result), "ctx-1")

	_, err = transport.Call(context.Background(), "force.error", map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such element")

	select {
	case evt, ok := <-sub:
		require.True(t, ok)
		require.Contains(t, string(evt), "ctx-1")
	case <-time.After(time.Second):
		t.Fatal("expected load event")
	}

	transport.removePending(100)
	_, ok := transport.pending[100]
	require.False(t, ok)
	require.NoError(t, transport.Close())
	select {
	case res := <-pendingCh:
		require.Error(t, res.err)
	case <-time.After(time.Second):
		t.Fatal("expected pending call to be failed on close")
	}
	_, ok = <-other
	require.False(t, ok)
	require.NoError(t, transport.Close())
}

func TestDoEvaluateResolveContextAndLaunchErrors(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": map[string]any{
				"error":   "no such element",
				"message": "missing",
			},
		})
	}))
	defer api.Close()

	backend := &Backend{client: api.Client(), baseURL: api.URL}
	_, err := backend.do(context.Background(), http.MethodPost, "/session", map[string]any{"hello": "world"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such element")

	backend = &Backend{transport: &fakeTransport{callFunc: func(method string, params map[string]any) (json.RawMessage, error) {
		switch method {
		case "script.evaluate":
			return json.RawMessage(`{"result":{"type":"string","value":"x"},"exceptionDetails":{"text":"boom"}}`), nil
		case "browsingContext.getTree":
			return json.RawMessage(`{"contexts":[]}`), nil
		default:
			return json.RawMessage(`{}`), nil
		}
	}}}

	_, err = backend.evaluate(context.Background(), "return 1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "script evaluation failed")

	err = backend.resolveContext(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")

	deny := contracts.CommandPolicyFunc(func(context.Context, contracts.CommandRequest) error {
		return errors.New("denied")
	})
	_, err = launchChromeDriver(context.Background(), Config{
		DriverPath:     writeBidiSleepScript(t),
		RemotePort:     0,
		StartupTimeout: time.Millisecond,
		Policy:         deny,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "denied")
}

func TestLaunchChromeDriverTimeoutBranch(t *testing.T) {
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := portListener.Addr().(*net.TCPAddr).Port
	portListener.Close()

	_, err = launchChromeDriver(context.Background(), Config{
		DriverPath:     writeBidiSleepScript(t),
		RemotePort:     port,
		StartupTimeout: time.Millisecond,
	})
	require.Error(t, err)
}

func TestNewMissingSessionMetadata(t *testing.T) {
	missingSession := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": map[string]any{
				"capabilities": map[string]any{
					"webSocketUrl": "ws://example.test",
				},
			},
		})
	}))
	defer missingSession.Close()
	_, err := New(context.Background(), Config{RemoteURL: missingSession.URL})
	require.Error(t, err)

	missingWS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": map[string]any{
				"sessionId":    "session-1",
				"capabilities": map[string]any{},
			},
		})
	}))
	defer missingWS.Close()
	_, err = New(context.Background(), Config{RemoteURL: missingWS.URL})
	require.Error(t, err)
}

type closingTransport struct{ err error }

func (c closingTransport) Call(context.Context, string, map[string]any) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (c closingTransport) Subscribe(string) <-chan json.RawMessage {
	ch := make(chan json.RawMessage)
	close(ch)
	return ch
}

func (c closingTransport) Close() error { return c.err }

func TestBackendCloseWithTransportError(t *testing.T) {
	backend := &Backend{
		transport: closingTransport{err: errors.New("transport close failed")},
	}
	require.Error(t, backend.Close())
}

func TestWaitForLoadTimeoutAndClosedStream(t *testing.T) {
	backend := &Backend{contextID: "ctx-1", loadEvents: make(chan json.RawMessage)}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := backend.waitForLoad(ctx)
	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrTimeout))

	closed := make(chan json.RawMessage)
	close(closed)
	backend.loadEvents = closed
	err = backend.waitForLoad(context.Background())
	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrBackendDisconnected))
}

func newBidiTestWebSocketServer(t *testing.T) (string, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		go func() {
			defer conn.Close()
			for {
				var req requestEnvelope
				if err := conn.ReadJSON(&req); err != nil {
					return
				}
				switch req.Method {
				case "session.subscribe":
					require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{}`)}))
					_ = conn.WriteJSON(responseEnvelope{
						Type:   "event",
						Method: "browsingContext.load",
						Params: json.RawMessage(`{"context":"ctx-1"}`),
					})
				case "browsingContext.getTree":
					require.NoError(t, conn.WriteJSON(responseEnvelope{
						Type:   "success",
						ID:     req.ID,
						Result: json.RawMessage(`{"contexts":[{"context":"ctx-1"}]}`),
					}))
				case "browsingContext.navigate":
					require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"navigation":"nav-1"}`)}))
				case "browsingContext.captureScreenshot":
					require.NoError(t, conn.WriteJSON(responseEnvelope{
						Type:   "success",
						ID:     req.ID,
						Result: json.RawMessage(`{"data":"` + base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47}) + `"}`),
					}))
				case "force.error":
					require.NoError(t, conn.WriteJSON(responseEnvelope{
						Type:    "error",
						ID:      req.ID,
						Error:   "no such element",
						Message: "missing",
					}))
				case "script.evaluate":
					expr, _ := req.Params["expression"].(string)
					switch {
					case strings.Contains(expr, "JSON.stringify"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"string","value":"{\"role\":\"document\",\"name\":\"Example Title\",\"url\":\"https://example.com/page\"}"}}`)}))
					case strings.Contains(expr, "innerText ?? el.textContent ??"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"string","value":"hello"}}`)}))
					case strings.Contains(expr, "document.title"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"string","value":"Example Title"}}`)}))
					case strings.Contains(expr, "document.links.length"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"object","value":{"links":2,"forms":3,"inputs":4,"buttons":5}}}`)}))
					case strings.Contains(expr, "document.documentElement.outerHTML"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"string","value":"<html><body>ok</body></html>"}}`)}))
					case strings.Contains(expr, "window.location.href"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"string","value":"https://example.com/page"}}`)}))
					case strings.Contains(expr, "includes("):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"boolean","value":true}}`)}))
					case strings.Contains(expr, "querySelector(") && strings.Contains(expr, "!== null"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"boolean","value":true}}`)}))
					case strings.Contains(expr, "querySelector(") && strings.Contains(expr, "=== null"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"boolean","value":true}}`)}))
					case strings.Contains(expr, "el.click()"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"boolean","value":true}}`)}))
					case strings.Contains(expr, "el.focus()"):
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"boolean","value":true}}`)}))
					default:
						require.NoError(t, conn.WriteJSON(responseEnvelope{Type: "success", ID: req.ID, Result: json.RawMessage(`{"result":{"type":"object","value":{"ok":true}}}`)}))
					}
				default:
					require.NoError(t, conn.WriteJSON(responseEnvelope{
						Type:    "error",
						ID:      req.ID,
						Error:   "unsupported",
						Message: req.Method,
					}))
				}
			}
		}()
	}))
	wsBase := "ws" + strings.TrimPrefix(server.URL, "http")
	return wsBase, server.Close
}

func writeBidiSleepScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/sleep.sh"
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nsleep 60\n"), 0o755))
	return path
}
