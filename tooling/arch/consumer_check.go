package arch

import (
	"fmt"
)

// docAnchors lists domain-root documentation packages exempt from the
// consumer rule. They exist as anchors for their domain's identity (the
// path prefix); they are meant to be imported transitively, not directly.
var docAnchors = map[string]bool{ //nolint:gochecknoglobals // immutable exemption table
	ModulePath + "/capability":   true,
	ModulePath + "/cognitionzoo": true,
	ModulePath + "/governance":   true,
	ModulePath + "/platform":     true,
	ModulePath + "/userconfig":   true,
}

// pathConsumedFixtures lists directory-prefixes whose packages are consumed
// by the test harness through the filesystem (compiled/executed by path),
// never by import. They are testdata, not library code.
var pathConsumedFixtures = []string{ //nolint:gochecknoglobals // immutable exemption table
	"testsuite/agenttest_fixtures",
}

// CheckConsumers ensures every non-main, non-test package has at least one
// importer, counting test-only importers (TestImports/XTestImports) equally
// with production imports — a package consumed only by tests is still
// load-bearing infrastructure here. Packages with zero importers of any kind
// are dead code.
func CheckConsumers(pkgs []GoPackage, allowlist Allowlist) []string {
	var violations []string
	importerOf := make(map[string][]string)
	for _, pkg := range pkgs {
		seen := make(map[string]bool)
		add := func(imports []string) {
			for _, imp := range imports {
				if imp == pkg.ImportPath || seen[imp] {
					continue
				}
				seen[imp] = true
				importerOf[imp] = append(importerOf[imp], pkg.ImportPath)
			}
		}
		add(pkg.Imports)
		add(pkg.TestImports)
		add(pkg.XTestImports)
	}

	for _, pkg := range pkgs {
		if pkg.Name == "main" {
			continue
		}
		if pkg.OnlyTestGoFiles {
			continue
		}
		if docAnchors[pkg.ImportPath] {
			continue
		}
		rel := TrimModulePrefix(pkg.ImportPath)
		consumedByPath := false
		for _, prefix := range pathConsumedFixtures {
			if rel == prefix || hasPathPrefix(rel, prefix) {
				consumedByPath = true
				break
			}
		}
		if consumedByPath {
			continue
		}
		if len(importerOf[pkg.ImportPath]) > 0 {
			continue
		}

		violation := fmt.Sprintf("consumer: %s has no importers", pkg.ImportPath)
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
