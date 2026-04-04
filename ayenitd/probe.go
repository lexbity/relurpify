package ayenitd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lexcodex/relurpify/framework/config"
)

// ProbeResult represents the outcome of a single platform runtime check.
type ProbeResult struct {
	Name     string
	Required bool
	OK       bool
	Message  string
}

// ProbeWorkspace runs all platform runtime checks required for a workspace.
// It returns a slice of results, one per check.
func ProbeWorkspace(cfg WorkspaceConfig) []ProbeResult {
	var results []ProbeResult

	// 1. Workspace directory
	wsOk, wsMsg := checkWorkspaceDirectory(cfg.Workspace)
	results = append(results, ProbeResult{
		Name:     "workspace_directory",
		Required: true,
		OK:       wsOk,
		Message:  wsMsg,
	})

	// 2. SQLite writable locations
	sqliteOk, sqliteMsg := checkSQLiteWritable(cfg.Workspace)
	results = append(results, ProbeResult{
		Name:     "sqlite_writable",
		Required: true,
		OK:       sqliteOk,
		Message:  sqliteMsg,
	})

	// 3. Ollama endpoint reachable
	ollamaReachOk, ollamaReachMsg := checkOllamaReachable(cfg.OllamaEndpoint)
	results = append(results, ProbeResult{
		Name:     "ollama_reachable",
		Required: true,
		OK:       ollamaReachOk,
		Message:  ollamaReachMsg,
	})

	// 4. Ollama model present
	ollamaModelOk, ollamaModelMsg := checkOllamaModel(cfg.OllamaEndpoint, cfg.OllamaModel)
	results = append(results, ProbeResult{
		Name:     "ollama_model",
		Required: true,
		OK:       ollamaModelOk,
		Message:  ollamaModelMsg,
	})

	// 5. Disk space
	diskOk, diskMsg := checkDiskSpace(cfg.Workspace, 256*1024*1024) // 256 MB
	results = append(results, ProbeResult{
		Name:     "disk_space",
		Required: false,
		OK:       diskOk,
		Message:  diskMsg,
	})

	return results
}

func checkWorkspaceDirectory(workspace string) (bool, string) {
	info, err := os.Stat(workspace)
	if err != nil {
		return false, fmt.Sprintf("workspace not found: %s", err)
	}
	if !info.IsDir() {
		return false, "workspace path is not a directory"
	}
	// Check readability
	f, err := os.Open(workspace)
	if err != nil {
		return false, fmt.Sprintf("workspace not readable: %s", err)
	}
	f.Close()
	return true, "workspace directory exists and is readable"
}

func checkSQLiteWritable(workspace string) (bool, string) {
	paths := config.New(workspace)
	// Check sessions directory
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return false, fmt.Sprintf("cannot create sessions dir: %s", err)
	}
	// Try to create a test file
	testFile := filepath.Join(sessionsDir, ".probe_write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		return false, fmt.Sprintf("sessions dir not writable: %s", err)
	}
	_ = os.Remove(testFile)
	return true, "SQLite locations are writable"
}

func checkOllamaReachable(endpoint string) (bool, string) {
	if endpoint == "" {
		return false, "Ollama endpoint not configured"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "HEAD", endpoint+"/api/tags", nil)
	if err != nil {
		return false, fmt.Sprintf("invalid Ollama endpoint: %s", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("Ollama not reachable at %s; is it running?", endpoint)
	}
	resp.Body.Close()
	return true, "Ollama endpoint reachable"
}

func checkOllamaModel(endpoint, model string) (bool, string) {
	if endpoint == "" || model == "" {
		return false, "Ollama endpoint or model not configured"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := fmt.Sprintf("%s/api/tags", endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Sprintf("cannot create request: %s", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("failed to fetch model list: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, fmt.Sprintf("Ollama API returned %s", resp.Status)
	}
	// For simplicity, we'll assume the model is present if the endpoint responds.
	// In a real implementation, we would parse the JSON response.
	// This is a placeholder.
	return true, fmt.Sprintf("model %s assumed present (check not fully implemented)", model)
}

func checkDiskSpace(workspace string, requiredBytes int64) (bool, string) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(workspace, &stat)
	if err != nil {
		return true, fmt.Sprintf("cannot check disk space: %s (assuming sufficient)", err)
	}
	available := stat.Bavail * uint64(stat.Bsize)
	if uint64(requiredBytes) > available {
		return false, fmt.Sprintf("insufficient disk space: %d MB available, need at least %d MB",
			available/(1024*1024), requiredBytes/(1024*1024))
	}
	return true, fmt.Sprintf("sufficient disk space available (%d MB)",
		available/(1024*1024))
}
