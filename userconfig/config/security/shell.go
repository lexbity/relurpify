package security

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ShellPolicyPath returns the canonical shell policy location.
func ShellPolicyPath(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "security", "shell.policy.yaml")
}

type shellPolicyFile struct {
	Rules []BlacklistRule `yaml:"rules,omitempty"`
}

// LoadShellPolicy loads and validates the shell policy file.
func LoadShellPolicy(path, workspace string, decode Decoder) (*ShellBlacklist, error) {
	var file shellPolicyFile
	if err := loadAndDecode(path, workspace, decode, ShellPolicyPath, &file); err != nil {
		return nil, err
	}
	if err := validateShellBlacklistRules(file.Rules); err != nil {
		return nil, err
	}
	return &ShellBlacklist{Rules: append([]BlacklistRule(nil), file.Rules...)}, nil
}

func validateShellBlacklistRules(rules []BlacklistRule) error {
	for i, rule := range rules {
		if strings.TrimSpace(rule.ID) == "" {
			return fmt.Errorf("shell blacklist rule[%d] missing id", i)
		}
		switch strings.ToLower(strings.TrimSpace(rule.Action)) {
		case "block", "hitl":
		default:
			return fmt.Errorf("shell blacklist rule %q action=%q invalid", rule.ID, rule.Action)
		}
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("shell blacklist rule %q pattern invalid: %w", rule.ID, err)
		}
	}
	return nil
}
