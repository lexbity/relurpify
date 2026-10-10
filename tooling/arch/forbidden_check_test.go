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
			Imports:    []string{ModulePath + "/platform/fs"}, // forbidden central vocabulary (prod)
		},
		{
			ImportPath:  ModulePath + "/context/knowledge",
			TestImports: []string{ModulePath + "/platform/observability"}, // forbidden (test)
		},
		{
			ImportPath: ModulePath + "/ayenitd",
			Imports:    []string{ModulePath + "/platform/contracts"}, // forbidden (prod)
		},
		{
			ImportPath: ModulePath + "/app/somewhere",
			Imports:    []string{ModulePath + "/platform/fsutils"}, // NOT a match (segment boundary)
		},
		{
			ImportPath: ModulePath + "/platform/fs", // the package itself is not an importer
			Imports:    []string{Fmt_forbidden_check_test},
		},
	}

	got := CheckForbiddenImports(pkgs, Allowlist{})
	if len(got) != 3 {
		t.Fatalf("want 3 violations, got %d: %v", len(got), got)
	}
	for _, v := range got {
		if !strings.Contains(v, "central-vocabulary package") {
			t.Errorf("expected central-vocabulary label: %s", v)
		}
		if strings.Contains(v, "fsutils") || strings.Contains(v, "platform/fs (") && strings.Contains(v, "app/somewhere") {
			t.Errorf("unexpected violation: %s", v)
		}
	}
	if strings.Contains(strings.Join(got, "\n"), "platform/fs ") && !strings.Contains(strings.Join(got, "\n"), "app/envcomposition") {
		t.Errorf("unexpected violation set: %v", got)
	}
}
