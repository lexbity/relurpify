package arch

import (
	"testing"
)

const (
	ImportPathA_consumer_check_test = "codeburg.org/lexbit/relurpify/a"
	ImportPathB_consumer_check_test = "codeburg.org/lexbit/relurpify/b"
	ImportPathC_consumer_check_test = "codeburg.org/lexbit/relurpify/c"
	ImportPathD_consumer_check_test = "codeburg.org/lexbit/relurpify/d"
	ImportPathUnused_consumer_check_test = "codeburg.org/lexbit/relurpify/unused"
)


func TestCheckConsumers_noViolation(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: ImportPathA_consumer_check_test, Name: "a", GoFiles: []string{"a.go"}},
		{ImportPath: ImportPathB_consumer_check_test, Name: "b", GoFiles: []string{"b.go"}},
		{ImportPath: ImportPathC_consumer_check_test, Name: "c", GoFiles: []string{"c.go"}},
		{ImportPath: ImportPathD_consumer_check_test, Name: "d", GoFiles: []string{"d.go"}},
	}
	reverse := map[string][]string{
		ImportPathA_consumer_check_test: {ImportPathB_consumer_check_test},
		ImportPathB_consumer_check_test: {ImportPathC_consumer_check_test},
		ImportPathC_consumer_check_test: {ImportPathD_consumer_check_test},
		ImportPathD_consumer_check_test: {ImportPathA_consumer_check_test},
	}
	violations := CheckConsumers(pkgs, reverse, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("expected no consumer violations, got %v", violations)
	}
}

func TestCheckConsumers_unusedPackage(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: ImportPathA_consumer_check_test, Name: "a", GoFiles: []string{"a.go"}},
		{ImportPath: ImportPathUnused_consumer_check_test, Name: "unused", GoFiles: []string{"unused.go"}},
	}
	reverse := map[string][]string{
		ImportPathA_consumer_check_test:      {},
		ImportPathUnused_consumer_check_test: {},
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
		{ImportPath: ImportPathUnused_consumer_check_test, Name: "unused", GoFiles: []string{"unused.go"}},
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
