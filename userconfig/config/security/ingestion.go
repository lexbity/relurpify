package security

import (
	"fmt"
	"path/filepath"
)

// WorkspaceIngestionPolicyPath returns the canonical workspace ingestion policy location.
func WorkspaceIngestionPolicyPath(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "security", "workspaceingestion.policy.yaml")
}

type workspaceIngestionPolicyFile struct {
	Rules []PolicyRule `yaml:"rules,omitempty"`
}

// LoadWorkspaceIngestionPolicy loads and validates the workspace ingestion policy file.
func LoadWorkspaceIngestionPolicy(path, workspace string, decode Decoder) ([]PolicyRule, error) {
	var file workspaceIngestionPolicyFile
	if err := loadAndDecode(path, workspace, decode, WorkspaceIngestionPolicyPath, &file); err != nil {
		return nil, err
	}
	if err := validatePolicyRules(file.Rules); err != nil {
		return nil, err
	}
	return append([]PolicyRule(nil), file.Rules...), nil
}

func validatePolicyRules(rules []PolicyRule) error {
	for i, rule := range rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rules[%d] invalid: %w", i, err)
		}
	}
	return nil
}
