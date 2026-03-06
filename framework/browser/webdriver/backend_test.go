package webdriver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/browser"
	"github.com/stretchr/testify/require"
)

func TestBackendFindElementAndClick(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/session/test/element":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"element-6066-11e4-a52e-4f735466cecf": "elem-1"}})
		case "/session/test/element/elem-1/click":
			_ = json.NewEncoder(w).Encode(map[string]any{"value": nil})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	backend := &Backend{
		client:    server.Client(),
		baseURL:   server.URL,
		sessionID: "test",
	}

	err := backend.Click(context.Background(), "#submit")

	require.NoError(t, err)
	require.Equal(t, []string{
		"POST /session/test/element",
		"POST /session/test/element/elem-1/click",
	}, requests)
}

func TestBackendScreenshotDecodesPNG(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"value": encoded})
	}))
	defer server.Close()

	backend := &Backend{
		client:    server.Client(),
		baseURL:   server.URL,
		sessionID: "test",
	}

	data, err := backend.Screenshot(context.Background())

	require.NoError(t, err)
	require.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, data)
}

func TestBackendMapsNoSuchElement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": map[string]any{
				"error":   "no such element",
				"message": "missing",
			},
		})
	}))
	defer server.Close()

	backend := &Backend{
		client:    server.Client(),
		baseURL:   server.URL,
		sessionID: "test",
	}

	_, err := backend.GetText(context.Background(), "#missing")

	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrNoSuchElement))
}

func TestBackendWaitUnsupportedCondition(t *testing.T) {
	backend := &Backend{}

	err := backend.WaitFor(context.Background(), browser.WaitCondition{Type: browser.WaitForNetworkIdle}, 10*time.Millisecond)

	require.Error(t, err)
	require.True(t, browser.IsErrorCode(err, browser.ErrUnsupportedOperation))
}
