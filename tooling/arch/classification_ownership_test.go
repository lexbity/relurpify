package arch

import (
	"path/filepath"
	"testing"
)

func TestCheckClassificationOwnership_noViolation(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/capability/agentspec"),
		mkPkg(ModulePath + "/governance/risk"),
	}
	pkgs[0].Imports = []string{"fmt", "errors"}
	pkgs[1].Imports = []string{"fmt"}

	vios := CheckClassificationOwnership(pkgs)
	if len(vios) != 0 {
		t.Errorf("expected no violations, got %d: %v", len(vios), vios)
	}
}

func TestCheckClassificationOwnership_violationCapabilityImportsTaxonomy(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/capability/agentspec"),
	}
	pkgs[0].Imports = []string{
		ModulePath + "/governance/taxonomy",
	}

	vios := CheckClassificationOwnership(pkgs)
	if len(vios) != 1 {
		t.Fatalf("expected 1 violation for capability → governance/taxonomy, got %d: %v", len(vios), vios)
	}
	if !contains(vios[0], "capability/agentspec") || !contains(vios[0], "governance/taxonomy") {
		t.Errorf("violation should mention both packages, got: %s", vios[0])
	}
}

func TestCheckClassificationOwnership_noViolationForOtherGovernanceImports(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/capability/agentspec"),
	}
	// Importing governance/policyresolve is NOT risk vocab — it's allowed.
	pkgs[0].Imports = []string{
		ModulePath + "/governance/policyresolve",
	}

	vios := CheckClassificationOwnership(pkgs)
	if len(vios) != 0 {
		t.Errorf("expected 0 violations for non-risk governance import, got %d: %v", len(vios), vios)
	}
}

func TestCheckClassificationOwnership_violationCapabilityImportsRisk(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/capability/descriptor"),
	}
	pkgs[0].Imports = []string{
		ModulePath + "/governance/risk",
	}

	vios := CheckClassificationOwnership(pkgs)
	if len(vios) != 1 {
		t.Fatalf("expected 1 violation for capability → governance/risk, got %d: %v", len(vios), vios)
	}
}

func TestCheckClassificationOwnership_notCapability(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/governance/policy"),
	}
	pkgs[0].Imports = []string{
		ModulePath + "/governance/taxonomy",
	}

	// Same-domain import should not be flagged
	vios := CheckClassificationOwnership(pkgs)
	if len(vios) != 0 {
		t.Errorf("expected 0 violations for governance internal import, got %d: %v", len(vios), vios)
	}
}

func TestCheckGovernanceRiskImports_allowedEdge(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/governance/risk"),
		mkPkg(ModulePath + "/capability/classification"),
	}
	pkgs[0].Imports = []string{
		ModulePath + "/capability/classification",
	}
	pkgs[1].Imports = []string{"fmt"}

	// governance/risk → capability/classification is the sanctioned direction
	vios := CheckGovernanceRiskImports(pkgs)
	if len(vios) != 0 {
		t.Errorf("expected 0 violations for governance/risk → capability/classification, got %d: %v", len(vios), vios)
	}
}

func TestCheckGovernanceRiskImports_violationGeneralGovImportsCap(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/governance/policy"),
		mkPkg(ModulePath + "/capability/descriptor"),
	}
	pkgs[0].Imports = []string{
		ModulePath + "/capability/descriptor",
	}
	pkgs[1].Imports = []string{"fmt"}

	// governance/policy → capability/descriptor is NOT in the allowance
	vios := CheckGovernanceRiskImports(pkgs)
	if len(vios) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(vios), vios)
	}
}

func TestCheckClassificationOwnership_liveTreeBaseline(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing failed: %v", err)
	}
	vios := CheckClassificationOwnership(pkgs)
	t.Logf("classification-ownership violations: %d", len(vios))
	// Baseline: capability/* packages importing governance/taxonomy
}
