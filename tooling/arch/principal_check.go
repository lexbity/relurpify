package arch

import (
	"fmt"
	"sort"
	"strings"
)

// CheckPrincipalContextWrite reports any package outside execution/agentlifecycle
// that calls ContextWithPrincipal or writes the governance:principal context key.
//
// NFR-8: The Principal context key MUST be writable only from
// execution/agentlifecycle. Everywhere else reads via PrincipalFromContext.
func CheckPrincipalContextWrite(pkgs []GoPackage) []string {
	var violations []string
	for _, pkg := range pkgs {
		if pkg.OnlyTestGoFiles {
			continue
		}
		rel := TrimModulePrefix(pkg.ImportPath)
		// Allow execution/agentlifecycle
		if rel == "execution/agentlifecycle" || strings.HasPrefix(rel, "execution/agentlifecycle/") {
			continue
		}
		// Check production imports for indicators of Principal write access.
		// ContextWithPrincipal is the only way to write the key.
		imports := append([]string{}, pkg.Imports...)
		for _, imp := range imports {
			if strings.Contains(imp, "governance/ports") {
				// This package imports governance/ports which exports ContextWithPrincipal.
				// We flag this as a potential violation (in practice only agentlifecycle should call it).
				// This is a heuristic: we can't determine from import alone whether the function
				// is actually called, but importing the package is the necessary precondition.
				violations = append(violations, fmt.Sprintf(
					"principal-context-write: %s imports governance/ports (potential ContextWithPrincipal caller outside execution/agentlifecycle)",
					pkg.ImportPath,
				))
			}
		}
	}
	sort.Strings(violations)
	return violations
}
