package arch

import (
	"fmt"
	"strings"
)

// CheckConsumers ensures every non-main, non-test package has at least one
// non-test importer.  Packages with zero consumers are dead code.
func CheckConsumers(pkgs []GoPackage, reverse map[string][]string, allowlist Allowlist) []string {
	var violations []string
	pkgMap := make(map[string]GoPackage)
	for _, pkg := range pkgs {
		pkgMap[pkg.ImportPath] = pkg
	}

	for _, pkg := range pkgs {
		if pkg.Name == "main" {
			continue
		}
		if pkg.OnlyTestGoFiles {
			continue
		}

		importers := reverse[pkg.ImportPath]
		nonTestImporters := 0
		for _, imp := range importers {
			if !strings.HasSuffix(imp, "_test") && imp != pkg.ImportPath {
				if impPkg, ok := pkgMap[imp]; ok && !impPkg.OnlyTestGoFiles {
					nonTestImporters++
				} else if !ok {
					nonTestImporters++
				}
			}
		}
		if nonTestImporters > 0 {
			continue
		}

		// Check self-import isn't counted
		violation := fmt.Sprintf("consumer: %s has no non-test importers", pkg.ImportPath)
		if !allowlist.Contains("consumer", violation) {
			violations = append(violations, violation)
		}
	}
	return violations
}

// CheckInternalConsumers is a stronger variant: every non-main package must be
// imported by at least one package outside its own domain tree.
func CheckInternalConsumers(pkgs []GoPackage, reverse map[string][]string, allowlist Allowlist) []string {
	var violations []string
	pkgMap := make(map[string]GoPackage)
	for _, pkg := range pkgs {
		pkgMap[pkg.ImportPath] = pkg
	}

	for _, pkg := range pkgs {
		if pkg.Name == "main" {
			continue
		}
		if pkg.OnlyTestGoFiles {
			continue
		}

		domain := PackageDomain(pkg.ImportPath)
		importers := reverse[pkg.ImportPath]
		externalImporters := 0
		for _, imp := range importers {
			if impPkg, ok := pkgMap[imp]; ok && !impPkg.OnlyTestGoFiles {
				impDomain := PackageDomain(imp)
				if impDomain != domain {
					externalImporters++
				}
			}
		}
		if externalImporters == 0 && domain != "" {
			violation := fmt.Sprintf("internal-consumer: %s has no importers outside %s", pkg.ImportPath, domain)
			if !allowlist.Contains("internal-consumer", violation) {
				violations = append(violations, violation)
			}
		}
	}
	return violations
}
