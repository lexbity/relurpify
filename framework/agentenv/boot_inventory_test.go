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

// TestBootRootInventory enumerates the known callers of composition-root
// functions (Open, BuildWorkspaceEnvironment) and fails if any new caller
// appears. This locks the baseline for Phases 5, 6, and 12.
//
// Current composition roots (both call BuildBuiltinCapabilityBundle):
//   - framework/agentenv/workspace.go   (Open -> BootstrapAgentRuntime)
//   - framework/agentenv/composition.go (BuildWorkspaceEnvironment, TO BE DELETED Phase 12)
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
