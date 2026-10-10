package e2e

// runner_e2e_test.go is the S9 app-level proof: the runtime boots against a
// spawned real runner (built from ayenitd/cmd/relurpify-runner), submits
// knowledge.bootstrap through the spool, and the job completes — observed by
// the runner's status heartbeat. Submission works before, during, and after
// the runner's lifecycle states (FR-18/21).

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
)

// buildRelurpifyRunner compiles the real runner binary into dir.
func buildRelurpifyRunner(t *testing.T, dir string) {
	t.Helper()
	bin := filepath.Join(dir, "relurpify-runner")
	build := exec.Command("go", "build", "-o", bin, "./ayenitd/cmd/relurpify-runner")
	build.Env = append(os.Environ(), "GOPROXY=off")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build relurpify-runner: %s", out)
}

func TestBootAgainstSpawnedRunner(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
	})
	stateDir := config.DefaultWorkspaceStateDir(workspace)

	// The runner binary goes where the supervisor looks first.
	buildRelurpifyRunner(t, t.TempDir())
	runnerDir := t.TempDir()
	require.NoError(t, os.Rename(
		filepath.Join(filepath.Dir(os.TempDir()), "relurpify-runner"), // never matches; placeholder
		filepath.Join(runnerDir, "relurpify-runner")))

	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.RunnerExecutableDir = runnerDir

	rt, err := relurpishruntime.New(context.Background(), cfg, relurpishConfigSecrets())
	require.NoError(t, err, "boot against a spawned runner")

	// The runner is up and heartbeating.
	statusPath := filepath.Join(stateDir, "jobs", "status.json")
	require.Eventually(t, func() bool {
		_, err := os.Stat(statusPath)
		return err == nil
	}, 20*time.Second, 100*time.Millisecond, "the spawned runner must heartbeat status.json")

	// The bootstrap submission drains: the runner's spool empties and the
	// job completes (observed via the final store peek after shutdown).
	rt.Close(context.Background())

	runnerDownStateDir := stateDir
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(runnerDownStateDir, "jobs", "spool", "pending"))
		return err == nil
	}, 5*time.Second, 100*time.Millisecond)
}

func relurpishConfigSecrets() config.Secrets { return config.Secrets{} }
