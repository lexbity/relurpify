package security

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
)

// LocalToolPolicyPath returns the canonical local tool policy location.
func LocalToolPolicyPath(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "security", "localtool.policy.yaml")
}

type localToolPolicyFile struct {
	Tools map[string]agentspec.ToolPolicy `yaml:"tools,omitempty"`
}

// LoadLocalToolPolicy loads and validates the local tool policy file.
func LoadLocalToolPolicy(path, workspace string, decode Decoder) (map[string]agentspec.ToolPolicy, error) {
	var file localToolPolicyFile
	if err := loadAndDecode(path, workspace, decode, LocalToolPolicyPath, &file); err != nil {
		return nil, err
	}
	if err := validateLocalToolPolicies(file.Tools); err != nil {
		return nil, err
	}
	out := make(map[string]agentspec.ToolPolicy, len(file.Tools))
	for name, policy := range file.Tools {
		out[name] = policy
	}
	return out, nil
}

func validateLocalToolPolicies(policies map[string]agentspec.ToolPolicy) error {
	for name, policy := range policies {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("local tool policy contains empty tool name")
		}
		switch policy.Execute {
		case agentspec.AgentPermissionAllow, agentspec.AgentPermissionAsk, agentspec.AgentPermissionDeny, "":
		default:
			return fmt.Errorf("local tool policy %s execute=%s invalid", name, policy.Execute)
		}
	}
	return nil
}
