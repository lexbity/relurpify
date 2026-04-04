package ayenitd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/ayenitd"
)

// findResult returns the ProbeResult with the given name, or fails the test.
func findResult(t *testing.T, results []ayenitd.ProbeResult, name string) ayenitd.ProbeResult {
	t.Helper()
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("probe result %q not found in %v", name, results)
	return ayenitd.ProbeResult{}
}

func TestProbeWorkspace_WorkspaceNotFound(t *testing.T) {
	// Create and immediately remove a temp dir to get a guaranteed-absent path.
	absent := t.TempDir()
	if err := os.RemoveAll(absent); err != nil {
		t.Fatal(err)
	}
	cfg := ayenitd.WorkspaceConfig{
		Workspace:      absent,
		OllamaEndpoint: "http://127.0.0.1:11435",
		OllamaModel:    "qwen2.5-coder:14b",
	}
	results := ayenitd.ProbeWorkspace(cfg)
	r := findResult(t, results, "workspace_directory")
	if r.OK {
		t.Error("workspace_directory: expected NOT OK for missing directory")
	}
	if !r.Required {
		t.Error("workspace_directory: should be required")
	}
}

func TestProbeWorkspace_WorkspaceIsFile(t *testing.T) {
	f, err := os.CreateTemp("", "ayenitd-probe-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	cfg := ayenitd.WorkspaceConfig{
		Workspace:      f.Name(),
		OllamaEndpoint: "http://127.0.0.1:11435",
		OllamaModel:    "qwen2.5-coder:14b",
	}
	results := ayenitd.ProbeWorkspace(cfg)
	r := findResult(t, results, "workspace_directory")
	if r.OK {
		t.Error("workspace_directory: expected NOT OK when path is a file, not a directory")
	}
}

func TestProbeWorkspace_WorkspaceExists(t *testing.T) {
	cfg := ayenitd.WorkspaceConfig{
		Workspace:      t.TempDir(),
		OllamaEndpoint: "http://127.0.0.1:11435",
		OllamaModel:    "qwen2.5-coder:14b",
	}
	results := ayenitd.ProbeWorkspace(cfg)
	r := findResult(t, results, "workspace_directory")
	if !r.OK {
		t.Errorf("workspace_directory: expected OK for existing temp dir, got: %s", r.Message)
	}
}

func TestProbeWorkspace_SQLiteWritable(t *testing.T) {
	cfg := ayenitd.WorkspaceConfig{
		Workspace:      t.TempDir(),
		OllamaEndpoint: "http://127.0.0.1:11435",
		OllamaModel:    "qwen2.5-coder:14b",
	}
	results := ayenitd.ProbeWorkspace(cfg)
	r := findResult(t, results, "sqlite_writable")
	if !r.OK {
		t.Errorf("sqlite_writable: expected OK for writable temp dir, got: %s", r.Message)
	}
}

func TestProbeWorkspace_OllamaUnreachable(t *testing.T) {
	cfg := ayenitd.WorkspaceConfig{
		Workspace:      t.TempDir(),
		OllamaEndpoint: "http://127.0.0.1:19999", // port unlikely to be in use
		OllamaModel:    "qwen2.5-coder:14b",
	}
	results := ayenitd.ProbeWorkspace(cfg)
	r := findResult(t, results, "ollama_reachable")
	if r.OK {
		t.Error("ollama_reachable: expected NOT OK for unreachable endpoint")
	}
	if !r.Required {
		t.Error("ollama_reachable: should be required")
	}
}

func TestProbeWorkspace_OllamaReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := ayenitd.WorkspaceConfig{
		Workspace:      t.TempDir(),
		OllamaEndpoint: srv.URL,
		OllamaModel:    "qwen2.5-coder:14b",
	}
	results := ayenitd.ProbeWorkspace(cfg)
	r := findResult(t, results, "ollama_reachable")
	if !r.OK {
		t.Errorf("ollama_reachable: expected OK for reachable mock server, got: %s", r.Message)
	}
}

func TestProbeWorkspace_OllamaModelPresent(t *testing.T) {
	const model = "qwen2.5-coder:14b"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			resp := map[string]interface{}{
				"models": []map[string]interface{}{
					{"name": model},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := ayenitd.WorkspaceConfig{
		Workspace:      t.TempDir(),
		OllamaEndpoint: srv.URL,
		OllamaModel:    model,
	}
	results := ayenitd.ProbeWorkspace(cfg)
	r := findResult(t, results, "ollama_model")
	if !r.OK {
		t.Errorf("ollama_model: expected OK when model is in list, got: %s", r.Message)
	}
}

func TestProbeWorkspace_OllamaModelMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			resp := map[string]interface{}{
				"models": []map[string]interface{}{
					{"name": "other-model:7b"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := ayenitd.WorkspaceConfig{
		Workspace:      t.TempDir(),
		OllamaEndpoint: srv.URL,
		OllamaModel:    "qwen2.5-coder:14b",
	}
	results := ayenitd.ProbeWorkspace(cfg)
	r := findResult(t, results, "ollama_model")
	if r.OK {
		t.Error("ollama_model: expected NOT OK when model is absent from list")
	}
	if !strings.Contains(r.Message, "ollama pull") {
		t.Errorf("ollama_model: message should suggest 'ollama pull', got: %s", r.Message)
	}
}

func TestProbeWorkspace_DiskSpaceIsNonRequired(t *testing.T) {
	cfg := ayenitd.WorkspaceConfig{
		Workspace:      t.TempDir(),
		OllamaEndpoint: "http://127.0.0.1:19999",
		OllamaModel:    "qwen2.5-coder:14b",
	}
	results := ayenitd.ProbeWorkspace(cfg)
	r := findResult(t, results, "disk_space")
	if r.Required {
		t.Error("disk_space: should not be required (warn-only)")
	}
}

func TestProbeWorkspace_AllResultNamesPresent(t *testing.T) {
	cfg := ayenitd.WorkspaceConfig{
		Workspace:      t.TempDir(),
		OllamaEndpoint: "http://127.0.0.1:19999",
		OllamaModel:    "qwen2.5-coder:14b",
	}
	results := ayenitd.ProbeWorkspace(cfg)
	want := []string{"workspace_directory", "sqlite_writable", "ollama_reachable", "ollama_model", "disk_space"}
	got := make(map[string]bool, len(results))
	for _, r := range results {
		got[r.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing expected probe result: %q", name)
		}
	}
}
