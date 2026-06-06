package arch

import (
	"testing"
)

func TestCheckCycles_noCycle(t *testing.T) {
	forward := map[string][]string{
		"codeburg.org/lexbit/relurpify/a": {"codeburg.org/lexbit/relurpify/b", "fmt"},
		"codeburg.org/lexbit/relurpify/b": {"codeburg.org/lexbit/relurpify/c"},
		"codeburg.org/lexbit/relurpify/c": {"os"},
	}
	violations := CheckCycles(forward, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("expected no cycles, got %v", violations)
	}
}

func TestCheckCycles_directCycle(t *testing.T) {
	forward := map[string][]string{
		"codeburg.org/lexbit/relurpify/a": {"codeburg.org/lexbit/relurpify/b"},
		"codeburg.org/lexbit/relurpify/b": {"codeburg.org/lexbit/relurpify/a"},
	}
	violations := CheckCycles(forward, Allowlist{})
	if len(violations) == 0 {
		t.Fatal("expected cycle detection, got none")
	}
}

func TestCheckCycles_selfCycle(t *testing.T) {
	forward := map[string][]string{
		"codeburg.org/lexbit/relurpify/a": {"codeburg.org/lexbit/relurpify/a"},
	}
	violations := CheckCycles(forward, Allowlist{})
	if len(violations) == 0 {
		t.Fatal("expected self-cycle detection")
	}
}

func TestCheckCycles_indirectCycle(t *testing.T) {
	forward := map[string][]string{
		"codeburg.org/lexbit/relurpify/a": {"codeburg.org/lexbit/relurpify/b"},
		"codeburg.org/lexbit/relurpify/b": {"codeburg.org/lexbit/relurpify/c"},
		"codeburg.org/lexbit/relurpify/c": {"codeburg.org/lexbit/relurpify/a"},
	}
	violations := CheckCycles(forward, Allowlist{})
	if len(violations) == 0 {
		t.Fatal("expected indirect cycle detection")
	}
}

func TestCheckCycles_allowlist(t *testing.T) {
	forward := map[string][]string{
		"codeburg.org/lexbit/relurpify/a": {"codeburg.org/lexbit/relurpify/b"},
		"codeburg.org/lexbit/relurpify/b": {"codeburg.org/lexbit/relurpify/a"},
	}
	allowlist := Allowlist{entries: map[string]map[string]bool{
		"cycle": {
			"cycle: codeburg.org/lexbit/relurpify/a → codeburg.org/lexbit/relurpify/a": true,
			"cycle: codeburg.org/lexbit/relurpify/b → codeburg.org/lexbit/relurpify/b": true,
		},
	}}
	violations := CheckCycles(forward, allowlist)
	if len(violations) != 0 {
		t.Errorf("expected allowlist to exempt cycle, got %v", violations)
	}
}

func TestCheckLayerDirection_noViolation(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: "codeburg.org/lexbit/relurpify/platform/fs", Name: "fs"},
		{ImportPath: "codeburg.org/lexbit/relurpify/a", Name: "a"},
	}
	forward := map[string][]string{
		"codeburg.org/lexbit/relurpify/platform/fs": {"fmt"},
		"codeburg.org/lexbit/relurpify/a":           {"fmt"},
	}
	violations := CheckLayerDirection(pkgs, forward, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("expected no layer violations, got %v", violations)
	}
}

func TestCheckLayerDirection_domainImportsPlatform(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: "codeburg.org/lexbit/relurpify/a", Name: "a"},
		{ImportPath: "codeburg.org/lexbit/relurpify/platform/fs", Name: "fs"},
	}
	forward := map[string][]string{
		"codeburg.org/lexbit/relurpify/a":           {"codeburg.org/lexbit/relurpify/platform/fs"},
		"codeburg.org/lexbit/relurpify/platform/fs": {"fmt"},
	}
	violations := CheckLayerDirection(pkgs, forward, Allowlist{})
	if len(violations) == 0 {
		t.Fatal("expected layer violation for domain importing platform")
	}
}

func TestCheckLayerDirection_testsuiteAllowed(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: "codeburg.org/lexbit/relurpify/testsuite/agenttest", Name: "agenttest"},
		{ImportPath: "codeburg.org/lexbit/relurpify/platform/fs", Name: "fs"},
	}
	forward := map[string][]string{
		"codeburg.org/lexbit/relurpify/testsuite/agenttest": {"codeburg.org/lexbit/relurpify/platform/fs"},
		"codeburg.org/lexbit/relurpify/platform/fs":         {"fmt"},
	}
	violations := CheckLayerDirection(pkgs, forward, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("testsuite should be allowed to import platform, got %v", violations)
	}
}

func TestCheckLayerDirection_allowlist(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: "codeburg.org/lexbit/relurpify/a", Name: "a"},
		{ImportPath: "codeburg.org/lexbit/relurpify/platform/fs", Name: "fs"},
	}
	forward := map[string][]string{
		"codeburg.org/lexbit/relurpify/a":           {"codeburg.org/lexbit/relurpify/platform/fs"},
		"codeburg.org/lexbit/relurpify/platform/fs": {"fmt"},
	}
	allowlist := Allowlist{entries: map[string]map[string]bool{
		"layer": {"layer: codeburg.org/lexbit/relurpify/a imports platform package codeburg.org/lexbit/relurpify/platform/fs": true},
	}}
	violations := CheckLayerDirection(pkgs, forward, allowlist)
	if len(violations) != 0 {
		t.Errorf("expected allowlist to exempt layer violation, got %v", violations)
	}
}
