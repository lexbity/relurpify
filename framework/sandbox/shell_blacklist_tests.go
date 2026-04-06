//go:build !integration
// +build !integration

package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShellBlacklistLoadMissingFile tests that Load() with a missing file returns an empty blacklist, no error.
func TestShellBlacklistLoadMissingFile(t *testing.T) {
	bl, err := Load(filepath.Join(os.TempDir(), "nonexistent_blacklist.yaml"))
	if err != nil {
		t.Errorf("Load() with missing file returned error: %v", err)
	}
	if bl != nil && len(bl.rules) != 0 {
		t.Errorf("Load() with missing file should return empty blacklist, got %d rules", len(bl.rules))
	}
}

// TestShellBlacklistLoadValidYAML tests that Load() with a valid YAML compiles patterns correctly.
func TestShellBlacklistLoadValidYAML(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: test-block-rule
    pattern: "rm\s+-rf"
    reason: "Dangerous rm command detected"
    action: block
  - id: test-hitl-rule
    pattern: "curl.*http://"
    reason: "Unverified curl usage"
    action: hitl`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Errorf("Load() with valid YAML returned error: %v", err)
	}
	if bl == nil {
		t.Error("Load() with valid YAML should return non-nil blacklist")
	}
	if len(bl.rules) != 2 {
		t.Errorf("Load() with valid YAML should load 2 rules, got %d", len(bl.rules))
	}

	// Verify both rules are properly compiled
	if !bl.rules[0].Pattern.MatchString("rm -rf /etc/passwd") {
		t.Error("block rule pattern not matched correctly")
	}
	if !bl.rules[1].Pattern.MatchString("curl http://example.com") {
		t.Error("hitl rule pattern not matched correctly")
	}
}

// TestShellBlacklistLoadInvalidRegex tests that Load() with an invalid regex returns a compile error.
func TestShellBlacklistLoadInvalidRegex(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: invalid-regex-rule
    pattern: "[invalid(regex"
    reason: "Invalid regex pattern"
    action: block`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err == nil {
		t.Error("Load() with invalid regex should return compile error")
	}
	if bl != nil && len(bl.rules) > 0 {
		t.Errorf("Load() with invalid regex should not load rules that failed compilation, got %d rules", len(bl.rules))
	}
}

// TestShellBlacklistCheckEmpty returns correct behavior for empty blacklist.
func TestShellBlacklistCheckEmpty(t *testing.T) {
	if bl := &ShellBlacklist{}; bl.Check("echo hello") != nil {
		t.Error("empty blacklist Check() should return nil on any command")
	}
}

// TestShellBlacklistCheckNil tests that Check() on a nil/empty blacklist always returns nil.
func TestShellBlacklistCheckNil(t *testing.T) {
	var bl *ShellBlacklist = nil
	if bl != nil && bl.Check("echo hello") != nil {
		t.Error("nil blacklist Check() should return nil")
	}
}

// TestShellBlacklistCheckBlockRule tests that block rule is matched and returned.
func TestShellBlacklistCheckBlockRule(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: test-block-rule
    pattern: "rm\s+-rf"
    reason: "Dangerous rm command detected"
    action: block`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	rule := bl.Check("rm -rf /")
	if rule == nil {
		t.Error("Check() should return block rule for matching command")
	}
	if rule.ID != "test-block-rule" {
		t.Errorf("Check() returned wrong rule ID: %s", rule.ID)
	}
	if rule.Action != BlacklistActionBlock {
		t.Errorf("Check() returned wrong action: %s", rule.Action)
	}
}

// TestShellBlacklistCheckHITLRule tests that hitl rule is matched and returned.
func TestShellBlacklistCheckHITLRule(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: test-hitl-rule
    pattern: "curl.*http://"
    reason: "Unverified curl usage"
    action: hitl`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	rule := bl.Check("curl http://test.com")
	if rule == nil {
		t.Error("Check() should return HITL rule for matching command")
	}
	if rule.ID != "test-hitl-rule" {
		t.Errorf("Check() returned wrong rule ID: %s", rule.ID)
	}
	if rule.Action != BlacklistActionHITL {
		t.Errorf("Check() returned wrong action: %s", rule.Action)
	}
}

// TestShellBlacklistCheckNoMatch tests that Check() returns nil when no rule matches.
func TestShellBlacklistCheckNoMatch(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: test-block-rule
    pattern: "rm\s+-rf"
    reason: "Dangerous rm command detected"
    action: block`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	rule := bl.Check("echo hello")
	if rule != nil {
		t.Error("Check() should return nil for non-matching command")
	}
}

// TestShellBlacklistCheckShortCircuit tests that Check() only returns first matching rule.
func TestShellBlacklistCheckShortCircuit(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: block-rule-first
    pattern: "echo"
    reason: "Block echo command"
    action: block
  - id: hitl-rule-second
    pattern: "echo.*test"
    reason: "HITL for test echo"
    action: hitl`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	rule := bl.Check("echo hello")
	if rule == nil {
		t.Error("Check() should return first matching rule only")
	}
	if rule.ID != "block-rule-first" {
		t.Errorf("Check() returned second rule instead of first: %s", rule.ID)
	}
	if rule.Action != BlacklistActionBlock {
		t.Errorf("Check() returned second rule action instead of first: %s", rule.Action)
	}
}

// TestShellBlacklistLoadEmptyRules tests loading YAML with empty rules array.
func TestShellBlacklistLoadEmptyRules(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules: []`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Errorf("Load() with empty rules array returned error: %v", err)
	}
	if len(bl.rules) != 0 {
		t.Errorf("Load() with empty rules should result in 0 rules, got %d", len(bl.rules))
	}

	rule := bl.Check("echo hello")
	if rule != nil {
		t.Error("Check() on empty rules blacklist should return nil")
	}
}

// TestShellBlacklistCheckEmptyCommand tests that empty command string still gets checked.
func TestShellBlacklistCheckEmptyCommand(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: empty-pattern-rule
    pattern: ".*"
    reason: "Match everything"
    action: block`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	rule := bl.Check("")
	if rule == nil {
		t.Error("Check() with empty command should still match .* pattern")
	}
}

// TestShellBlacklistRuleFields tests that all fields are preserved when loading rules.
func TestShellBlacklistRuleFields(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: complete-rule-test
    pattern: "test.*"
    reason: "Test rule for field preservation"
    action: block`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	rule := bl.rules[0]
	if rule.ID != "complete-rule-test" {
		t.Errorf("rule.ID not preserved: expected 'complete-rule-test', got '%s'", rule.ID)
	}
	if rule.Raw != "test.*" {
		t.Errorf("rule.Raw not preserved: expected 'test.*', got '%s'", rule.Raw)
	}
	if rule.Reason != "Test rule for field preservation" {
		t.Errorf("rule.Reason not preserved: expected 'Test rule for field preservation', got '%s'", rule.Reason)
	}
	if rule.Action != BlacklistActionBlock {
		t.Errorf("rule.Action not preserved: expected 'block', got '%s'", rule.Action)
	}
}

// TestShellBlacklistMultiplePatterns tests complex regex patterns.
func TestShellBlacklistMultiplePatterns(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: multi-glob-rule
    pattern: "ls (|-a|^-l)"
    reason: "Block ls with flags"
    action: block`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	rule := bl.Check("ls -l /etc")
	if rule == nil {
		t.Error("Check() should match pattern 'ls (-|-a|^-l)' for 'ls -l'")
	}
}

// TestShellBlacklistCaseSensitive tests that patterns are case-sensitive.
func TestShellBlacklistCaseSensitive(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	yamlContent := `version: 1.0
rules:
  - id: case-sensitive-test
    pattern: "echo.*[Hh]ello"
    reason: "Case sensitive test"
    action: block`

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	rule1 := bl.Check("echo Hello World") // Capital H
	rule2 := bl.Check("echo hello World") // lowercase h
	
	if rule1 == nil {
		t.Error("Check() should match uppercase 'H' in pattern")
	}
	if rule2 != nil {
		t.Error("Check() should NOT match lowercase 'h' in case-sensitive pattern")
	}
}

// TestShellBlacklistLargeRules tests that many rules can be loaded.
func TestShellBlacklistLargeRules(t *testing.T) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "test_*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	var content strings.Builder
	content.WriteString(`version: 1.0\nrules:\n`)
	for i := 1; i <= 100; i++ {
		content.WriteString(fmt.Sprintf("  - id: rule-%d\n", i))
		content.WriteString(fmt.Sprintf("    pattern: \"rule%d.*\"\n", i))
		content.WriteString(`    reason: "Rule %d\n`, i)
		content.WriteString(fmt.Sprintf("    action: block\n\n"))
	}

	if _, err := tmpFile.Write([]byte(content.String())); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		t.Errorf("Load() with large ruleset returned error: %v", err)
	}
	if len(bl.rules) < 100 {
		t.Errorf("Load() should load all 100 rules, got %d", len(bl.rules))
	}

	rule := bl.Check("rule02.*")
	if rule == nil || rule.ID != "rule-02" {
		t.Error("Check() should find correct rule in large ruleset")
	}
}
