package arch

import (
	"fmt"
	"sort"
	"strings"
)

// RiskVocabPrefixes lists governance package prefixes that are risk vocabulary.
// capability/* packages must not import these; risk is a governance judgment,
// not a capability-declared fact.
//
// EffectClass and CapabilityScope (self-declared facts) live in
// capability/classification; RiskClass (a judgment) lives in governance/risk.
var RiskVocabPrefixes = []string{
	"governance/taxonomy",
	"governance/risk",
}

// CheckClassificationOwnership enforces the target classification ownership
// direction from Q1:
//
//   - capability/* MUST NOT import governance risk vocabulary
//     (RiskClass is a governance judgment, not a capability declaration)
//   - governance/risk MAY import capability/classification
//     (the judge reads declared facts; this is the sanctioned direction)
//
// In warn mode this flag shows current violations. Once governance/taxonomy
// is deleted and governance/risk exists, the rule catches regressions.
func CheckClassificationOwnership(pkgs []GoPackage) []string {
	var violations []string
	for _, pkg := range pkgs {
		if pkg.OnlyTestGoFiles {
			continue
		}
		if pkg.Name == "main" {
			continue
		}
		domain := PackageDomain(pkg.ImportPath)
		imports := append([]string{}, pkg.Imports...)
		imports = append(imports, pkg.TestImports...)
		imports = append(imports, pkg.XTestImports...)

		for _, imp := range imports {
			if IsStandardLib(imp) {
				continue
			}
			impRel := TrimModulePrefix(imp)
			impDomain := PackageDomain(imp)

			// Forbid: capability/* → governance risk vocabulary
			if domain == "capability" && impDomain == "governance" {
				for _, forbid := range RiskVocabPrefixes {
					if impRel == forbid || strings.HasPrefix(impRel, forbid+"/") {
						violations = append(violations,
							fmt.Sprintf("classification-ownership: %s imports risk vocabulary %s (RiskClass is a governance judgment, not a capability fact)", pkg.ImportPath, imp))
						break
					}
				}
				continue
			}
		}
	}
	sort.Strings(violations)
	return violations
}

// CheckGovernanceRiskImports reports governance/* packages that import
// capability/* while providing the complementary exception check.
// governance/risk MAY import capability/classification (the judge reads
// declared facts). Other governance→capability imports are flagged if
// they import beyond capability/classification.
//
// This is the complement to CheckClassificationOwnership: it ensures
// the allowance direction (governance/risk → capability/classification)
// is the ONLY governance→capability edge for risk concerns.
func CheckGovernanceRiskImports(pkgs []GoPackage) []string {
	var violations []string
	for _, pkg := range pkgs {
		if pkg.OnlyTestGoFiles {
			continue
		}
		if pkg.Name == "main" {
			continue
		}
		rel := TrimModulePrefix(pkg.ImportPath)
		domain := PackageDomain(pkg.ImportPath)
		if domain != "governance" {
			continue
		}
		imports := append([]string{}, pkg.Imports...)
		imports = append(imports, pkg.TestImports...)
		imports = append(imports, pkg.XTestImports...)
		for _, imp := range imports {
			if IsStandardLib(imp) {
				continue
			}
			impRel := TrimModulePrefix(imp)
			impDomain := PackageDomain(imp)
			if impDomain != "capability" {
				continue
			}
			// Allow: governance/risk → capability/classification
			isRiskPkg := strings.HasPrefix(rel, "governance/risk")
			isClassPkg := strings.HasPrefix(impRel, "capability/classification")
			if isRiskPkg && isClassPkg {
				continue
			}
			violations = append(violations,
				fmt.Sprintf("governance-risk-import: %s imports capability package %s (only governance/risk → capability/classification is allowed for risk concerns)", pkg.ImportPath, imp))
		}
	}
	sort.Strings(violations)
	return violations
}
