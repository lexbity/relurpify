package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// randSuffix returns a short hex string for container naming.
func randSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

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
func (r *SandboxCommandRunner) Run(ctx context.Context, req CommandRequest) (*ports.CommandResult, error) {
	if r == nil {
		return nil, errors.New("sandbox command runner missing")
	}
	if len(req.Args) == 0 {
		return nil, errors.New("command arguments required")
	}
	runtimeBinary := r.config.ContainerRuntime
	if runtimeBinary == "" {
		runtimeBinary = "docker"
	}
	containerWorkdir, err := r.containerWorkdir(req.Workdir)
	if err != nil {
		return nil, err
	}

	// Container identity for lifecycle management.
	containerName := "relurpify-sandbox-" + randSuffix()

	args := r.runArgs(containerName, containerWorkdir, req)
	runtimeBinaryPath, err := exec.LookPath(runtimeBinary)
	if err != nil {
		return nil, fmt.Errorf("%s not found: %w", runtimeBinary, err)
	}
	start := time.Now()
	cmd := &exec.Cmd{
		Path: runtimeBinaryPath,
		Args: append([]string{runtimeBinaryPath}, args...),
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	ceiling := OutputCeilingOrDefault(req.OutputCeiling)
	stdoutBuf := newSpillWriter(ceiling)
	stderrBuf := newSpillWriter(ceiling)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	if req.Input != "" {
		cmd.Stdin = strings.NewReader(req.Input)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	tornDown := atomic.Bool{}

	grace := GracePeriodOrDefault(req.GracePeriod)

	go func() {
		timer := time.NewTimer(req.Timeout)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		case <-done:
			return
		}
		tornDown.Store(true)

		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-done:
			return
		case <-time.After(grace):
		}

		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}()

	err = <-done
	res := NewCommandResult(stdoutBuf.String(), stderrBuf.String(), err, time.Since(start), tornDown.Load())
	markOOM(res)
	res.StdoutBytes = int64(stdoutBuf.Len())
	res.StderrBytes = int64(stderrBuf.Len())
	return res, nil
}

// markOOM classifies a container exit as OOM-killed when the runner did not
// issue the kill and the process was terminated by SIGKILL (exit 137). Docker
// propagates the container's exit code through `docker run`, so a kernel/cgroup
// OOM kill surfaces as 137 without the runner tearing the process down.
func markOOM(res *ports.CommandResult) {
	if res == nil || res.TornDown {
		return
	}
	if res.ExitCode == 137 {
		res.OOMKilled = true
		res.Signaled = true
	}
}

// runArgs builds the container-engine argument vector for a command. It is a
// pure function of the runner's configuration so the exact isolation flags can
// be asserted without launching a container.
func (r *SandboxCommandRunner) runArgs(containerName, containerWorkdir string, req CommandRequest) []string {
	args := []string{"run", "--rm", "--name", containerName}
	if !r.nativeDocker {
		runtimeName := filepath.Base(r.config.RunscPath)
		if runtimeName == "" {
			runtimeName = "runsc"
		}
		args = append(args, "--runtime", runtimeName)
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
