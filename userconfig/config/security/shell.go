package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// ShellPolicyPath returns the canonical shell policy location.
func ShellPolicyPath(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "security", "shell.policy.yaml")
}

type shellPolicyFile struct {
	Rules []sandbox.BlacklistRule `yaml:"rules,omitempty"`
}

// LoadShellPolicy loads and validates the shell policy file.
func LoadShellPolicy(path, workspace string, decode Decoder) (*sandbox.ShellBlacklist, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}
	if strings.TrimSpace(path) == "" {
		path = ShellPolicyPath(workspace)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read shell policy %s: %w", path, err)
	}
	var file shellPolicyFile
	if decode == nil {
		return nil, fmt.Errorf("decoder required")
	}
	if _, err := decode(path, data, &file); err != nil {
		return nil, err
	}
	return sandbox.NewShellBlacklist(file.Rules)
}
