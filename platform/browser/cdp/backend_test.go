package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/platform/browser"
	"github.com/stretchr/testify/require"
)

type fakeTransport struct {
	calls     []transportCall
	responses map[string]json.RawMessage
	errors    map[string]error
	callFunc  func(method string, params map[string]any) (json.RawMessage, error)
}

type transportCall struct {
	method string
	params map[string]any
}

func (f *fakeTransport) Call(_ context.Context, method string, params map[string]any) (json.RawMessage, error) {
	f.calls = append(f.calls, transportCall{method: method, params: params})
	if f.callFunc != nil {
		return f.callFunc(method, params)
	}
	if err := f.errors[method]; err != nil {
		return nil, err
	}
	return f.responses[method], nil
}

func (f *fakeTransport) Close() error { return nil }

func TestBackendGetHTMLUsesDOMCommands(t *testing.T) {
	transport := &fakeTransport{
		responses: map[string]json.RawMessage{
			"DOM.getDocument":  json.RawMessage(`{"root":{"nodeId":42}}`),
			"DOM.getOuterHTML": json.RawMessage(`{"outerHTML":"<html><body>ok</body></html>"}`),
		},
		errors: map[string]error{},
	}
	backend := &Backend{transport: transport}

	html, err := backend.GetHTML(context.Background())

	require.NoError(t, err)
	require.Equal(t, "<html><body>ok</body></html>", html)
	require.Len(t, transport.calls, 2)
	require.Equal(t, "DOM.getDocument", transport.calls[0].method)
	require.Equal(t, "DOM.getOuterHTML", transport.calls[1].method)
}

func TestBackendScreenshotDecodesPNGBytes(t *testing.T) {
	transport := &fakeTransport{
		responses: map[string]json.RawMessage{
			"Page.captureScreenshot": json.RawMessage(`{"data":"` + base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47}) + `"}`),
		},
		errors: map[string]error{},
	}
	backend := &Backend{transport: transport}

	data, err := backend.Screenshot(context.Background())

	require.NoError(t, err)
	require.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, data)
}

func TestBackendMapsMissingElementError(t *testing.T) {
	transport := &fakeTransport{
		responses: map[string]json.RawMessage{
			"Runtime.evaluate": json.RawMessage(`{"result":{"type":"string","value":""},"exceptionDetails":{"text":"no such element"}}`),
		},
		errors: map[string]error{},
	}
	backend := &Backend{transport: transport}

	err := backend.Click(context.Background(), "#missing")

	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrNoSuchElement))
}

func TestBackendWaitUnsupportedCondition(t *testing.T) {
	backend := &Backend{transport: &fakeTransport{responses: map[string]json.RawMessage{}, errors: map[string]error{}}}

	err := backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForNetworkIdle}, 10*time.Millisecond)

	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrUnsupportedOperation))
}

func TestMapCDPErrorClosedTransport(t *testing.T) {
	err := mapCDPError("navigate", errors.New("transport closed"))

	require.True(t, browser.IsErrorCode(err, browser.ErrBackendDisconnected))
}
