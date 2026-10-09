// Command envcheck reports os.Getenv/os.LookupEnv/os.Environ references
// outside userconfig. Configuration enters the program through
// userconfig/config only, so every other read site is a violation.
package main

import (
	"fmt"
	"os"

	"codeburg.org/lexbit/relurpify/tooling/arch"
	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

// allowedRoot is the one tree permitted to read the process environment:
// userconfig/config snapshots it once and passes the snapshot down.
const allowedRoot = "userconfig"

func main() {
	os.Exit(run("."))
}

// run scans root and returns the process exit code. It is separated from main
// so the pass, fail, and walk-error paths are covered by tests against
// fixtures rather than only by the live tree.
func run(root string) int {
	files, parseErrors, err := gatescan.Walk(gatescan.Options{Root: root})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] envcheck: %v\n", err)
		return 1
	}

	violations := arch.CheckEnvAccess(files, allowedRoot)
	for _, line := range parseErrors {
		fmt.Printf("  [warn] envcheck: unparseable file %s\n", line)
	}
	for _, v := range violations {
		fmt.Printf("  [FAIL] %s\n", v)
	}
	if len(violations) > 0 {
		fmt.Printf("[FAIL] envcheck: %d env access violation(s) outside %s\n", len(violations), allowedRoot)
		return 1
	}
	fmt.Printf("[PASS] envcheck: no env access outside %s (%d files scanned)\n", allowedRoot, len(files))
	return 0
}
