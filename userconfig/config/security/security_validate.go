package security

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// Decoder decodes a config file's bytes into out.
type Decoder func(path string, data []byte, out any) (any, error)

// Bundle groups the typed security policy files loaded from relurpify_cfg/security.
type Bundle struct {
	Sandbox   *sandbox.SandboxPolicy
	Shell     *sandbox.ShellBlacklist
	LocalTool map[string]agentspec.ToolPolicy
	Ingestion []PolicyRule
}

// LoadBundle loads the full security policy set for a workspace.
func LoadBundle(workspace string, decode Decoder) (*Bundle, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	var errs []error
	sandboxPolicy, err := LoadSandboxPolicy("", absWorkspace, decode)
	if err != nil {
		errs = append(errs, fmt.Errorf("load sandbox policy: %w", err))
	}
	shellPolicy, err := LoadShellPolicy("", absWorkspace, decode)
	if err != nil {
		errs = append(errs, fmt.Errorf("load shell policy: %w", err))
	}
	localToolPolicy, err := LoadLocalToolPolicy("", absWorkspace, decode)
	if err != nil {
		errs = append(errs, fmt.Errorf("load local tool policy: %w", err))
	}
	ingestionRules, err := LoadWorkspaceIngestionPolicy("", absWorkspace, decode)
	if err != nil {
		errs = append(errs, fmt.Errorf("load workspace ingestion policy: %w", err))
	}
	if len(errs) > 0 {
		return &Bundle{
			Sandbox:   sandboxPolicy,
			Shell:     shellPolicy,
			LocalTool: localToolPolicy,
			Ingestion: ingestionRules,
		}, errors.Join(errs...)
	}
	return &Bundle{
		Sandbox:   sandboxPolicy,
		Shell:     shellPolicy,
		LocalTool: localToolPolicy,
		Ingestion: ingestionRules,
	}, nil
}
