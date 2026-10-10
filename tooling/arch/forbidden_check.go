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
var ForbiddenImportPrefixes = []string{ //nolint:gochecknoglobals // immutable forbidden-prefix table
	"capability/types",
	"framework/core",
}

// CentralVocabularyPrefixes lists module-relative package paths that carry
// central vocabulary or substrate code which must not re-accumulate new
// importers. This is the named-list successor to the deleted blanket layer
// rule (platform was importable by every domain by design; only specific
// non-adapter packages are banned). Entries are removed when the package is
// relocated or deleted: platform/fs and platform/observability left in S4
// (capability/fs and telemetry/observing); platform/browser joins when it is
// deleted in S5.
var CentralVocabularyPrefixes = []string{ //nolint:gochecknoglobals // immutable central-vocabulary table
	"platform/contracts",
	"platform/browser",
}

// CheckForbiddenImports reports any package whose imports (production or test)
// reference a forbidden, deleted package, or a central-vocabulary package on
// the grave list. It checks Imports, TestImports, and XTestImports so a
// regression cannot hide in test-only code.
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
				label, prefixes := "deleted package", ForbiddenImportPrefixes
				if matchesAny(rel, CentralVocabularyPrefixes) {
					label, prefixes = "central-vocabulary package", CentralVocabularyPrefixes
				}
				for _, forbidden := range prefixes {
					if rel != forbidden && !hasPathPrefix(rel, forbidden) {
						continue
					}
					key := pkg.ImportPath + "→" + imp + "(" + kind + ")"
					if seen[key] {
						continue
					}
					seen[key] = true
					v := fmt.Sprintf("forbidden: %s imports %s %s (%s)", pkg.ImportPath, label, imp, kind)
					if !allowlist.Contains("forbidden", v) {
						violations = append(violations, v)
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

// matchesAny reports whether rel equals or is below any of the prefixes,
// matching on path segment boundaries (so "framework/coreutil" is not a match
// for "framework/core").
func matchesAny(rel string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if rel == prefix || hasPathPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

// hasPathPrefix reports whether rel is at or below the forbidden package path,
// matching on path segment boundaries (so "framework/coreutil" is not a match
// for "framework/core").
func hasPathPrefix(rel, prefix string) bool {
	return len(rel) > len(prefix) && rel[:len(prefix)] == prefix && rel[len(prefix)] == '/'
}
