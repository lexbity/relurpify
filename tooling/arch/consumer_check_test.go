package arch

import (
	"testing"
)

func TestCheckConsumers_noViolation(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: "codeburg.org/lexbit/relurpify/a", Name: "a", GoFiles: []string{"a.go"}},
		{ImportPath: "codeburg.org/lexbit/relurpify/b", Name: "b", GoFiles: []string{"b.go"}},
		{ImportPath: "codeburg.org/lexbit/relurpify/c", Name: "c", GoFiles: []string{"c.go"}},
		{ImportPath: "codeburg.org/lexbit/relurpify/d", Name: "d", GoFiles: []string{"d.go"}},
	}
	reverse := map[string][]string{
		"codeburg.org/lexbit/relurpify/a": {"codeburg.org/lexbit/relurpify/b"},
		"codeburg.org/lexbit/relurpify/b": {"codeburg.org/lexbit/relurpify/c"},
		"codeburg.org/lexbit/relurpify/c": {"codeburg.org/lexbit/relurpify/d"},
		"codeburg.org/lexbit/relurpify/d": {"codeburg.org/lexbit/relurpify/a"},
	}
	violations := CheckConsumers(pkgs, reverse, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("expected no consumer violations, got %v", violations)
	}
}

func TestCheckConsumers_unusedPackage(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: "codeburg.org/lexbit/relurpify/a", Name: "a", GoFiles: []string{"a.go"}},
		{ImportPath: "codeburg.org/lexbit/relurpify/unused", Name: "unused", GoFiles: []string{"unused.go"}},
	}
	reverse := map[string][]string{
		"codeburg.org/lexbit/relurpify/a":      {},
		"codeburg.org/lexbit/relurpify/unused": {},
	}
	violations := CheckConsumers(pkgs, reverse, Allowlist{})
	if len(violations) == 0 {
		t.Fatal("expected consumer violation for unused package")
	}
}

func TestCheckConsumers_mainPackage(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: "codeburg.org/lexbit/relurpify/cmd/tool", Name: "main", GoFiles: []string{"main.go"}},
	}
	reverse := map[string][]string{
		"codeburg.org/lexbit/relurpify/cmd/tool": {},
	}
	violations := CheckConsumers(pkgs, reverse, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("main package should be exempt, got %v", violations)
	}
}

func TestCheckConsumers_testOnlyPackage(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath:      "codeburg.org/lexbit/relurpify/testhelper",
			Name:            "testhelper",
			GoFiles:         []string{},
			TestGoFiles:     []string{"helper_test.go"},
			OnlyTestGoFiles: true,
		},
	}
	reverse := map[string][]string{}
	violations := CheckConsumers(pkgs, reverse, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("test-only package should be exempt, got %v", violations)
	}
}

func TestCheckConsumers_allowlist(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: "codeburg.org/lexbit/relurpify/unused", Name: "unused", GoFiles: []string{"unused.go"}},
	}
	reverse := map[string][]string{}
	allowlist := Allowlist{entries: map[string]map[string]bool{
		"consumer": {"consumer: codeburg.org/lexbit/relurpify/unused has no non-test importers": true},
	}}
	violations := CheckConsumers(pkgs, reverse, allowlist)
	if len(violations) != 0 {
		t.Errorf("expected allowlist to exempt consumer violation, got %v", violations)
	}
}
