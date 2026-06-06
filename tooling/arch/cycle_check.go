package arch

import (
	"fmt"
	"sort"
	"strings"
)

// CheckCycles detects import cycles in the package graph.
// Returns a list of cycle descriptions.
func CheckCycles(forward map[string][]string, allowlist Allowlist) []string {
	var violations []string
	seen := make(map[string]bool)

	for path := range forward {
		if seen[path] {
			continue
		}
		cycle := findCycle(path, forward, make(map[string]bool), seen)
		if cycle != nil {
			violation := fmt.Sprintf("cycle: %s", strings.Join(cycle, " → "))
			if !allowlist.Contains("cycle", violation) {
				violations = append(violations, violation)
			}
		}
	}
	return violations
}

// findCycle performs DFS from start looking for a cycle.
// pathSet tracks nodes on the current DFS path.
// seen tracks nodes that have been fully explored across calls.
func findCycle(start string, graph map[string][]string, pathSet map[string]bool, seen map[string]bool) []string {
	if pathSet[start] {
		// Found cycle - reconstruct. pathSet contains the full path.
		// We need to extract from this node back to itself.
		return extractCycle(start, pathSet)
	}
	if seen[start] {
		return nil
	}
	pathSet[start] = true

	imports := graph[start]
	// Sort for deterministic output
	sorted := make([]string, len(imports))
	copy(sorted, imports)
	sort.Strings(sorted)

	for _, imp := range sorted {
		if IsStandardLib(imp) {
			continue
		}
		cycle := findCycle(imp, graph, pathSet, seen)
		if cycle != nil {
			return cycle
		}
	}

	delete(pathSet, start)
	seen[start] = true
	return nil
}

// extractCycle builds a cycle string starting from the cycle node.
func extractCycle(start string, pathSet map[string]bool) []string {
	return []string{start, start}
}

// CheckLayerDirection enforces adapter→domain layering.
// Platform packages (platform/) must NOT be imported by any non-platform package.
func CheckLayerDirection(pkgs []GoPackage, forward map[string][]string, allowlist Allowlist) []string {
	var violations []string
	pkgMap := make(map[string]GoPackage)
	for _, pkg := range pkgs {
		pkgMap[pkg.ImportPath] = pkg
	}

	for _, pkg := range pkgs {
		if pkg.OnlyTestGoFiles {
			continue
		}
		pkgDomain := PackageDomain(pkg.ImportPath)
		if pkgDomain == "platform" {
			continue
		}
		imports := forward[pkg.ImportPath]
		for _, imp := range imports {
			if IsStandardLib(imp) {
				continue
			}
			impDomain := PackageDomain(imp)
			if impDomain == "platform" {
				if pkgDomain == "testsuite" {
					continue
				}
				violation := fmt.Sprintf("layer: %s imports platform package %s", pkg.ImportPath, imp)
				if !allowlist.Contains("layer", violation) {
					violations = append(violations, violation)
				}
			}
		}
	}
	return violations
}
