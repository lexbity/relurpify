package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LocalToolPolicyPath returns the canonical local tool policy location.
func LocalToolPolicyPath(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "security", "localtool.policy.yaml")
}

type localToolPolicyFile struct {
	Tools map[string]ToolPolicy `yaml:"tools,omitempty"`
}

// LoadLocalToolPolicy loads and validates the local tool policy file.
func LoadLocalToolPolicy(path, workspace string, decode Decoder) (map[string]ToolPolicy, error) {
	var file localToolPolicyFile
	if err := loadAndDecode(path, workspace, decode, LocalToolPolicyPath, &file); err != nil {
		return nil, err
	}
	if err := validateLocalToolPolicies(file.Tools); err != nil {
		return nil, err
	}
	out := make(map[string]ToolPolicy, len(file.Tools))
	for name, policy := range file.Tools {
		out[name] = policy
	}
	return out, nil
}

func validateLocalToolPolicies(policies map[string]ToolPolicy) error {
	for name, policy := range policies {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("local tool policy contains empty tool name")
		}
		switch policy.Execute {
		case "allow", "ask", "deny", "":
		default:
			return fmt.Errorf("local tool policy %s execute=%s invalid", name, policy.Execute)
		}
	}
	return nil
}
