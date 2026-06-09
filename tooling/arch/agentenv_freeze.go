package arch

import (
	"fmt"
	"sort"
)

// AllowedAgentenvImporters lists packages currently permitted to import
// execution/agentenv during the domain-split transition.
//
// No new package may be added to this list. New code MUST consume
// execution/session, execution/workspace, or focused domain controllers
// instead of importing execution/agentenv.
//
// Each entry is a module-relative package path.
// This list shrinks as slices eliminate agentenv dependencies.
var AllowedAgentenvImporters = []string{}

var agentenvImportPath = ModulePath + "/execution/agentenv"

// allowedImporterSet builds a lookup set from the allowed list.
func allowedImporterSet() map[string]bool {
	m := make(map[string]bool, len(AllowedAgentenvImporters))
	for _, p := range AllowedAgentenvImporters {
		m[ModulePath+"/"+p] = true
	}
	return m
}

// CheckNoNewAgentenvImporters reports any package that imports
// execution/agentenv but is not in the frozen allowlist.
func CheckNoNewAgentenvImporters(pkgs []GoPackage, forward map[string][]string) []string {
	allowed := allowedImporterSet()
	seen := make(map[string]bool)
	var violations []string

	for _, pkg := range pkgs {
		imports := forward[pkg.ImportPath]
		importsAgentenv := false
		for _, imp := range imports {
			if imp == agentenvImportPath {
				importsAgentenv = true
				break
			}
		}
		if !importsAgentenv {
			continue
		}

		rel := TrimModulePrefix(pkg.ImportPath)
		if allowed[pkg.ImportPath] {
			continue
		}
		if seen[pkg.ImportPath] {
			continue
		}
		seen[pkg.ImportPath] = true
		violations = append(violations,
			fmt.Sprintf("agentenv-freeze: %s imports execution/agentenv (not in frozen allowlist)", rel))
	}
	sort.Strings(violations)
	return violations
}

// AllowedAgentenvImportersReport lists the current allowed importers and
// whether each still imports agentenv. This helps track migration progress.
func AllowedAgentenvImportersReport(forward map[string][]string) []string {
	allowed := allowedImporterSet()
	var lines []string
	for pkg := range allowed {
		imports := forward[pkg]
		stillImports := false
		for _, imp := range imports {
			if imp == agentenvImportPath {
				stillImports = true
				break
			}
		}
		rel := TrimModulePrefix(pkg)
		status := "STILL IMPORTS"
		if !stillImports {
			status = "CLEARED"
		}
		lines = append(lines, fmt.Sprintf("  %s: %s", status, rel))
	}
	sort.Strings(lines)
	return lines
}



