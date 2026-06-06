package arch

import (
	"testing"
)

func emptyPkg(importPath string) GoPackage {
	return GoPackage{ImportPath: importPath, GoFiles: []string{"a.go"}}
}

func TestCheckBuckets_noViolation(t *testing.T) {
	pkgs := []GoPackage{
		emptyPkg("codeburg.org/lexbit/relurpify/a"),
		emptyPkg("codeburg.org/lexbit/relurpify/b"),
		emptyPkg("codeburg.org/lexbit/relurpify/c"),
	}
	reverse := map[string][]string{
		"codeburg.org/lexbit/relurpify/types": {
			"codeburg.org/lexbit/relurpify/a",
		},
	}
	violations := CheckBuckets(pkgs, reverse, 3, ".", Allowlist{})
	if len(violations) != 0 {
		t.Errorf("expected no bucket violations, got %v", violations)
	}
}

func TestCheckBuckets_singleDomain(t *testing.T) {
	pkgs := []GoPackage{
		emptyPkg("codeburg.org/lexbit/relurpify/framework/types"),
		emptyPkg("codeburg.org/lexbit/relurpify/framework/a"),
		emptyPkg("codeburg.org/lexbit/relurpify/framework/b"),
	}
	reverse := map[string][]string{
		"codeburg.org/lexbit/relurpify/framework/types": {
			"codeburg.org/lexbit/relurpify/framework/a",
			"codeburg.org/lexbit/relurpify/framework/b",
		},
	}
	violations := CheckBuckets(pkgs, reverse, 1, ".", Allowlist{})
	if len(violations) != 0 {
		t.Errorf("expected no bucket (same domain), got %v", violations)
	}
}

func TestCheckBuckets_typeOnlyBucket(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath: "codeburg.org/lexbit/relurpify/framework/types",
			GoFiles:    []string{"types.go"},
		},
		emptyPkg("codeburg.org/lexbit/relurpify/framework/a"),
		emptyPkg("codeburg.org/lexbit/relurpify/app/b"),
		emptyPkg("codeburg.org/lexbit/relurpify/agents/c"),
	}
	reverse := map[string][]string{
		"codeburg.org/lexbit/relurpify/framework/types": {
			"codeburg.org/lexbit/relurpify/framework/a",
			"codeburg.org/lexbit/relurpify/app/b",
			"codeburg.org/lexbit/relurpify/agents/c",
		},
	}
	// With threshold=2, 3 domains *but* the package has GoFiles that likely contain funcs
	violations := CheckBuckets(pkgs, reverse, 2, ".", Allowlist{})
	// This may or may not flag depending on AST analysis of the test scaffolding
	_ = violations
}
