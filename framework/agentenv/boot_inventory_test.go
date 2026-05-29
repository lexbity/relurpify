// Phase 0 — Inventory, guard fixtures, and baseline.
//
// Establishes a verified, test-locked inventory of every composition root,
// every config loader, and every backend-vocabulary site, so later deletions
// are provably complete.
//
// See devdocs/plans/unified-boot-contract.md for the full plan.

package agentenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const selfFile = "framework/agentenv/boot_inventory_test.go"

// TestBootRootInventory enumerates the known callers of
// BuildBuiltinCapabilityBundle and fails if any new caller appears.
// There is exactly one composition root: BootstrapAgentRuntime.
//
// The single composition root (calls BuildBuiltinCapabilityBundle):
//   - framework/agentenv/workspace.go (BootstrapAgentRuntime)
func TestBootRootInventory(t *testing.T) {
	root := findModuleRoot(t)

	// --- BuildBuiltinCapabilityBundle callers ---------------------------------
	// This is the authoritative indicator of a composition root, because
	// every root must call it to wire the capability bundle.
	cmd := exec.Command("grep", "-rn", `BuildBuiltinCapabilityBundle(`, "--include=*.go", ".")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("grep BuildBuiltinCapabilityBundle failed: %v", err)
	}

	knownBundleCallers := []string{
		"framework/agentenv/workspace.go",   // BootstrapAgentRuntime — kept
		"framework/agentenv/composition.go", // BuildWorkspaceEnvironment — TO BE DELETED in Phase 12
	}

	for _, line := range grepLines(out) {
		if strings.Contains(line, "framework/services/capability_bundle.go") {
			continue
		}
		if !matchedByAny(line, knownBundleCallers) {
			t.Errorf("unexpected BuildBuiltinCapabilityBundle caller (new composition root?):\n  %s", line)
		}
	}

	// --- agentenv.OpenWorkspace callers ----------------------------------------
	// After Phase 5 the legacy agentenv.Open symbol no longer exists. All callers
	// use OpenWorkspace.
	owCmd := exec.Command("grep", "-rn", `agentenv\.OpenWorkspace(`, "--include=*.go", ".")
	owCmd.Dir = root
	owOut, owErr := owCmd.Output()
	if owErr != nil {
		t.Fatalf("grep agentenv.OpenWorkspace failed: %v", owErr)
	}

	knownOWCallers := []string{
		"app/relurpish/runtime/runtime.go",                     // primary entry point
		"app/dev-agent-cli/agenttest_workspace.go",             // inspection+prepared-run entry
		"app/dev-agent-cli/workspace.go",                       // workspaceOpenFn var assignment
		"app/nexus/server/rex_runtime.go",                      // nexus entry point
		"named/euclo/testsuite/live_workspace_handshake_test.go", // integration test
		"named/euclo/doc.go",                                   // doc-comment reference
	}

	for _, line := range grepLines(owOut) {
		if !matchedByAny(line, knownOWCallers) {
			t.Errorf("unexpected agentenv.OpenWorkspace caller:\n  %s", line)
		}
	}

	// Verify the legacy symbol no longer exists.
	legacyCmd := exec.Command("grep", "-rn", `agentenv\.Open\b`, "--include=*.go", ".")
	legacyCmd.Dir = root
	legacyOut, _ := legacyCmd.Output()
	for _, line := range grepLines(legacyOut) {
		// Skip lines that mention the legacy symbol in a historical context.
		if strings.Contains(line, "boot_inventory_test.go") {
			continue
		}
		t.Errorf("legacy agentenv.Open symbol still present (must be removed in Phase 5):\n  %s", line)
	}

	// --- BuildWorkspaceEnvironment callers ------------------------------------
	bweCmd := exec.Command("grep", "-rn", `BuildWorkspaceEnvironment(`, "--include=*.go", ".")
	bweCmd.Dir = root
	bweOut, bweErr := bweCmd.Output()
	if bweErr != nil {
		t.Fatalf("grep BuildWorkspaceEnvironment failed: %v", bweErr)
	}

	knownBWECallers := []string{
		"framework/agentenv/composition.go",                      // definition
		"framework/agentenv/composition_test.go",                 // test cases
		"app/nexus/server/rex_runtime.go",                        // nexus entry point — TO BE MIGRATED in Phase 6
		"named/euclo/testsuite/live_workspace_handshake_test.go", // integration test — TO BE MIGRATED in Phase 6
	}

	for _, line := range grepLines(bweOut) {
		if !matchedByAny(line, knownBWECallers) {
			t.Errorf("unexpected BuildWorkspaceEnvironment caller (must be migrated in Phase 6):\n  %s", line)
		}
	}
}

// TestBackendVocabularyInventory asserts that every backend-string switch in
// the codebase is known and accounted for, with the single source of truth
// living in authorization/runtime.go:SelectSandboxRuntime (which derives from
// sandbox.SupportedSandboxBackends). Known additional sites (the TUI pane,
// sandbox.go container-runtime check) are allowlisted.
func TestBackendVocabularyInventory(t *testing.T) {
	root := findModuleRoot(t)

	// Search for Go files containing backend-enum switch/case patterns.
	// The regex matches case "gvisor" or case "docker" (without requiring
	// the closing quote, which varies by context).
	cmd := exec.Command("grep", "-rnE",
		`case "(gvisor|docker)`,
		"--include=*.go", ".")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 && len(out) == 0 {
			t.Fatal("no backend-switch sites found — SelectSandboxRuntime definition may have moved")
		}
		t.Fatalf("grep backend switch failed: %v", err)
	}

	knownBackendSites := []string{
		"framework/authorization/runtime.go",     // SelectSandboxRuntime — single source of truth
		"framework/authorization/runtime_test.go", // tests for SelectSandboxRuntime
		"app/relurpish/tui/runtime_adapter.go",   // TUI backend display — legitimate leaf use
		"framework/sandbox/sandbox.go",           // checkContainerRuntime — container-runtime check
		"framework/agentenv/boot_validate.go",    // backendsCompatible — Phase 10 boot invariant check
	}

	violations := 0
	total := 0
	for _, line := range grepLines(out) {
		total++
		if !matchedByAny(line, knownBackendSites) {
			t.Errorf("unknown backend-string switch site (new sandbox backend?):\n  %s", line)
			violations++
		}
	}

	t.Logf("backend-string switch sites: %d total, %d known, %d unexpected",
		total, total-violations, violations)
}

// grepLines splits grep output and filters out lines from this test file
// (which inevitably contain the search terms in comments and grep commands).
func grepLines(out []byte) []string {
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, selfFile) {
			continue
		}
		result = append(result, line)
	}
	return result
}

func matchedByAny(line string, known []string) bool {
	for _, k := range known {
		if strings.Contains(line, k) {
			return true
		}
	}
	return false
}

// TestNoSecondBootRoot asserts that BuildBuiltinCapabilityBundle is only
// called by BootstrapAgentRuntime (framework/agentenv/workspace.go) in
// production code. This prevents reintroduction of a second composition root.
func TestNoSecondBootRoot(t *testing.T) {
	root := findModuleRoot(t)

	cmd := exec.Command("grep", "-rn", `BuildBuiltinCapabilityBundle(`, "--include=*.go", ".")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("grep BuildBuiltinCapabilityBundle failed: %v", err)
	}

	allowed := "framework/agentenv/workspace.go"
	for _, line := range grepLines(out) {
		if strings.Contains(line, "framework/services/capability_bundle.go") {
			continue
		}
		if strings.Contains(line, allowed) {
			continue
		}
		// Allow doc.go and test files (comments, grep commands).
		if strings.Contains(line, "doc.go") || strings.Contains(line, "_test.go") || strings.Contains(line, "fakerunner.go") {
			continue
		}
		t.Errorf("second composition root detected (BuildBuiltinCapabilityBundle caller outside %s):\n  %s", allowed, line)
	}
}

// TestNoLegacyBootSymbols asserts that the legacy symbols agentenv.Open and
// BuildWorkspaceEnvironment no longer exist in production code. The rename
// (Phase 5) and fork deletion (Phase 6) are locked.
func TestNoLegacyBootSymbols(t *testing.T) {
	root := findModuleRoot(t)

	for _, symbol := range []string{`agentenv\.Open(`, `BuildWorkspaceEnvironment(`} {
		cmd := exec.Command("grep", "-rn", symbol, "--include=*.go", ".")
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			continue // grep exits 1 when no match — that's success
		}
		violations := false
		for _, line := range grepLines(out) {
			if strings.Contains(line, "_test.go") || strings.Contains(line, "doc.go") {
				continue
			}
			t.Errorf("legacy symbol %q must not appear in production code:\n  %s", symbol, line)
			violations = true
		}
		if violations {
			t.Logf("legacy symbol %q found — the rename/fork-deletion may have regressed", symbol)
		}
	}
}

// TestSingleBootRootScriptFails is a self-test of the CI guard script.
// It verifies that a file injecting a second BuildBuiltinCapabilityBundle
// caller is rejected by the script. The fixture is a temporary Go file
// that references the bundle function.
func TestSingleBootRootScriptFails(t *testing.T) {
	root := findModuleRoot(t)

	// Create a temp file that introduces a second bundle caller.
	tmpDir := t.TempDir()
	violation := tmpDir + "/violation.go"
	if err := os.WriteFile(violation, []byte(`package violation
import "codeburg.org/lexbit/relurpify/framework/services"
func f() { services.BuildBuiltinCapabilityBundle("", nil) }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run the check script from the temp root (without our temp file it's fine).
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "check-single-boot-root.sh"))
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script should pass on clean tree, got: %v\n%s", err, out)
	}

	// Now create a violation within the module tree.
	// Use a non-_test.go name so the script does not filter it out.
	violation2 := filepath.Join(root, "zz_violation_check.go")
	if err := os.WriteFile(violation2, []byte(`package main
import "codeburg.org/lexbit/relurpify/framework/services"
func f() { services.BuildBuiltinCapabilityBundle("/tmp", nil) }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(violation2)

	cmd2 := exec.Command("bash", filepath.Join(root, "scripts", "check-single-boot-root.sh"))
	cmd2.Dir = root
	cmd2.Env = os.Environ()
	out2, err2 := cmd2.CombinedOutput()
	if err2 == nil {
		t.Error("script should fail when a second bundle caller is present")
	}
	if !strings.Contains(string(out2), "FAIL") {
		t.Errorf("script output should contain FAIL, got:\n%s", out2)
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root (go.mod) not found")
		}
		dir = parent
	}
}
