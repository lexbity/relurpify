package arch

import (
	"fmt"
	"sort"
)

// CheckGovernanceNoOrchestration reports governance/* packages that illegally
// import execution packages (orchestration domain). Governance owns policy
// decisions and must not contain orchestration logic.
//
// Governance consumes execution's ports (governance-owned consumer interfaces
// in governance/ports), never execution's concrete packages.
//
// See Q2: governance defines policy decisions via Enforcer.Check; it must not
// import execution to recover runtime structs or call executor methods.
func CheckGovernanceNoOrchestration(pkgs []GoPackage) []string {
	var violations []string
	for _, pkg := range pkgs {
		if pkg.OnlyTestGoFiles {
			continue
		}
		if pkg.Name == "main" {
			continue
		}
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
			impDomain := PackageDomain(imp)
			if impDomain != "execution" {
				continue
			}
			violations = append(violations,
				fmt.Sprintf("governance-orchestration: %s imports execution package %s", pkg.ImportPath, imp))
		}
	}
	sort.Strings(violations)
	return violations
}
