package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// SaveLocalToolPolicy atomically writes a local tool policy file with backup.
func SaveLocalToolPolicy(path string, tools map[string]security.ToolPolicy) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path required")
	}
	if err := security.ValidateLocalToolPolicies(tools); err != nil {
		return fmt.Errorf("validate local tool policies: %w", err)
	}
	if _, err := CreateTimestampedBackup(path); err != nil {
		return fmt.Errorf("backup local tool policy: %w", err)
	}
	if err := WriteWithSchema(path, "relurpify/policy/localtool/v1", security.LocalToolPolicyFile{
		Tools: tools,
	}); err != nil {
		return fmt.Errorf("write local tool policy: %w", err)
	}
	return nil
}

// SaveSandboxPolicy atomically writes a sandbox policy file with backup.
func SaveSandboxPolicy(path string, policy *security.SandboxPolicy) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path required")
	}
	if policy == nil {
		return fmt.Errorf("sandbox policy required")
	}
	if _, err := CreateTimestampedBackup(path); err != nil {
		return fmt.Errorf("backup sandbox policy: %w", err)
	}
	if err := WriteWithSchema(path, "relurpify/policy/sandbox/v1", policy); err != nil {
		return fmt.Errorf("write sandbox policy: %w", err)
	}
	return nil
}

// ShellPolicyPath returns the canonical shell policy path.
func ShellPolicyPath(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "security", "shell.policy.yaml")
}
