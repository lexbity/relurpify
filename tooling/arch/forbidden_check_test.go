package arch

import "testing"

func TestCheckForbiddenImports(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + "/framework/authorization",
			Imports:    []string{ModulePath + "/governance/policy", "fmt"},
		},
		{
			ImportPath: ModulePath + "/agents/react",
			Imports:    []string{ModulePath + "/framework/core"}, // forbidden (prod)
		},
		{
			ImportPath:  ModulePath + "/named/euclo",
			TestImports: []string{ModulePath + "/framework/core"}, // forbidden (test)
		},
		{
			ImportPath: ModulePath + "/framework/coreutil", // NOT a match (segment boundary)
			Imports:    []string{"fmt"},
		},
	}

	got := CheckForbiddenImports(pkgs, Allowlist{})
	if len(got) != 2 {
		t.Fatalf("want 2 violations, got %d: %v", len(got), got)
	}
	// the legitimate package and the look-alike must not be flagged
	for _, v := range got {
		if contains(v, "framework/authorization") || contains(v, "coreutil") {
			t.Errorf("unexpected violation: %s", v)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
