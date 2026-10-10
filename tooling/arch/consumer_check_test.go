package arch

import (
	"testing"
)

const (
	ImportPathA_consumer_check_test      = "codeburg.org/lexbit/relurpify/a"
	ImportPathB_consumer_check_test      = "codeburg.org/lexbit/relurpify/b"
	ImportPathC_consumer_check_test      = "codeburg.org/lexbit/relurpify/c"
	ImportPathD_consumer_check_test      = "codeburg.org/lexbit/relurpify/d"
	ImportPathUnused_consumer_check_test = "codeburg.org/lexbit/relurpify/unused"
)

func TestCheckConsumers_noViolation(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: ImportPathA_consumer_check_test, Name: "a", GoFiles: []string{"a.go"}},
		{ImportPath: ImportPathB_consumer_check_test, Name: "b", GoFiles: []string{"b.go"}},
		{ImportPath: ImportPathC_consumer_check_test, Name: "c", GoFiles: []string{"c.go"}},
		{ImportPath: ImportPathD_consumer_check_test, Name: "d", GoFiles: []string{"d.go"}},
	}
	// a imports b, b imports c, c imports d, d imports a (production imports).
	setImports := func(pkg GoPackage, imp string) GoPackage {
		pkg.Imports = []string{imp}
		return pkg
	}
	pkgs[0] = setImports(pkgs[0], ImportPathB_consumer_check_test)
	pkgs[1] = setImports(pkgs[1], ImportPathC_consumer_check_test)
	pkgs[2] = setImports(pkgs[2], ImportPathD_consumer_check_test)
	pkgs[3] = setImports(pkgs[3], ImportPathA_consumer_check_test)
	violations := CheckConsumers(pkgs, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("expected no consumer violations, got %v", violations)
	}
}

func TestCheckConsumers_testOnlyConsumedPackage(t *testing.T) {
	// A package consumed exclusively by test files (TestImports/XTestImports)
	// must pass: test-support packages are load-bearing infrastructure.
	pkgs := []GoPackage{
		{ImportPath: ImportPathA_consumer_check_test, Name: "a", GoFiles: []string{"a.go"}},
		{
			ImportPath:   ImportPathB_consumer_check_test,
			Name:         "b",
			GoFiles:      []string{"b.go"},
			TestGoFiles:  []string{"b_test.go"},
			TestImports:  []string{ImportPathA_consumer_check_test},
			XTestImports: []string{ImportPathA_consumer_check_test},
		},
		// governance is a doc anchor: exempt, terminating the chain.
		{ImportPath: ModulePath + "/governance", Name: "governance", GoFiles: []string{"doc.go"}, Imports: []string{ImportPathB_consumer_check_test}},
	}
	violations := CheckConsumers(pkgs, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("test-only-consumed package must pass, got %v", violations)
	}
}

func TestCheckConsumers_zeroImporters(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: ImportPathA_consumer_check_test, Name: "a", GoFiles: []string{"a.go"}},
		{ImportPath: ImportPathB_consumer_check_test, Name: "b", GoFiles: []string{"b.go"}, Imports: []string{ImportPathA_consumer_check_test}},
		// governance is a doc anchor: exempt, terminating the chain.
		{ImportPath: ModulePath + "/governance", Name: "governance", GoFiles: []string{"doc.go"}, Imports: []string{ImportPathB_consumer_check_test}},
		{ImportPath: ImportPathUnused_consumer_check_test, Name: "unused", GoFiles: []string{"unused.go"}},
	}
	violations := CheckConsumers(pkgs, Allowlist{})
	if len(violations) != 1 {
		t.Fatalf("expected 1 consumer violation for zero-importer package, got %v", violations)
	}
	want := "consumer: " + ImportPathUnused_consumer_check_test + " has no importers"
	if violations[0] != want {
		t.Errorf("want %q, got %q", want, violations[0])
	}
}

func TestCheckConsumers_mainPackage(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: "codeburg.org/lexbit/relurpify/cmd/tool", Name: "main", GoFiles: []string{"main.go"}},
	}
	violations := CheckConsumers(pkgs, Allowlist{})
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
	violations := CheckConsumers(pkgs, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("test-only package should be exempt, got %v", violations)
	}
}

func TestCheckConsumers_docAnchorExempt(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: ModulePath + "/capability", Name: "capability", GoFiles: []string{"doc.go"}},
		{ImportPath: ModulePath + "/governance", Name: "governance", GoFiles: []string{"doc.go"}},
		{ImportPath: ModulePath + "/platform", Name: "platform", GoFiles: []string{"doc.go"}},
		{ImportPath: ModulePath + "/userconfig", Name: "userconfig", GoFiles: []string{"doc.go"}},
		{ImportPath: ModulePath + "/cognitionzoo", Name: "cognitionzoo", GoFiles: []string{"doc.go"}},
		// A subpackage with the anchor as a name-segment prefix is NOT exempt.
		{ImportPath: ModulePath + "/capability/notananchor", Name: "notananchor", GoFiles: []string{"x.go"}},
	}
	violations := CheckConsumers(pkgs, Allowlist{})
	if len(violations) != 1 {
		t.Fatalf("expected only the non-anchor package flagged, got %v", violations)
	}
	if want := "consumer: " + ModulePath + "/capability/notananchor has no importers"; violations[0] != want {
		t.Errorf("want %q, got %q", want, violations[0])
	}
}

func TestCheckConsumers_allowlist(t *testing.T) {
	pkgs := []GoPackage{
		{ImportPath: ImportPathUnused_consumer_check_test, Name: "unused", GoFiles: []string{"unused.go"}},
	}
	allowlist := Allowlist{entries: map[string]map[string]bool{
		"consumer": {"consumer: " + ImportPathUnused_consumer_check_test + " has no importers": true},
	}}
	violations := CheckConsumers(pkgs, allowlist)
	if len(violations) != 0 {
		t.Errorf("expected allowlist to exempt consumer violation, got %v", violations)
	}
}

func TestCheckConsumers_InterimUnwiredExempt(t *testing.T) {
	// The interim-unwired packages (ayenitd charter-only between S7 and S8;
	// context/jobsstore awaiting its runner consumer) are exempt — and the
	// exemption is package-exact, not a prefix.
	pkgs := []GoPackage{
		{ImportPath: ModulePath + "/ayenitd", Name: "ayenitd", GoFiles: []string{"doc.go"}},
		{ImportPath: ModulePath + "/context/jobsstore", Name: "jobsstore", GoFiles: []string{"jobsstore.go"}},
		// A look-alike is NOT exempt.
		{ImportPath: ModulePath + "/context/jobsstoreutil", Name: "jobsstoreutil", GoFiles: []string{"x.go"}},
	}
	violations := CheckConsumers(pkgs, Allowlist{})
	if len(violations) != 1 {
		t.Fatalf("expected only the look-alike flagged, got %v", violations)
	}
	if want := "consumer: " + ModulePath + "/context/jobsstoreutil has no importers"; violations[0] != want {
		t.Errorf("want %q, got %q", want, violations[0])
	}
}
