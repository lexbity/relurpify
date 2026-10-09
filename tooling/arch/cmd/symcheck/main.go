// Command symcheck reports identifiers for symbols that earlier remediation
// slices removed. Their return would resurrect the design those slices
// deleted, so every occurrence in production code fails the gate.
package main

import (
	"fmt"
	"os"

	"codeburg.org/lexbit/relurpify/tooling/arch"
	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

// exempt records where a removed symbol's name survives as a different, live
// declaration. The no-dead grep gate hid these behind basename filters
// (state.go, state_adapter.go, session_overlay.go, edit_record.go) that would
// have exempted any file so named anywhere in the tree; the AST gate names the
// exact paths instead, so a new state.go is still checked.
//
// Unlike the grep gate, this gate needs no exemption for tooling/arch: it
// matches identifiers, so the symbol table written as string literals in
// symcheck.go is invisible to it. TestExemptionsAreLoadBearing fails if any
// entry here ever stops earning its place.
var exempt = arch.Exemption{ //nolint:gochecknoglobals // gate policy: fixed exemption table, read-only
	Files: []string{
		// GetWorkingValue was a deprecated method on contextdata.Envelope,
		// deleted in the SA1019 slice when callers moved to GetTyped[T]. The
		// identical name lives on the capability/ports.State interface, and on
		// the implementations below, as a current and different declaration.
		"capability/ports/state.go",
		"capability/registry/edit_record.go",
		"capability/registry/session_overlay.go",
		"context/contextdata/state_adapter.go",
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
		fmt.Fprintf(os.Stderr, "[FAIL] symcheck: %v\n", err)
		return 1
	}

	violations := arch.CheckRemovedSymbols(files, exempt)
	for _, line := range parseErrors {
		fmt.Printf("  [warn] symcheck: unparseable file %s\n", line)
	}
	for _, v := range violations {
		fmt.Printf("  [FAIL] %s\n", v)
	}
	if len(violations) > 0 {
		fmt.Printf("[FAIL] symcheck: %d removed-symbol occurrence(s)\n", len(violations))
		return 1
	}
	fmt.Printf("[PASS] symcheck: no removed symbols (%d files scanned)\n", len(files))
	return 0
}
