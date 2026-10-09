// Command shimcheck reports string literals carrying shim, compat, or stub
// language. Those words mark compatibility layers, and the project keeps none:
// the owner of the behavior is fixed instead of wrapping it. The short spelling
// "compat" is matched only as a whole word, so the provider vocabulary
// (openaicompat, "openai_compatible", "euclo:cap.api_compat") is not flagged.
package main

import (
	"fmt"
	"os"

	"codeburg.org/lexbit/relurpify/tooling/arch"
	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

// exempt records the sites whose forbidden vocabulary is the product's own
// feature language rather than an architecture shim. Every entry is an exact
// path — never a basename — so a new file beside an exempt one is still
// policed, and each carries the reason it is accepted.
var exempt = arch.Exemption{ //nolint:gochecknoglobals // gate policy: fixed exemption table, read-only
	// The gate's own tree necessarily contains the vocabulary it forbids:
	// the pattern table in tooling/arch/shimcheck.go, this command's violation
	// messages, and the arch runner's stub gate and allowlist category.
	Prefixes: []string{"tooling/arch"},
	Files: []string{
		// Built-in capabilities whose feature is the vocabulary itself. The
		// public ID euclo:cap.api_compat names the API-compatibility checker,
		// and code_review flags "stub"/"todo"/"fixme" markers in the code it
		// reviews. Their strings describe the user's code, not a layer inside
		// Relurpify, and renaming the capability would change its published ID.
		"named/euclo/relurpicabilities/api_compat.go",
		"named/euclo/relurpicabilities/code_review.go",
		// The capability catalog speaks the same feature language ("Migration
		// Compatibility" family), and the ingestion placeholder
		// stub_ingested_content_for_* is a state key asserted verbatim by
		// named/euclo/orchestrate/ingestion_test.go.
		"named/euclo/capabilities/families.go",
		"named/euclo/orchestrate/ingestion.go",
	},
}

func main() {
	os.Exit(run("."))
}

// run scans root and returns the process exit code. It is separated from main
// so the pass, fail, and walk-error paths are covered by tests against
// fixtures rather than only by the live tree.
func run(root string) int {
	files, parseErrors, err := gatescan.Walk(gatescan.Options{Root: root})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] shimcheck: %v\n", err)
		return 1
	}

	violations := arch.CheckShimLanguage(files, exempt)
	for _, line := range parseErrors {
		fmt.Printf("  [warn] shimcheck: unparseable file %s\n", line)
	}
	for _, v := range violations {
		fmt.Printf("  [FAIL] %s\n", v)
	}
	if len(violations) > 0 {
		fmt.Printf("[FAIL] shimcheck: %d forbidden-language literal(s)\n", len(violations))
		return 1
	}
	fmt.Printf("[PASS] shimcheck: no forbidden language (%d files scanned)\n", len(files))
	return 0
}
