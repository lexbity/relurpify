package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config/secretscan"
)

const runtimeStateDirName = secretscan.RuntimeStateDirName

// ReadConfigFile reads a workspace config file after enforcing workspace-local
// access and rejecting runtime-state paths.
func ReadConfigFile(workspaceRoot, path string) ([]byte, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	path = strings.TrimSpace(path)
	if workspaceRoot == "" {
		return nil, fmt.Errorf("workspace root required")
	}
	if path == "" {
		return nil, fmt.Errorf("config path required")
	}

	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	rel, err := filepath.Rel(absWorkspace, absPath)
	if err != nil {
		return nil, fmt.Errorf("check config path %q against workspace %q: %w", absPath, absWorkspace, err)
	}
	if rel == "." {
		return nil, fmt.Errorf("config path %q must reference a file", absPath)
	}
	if rel == runtimeStateDirName || strings.HasPrefix(rel, runtimeStateDirName+string(filepath.Separator)) {
		return nil, fmt.Errorf("config path %q is inside runtime state dir %q", absPath, runtimeStateDirName)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("config path %q is outside workspace root %q", absPath, absWorkspace)
	}

	return os.ReadFile(filepath.Clean(absPath))
}

// ReadFileRaw reads a file path after cleaning it. It is intended for trusted
// fixture and migration code that already owns the path.
func ReadFileRaw(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("file path required")
	}
	return os.ReadFile(filepath.Clean(path))
}
