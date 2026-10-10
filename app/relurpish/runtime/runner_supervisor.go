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

// childStopGrace bounds the quiesce stop: SIGTERM → 5 s → SIGKILL (S9 r2:
// the stop runs as a quiesce step, not a bespoke kill inside rt.Close).
const childStopGrace = 5 * time.Second

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
	if now.Sub(status.HeartbeatAt) > runnerHeartbeatStale {
		return false
	}
	return processAlive(status.PID)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
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
		return runnerDegradedStatus(fmt.Sprintf("runner heartbeat not observed within %s", spawnWait)), childStop(child), nil
	}
	return DependencyStatus{
		Name:      "runner",
		Required:  false,
		Available: true,
		Details:   fmt.Sprintf("spawned (pid %d)", child.Pid),
	}, childStop(child), nil
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

// childStop returns the quiesce step: SIGTERM → grace → SIGKILL, and the
// wait reaps the child. Registered on the runtime's quiesce sequence so
// Close stops the runner after the app's own services (S9 r2).
func childStop(child *os.Process) func() {
	return func() {
		if child == nil {
			return
		}
		_ = child.Signal(syscall.SIGTERM)
		deadline := time.Now().Add(childStopGrace)
		for {
			if !processAlive(child.Pid) {
				_, _ = child.Wait()
				return
			}
			if time.Now().After(deadline) {
				_ = child.Kill()
				_, _ = child.Wait()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// runnerDependencyDetails is the status.json summary doctor renders when the
// runner is up: IDs and counts only, never prompt content (§5.7).
func runnerDependencyDetails(stateDir string) string {
	status, ok := readRunnerStatus(stateDir)
	if !ok {
		return "no status"
	}
	return fmt.Sprintf("runner %s pid %d heartbeat %s",
		status.RunnerID, status.PID, status.HeartbeatAt.Format(time.RFC3339))
}
