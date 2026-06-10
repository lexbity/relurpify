package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckContextPortsNoInternalImports_noViolation(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + "/context/ports",
			GoFiles:    []string{"policy.go"},
			Imports:    []string{"fmt"},
		},
	}
	vios := CheckContextPortsNoInternalImports(pkgs)
	if len(vios) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(vios), vios)
	}
}

func TestCheckContextPortsNoInternalImports_violation(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + "/context/ports",
			GoFiles:    []string{"policy.go"},
			Imports:    []string{"codeburg.org/lexbit/relurpify/governance/risk"},
		},
	}
	vios := CheckContextPortsNoInternalImports(pkgs)
	if len(vios) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(vios), vios)
	}
	if !strings.Contains(vios[0], "context/ports") {
		t.Errorf("violation should mention context/ports, got: %s", vios[0])
	}
}

func TestCheckContextPortsNoInternalImports_liveTree(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing failed: %v", err)
	}
	vios := CheckContextPortsNoInternalImports(pkgs)
	if len(vios) > 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(vios), vios)
	}
}
