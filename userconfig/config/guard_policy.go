package config

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// GuardSection identifies a canonical policy document, never a caller path.
type GuardSection string

// Canonical guard policy sections.
const (
	GuardShell     GuardSection = "shell"
	GuardIngestion GuardSection = "ingestion"
)

// GuardPolicyDocument is a revision-bound read-only source snapshot.
type GuardPolicyDocument struct {
	Section          GuardSection
	Source, Revision string
}

// GuardPolicyRules contains decoded policy values for one source snapshot.
type GuardPolicyRules struct {
	Shell     []security.BlacklistRule
	Ingestion []security.PolicyRule
}

func guardRelative(section GuardSection) (string, error) {
	switch section {
	case GuardShell:
		return "relurpify_cfg/security/shell.policy.yaml", nil
	case GuardIngestion:
		return "relurpify_cfg/security/workspaceingestion.policy.yaml", nil
	default:
		return "", fmt.Errorf("unknown guard policy section %q", section)
	}
}

// ParseGuardPolicy decodes bounded canonical policy declarations for inspection.
// It does not grant permission, contact a provider, or execute a sample command.
func ParseGuardPolicy(section GuardSection, source string) (GuardPolicyRules, error) {
	path, err := guardRelative(section)
	if err != nil {
		return GuardPolicyRules{}, err
	}
	if len(source) > PolicySourceLimit || !utf8.ValidString(source) {
		return GuardPolicyRules{}, fmt.Errorf("guard source requires at most 1 MiB of UTF-8")
	}
	rules := GuardPolicyRules{}
	var declaration SchemaDeclaration
	if section == GuardShell {
		var document security.ShellBlacklist
		declaration, err = DecodeWithSchema(path, []byte(source), NewSchemaRegistry(), &document)
		rules.Shell = document.Rules
	} else {
		var document struct {
			Rules []security.PolicyRule `yaml:"rules"`
		}
		declaration, err = DecodeWithSchema(path, []byte(source), NewSchemaRegistry(), &document)
		rules.Ingestion = document.Rules
	}
	if err != nil {
		return GuardPolicyRules{}, err
	}
	if declaration.Raw != "relurpify/policy/"+string(section)+"/v1" {
		return GuardPolicyRules{}, fmt.Errorf("source schema does not match selected guard section")
	}
	if len(rules.Shell)+len(rules.Ingestion) > 256 {
		return GuardPolicyRules{}, fmt.Errorf("guard policy supports at most 256 rules per section")
	}
	// Stable, unique IDs are required for selected-rule tests. An ambiguous
	// declaration must never silently test a different rule with the same ID.
	seen := make(map[string]bool)
	checkID := func(id string) error {
		if strings.TrimSpace(id) == "" || seen[id] {
			return fmt.Errorf("guard rule IDs must be nonempty and unique: %q", id)
		}
		seen[id] = true
		return nil
	}
	for _, rule := range rules.Shell {
		if err := checkID(rule.ID); err != nil {
			return GuardPolicyRules{}, err
		}
	}
	for _, rule := range rules.Ingestion {
		if err := checkID(rule.ID); err != nil {
			return GuardPolicyRules{}, err
		}
	}

	return rules, nil
}

// ReadGuardPolicy reads one bounded canonical source without materializing files.
func ReadGuardPolicy(ctx context.Context, workspace string, section GuardSection) (document GuardPolicyDocument, err error) {
	if err := ctx.Err(); err != nil {
		return document, err
	}
	relative, err := guardRelative(section)
	if err != nil {
		return document, err
	}
	data, revision, err := ReadWorkspacePolicySource(ctx, workspace, filepath.Base(relative))
	if err != nil {
		return document, err
	}
	source := string(data)
	// Invalid disk source is still returned for inspection and repair. Parsing
	// diagnostics belong to the inspector; read failure must never become empty rules.
	return GuardPolicyDocument{Section: section, Source: source, Revision: revision}, ctx.Err()
}
