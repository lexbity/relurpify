//go:build !integration
// +build !integration

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellBlacklistLoadMissingFile(t *testing.T) {
	t.Parallel()

	bl, err := Load(filepath.Join(t.TempDir(), "missing_blacklist.yaml"))
	if err != nil {
		t.Fatalf("Load() with missing file returned error: %v", err)
	}
	if bl == nil {
		t.Fatal("Load() with missing file returned nil blacklist")
	}
	if got := len(bl.rules); got != 0 {
		t.Fatalf("Load() with missing file should return empty blacklist, got %d rules", got)
	}
}

func TestShellBlacklistLoadFromDirectoryReturnsError(t *testing.T) {
	t.Parallel()

	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("Load() on a directory should return an error")
	}
}

func TestShellBlacklistLoadInvalidYAML(t *testing.T) {
	t.Parallel()

	path := writeBlacklistFixture(t, "version: [broken")
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() with invalid YAML should return an error")
	}
}

func TestShellBlacklistLoadInvalidRegex(t *testing.T) {
	t.Parallel()

	path := writeBlacklistFixture(t, `version: 1.0
rules:
  - id: invalid-regex-rule
    pattern: "[invalid(regex"
    reason: "Invalid regex pattern"
    action: block`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() with invalid regex should return an error")
	}
}

func TestShellBlacklistLoadValidYAML(t *testing.T) {
	t.Parallel()

	path := writeBlacklistFixture(t, `version: 1.0
rules:
  - id: test-block-rule
    pattern: "rm\\s+-rf"
    reason: "Dangerous rm command detected"
    action: block
  - id: test-hitl-rule
    pattern: "curl.*http://"
    reason: "Unverified curl usage"
    action: hitl`)

	bl, err := Load(path)
	if err != nil {
		t.Fatalf("Load() with valid YAML returned error: %v", err)
	}
	if bl == nil {
		t.Fatal("Load() with valid YAML returned nil blacklist")
	}
	if got := len(bl.rules); got != 2 {
		t.Fatalf("Load() with valid YAML should load 2 rules, got %d", got)
	}

	if !bl.rules[0].Pattern.MatchString("rm -rf /etc/passwd") {
		t.Error("block rule pattern not matched correctly")
	}
	if !bl.rules[1].Pattern.MatchString("curl http://example.com") {
		t.Error("hitl rule pattern not matched correctly")
	}
	if bl.rules[0].ID != "test-block-rule" || bl.rules[0].Action != BlacklistActionBlock {
		t.Fatalf("first rule fields were not preserved: %+v", bl.rules[0])
	}
	if bl.rules[1].ID != "test-hitl-rule" || bl.rules[1].Action != BlacklistActionHITL {
		t.Fatalf("second rule fields were not preserved: %+v", bl.rules[1])
	}
}

func TestShellBlacklistLoadEmptyRules(t *testing.T) {
	t.Parallel()

	path := writeBlacklistFixture(t, `version: 1.0
rules: []`)

	bl, err := Load(path)
	if err != nil {
		t.Fatalf("Load() with empty rules array returned error: %v", err)
	}
	if got := len(bl.rules); got != 0 {
		t.Fatalf("Load() with empty rules should result in 0 rules, got %d", got)
	}
	if rule := bl.Check("echo hello"); rule != nil {
		t.Fatalf("Check() on empty blacklist returned %#v", rule)
	}
}

func TestShellBlacklistCheckNil(t *testing.T) {
	t.Parallel()

	var bl *ShellBlacklist
	if rule := bl.Check("echo hello"); rule != nil {
		t.Fatalf("nil blacklist Check() should return nil, got %#v", rule)
	}
}

func TestShellBlacklistCheckEmpty(t *testing.T) {
	t.Parallel()

	bl := &ShellBlacklist{}
	if rule := bl.Check("echo hello"); rule != nil {
		t.Fatalf("empty blacklist Check() should return nil, got %#v", rule)
	}
}

func TestShellBlacklistCheckBlockRule(t *testing.T) {
	t.Parallel()

	bl := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: test-block-rule
    pattern: "rm\\s+-rf"
    reason: "Dangerous rm command detected"
    action: block`)

	rule := bl.Check("rm -rf /")
	if rule == nil {
		t.Fatal("Check() should return a matching block rule")
	}
	if rule.ID != "test-block-rule" {
		t.Fatalf("Check() returned wrong rule ID: %s", rule.ID)
	}
	if rule.Action != BlacklistActionBlock {
		t.Fatalf("Check() returned wrong action: %s", rule.Action)
	}
}

func TestShellBlacklistCheckHITLRule(t *testing.T) {
	t.Parallel()

	bl := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: test-hitl-rule
    pattern: "curl.*http://"
    reason: "Unverified curl usage"
    action: hitl`)

	rule := bl.Check("curl http://test.com")
	if rule == nil {
		t.Fatal("Check() should return a matching HITL rule")
	}
	if rule.ID != "test-hitl-rule" {
		t.Fatalf("Check() returned wrong rule ID: %s", rule.ID)
	}
	if rule.Action != BlacklistActionHITL {
		t.Fatalf("Check() returned wrong action: %s", rule.Action)
	}
}

func TestShellBlacklistCheckNoMatch(t *testing.T) {
	t.Parallel()

	bl := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: test-block-rule
    pattern: "rm\\s+-rf"
    reason: "Dangerous rm command detected"
    action: block`)

	if rule := bl.Check("echo hello"); rule != nil {
		t.Fatalf("Check() should return nil for non-matching command, got %#v", rule)
	}
}

func TestShellBlacklistCheckShortCircuit(t *testing.T) {
	t.Parallel()

	bl := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: block-rule-first
    pattern: "echo"
    reason: "Block echo command"
    action: block
  - id: hitl-rule-second
    pattern: "echo.*test"
    reason: "HITL for test echo"
    action: hitl`)

	rule := bl.Check("echo hello")
	if rule == nil {
		t.Fatal("Check() should return the first matching rule")
	}
	if rule.ID != "block-rule-first" {
		t.Fatalf("Check() returned second rule instead of first: %s", rule.ID)
	}
	if rule.Action != BlacklistActionBlock {
		t.Fatalf("Check() returned wrong action: %s", rule.Action)
	}
}

func TestShellBlacklistCheckEmptyCommand(t *testing.T) {
	t.Parallel()

	bl := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: empty-pattern-rule
    pattern: ".*"
    reason: "Match everything"
    action: block`)

	if rule := bl.Check(""); rule == nil {
		t.Fatal("Check() with empty command should still match .* pattern")
	}
}

func TestShellBlacklistRuleFields(t *testing.T) {
	t.Parallel()

	bl := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: complete-rule-test
    pattern: "test.*"
    reason: "Test rule for field preservation"
    action: block`)

	if got := bl.rules[0]; got.ID != "complete-rule-test" || got.Raw != "test.*" || got.Reason != "Test rule for field preservation" || got.Action != BlacklistActionBlock {
		t.Fatalf("rule fields were not preserved: %+v", got)
	}
}

func TestShellBlacklistMultiplePatterns(t *testing.T) {
	t.Parallel()

	bl := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: multi-glob-rule
    pattern: "ls (|-a|^-l)"
    reason: "Block ls with flags"
    action: block`)

	if rule := bl.Check("ls -l /etc"); rule == nil {
		t.Fatal("Check() should match the expected pattern")
	}
}

func TestShellBlacklistCaseSensitive(t *testing.T) {
	t.Parallel()

	bl := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: case-sensitive-test
    pattern: "echo.*[Hh]ello"
    reason: "Case sensitive test"
    action: block`)

	if rule := bl.Check("echo Hello World"); rule == nil {
		t.Fatal("Check() should match uppercase H in the pattern")
	}
	if rule := bl.Check("echo hello World"); rule == nil {
		t.Fatal("Check() should match lowercase h in the pattern")
	}
}

func TestShellBlacklistLargeRules(t *testing.T) {
	t.Parallel()

	var content strings.Builder
	content.WriteString("version: 1.0\nrules:\n")
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&content, "  - id: rule-%d\n", i)
		fmt.Fprintf(&content, "    pattern: \"^rule-%d$\"\n", i)
		fmt.Fprintf(&content, "    reason: \"Rule %d\"\n", i)
		fmt.Fprintf(&content, "    action: block\n")
	}

	bl := mustLoadBlacklistFromYAML(t, content.String())
	if got := len(bl.rules); got != 100 {
		t.Fatalf("expected 100 rules, got %d", got)
	}
	if rule := bl.Check("rule-100"); rule == nil || rule.ID != "rule-100" {
		t.Fatalf("expected to match rule-100, got %#v", rule)
	}
}

func writeBlacklistFixture(t *testing.T, yamlContent string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "shell_blacklist.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	return path
}

func mustLoadBlacklistFromYAML(t *testing.T, yamlContent string) *ShellBlacklist {
	t.Helper()

	bl, err := Load(writeBlacklistFixture(t, yamlContent))
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	return bl
}
