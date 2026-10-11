// runner_supervisor.go is the app-side runner lifecycle (Q16, FR-21):
// doctor-first (a fresh status.json heartbeat plus a live PID means attach,
// not spawn), spawn otherwise (executable-dir resolution, Setsid process
// group, log redirect to <state>/logs/runner.log, ≤10 s wait for the child's
// heartbeat), and degrade-on-timeout — a degraded runner never aborts boot
// (the degraded-boot contract: degrade, log, emit boot.degraded).
//
// The supervisor never imports context/jobsstore: submission is spool-only
// (ayenitd.SpoolClient), and the Badger lock stays with the runner (R3).
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// runnerHeartbeatStale is the doctor's runner-down threshold (FR-22/NFR-5):
// heartbeats land every 5 s; 15 s of silence means the runner is down.
const runnerHeartbeatStale = 15 * time.Second

// spawnWaitBounds the post-spawn heartbeat wait (Q16: 10 s, 200 ms poll).
const (
	spawnWait      = 10 * time.Second
	spawnPollEvery = 200 * time.Millisecond
)

// childStopGrace bounds the quiesce stop: SIGTERM → 25 s → SIGKILL. The 25 s
// honors the runner's own 20 s drain window (NFR-2) end-to-end — SIGKILL at
// 5 s silently voided the entire drain design and converted every app close
// into crash recovery for in-flight jobs. (S9 r2: the stop runs as a quiesce
// step, not a bespoke kill inside rt.Close.)
const childStopGrace = 25 * time.Second

// RunnerStatus mirrors ayenitd.RunnerStatus — the app reads status.json as
// plain data and must not import the runner's binary packages for one struct.
type runnerStatus struct {
	PID         int       `json:"pid"`
	HeartbeatAt time.Time `json:"heartbeat_at"`
	Draining    bool      `json:"draining"`
	RunnerID    string    `json:"runner_id"`
}

// readRunnerStatus loads and parses <state>/jobs/status.json.
func readRunnerStatus(stateDir string) (runnerStatus, bool) {
	raw, err := os.ReadFile(filepath.Join(stateDir, "jobs", "status.json"))
	if err != nil {
		return runnerStatus{}, false
	}
	var status runnerStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return runnerStatus{}, false
	}
	return status, true
}

// runnerAlive reports whether the status heartbeat is fresh and the recorded
// PID is live (signal-0 probe). A hung runner (live PID, stale heartbeat) is
// reported degraded — never ambient-killed (Q16: killing a Badger holder is
// an operator decision).
func runnerAlive(status runnerStatus, now time.Time) bool {
	if status.PID <= 0 {
		return false
	}
	if status.Draining {
		return false
	}
	// ≥ 15 s is runner-down (FR-22): the comparison is inclusive.
	if now.Sub(status.HeartbeatAt) >= runnerHeartbeatStale {
		return false
	}
	return processAlive(status.PID)
}

// RunnerSupervisorConfig carries the spawn parameters.
type RunnerSupervisorConfig struct {
	Workspace       string        // workspace root handed to the runner
	StateDir        string        // runtime state dir (spool, status.json, logs)
	RefreshInterval time.Duration // scheduled refresh cadence handed to the runner (0 = off)
	// ExecutableDir overrides the runner-binary search directory. Empty means
	// next to the current executable (the shipped layout), then PATH.
	ExecutableDir string
	Tel           telemetry.Telemetry
}

// SuperviseRunner runs the doctor-first attach/spawn/degrade decision and
// returns the dependency status plus the stop function for the quiesce
// sequence (nil when attached — an attached runner is not ours to stop).
func SuperviseRunner(ctx context.Context, cfg RunnerSupervisorConfig) (DependencyStatus, func(), error) {
	_ = os.MkdirAll(filepath.Join(cfg.StateDir, "logs"), 0o700)

	if status, ok := readRunnerStatus(cfg.StateDir); ok && runnerAlive(status, time.Now()) {
		return DependencyStatus{
			Name:      "runner",
			Required:  false,
			Available: true,
			Details:   fmt.Sprintf("attached (runner %s, pid %d)", status.RunnerID, status.PID),
		}, nil, nil
	}

	// Stale status.json from a crashed runner: it ages out on its own; the
	// spawn below writes a fresh one.
	child, err := spawnRunner(cfg)
	if err != nil {
		return runnerDegradedStatus(fmt.Sprintf("spawn unavailable: %v", err)), nil, nil
	}

	if err := awaitChildHeartbeat(cfg.StateDir, child.Pid, spawnWait); err != nil {
		// Degrade, never abort boot. The child may still come up; the stop
		// hook stays registered so Close reaps it either way.
		return runnerDegradedStatus(fmt.Sprintf("runner heartbeat not observed within %s", spawnWait)), childStopWithGrace(child, cfg.Tel, childStopGrace), nil
	}
	return DependencyStatus{
		Name:      "runner",
		Required:  false,
		Available: true,
		Details:   fmt.Sprintf("spawned (pid %d)", child.Pid),
	}, childStopWithGrace(child, cfg.Tel, childStopGrace), nil
}

func runnerDegradedStatus(reason string) DependencyStatus {
	return DependencyStatus{
		Name:      "runner",
		Required:  false,
		Available: false,
		Blocking:  false,
		Details:   reason,
	}
}

// spawnRunner resolves the runner executable next to the current binary,
// then the PATH, and starts it in its own process group (Setsid) with
// stdout/stderr appended to <state>/logs/runner.log. cmd.Env is inherited —
// the runner reads config through userconfig like everything else and reads
// no process env directly (AGENTS.md).
func spawnRunner(cfg RunnerSupervisorConfig) (*os.Process, error) {
	exe, err := resolveRunnerExecutable(cfg)
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(cfg.StateDir, "logs", "runner.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open runner log: %w", err)
	}
	args := []string{
		"--workspace", cfg.Workspace,
		"--state-dir", cfg.StateDir,
	}
	if cfg.RefreshInterval > 0 {
		args = append(args, "--refresh-interval", cfg.RefreshInterval.String())
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start %s: %w", exe, err)
	}
	go func() { _ = logFile.Close() }() // the child holds its own descriptors
	return cmd.Process, nil
}

// resolveRunnerExecutable prefers cfg.ExecutableDir, then the executable's
// own directory (the shipped layout: relurpish and relurpify-runner side by
// side), then PATH.
func resolveRunnerExecutable(cfg RunnerSupervisorConfig) (string, error) {
	for _, dir := range []string{cfg.ExecutableDir, executableDir()} {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "relurpify-runner")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return exec.LookPath("relurpify-runner")
}

// executableDir is the directory of the running binary.
func executableDir() string {
	if self, err := os.Executable(); err == nil {
		return filepath.Dir(self)
	}
	return ""
}

// awaitChildHeartbeat polls status.json for a fresh heartbeat naming the
// child's PID (Q16: 10 s at 200 ms).
func awaitChildHeartbeat(stateDir string, childPid int, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		if status, ok := readRunnerStatus(stateDir); ok &&
			status.PID == childPid &&
			time.Since(status.HeartbeatAt) <= runnerHeartbeatStale {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("heartbeat wait elapsed")
		}
		time.Sleep(spawnPollEvery)
	}
}

// childStop returns the quiesce step: SIGTERM → grace → SIGKILL, with the
// child reaped by a dedicated waiter goroutine. The waiter records the exit
// status under a mutex, so the liveness probe below reflects reality —
// without it, an un-Waited child is a zombie and kill(pid,0) succeeds on it,
// burning the whole grace on every stop (D7). A grace-exhausted kill emits
// runtime.runner_stop_timeout (NFR-6). Registered on the runtime's quiesce
// sequence so Close stops the runner after the app's own services (S9 r2).
// childStop is the production quiesce step: the configured 25 s grace.
func childStop(child *os.Process, tel telemetry.Telemetry) func() {
	return childStopWithGrace(child, tel, childStopGrace)
}

// childStopWithGrace is childStop parameterized over the grace so tests can
// exercise the SIGKILL path at millisecond scale.
func childStopWithGrace(child *os.Process, tel telemetry.Telemetry, grace time.Duration) func() {
	var (
		mu       sync.Mutex
		waitDone bool
	)
	release := make(chan struct{})
	go func() {
		childWait(child, release)
		mu.Lock()
		waitDone = true
		mu.Unlock()
	}()

	return func() {
		if child == nil {
			return
		}
		_ = child.Signal(syscall.SIGTERM)
		deadline := time.Now().Add(grace)
		for {
			mu.Lock()
			reaped := waitDone
			mu.Unlock()
			if reaped {
				return
			}
			if time.Now().After(deadline) {
				_ = child.Kill()
				close(release)
				emitRunnerStopTimeout(tel, child.Pid, grace)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// childWait waits for the child to exit, returning immediately when release
// is closed (the SIGKILL path): the kill unblocks the wait, and the waiter
// reaps whatever status the kill produced.
func childWait(child *os.Process, release <-chan struct{}) {
	waitc := make(chan struct{})
	go func() {
		_, _ = child.Wait()
		close(waitc)
	}()
	select {
	case <-waitc:
	case <-release:
		<-waitc
	}
}

func emitRunnerStopTimeout(tel telemetry.Telemetry, pid int, grace time.Duration) {
	if tel == nil {
		return
	}
	tel.Emit(telemetry.Event{
		Type:      telemetry.EventRunnerStopTimeout,
		Message:   "runner stop grace exhausted; killed",
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"pid": pid, "grace": grace.String()},
	})
}
