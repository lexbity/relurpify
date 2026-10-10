package e2e

// runner_e2e_test.go is the S9 app-level proof of the runner dual path:
//
//   - TestBootAgainstSpawnedRunner: the runtime boots against a spawned real
//     runner, submits knowledge.bootstrap through the spool, and the job
//     completes — observed by the runner's status heartbeat. Close stops the
//     spawned runner through the quiesce step (the final draining flush).
//   - TestBootDegradedRunnerDoesNotSpawn: with no runner executable
//     discoverable the supervisor degrades (non-blocking), boot proceeds, and
//     nothing is spawned or submitted — the in-process bootstrap path is
//     taken. That path's happy case (index pass runs, file lands in the
//     index, BootstrapComplete emitted) is unit-proven in
//     context/knowledge/bootstrap_service_test.go.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// buildRelurpifyRunner compiles the real runner binary into dir. The build
// runs from the repo root (the test binary's cwd is the package dir).
func buildRelurpifyRunner(t *testing.T, dir string) {
	t.Helper()
	bin := filepath.Join(dir, "relurpify-runner")
	root, err := repoRoot()
	require.NoError(t, err, "locate repo root")
	build := exec.Command("go", "build", "-C", root, "-o", bin, "./ayenitd/cmd/relurpify-runner")
	build.Env = append(os.Environ(), "GOPROXY=off")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build relurpify-runner: %s", out)
}

// repoRoot returns the module root, located from this file's path
// (app/relurpish/e2e/ → three levels up).
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locate test source file")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// runnerStatusFile is the app-side view of <state>/jobs/status.json — the app
// reads it as plain data and must not import the runner's binary packages
// for one struct.
type runnerStatusFile struct {
	Draining bool `json:"draining"`
	Recent   []struct {
		CorrelateID string `json:"correlate_id"`
		Kind        string `json:"kind"`
		State       string `json:"state"`
	} `json:"recent"`
}

func readRunnerStatusFile(t *testing.T, stateDir string) (runnerStatusFile, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(stateDir, "jobs", "status.json"))
	if err != nil {
		return runnerStatusFile{}, false
	}
	var st runnerStatusFile
	if err := json.Unmarshal(raw, &st); err != nil {
		return runnerStatusFile{}, false
	}
	return st, true
}

func TestBootAgainstSpawnedRunner(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
	})
	stateDir := config.DefaultWorkspaceStateDir(workspace)

	// The runner binary goes where the supervisor looks first.
	runnerDir := t.TempDir()
	buildRelurpifyRunner(t, runnerDir)

	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.RunnerExecutableDir = runnerDir

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	require.NoError(t, err, "boot against a spawned runner")

	// The runner is up and heartbeating.
	statusPath := filepath.Join(stateDir, "jobs", "status.json")
	require.Eventually(t, func() bool {
		_, err := os.Stat(statusPath)
		return err == nil
	}, 20*time.Second, 100*time.Millisecond, "the spawned runner must heartbeat status.json")

	// The bootstrap submission drains and the job completes — observed via
	// the status heartbeat's recent window.
	require.Eventually(t, func() bool {
		st, ok := readRunnerStatusFile(t, stateDir)
		if !ok {
			return false
		}
		for _, j := range st.Recent {
			if j.Kind == "knowledge.bootstrap" && j.State == "completed" {
				return true
			}
		}
		return false
	}, 60*time.Second, 250*time.Millisecond, "the bootstrap job must complete in the runner")

	// Close stops the spawned runner through the quiesce step: the runner's
	// final status flush carries draining=true.
	rt.Close(context.Background())
	require.Eventually(t, func() bool {
		st, ok := readRunnerStatusFile(t, stateDir)
		return ok && st.Draining
	}, 10*time.Second, 100*time.Millisecond, "Close must drain the runner via the quiesce step")
}

// TestBootDegradedRunnerDoesNotSpawn is the degraded half of the S9 dual
// path: with no runner executable discoverable, the supervisor degrades
// (non-blocking), boot proceeds, and nothing is spawned or submitted — the
// in-process bootstrap path is taken.
func TestBootDegradedRunnerDoesNotSpawn(t *testing.T) {
	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
	})
	stateDir := config.DefaultWorkspaceStateDir(workspace)

	// An empty executable dir guarantees the supervisor finds no runner:
	// spawn unavailable → degraded, boot proceeds (the degraded-boot
	// contract, FR-21).
	cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
	cfg.InferenceProvider = "offline"
	cfg.InferenceModel = "offline-synthetic"
	cfg.RunnerExecutableDir = t.TempDir()

	rt, err := relurpishruntime.New(context.Background(), cfg, config.Secrets{})
	require.NoError(t, err, "a degraded runner never aborts boot")

	// Nothing was spawned: no runner status exists.
	_, statErr := os.Stat(filepath.Join(stateDir, "jobs", "status.json"))
	require.True(t, os.IsNotExist(statErr), "a degraded runner must not spawn")

	// The in-process path was taken: no bootstrap submission reached the
	// spool (a healthy runner would have spooled one — see the test above).
	_, spoolErr := os.Stat(filepath.Join(stateDir, "jobs", "spool", "pending"))
	require.True(t, os.IsNotExist(spoolErr), "a degraded runner must not submit through the spool")

	require.NoError(t, rt.Close(context.Background()))
}
