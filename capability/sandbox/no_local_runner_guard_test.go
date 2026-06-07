// Guard test — runs in normal CI to prevent reintroduction of the host-exec
// path. Asserts that NewLocalCommandRunner is absent from the entire module.
//
// NewLocalCommandRunner was removed in Phase 5 of the sandbox-centrality
// rework (2026-05-28). The following were deleted:
//   framework/sandbox/local_command_runner.go
//   framework/sandbox/local_command_runner_env_test.go
//   framework/sandbox/local_command_runner_processgroup_test.go
//   framework/sandbox/enforcement_env.go
//   framework/sandbox/enforcement_env_test.go
//   framework/cfgload/security/subprocess_env.go
//
// The test-harness fallback in testsuite/agenttest/verification_driver.go
// was switched to error-on-nil-runner in Phase 5.
//
// All eight phases of the sandbox-centrality rework are complete.
// See devdocs/plans/sandbox-central-rework.md for the full plan.

package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoLocalCommandRunner searches the entire module for the NewLocalCommandRunner
// identifier and fails if any occurrence remains. This guard exists to prevent
// reintroduction of the host-exec path after it has been removed.
func TestNoLocalCommandRunner(t *testing.T) {
	// Walk up to find the module root (go.mod)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root not found")
		}
		dir = parent
	}

	// Search all Go files in the module for the banned identifier.
	// grep exits 0 when matches are found (presence = failure) and exits 1
	// when none are found (absence = success).
	cmd := exec.Command("grep", "-rln", "NewLocalCommandRunner", "--include=*.go", "--exclude=no_local_runner_guard_test.go", ".")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		t.Errorf("NewLocalCommandRunner must be absent, but found in:\n%s", strings.TrimSpace(string(out)))
	}
}
