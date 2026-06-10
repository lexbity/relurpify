package arch

import (
	"fmt"
	"sort"
)

// CheckContextPortsNoInternalImports verifies that context/ports imports
// nothing from this module (stdlib only). This guarantees the package is
// a leaf — adding a field of type governance.X would fail the build,
// preventing cycle re-introduction.
func CheckContextPortsNoInternalImports(pkgs []GoPackage) []string {
	var violations []string
	for _, pkg := range pkgs {
		rel := TrimModulePrefix(pkg.ImportPath)
		if rel != "context/ports" && !hasPathPrefix(rel, "context/ports/") {
			continue
		}
		imports := append([]string{}, pkg.Imports...)
		imports = append(imports, pkg.TestImports...)
		imports = append(imports, pkg.XTestImports...)
		for _, imp := range imports {
			if !IsStandardLib(imp) {
				violations = append(violations, fmt.Sprintf(
					"context-ports-no-internal: %s imports %s (context/ports must be stdlib-only)",
					pkg.ImportPath, imp,
				))
			}
		}
	}
	sort.Strings(violations)
	return violations
}
