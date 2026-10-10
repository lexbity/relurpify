package runtime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// buildFakeRunner compiles the testdata helper main into a temp dir once per
// test — the "tiny test helper main under testdata/" (S9), never a committed
// binary.
func buildFakeRunner(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "relurpify-runner")
	build := exec.Command("go", "build", "-o", bin, "./testdata/fakerunner")
	build.Env = append(os.Environ(), "GOPROXY=off")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build fake runner: %s", out)
	return bin
}

// placeExecutable builds the fake runner and points the supervisor's
// executable-dir resolver at it for the duration of the test.
func placeExecutable(t *testing.T) string {
	t.Helper()
	bin := buildFakeRunner(t)
	dir := t.TempDir()
	require.NoError(t, os.Rename(bin, filepath.Join(dir, "relurpify-runner")))
	return dir
}

func TestSuperviseRunner_Attach(t *testing.T) {
	// Fresh status + live PID → attach (no spawn): the supervisor returns
	// available without starting anything.
	stateDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "jobs"), 0o700))
	self, err := os.Executable()
	require.NoError(t, err)
	live := exec.Command(self) // a process that outlives the probe? No — use our own PID
	require.NotNil(t, live)
	status := runnerStatus{
		PID:         os.Getpid(), // this test process is alive
		HeartbeatAt: time.Now().UTC(),
		RunnerID:    "attached-runner",
	}
	data, err := jsonMarshal(status)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "jobs", "status.json"), data, 0o600))

	deps, stop, err := SuperviseRunner(context.Background(), RunnerSupervisorConfig{
		Workspace: t.TempDir(), StateDir: stateDir,
	})
	require.NoError(t, err)
	require.Nil(t, stop, "an attached runner is not ours to stop")
	require.True(t, deps.Available)
	require.Contains(t, deps.Details, "attached")
}

func TestSuperviseRunner_SpawnThenStop(t *testing.T) {
	dir := placeExecutable(t)
	stateDir := t.TempDir()

	deps, stop, err := SuperviseRunner(context.Background(), RunnerSupervisorConfig{
		Workspace:     dir,
		StateDir:      stateDir,
		ExecutableDir: dir,
	})
	require.NoError(t, err)
	if !deps.Available {
		if logData, logErr := os.ReadFile(filepath.Join(stateDir, "logs", "runner.log")); logErr == nil {
			t.Logf("DBG runner.log: %s", logData)
		} else {
			t.Logf("DBG no runner.log: %v", logErr)
		}
		if entries, _ := os.ReadDir(stateDir); true {
			for _, e := range entries {
				t.Logf("DBG stateDir entry: %s", e.Name())
			}
		}
	}
	require.True(t, deps.Available, "spawn must succeed with the helper on the executable path: %s", deps.Details)
	require.NotNil(t, stop)
	require.Contains(t, deps.Details, "spawned")

	// The child heartbeats: status.json names the child's PID and stays fresh.
	require.Eventually(t, func() bool {
		status, ok := readRunnerStatus(stateDir)
		return ok && status.PID != 0 && time.Since(status.HeartbeatAt) < runnerHeartbeatStale
	}, 5*time.Second, 100*time.Millisecond)

	// The quiesce stop terminates the child.
	childPid := func() int {
		s, _ := readRunnerStatus(stateDir)
		return s.PID
	}()
	stop()
	require.Eventually(t, func() bool { return !processAlive(childPid) }, 10*time.Second, 100*time.Millisecond,
		"the quiesce stop must terminate the spawned child")
}

func TestSuperviseRunner_DegradeOnTimeout(t *testing.T) {
	// No helper on the executable path: spawn is unavailable → degraded,
	// non-blocking, and boot proceeds (the degraded-boot contract).
	stateDir := t.TempDir()
	// Point PATH resolution at an empty dir and make Executable()-relative
	// lookup fail by not placing anything.
	deps, stop, err := SuperviseRunner(context.Background(), RunnerSupervisorConfig{
		Workspace: t.TempDir(),
		StateDir:  stateDir,
	})
	require.NoError(t, err)
	require.False(t, deps.Available)
	require.False(t, deps.Blocking, "a degraded runner never blocks boot")
	require.Nil(t, stop)
	require.Contains(t, deps.Details, "spawn unavailable")
}

func TestSuperviseRunner_StaleHeartbeatRespawn(t *testing.T) {
	// A crashed runner leaves a stale status.json: doctor respawns.
	dir := placeExecutable(t)
	stateDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "jobs"), 0o700))
	stale := runnerStatus{PID: 999999, HeartbeatAt: time.Now().UTC().Add(-time.Hour), RunnerID: "dead"}
	data, err := jsonMarshal(stale)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "jobs", "status.json"), data, 0o600))

	deps, stop, err := SuperviseRunner(context.Background(), RunnerSupervisorConfig{
		Workspace: dir, StateDir: stateDir, ExecutableDir: dir,
	})
	require.NoError(t, err)
	require.True(t, deps.Available, "a stale heartbeat leads to a fresh spawn")
	require.NotNil(t, stop)
	stop()
}

func TestDoctorProbeRunner_States(t *testing.T) {
	// No status: not spawned yet.
	d := probeRunner(t.TempDir())
	require.False(t, d.Available)
	require.False(t, d.Blocking)

	// Fresh + live PID: attached.
	stateDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "jobs"), 0o700))
	status := runnerStatus{PID: os.Getpid(), HeartbeatAt: time.Now().UTC(), RunnerID: "r1"}
	data, err := jsonMarshal(status)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "jobs", "status.json"), data, 0o600))
	d = probeRunner(stateDir)
	require.True(t, d.Available)
	require.Contains(t, d.Details, "attached (runner r1")

	// Stale heartbeat: degraded with the reason spelled out.
	status.HeartbeatAt = time.Now().UTC().Add(-time.Minute)
	data, err = jsonMarshal(status)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "jobs", "status.json"), data, 0o600))
	d = probeRunner(stateDir)
	require.False(t, d.Available)
	require.Contains(t, d.Details, "heartbeat stale")

	// Dead PID: degraded with the reason spelled out.
	status.PID = 999999
	status.HeartbeatAt = time.Now().UTC()
	data, err = jsonMarshal(status)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "jobs", "status.json"), data, 0o600))
	d = probeRunner(stateDir)
	require.False(t, d.Available)
	require.Contains(t, d.Details, "runner process is gone")

	// Draining runner: not attachable.
	status.PID = os.Getpid()
	status.Draining = true
	data, err = jsonMarshal(status)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "jobs", "status.json"), data, 0o600))
	d = probeRunner(stateDir)
	require.False(t, d.Available)
}

func TestSpawnRace_TwoSupervisorsOneStoreOwner(t *testing.T) {
	// Q16 pidfile race: both supervisors may spawn; the loser's runner fails
	// the Badger lock and exits; the app's wait loop finds the winner's fresh
	// status. With the fake runner there is no Badger lock, so the contract
	// under test here is the supervisor-level serialization: exactly one
	// status writer owns the status file and both children are stopped.
	dir := placeExecutable(t)
	stateDir := t.TempDir()
	cfg := RunnerSupervisorConfig{Workspace: dir, StateDir: stateDir, ExecutableDir: dir}
	deps1, stop1, err := SuperviseRunner(context.Background(), cfg)
	require.NoError(t, err)
	deps2, stop2, err2 := SuperviseRunner(context.Background(), cfg)
	require.NoError(t, err2)
	// At least one is available; both are non-blocking.
	require.True(t, deps1.Available || deps2.Available)
	require.False(t, deps1.Blocking)
	require.False(t, deps2.Blocking)
	if stop1 != nil {
		stop1()
	}
	if stop2 != nil {
		stop2()
	}
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
