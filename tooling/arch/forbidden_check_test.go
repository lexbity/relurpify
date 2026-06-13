package arch

import (
	"strings"
	"testing"
)

const (
	Fmt_forbidden_check_test = "fmt"
)


func TestCheckForbiddenImports(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + "/framework/authorization",
			Imports:    []string{ModulePath + "/governance/policy", Fmt_forbidden_check_test},
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
			ImportPath: ModulePath + "/agents/react",
			Imports:    []string{ModulePath + "/capability/types"}, // forbidden deleted bucket
		},
		{
			ImportPath: ModulePath + "/framework/coreutil", // NOT a match (segment boundary)
			Imports:    []string{Fmt_forbidden_check_test},
		},
		{
			ImportPath: ModulePath + "/capability/typesafe", // NOT a match (segment boundary)
			Imports:    []string{Fmt_forbidden_check_test},
		},
	}

	got := CheckForbiddenImports(pkgs, Allowlist{})
	if len(got) != 3 {
		t.Fatalf("want 3 violations, got %d: %v", len(got), got)
	}
	// the legitimate package and the look-alike must not be flagged
	for _, v := range got {
		if strings.Contains(v, "framework/authorization") || strings.Contains(v, "coreutil") || strings.Contains(v, "typesafe") {
			t.Errorf("unexpected violation: %s", v)
		}
	}
}
