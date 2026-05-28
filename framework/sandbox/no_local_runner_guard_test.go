//go:build guard

// Guard test — enabled with `go test -tags guard ./framework/sandbox/`.
// Asserts that NewLocalCommandRunner is absent from the entire module.
// Remove the build tag in Phase 5 to run in normal CI.
//
// Inventory of NewLocalCommandRunner references (Phase 0 baseline, 2026-05-28):
//
// Production call sites (3):
//   framework/agentenv/composition.go:116   BuildWorkspaceEnvironment (nexus server)
//   app/dev-agent-cli/tool_exec.go:55       dev CLI tool-exec command
//   testsuite/agenttest/verification_driver.go:37  test harness
//
// Test files (2):
//   framework/sandbox/local_command_runner_env_test.go
//   framework/sandbox/local_command_runner_processgroup_test.go
//
// Definition (1):
//   framework/sandbox/local_command_runner.go
//
// Helpers used ONLY by the local-runner path:
//   cappedBuffer                   local_command_runner.go (also copied in dockersandbox/runner.go)
//   assembleSubprocessEnv          enforcement_env.go — only called from local_command_runner.go
//   splitEnvEntry                  enforcement_env.go — only called from assembleSubprocessEnv
//   SubprocessEnvAllowlist         cfgload/security/subprocess_env.go — only called from composition.go:118
//   defaultSubprocessEnvAllowlist  local_command_runner.go — only referenced in local_command_runner.go
//   DefaultTimeout                 local_command_runner.go — only referenced in local_command_runner.go
//
// Phase 5 deletes: local_command_runner.go, enforcement_env.go, enforcement_env_test.go,
// local_command_runner_env_test.go, local_command_runner_processgroup_test.go,
// and cfgload/security/subprocess_env.go (when its only caller composition.go migrates in Phase 3).

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
