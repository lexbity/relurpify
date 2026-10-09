package arch

import (
	"fmt"
	"go/ast"
	"sort"

	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

// RemovedSymbols are identifiers deleted in earlier remediation slices. Their
// return would resurrect the design the slices removed, so the gate fails on
// any occurrence in production code — a rename-adjacent symbol is a different
// identifier and stays allowed. This list is the AST counterpart of the
// no-dead grep gate in the Makefile; keep the two in sync.
var RemovedSymbols = []string{ //nolint:gochecknoglobals // immutable removed-symbol table
	"InvokeOnBestNode",
	"RegisterNodeProvider",
	"NodeSelectionCriteria",
	"RateLimiter",
	"GetWorkingValue",
	"executionStepFromAgent",
	"inheritExecutionStepScope",
	"summarizeCaptureBindings",
	"summarizeToolScopeFrames",
	"CompiledThoughtRecipe",
	"CompiledStep",
	"CompiledParallelGroup",
	"CompiledConditionalGroup",
	"buildParallelSection",
	"buildConditionalSection",
	"buildBranchSequence",
	"evaluateThoughtRecipeCondition",
	"emitParallelFanouts",
	"BackendModelProfileProvenance",
	"BackendProviderProvenance",
	"VerifyStepResult",
	"WriteBenchmarkBaseline",
	"BuildBenchmarkBaseline",
	"BuildPhaseMetrics",
	"ComparePerformanceBaseline",
	"WrapRegistryWithInterceptor",
	"ReadTelemetryJSONL",
	"LoadGoldenFingerprint",
	"LoadTape",
	"SetHandleScoped",
	"GetHandle",
	"LifecycleView",
}

// CheckRemovedSymbols reports every occurrence of a removed symbol identifier
// in files not covered by exempt.
//
// Identifiers are matched whatever surrounds them — a call, a type position, a
// method receiver — so the gate cannot be defeated by moving the symbol into a
// wrapper or renaming the file around it. Exempt paths keep the historical
// exceptions visible instead of hiding them in grep -v filters.
func CheckRemovedSymbols(files []gatescan.File, exempt Exemption) []string {
	removed := map[string]bool{}
	for _, sym := range RemovedSymbols {
		removed[sym] = true
	}

	var violations []string
	for _, f := range files {
		if exempt.Covers(f.Path) {
			continue
		}
		ast.Inspect(f.AST, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok || !removed[ident.Name] {
				return true
			}
			violations = append(violations, fmt.Sprintf(
				"dead: %s uses removed symbol %s", gatescan.Position(f, ident), ident.Name))
			return true
		})
	}
	sort.Strings(violations)
	return violations
}
