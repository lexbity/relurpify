package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

type (
	CommandRequest = ports.CommandRequest
	CommandRunner  = ports.CommandRunner
)

// NewCommandRunner returns a backend-specific runner when the runtime supports
// one, otherwise it falls back to the standard sandbox command runner.
func NewCommandRunner(config *CommandRunnerConfig, runtime SandboxRuntime) (CommandRunner, error) {
	if provider, ok := runtime.(CommandRunnerProvider); ok {
		return provider.NewCommandRunner(config)
	}
	return NewSandboxCommandRunner(config, runtime)
}

// Compile-time guarantee that the sandbox runner satisfies the CommandRunner API.
var _ CommandRunner = (*SandboxCommandRunner)(nil)

// SandboxCommandRunner launches commands via the configured sandbox runtime.
type SandboxCommandRunner struct {
	config          SandboxConfig
	rt              SandboxRuntime
	image           string
	workspace       string
	user            int
	readOnlyRoot    bool
	noNewPrivileges bool
	// nativeDocker selects Docker's native isolation (no `--runtime runsc`).
	// It is set by the docker backend's CommandRunnerProvider.
	nativeDocker bool
}

// NewSandboxCommandRunner wires the config/runtime metadata into a runner that
// launches commands through the runtime's container engine.
func NewSandboxCommandRunner(config *CommandRunnerConfig, runtime SandboxRuntime) (*SandboxCommandRunner, error) {
	return newSandboxCommandRunner(config, runtime, false)
}

func newSandboxCommandRunner(config *CommandRunnerConfig, runtime SandboxRuntime, nativeDocker bool) (*SandboxCommandRunner, error) {
	if config == nil {
		return nil, errors.New("config required")
	}
	if runtime == nil {
		return nil, errors.New("sandbox runtime required")
	}
	workspace := config.Workspace
	if workspace == "" {
		return nil, errors.New("workspace required")
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	absWorkspace = filepath.Clean(absWorkspace)
	return &SandboxCommandRunner{
		config:          runtime.RunConfig(),
		rt:              runtime,
		image:           config.Image,
		workspace:       absWorkspace,
		user:            config.RunAsUser,
		readOnlyRoot:    config.ReadOnlyRoot,
		noNewPrivileges: config.NoNewPrivileges,
		nativeDocker:    nativeDocker,
	}, nil
}

// Run executes the requested command inside the sandboxed container runtime.
//
// Lifecycle (SBH-1 D-11): the container is started DETACHED with deterministic
// owner labels, then `docker attach` streams stdout/stderr (stdin fed from
// Input) while `docker wait` yields the exit code. Teardown is owned by a
// ContainerHandle — the watchdog (timeout/context-cancel) triggers stop→rm -f
// (a guaranteed deferred rm -f covers every other path) — so a container can
// only outlive its supervisor until the next boot's orphan sweep.
func (r *SandboxCommandRunner) Run(ctx context.Context, req CommandRequest) (*ports.CommandResult, error) {
	if r == nil {
		return nil, errors.New("sandbox command runner missing")
	}
	if len(req.Args) == 0 {
		return nil, errors.New("command arguments required")
	}
	runtimeBinary := r.config.ContainerRuntime
	if strings.TrimSpace(runtimeBinary) == "" {
		runtimeBinary = "docker"
	}
	binaryPath, err := exec.LookPath(runtimeBinary)
	if err != nil {
		return nil, fmt.Errorf("%s not found: %w", runtimeBinary, err)
	}
	containerWorkdir, err := r.containerWorkdir(req.Workdir)
	if err != nil {
		return nil, err
	}

	// Container identity: deterministic, DNS-safe, owner-labeled.
	name := newContainerName(r.workspace)
	labels := containerLabels(r.workspace)
	args := r.runArgs(name, labels, containerWorkdir, req)

	// 1. Start detached: `docker run -d -i`. The container ID is cosmetic;
	// the NAME is the lifecycle key.
	start := time.Now()
	runCmd := spawn(binaryPath, args)
	runOut, runErr := runCmd.Output()
	if runErr != nil {
		return nil, fmt.Errorf("docker run: %w", runErr)
	}
	// The harness/CLI prints the container ID on stdout; we keep the name as
	// the durable identifier and only sanity-check that a run happened.
	_ = strings.TrimSpace(string(runOut))

	grace := GracePeriodOrDefault(req.GracePeriod)
	handle := NewContainerHandle(name, labels, binaryPath)
	// Guaranteed removal on every path: stop on an already-exited container is
	// a harmless error; the rm -f half always runs.
	defer handle.Teardown(context.Background(), grace)

	// 2. Attach: stdin + stdout/stderr. Its completion gates nothing — wait
	// dominates; a failed attach after container exit races benignly.
	ceiling := OutputCeilingOrDefault(req.OutputCeiling)
	attachStdout := newSpillWriter(ceiling)
	attachStderr := newSpillWriter(ceiling)
	attachCmd := spawn(binaryPath, []string{"attach", name})
	attachCmd.Stdout = attachStdout
	attachCmd.Stderr = attachStderr
	stdinPipe, stdinErr := attachCmd.StdinPipe()
	if err := attachCmd.Start(); err != nil {
		return nil, fmt.Errorf("docker attach: %w", err)
	}
	if stdinErr == nil {
		go func() {
			if req.Input != "" {
				_, _ = stdinPipe.Write([]byte(req.Input))
			}
			_ = stdinPipe.Close()
		}()
	}
	// Attach completion gates the result read: the spill writers must finish
	// draining before the main goroutine reads them (spill-writer ownership).
	attachDone := make(chan struct{})
	go func() {
		_ = attachCmd.Wait()
		close(attachDone)
	}()

	// 3. Wait concurrently: yields the container exit code.
	waitOut := &bytes.Buffer{}
	waitCmd := spawn(binaryPath, []string{"wait", name})
	waitCmd.Stdout = waitOut
	var waitErr error
	waitDone := make(chan struct{})
	if err := waitCmd.Start(); err != nil {
		return nil, fmt.Errorf("docker wait: %w", err)
	}
	go func() {
		waitErr = waitCmd.Wait()
		close(waitDone)
	}()

	// 4. Watchdog: timeout/cancel tears the container down and unblocks the
	// local docker CLI children (attach/wait process groups).
	tornDown := atomic.Bool{}
	go func() {
		var timerC <-chan time.Time
		if req.Timeout > 0 {
			timer := time.NewTimer(req.Timeout)
			defer timer.Stop()
			timerC = timer.C
		}
		select {
		case <-ctx.Done():
		case <-timerC:
		case <-waitDone:
			return
		}
		tornDown.Store(true)
		// Teardown must NOT inherit the cancelled ctx — the stop/rm commands
		// would fail before spawning.
		handle.Teardown(context.Background(), grace)
		killProcessGroup(attachCmd.Process)
		killProcessGroup(waitCmd.Process)
	}()

	<-waitDone
	// The container exited; ensure its output stream has been drained before
	// reading the spill writers.
	<-attachDone
	elapsed := time.Since(start)
	res := &ports.CommandResult{
		Stdout:      attachStdout.String(),
		Stderr:      attachStderr.String(),
		StdoutBytes: int64(attachStdout.Len()),
		StderrBytes: int64(attachStderr.Len()),
		Duration:    elapsed,
		TornDown:    tornDown.Load(),
	}
	if res.TornDown {
		res.ExitCode = -1
		res.TimedOut = true
		res.Signaled = true
	} else {
		res.ExitCode = waitExitCode(waitOut.String(), waitErr)
	}
	markOOM(res)
	return res, nil
}

// waitExitCode parses the exit code `docker wait` prints, falling back to the
// CLI's own exit status.
func waitExitCode(waitOut string, waitErr error) int {
	if trimmed := strings.TrimSpace(waitOut); trimmed != "" {
		if code, err := strconv.Atoi(trimmed); err == nil {
			return code
		}
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// spawn builds an exec.Cmd in its own process group so the watchdog can signal
// the whole docker CLI child set.
func spawn(binaryPath string, args []string) *exec.Cmd {
	return &exec.Cmd{
		Path:        binaryPath,
		Args:        append([]string{binaryPath}, args...),
		SysProcAttr: &syscall.SysProcAttr{Setpgid: true},
	}
}

// killProcessGroup SIGKILLs a local process group (nil-safe; a started-attach
// that already exited is a no-op).
func killProcessGroup(proc *os.Process) {
	if proc == nil {
		return
	}
	_ = syscall.Kill(-proc.Pid, syscall.SIGKILL)
}

// markOOM classifies a container exit as OOM-killed when the runner did not
// issue the kill and the process was terminated by SIGKILL (exit 137). Docker
// propagates the container's exit code through `docker wait`, so a
// kernel/cgroup OOM kill surfaces as 137 without the runner tearing the
// process down.
func markOOM(res *ports.CommandResult) {
	if res == nil || res.TornDown {
		return
	}
	if res.ExitCode == 137 {
		res.OOMKilled = true
		res.Signaled = true
	}
}

// workspaceHash8 derives the deterministic 8-hex workspace identity used in
// container names and owner labels.
func workspaceHash8(workspace string) string {
	sum := sha256.Sum256([]byte(workspace))
	return hex.EncodeToString(sum[:4])
}

// containerLabels builds the owner labels applied to every managed container.
func containerLabels(workspace string) map[string]string {
	return map[string]string{
		LabelManaged:   "true",
		LabelWorkspace: workspaceHash8(workspace),
		LabelPID:       strconv.Itoa(os.Getpid()),
		LabelCreated:   strconv.FormatInt(time.Now().Unix(), 10),
	}
}

// newContainerName produces the deterministic, DNS-safe container name:
// relurpify-<wsHash8>-<rand6>. The workspace prefix makes post-hoc reaping
// scoped; the random suffix keeps concurrent commands in one workspace apart.
func newContainerName(workspace string) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return "relurpify-" + workspaceHash8(workspace) + "-" + hex.EncodeToString(b)
}

// runArgs builds the container-engine argument vector for a command. It is a
// pure function of the runner's configuration so the exact isolation flags can
// be asserted without launching a container.
func (r *SandboxCommandRunner) runArgs(containerName string, labels map[string]string, containerWorkdir string, req CommandRequest) []string {
	args := []string{"run", "-d", "-i", "--name", containerName}
	if !r.nativeDocker {
		runtimeName := filepath.Base(r.config.RunscPath)
		if runtimeName == "" {
			runtimeName = "runsc"
		}
		args = append(args, "--runtime", runtimeName)
	}
	for _, key := range sortedLabelKeys(labels) {
		args = append(args, "--label", key+"="+labels[key])
	}
	args = append(args, "-v", fmt.Sprintf("%s:/workspace", r.workspace), "-w", containerWorkdir)
	for _, mount := range r.protectedMounts() {
		args = append(args, "-v", mount)
	}
	if r.user > 0 {
		args = append(args, "-u", strconv.Itoa(r.user))
	}
	if r.nativeDocker {
		// Docker native isolation: drop every Linux capability and rely on
		// seccomp plus Docker's default AppArmor profile for syscall
		// confinement (gVisor's runsc provides the equivalent boundary for the
		// gvisor backend).
		args = append(args, "--cap-drop", "ALL")
	}
	if r.readOnlyRoot {
		args = append(args, "--read-only")
		if r.nativeDocker {
			args = append(args, "--tmpfs", "/tmp")
		}
	}
	if r.noNewPrivileges {
		args = append(args, "--security-opt", "no-new-privileges")
	}
	if r.config.SeccompProfile != "" {
		args = append(args, "--security-opt", "seccomp="+r.config.SeccompProfile)
	}
	// Network isolation: always pass --network none when isolation is requested.
	// Declared NetworkRules do NOT relax isolation — granular per-rule egress is
	// not enforceable at the packet level here, so opening the container network
	// because rules exist would be an unsafe fallback (SF-3). Network access
	// requires explicitly disabling NetworkIsolation in the sandbox config;
	// brokered egress filtering is a separate, future capability.
	if r.config.NetworkIsolation {
		args = append(args, "--network", "none")
	}
	// Resource limits from CommandRequest (defaults applied from contracts).
	args = append(args, "--memory", strconv.FormatInt(MemoryBytesOrDefault(req.MemoryBytes), 10))
	args = append(args, "--pids-limit", strconv.FormatInt(PidsLimitOrDefault(req.PidsLimit), 10))
	args = append(args, "--cpus", strconv.FormatFloat(CPUsOrDefault(req.CPUs), 'f', -1, 64))
	for _, env := range req.Env {
		if env == "" {
			continue
		}
		args = append(args, "-e", env)
	}
	image := r.image
	if strings.TrimSpace(image) == "" {
		image = "ghcr.io/lexcodex/relurpify/runtime:0.4.1"
	}
	args = append(args, image)
	args = append(args, req.Args...)
	return args
}

// sortedLabelKeys returns label keys in a stable order so argv can be
// asserted deterministically.
func sortedLabelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (r *SandboxCommandRunner) protectedMounts() []string {
	if r == nil || r.rt == nil {
		return nil
	}
	policy := r.rt.Policy()
	if len(policy.ProtectedPaths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(policy.ProtectedPaths))
	var mounts []string
	for _, path := range policy.ProtectedPaths {
		path = filepath.Clean(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		rel, err := filepath.Rel(r.workspace, path)
		if err != nil {
			continue
		}
		if strings.HasPrefix(rel, "..") {
			continue
		}
		containerPath := filepath.ToSlash(filepath.Join("/workspace", rel))
		seen[path] = struct{}{}
		mounts = append(mounts, fmt.Sprintf("%s:%s:ro", path, containerPath))
	}
	return mounts
}

// containerWorkdir maps the host workdir into the container mount.
// Uses filepath.Rel + ".." prefix check to avoid the HasPrefix confinement
// bypass (SEC-3). Both backends share this single confinement routine.
func (r *SandboxCommandRunner) containerWorkdir(workdir string) (string, error) {
	if r == nil {
		return "", errors.New("sandbox command runner missing")
	}
	if workdir == "" {
		return "/workspace", nil
	}
	abs := workdir
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(r.workspace, workdir)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(r.workspace, abs)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("workdir %s outside workspace %s", abs, r.workspace)
	}
	containerPath := "/workspace"
	if rel != "." {
		containerPath = filepath.ToSlash(filepath.Join(containerPath, rel))
	}
	return containerPath, nil
}
