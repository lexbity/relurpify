package conformance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type BackendConformanceSpec struct {
	Name                  string
	NewBackend            func(t *testing.T, endpoint string, nativeToolCalling bool) any
	NewServer             func(t *testing.T, scenario string, nativeToolCalling bool) *httptest.Server
	Generate              func(backend any) (string, error)
	Chat                  func(backend any) (string, error)
	ChatWithToolsNative   func(backend any) error
	ChatWithToolsFallback func(backend any) (string, error)
	Warm                  func(backend any) error
	WarmFail              func(backend any) error
	Health                func(backend any) any
	ListModels            func(backend any) any
	Embed                 func(backend any) ([][]float32, error)
	Close                 func(backend any) error
	SetDebugLogging       func(backend any, enabled bool)
}

func BackendConformanceSuite(t *testing.T, spec BackendConformanceSpec) {
	t.Helper()
	require.NotEmpty(t, spec.Name)
	require.NotNil(t, spec.NewBackend)
	require.NotNil(t, spec.NewServer)
	require.NotNil(t, spec.Generate)
	require.NotNil(t, spec.Chat)
	require.NotNil(t, spec.ChatWithToolsNative)
	require.NotNil(t, spec.ChatWithToolsFallback)
	require.NotNil(t, spec.Warm)
	require.NotNil(t, spec.WarmFail)
	require.NotNil(t, spec.Health)
	require.NotNil(t, spec.ListModels)
	require.NotNil(t, spec.Embed)
	require.NotNil(t, spec.Close)
	require.NotNil(t, spec.SetDebugLogging)

	t.Run(spec.Name+"/Generate", func(t *testing.T) {
		srv := spec.NewServer(t, "generate", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		text, err := spec.Generate(backend)
		require.NoError(t, err)
		require.NotEmpty(t, text)
	})

	t.Run(spec.Name+"/Chat", func(t *testing.T) {
		srv := spec.NewServer(t, "chat", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		text, err := spec.Chat(backend)
		require.NoError(t, err)
		require.NotEmpty(t, text)
	})

	t.Run(spec.Name+"/ChatWithToolsNative", func(t *testing.T) {
		srv := spec.NewServer(t, "chat-tools-native", true)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, true)
		require.NoError(t, spec.ChatWithToolsNative(backend))
	})

	t.Run(spec.Name+"/ChatWithToolsFallback", func(t *testing.T) {
		srv := spec.NewServer(t, "chat-tools-fallback", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		text, err := spec.ChatWithToolsFallback(backend)
		require.NoError(t, err)
		require.NotEmpty(t, text)
	})

	t.Run(spec.Name+"/Warm", func(t *testing.T) {
		srv := spec.NewServer(t, "warm-ok", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		require.NoError(t, spec.Warm(backend))
	})

	t.Run(spec.Name+"/WarmFailure", func(t *testing.T) {
		srv := spec.NewServer(t, "warm-fail", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		require.Error(t, spec.WarmFail(backend))
	})

	t.Run(spec.Name+"/Health", func(t *testing.T) {
		srv := spec.NewServer(t, "health", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		report := spec.Health(backend)
		require.NotNil(t, report)
		data, err := json.Marshal(report)
		require.NoError(t, err)
		require.Contains(t, string(data), "ready")
	})

	t.Run(spec.Name+"/ListModels", func(t *testing.T) {
		srv := spec.NewServer(t, "models", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		models := spec.ListModels(backend)
		require.NotNil(t, models)
		data, err := json.Marshal(models)
		require.NoError(t, err)
		require.NotEmpty(t, string(data))
	})

	t.Run(spec.Name+"/Embedder", func(t *testing.T) {
		srv := spec.NewServer(t, "embed", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		vectors, err := spec.Embed(backend)
		require.NoError(t, err)
		require.Len(t, vectors, 1)
		require.NotEmpty(t, vectors[0])
	})

	t.Run(spec.Name+"/CloseIdempotent", func(t *testing.T) {
		srv := spec.NewServer(t, "health", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		require.NoError(t, spec.Close(backend))
		require.NoError(t, spec.Close(backend))
	})

	t.Run(spec.Name+"/DebugLogging", func(t *testing.T) {
		srv := spec.NewServer(t, "health", false)
		t.Cleanup(srv.Close)
		backend := spec.NewBackend(t, srv.URL, false)
		spec.SetDebugLogging(backend, true)
	})
}

func writeJSONResponse(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func requestBodyString(r *http.Request) string {
	data, _ := io.ReadAll(r.Body)
	return string(data)
}

func withContext() context.Context {
	return context.Background()
}
