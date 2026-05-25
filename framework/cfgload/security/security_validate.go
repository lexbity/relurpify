package security

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

// DecodeWithSchema is injected by cfgload at init to break import cycles.
var DecodeWithSchema func(path string, data []byte, out any) (any, error)

// Bundle groups the typed security policy files loaded from relurpify_cfg/security.
type Bundle struct {
	Sandbox   *sandbox.SandboxPolicy
	Shell     *sandbox.ShellBlacklist
	LocalTool map[string]agentspec.ToolPolicy
	Ingestion []core.PolicyRule
}

// LoadBundle loads the full security policy set for a workspace.
func LoadBundle(workspace string) (*Bundle, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	sandboxPolicy, err := LoadSandboxPolicy("", absWorkspace)
	if err != nil {
		return nil, fmt.Errorf("load sandbox policy: %w", err)
	}
	shellPolicy, err := LoadShellPolicy("", absWorkspace)
	if err != nil {
		return nil, fmt.Errorf("load shell policy: %w", err)
	}
	localToolPolicy, err := LoadLocalToolPolicy("", absWorkspace)
	if err != nil {
		return nil, fmt.Errorf("load local tool policy: %w", err)
	}
	ingestionRules, err := LoadWorkspaceIngestionPolicy("", absWorkspace)
	if err != nil {
		return nil, fmt.Errorf("load workspace ingestion policy: %w", err)
	}
	return &Bundle{
		Sandbox:   sandboxPolicy,
		Shell:     shellPolicy,
		LocalTool: localToolPolicy,
		Ingestion: ingestionRules,
	}, nil
}
