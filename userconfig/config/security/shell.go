package security

import (
	"path/filepath"

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
	var file shellPolicyFile
	if err := loadAndDecode(path, workspace, decode, ShellPolicyPath, &file); err != nil {
		return nil, err
	}
	return sandbox.NewShellBlacklist(file.Rules)
}
