package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

const (
	Execution_governance_no_orchestration_test               = "/execution"
	Executionagentlifecycle_governance_no_orchestration_test = "/execution/agentlifecycle"
	Fmt_governance_no_orchestration_test                     = "fmt"
)

func TestCheckGovernanceNoOrchestration_noViolation(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/governance/policy"),
		mkPkg(ModulePath + Executionagentlifecycle_governance_no_orchestration_test),
	}
	// governance imports only stdlib — no execution
	pkgs[0].Imports = []string{Fmt_governance_no_orchestration_test, "errors"}
	pkgs[1].Imports = []string{Fmt_governance_no_orchestration_test}

	vios := CheckGovernanceNoOrchestration(pkgs)
	if len(vios) != 0 {
		t.Errorf("expected no violations, got %d: %v", len(vios), vios)
	}
}

func TestCheckGovernanceNoOrchestration_violation(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/governance/authorization"),
		mkPkg(ModulePath + Executionagentlifecycle_governance_no_orchestration_test),
	}
	pkgs[0].Imports = []string{
		ModulePath + Executionagentlifecycle_governance_no_orchestration_test,
		Fmt_governance_no_orchestration_test,
	}
	pkgs[1].Imports = []string{Fmt_governance_no_orchestration_test}

	vios := CheckGovernanceNoOrchestration(pkgs)
	if len(vios) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(vios), vios)
	}
	if !strings.Contains(vios[0], "governance/authorization") || !strings.Contains(vios[0], "execution/agentlifecycle") {
		t.Errorf("violation should mention both packages, got: %s", vios[0])
	}
}

func TestCheckGovernanceNoOrchestration_governanceRootImport(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/governance"),
		mkPkg(ModulePath + "/execution/context"),
	}
	pkgs[0].Imports = []string{
		ModulePath + "/execution/context",
	}
	pkgs[1].Imports = []string{Fmt_governance_no_orchestration_test}

	vios := CheckGovernanceNoOrchestration(pkgs)
	if len(vios) != 1 {
		t.Fatalf("expected 1 violation for governance root → execution, got %d", len(vios))
	}
}

func TestCheckGovernanceNoOrchestration_notGovernance(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/capability/agentspec"),
		mkPkg(ModulePath + Execution_governance_no_orchestration_test),
	}
	pkgs[0].Imports = []string{
		ModulePath + Execution_governance_no_orchestration_test,
	}
	pkgs[1].Imports = []string{Fmt_governance_no_orchestration_test}

	vios := CheckGovernanceNoOrchestration(pkgs)
	if len(vios) != 0 {
		t.Errorf("expected 0 violations (capability is not governance), got %d: %v", len(vios), vios)
	}
}

func TestCheckGovernanceNoOrchestration_onlyTestFiles(t *testing.T) {
	pkgs := []GoPackage{
		{
			ImportPath:      ModulePath + "/governance/authorization",
			OnlyTestGoFiles: true,
			TestGoFiles:     []string{"auth_test.go"},
			TestImports:     []string{ModulePath + Execution_governance_no_orchestration_test},
		},
	}
	vios := CheckGovernanceNoOrchestration(pkgs)
	if len(vios) != 0 {
		t.Errorf("expected 0 violations for test-only governance package, got %d: %v", len(vios), vios)
	}
}

func TestCheckGovernanceNoOrchestration_liveTreeBaseline(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing failed: %v", err)
	}
	vios := CheckGovernanceNoOrchestration(pkgs)
	// Warn-mode baseline: record violation count for tracking
	t.Logf("governance→execution violations: %d", len(vios))
	// Until P14 is retired, expect known violations.
	// Currently governance/authorization imports execution.
}
