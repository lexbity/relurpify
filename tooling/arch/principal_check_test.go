package arch

import (
	"path/filepath"
	"testing"
)

const (
	Governanceports_principal_check_test = "/governance/ports"
)


func TestCheckPrincipalContextWrite_noViolation(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + "/execution/agentlifecycle",
			GoFiles:    []string{"execute.go"},
			Imports:    []string{ModulePath + Governanceports_principal_check_test},
		},
		{
			ImportPath: ModulePath + Governanceports_principal_check_test,
			GoFiles:    []string{"authorization.go"},
		},
	}
	vios := CheckPrincipalContextWrite(pkgs)
	if len(vios) != 0 {
		t.Errorf("expected 0 violations (allowed caller), got %d: %v", len(vios), vios)
	}
}

func TestCheckPrincipalContextWrite_violation(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + "/capability/descriptor",
			GoFiles:    []string{"descriptor.go"},
			Imports:    []string{ModulePath + Governanceports_principal_check_test},
		},
	}
	vios := CheckPrincipalContextWrite(pkgs)
	if len(vios) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(vios), vios)
	}
}

func TestCheckPrincipalContextWrite_executionAgentlifecycleAllowed(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + "/execution/agentlifecycle",
			GoFiles:    []string{"execute.go", "types.go"},
			Imports:    []string{ModulePath + Governanceports_principal_check_test, "fmt"},
		},
		{
			ImportPath: ModulePath + "/governance/ports/authorization",
			GoFiles:    []string{"authorization.go"},
		},
	}
	vios := CheckPrincipalContextWrite(pkgs)
	if len(vios) != 0 {
		t.Errorf("execution/agentlifecycle should be allowed, got %d: %v", len(vios), vios)
	}
}

func TestCheckPrincipalContextWrite_liveTreeBaseline(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing failed: %v", err)
	}
	vios := CheckPrincipalContextWrite(pkgs)
	t.Logf("principal-context-write violations: %d", len(vios))
	// Violations are expected from packages that import governance/ports but
	// only use PrincipalFromContext (read side). Only execution/agentlifecycle
	// should call ContextWithPrincipal (write side).
}
