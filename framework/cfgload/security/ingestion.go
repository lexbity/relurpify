package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// WorkspaceIngestionPolicyPath returns the canonical workspace ingestion policy location.
func WorkspaceIngestionPolicyPath(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "security", "workspaceingestion.policy.yaml")
}

type workspaceIngestionPolicyFile struct {
	Rules []core.PolicyRule `yaml:"rules,omitempty"`
}

// LoadWorkspaceIngestionPolicy loads and validates the workspace ingestion policy file.
func LoadWorkspaceIngestionPolicy(path, workspace string, decode Decoder) ([]core.PolicyRule, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}
	if strings.TrimSpace(path) == "" {
		path = WorkspaceIngestionPolicyPath(workspace)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workspace ingestion policy %s: %w", path, err)
	}
	var file workspaceIngestionPolicyFile
	if decode == nil {
		return nil, fmt.Errorf("decoder required")
	}
	if _, err := decode(path, data, &file); err != nil {
		return nil, err
	}
	if err := validatePolicyRules(file.Rules); err != nil {
		return nil, err
	}
	return append([]core.PolicyRule(nil), file.Rules...), nil
}

func validatePolicyRules(rules []core.PolicyRule) error {
	for i, rule := range rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rules[%d] invalid: %w", i, err)
		}
	}
	return nil
}
