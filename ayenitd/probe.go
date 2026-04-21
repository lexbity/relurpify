package ayenitd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/platform/llm"
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
func ProbeWorkspace(cfg WorkspaceConfig, backend llm.ManagedBackend) []ProbeResult {
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

	// 3. Inference backend reachable
	inferenceOk, inferenceMsg := checkInferenceBackend(cfg, backend)
	results = append(results, ProbeResult{
		Name:     "inference_backend",
		Required: true,
		OK:       inferenceOk,
		Message:  inferenceMsg,
	})

	// 4. Disk space
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
	f, err := os.Open(workspace)
	if err != nil {
		return false, fmt.Sprintf("workspace not readable: %s", err)
	}
	f.Close()
	return true, "workspace directory exists and is readable"
}

func checkSQLiteWritable(workspace string) (bool, string) {
	paths := config.New(workspace)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return false, fmt.Sprintf("cannot create sessions dir: %s", err)
	}
	testFile := filepath.Join(sessionsDir, ".probe_write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		return false, fmt.Sprintf("sessions dir not writable: %s", err)
	}
	_ = os.Remove(testFile)
	return true, "SQLite locations are writable"
}

func checkInferenceBackend(cfg WorkspaceConfig, backend llm.ManagedBackend) (bool, string) {
	if backend == nil {
		var err error
		backend, err = llm.New(llm.ProviderConfigFromRuntimeConfig(cfg))
		if err != nil {
			return false, fmt.Sprintf("build inference backend: %s", err)
		}
		defer backend.Close()
	}
	if err := backend.Warm(context.Background()); err != nil {
		return false, fmt.Sprintf("inference backend unhealthy: %s", err)
	}
	models, err := backend.ListModels(context.Background())
	if err != nil {
		return false, fmt.Sprintf("inference backend model list failed: %s", err)
	}
	if len(models) == 0 {
		return false, "inference backend returned no models"
	}
	selected := cfg.InferenceModel
	if selected == "" {
		selected = models[0].Name
	}
	for _, model := range models {
		if model.Name == selected {
			return true, fmt.Sprintf("inference backend reachable; model %s present", selected)
		}
	}
	return false, fmt.Sprintf("model %s not found in inference backend", selected)
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
