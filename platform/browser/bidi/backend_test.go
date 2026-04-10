package bidi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/platform/browser"
	"github.com/stretchr/testify/require"
)

type fakeTransport struct {
	calls       []transportCall
	responses   map[string]json.RawMessage
	errors      map[string]error
	subscribers map[string]chan json.RawMessage
	callFunc    func(method string, params map[string]any) (json.RawMessage, error)
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

func (f *fakeTransport) Subscribe(method string) <-chan json.RawMessage {
	ch := make(chan json.RawMessage, 4)
	if f.subscribers == nil {
		f.subscribers = map[string]chan json.RawMessage{}
	}
	f.subscribers[method] = ch
	return ch
}

func (f *fakeTransport) Close() error { return nil }

func TestBackendNavigateUsesBrowsingContextNavigate(t *testing.T) {
	transport := &fakeTransport{
		responses: map[string]json.RawMessage{
			"browsingContext.navigate": json.RawMessage(`{"navigation":"nav-1","url":"https://example.com"}`),
		},
		errors: map[string]error{},
	}
	backend := &Backend{transport: transport, contextID: "ctx-1"}

	err := backend.Navigate(context.Background(), "https://example.com")

	require.NoError(t, err)
	require.Len(t, transport.calls, 1)
	require.Equal(t, "browsingContext.navigate", transport.calls[0].method)
	require.Equal(t, "ctx-1", transport.calls[0].params["context"])
}

func TestBackendScreenshotDecodesPNGBytes(t *testing.T) {
	transport := &fakeTransport{
		responses: map[string]json.RawMessage{
			"browsingContext.captureScreenshot": json.RawMessage(`{"data":"` + base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47}) + `"}`),
		},
		errors: map[string]error{},
	}
	backend := &Backend{transport: transport, contextID: "ctx-1"}

	data, err := backend.Screenshot(context.Background())

	require.NoError(t, err)
	require.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, data)
}

func TestBackendWaitForLoadConsumesSubscribedEvent(t *testing.T) {
	transport := &fakeTransport{
		responses: map[string]json.RawMessage{},
		errors:    map[string]error{},
	}
	loadEvents := make(chan json.RawMessage, 1)
	backend := &Backend{
		transport:  transport,
		contextID:  "ctx-1",
		loadEvents: loadEvents,
	}
	loadEvents <- json.RawMessage(`{"context":"ctx-1"}`)

	err := backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForLoad}, time.Second)

	require.NoError(t, err)
}

func TestMapBiDiErrorClosedTransport(t *testing.T) {
	err := mapBiDiError("navigate", errors.New("transport closed"))

	require.True(t, browser.IsErrorCode(err, browser.ErrBackendDisconnected))
}
