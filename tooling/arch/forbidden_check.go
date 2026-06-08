package arch

import (
	"fmt"
	"sort"
)

// ForbiddenImportPrefixes lists module-relative package paths that must never be
// imported again. framework/core was the dissolved "vocabulary bucket": it kept
// re-forming because there was a universal import target. With the bucket gone,
// this gate fails the build the moment any package (including tests) reaches for
// it, so it cannot silently return. Add a prefix here only when a package has
// been deliberately deleted and its types rehomed into owning domains.
var ForbiddenImportPrefixes = []string{
	"capability/types",
	"framework/core",
}

// CheckForbiddenImports reports any package whose imports (production or test)
// reference a forbidden, deleted package. It checks Imports, TestImports, and
// XTestImports so a regression cannot hide in test-only code.
func CheckForbiddenImports(pkgs []GoPackage, allowlist Allowlist) []string {
	var violations []string
	for _, pkg := range pkgs {
		seen := make(map[string]bool)
		check := func(kind string, imports []string) {
			for _, imp := range imports {
				if IsStandardLib(imp) {
					continue
				}
				rel := TrimModulePrefix(imp)
				for _, forbidden := range ForbiddenImportPrefixes {
					if rel == forbidden || hasPathPrefix(rel, forbidden) {
						key := pkg.ImportPath + "→" + imp + "(" + kind + ")"
						if seen[key] {
							continue
						}
						seen[key] = true
						v := fmt.Sprintf("forbidden: %s imports deleted package %s (%s)", pkg.ImportPath, imp, kind)
						if !allowlist.Contains("forbidden", v) {
							violations = append(violations, v)
						}
					}
				}
			}
		}
		check("import", pkg.Imports)
		check("test", pkg.TestImports)
		check("xtest", pkg.XTestImports)
	}
	sort.Strings(violations)
	return violations
}

// hasPathPrefix reports whether rel is at or below the forbidden package path,
// matching on path segment boundaries (so "framework/coreutil" is not a match
// for "framework/core").
func hasPathPrefix(rel, prefix string) bool {
	return len(rel) > len(prefix) && rel[:len(prefix)] == prefix && rel[len(prefix)] == '/'
}
