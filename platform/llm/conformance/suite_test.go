package conformance_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/llm/conformance"
)

// fakeBackend is a minimal backend that talks to the scenario HTTP server
// selected by the spec. Each call performs a real HTTP request so the suite
// exercises end-to-end request/response behavior rather than stub returns.
type fakeBackend struct {
	baseURL string
	debug   bool
	closed  int
}

func fakeServerHandler(scenario string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch scenario {
		case "generate", "chat", "chat-tools-fallback":
			if len(body) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"text":"echo:%s"}`, scenario)
		case "chat-tools-native":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"tool":"called"}`)
		case "warm-ok":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"status":"warm"}`)
		case "warm-fail":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"model not loaded"}`)
		case "health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ready":true,"models":["fake-1"]}`)
		case "models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"models":["fake-1","fake-2"]}`)
		case "embed":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"vectors":[[0.25,0.5,0.75]]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func postScenario(backend any, scenario, requestBody string) (string, int, error) {
	b, ok := backend.(*fakeBackend)
	if !ok {
		return "", 0, fmt.Errorf("unexpected backend type %T", backend)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(b.baseURL+"/v1/"+scenario, "application/json", strings.NewReader(requestBody))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(data), resp.StatusCode, nil
}

func expectOK(backend any, scenario string) (string, error) {
	body, status, err := postScenario(backend, scenario, `{"prompt":"hi"}`)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("%s: unexpected status %d", scenario, status)
	}
	return body, nil
}

func fullSpec() conformance.BackendConformanceSpec {
	return conformance.BackendConformanceSpec{
		Name: "fake",
		NewBackend: func(t *testing.T, endpoint string, nativeToolCalling bool) any {
			return &fakeBackend{baseURL: endpoint}
		},
		NewServer: func(t *testing.T, scenario string, nativeToolCalling bool) *httptest.Server {
			return httptest.NewServer(fakeServerHandler(scenario))
		},
		Generate: func(backend any) (string, error) {
			return expectOK(backend, "generate")
		},
		Chat: func(backend any) (string, error) {
			return expectOK(backend, "chat")
		},
		ChatWithToolsNative: func(backend any) error {
			_, err := expectOK(backend, "chat-tools-native")
			return err
		},
		ChatWithToolsFallback: func(backend any) (string, error) {
			return expectOK(backend, "chat-tools-fallback")
		},
		Warm: func(backend any) error {
			_, err := expectOK(backend, "warm-ok")
			return err
		},
		WarmFail: func(backend any) error {
			body, status, err := postScenario(backend, "warm-fail", `{"prompt":"hi"}`)
			if err != nil {
				return err
			}
			if status == http.StatusServiceUnavailable {
				return fmt.Errorf("warm failed: %s", body)
			}
			return nil
		},
		Health: func(backend any) any {
			body, _ := expectOK(backend, "health")
			var report map[string]any
			_ = json.Unmarshal([]byte(body), &report)
			return report
		},
		ListModels: func(backend any) any {
			body, _ := expectOK(backend, "models")
			var out map[string][]string
			_ = json.Unmarshal([]byte(body), &out)
			return out
		},
		Embed: func(backend any) ([][]float32, error) {
			body, err := expectOK(backend, "embed")
			if err != nil {
				return nil, err
			}
			var parsed struct {
				Vectors [][]float32 `json:"vectors"`
			}
			if err := json.Unmarshal([]byte(body), &parsed); err != nil {
				return nil, err
			}
			return parsed.Vectors, nil
		},
		Close: func(backend any) error {
			backend.(*fakeBackend).closed++
			return nil
		},
		SetDebugLogging: func(backend any, enabled bool) {
			backend.(*fakeBackend).debug = enabled
		},
	}
}

func TestBackendConformanceSuite_FakeBackendPasses(t *testing.T) {
	conformance.BackendConformanceSuite(t, fullSpec())
}

func TestFullSpecHasEveryFieldSet(t *testing.T) {
	spec := fullSpec()
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
}
