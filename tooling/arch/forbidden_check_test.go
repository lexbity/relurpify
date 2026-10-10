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

func TestCheckForbiddenImports_centralVocabulary(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + "/app/envcomposition",
			Imports:    []string{ModulePath + "/platform/contracts"}, // forbidden central vocabulary (prod)
		},
		{
			ImportPath:  ModulePath + "/context/knowledge",
			TestImports: []string{ModulePath + "/platform/contracts"}, // forbidden (test)
		},
		{
			// platform/fs and platform/observability left the central-vocabulary
			// list when they relocated (S4): capability/fs and telemetry/observing
			// are legal imports for their consumers.
			ImportPath: ModulePath + "/context/knowledge",
			Imports:    []string{ModulePath + "/capability/fs"},
		},
		{
			ImportPath: ModulePath + "/app/somewhere",
			Imports:    []string{ModulePath + "/platform/contractsutils"}, // NOT a match (segment boundary)
		},
	}

	got := CheckForbiddenImports(pkgs, Allowlist{})
	if len(got) != 2 {
		t.Fatalf("want 2 violations, got %d: %v", len(got), got)
	}
	for _, v := range got {
		if !strings.Contains(v, "central-vocabulary package") {
			t.Errorf("expected central-vocabulary label: %s", v)
		}
		if strings.Contains(v, "capability/fs") || strings.Contains(v, "contractsutils") {
			t.Errorf("unexpected violation: %s", v)
		}
	}
}
