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

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// randSuffix returns a short hex string for container naming.
func randSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CommandRequest and CommandRunner are defined canonically in platform/contracts
// so that platform-level backends can satisfy them without importing framework.
type (
	CommandRequest = contracts.CommandRequest
	CommandRunner  = contracts.CommandRunner
)

// NewCommandRunner returns a backend-specific runner when the runtime supports
// one, otherwise it falls back to the standard sandbox command runner.
func NewCommandRunner(config *contracts.CommandRunnerConfig, runtime SandboxRuntime) (CommandRunner, error) {
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
	workspaceSlash  string
	user            int
	readOnlyRoot    bool
	noNewPrivileges bool
}

// NewSandboxCommandRunner wires the config/runtime metadata into a runner.
func NewSandboxCommandRunner(config *contracts.CommandRunnerConfig, runtime SandboxRuntime) (*SandboxCommandRunner, error) {
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
		workspaceSlash:  filepath.ToSlash(absWorkspace),
		user:            config.RunAsUser,
		readOnlyRoot:    config.ReadOnlyRoot,
		noNewPrivileges: config.NoNewPrivileges,
	}, nil
}

// Run executes the requested command inside the sandboxed container runtime.
func (r *SandboxCommandRunner) Run(ctx context.Context, req CommandRequest) (*contracts.CommandResult, error) {
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
	runtimeName := filepath.Base(r.config.RunscPath)
	if runtimeName == "" {
		runtimeName = "runsc"
	}
	containerWorkdir, err := r.containerWorkdir(req.Workdir)
	if err != nil {
		return nil, err
	}

	// Container identity for lifecycle management.
	containerName := "relurpify-sandbox-" + randSuffix()
	handle := NewContainerHandle(containerName, runtimeName, runtimeBinary)

	args := []string{"run", "--rm", "--name", containerName, "--runtime", runtimeName, "-v", fmt.Sprintf("%s:/workspace", r.workspace), "-w", containerWorkdir}
	for _, mount := range r.protectedMounts() {
		args = append(args, "-v", mount)
	}
	if r.user > 0 {
		args = append(args, "-u", strconv.Itoa(r.user))
	}
	if r.readOnlyRoot {
		args = append(args, "--read-only")
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
	args = append(args, "--memory", strconv.FormatInt(contracts.MemoryBytesOrDefault(req.MemoryBytes), 10))
	args = append(args, "--pids-limit", strconv.FormatInt(contracts.PidsLimitOrDefault(req.PidsLimit), 10))
	args = append(args, "--cpus", strconv.FormatFloat(contracts.CPUsOrDefault(req.CPUs), 'f', -1, 64))
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
	execCtx, cancel := context.WithCancel(ctx)
	if req.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()
	start := time.Now()
	cmd := exec.Command(runtimeBinary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	ceiling := contracts.OutputCeilingOrDefault(req.OutputCeiling)
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

	// Teardown goroutine: on context cancellation (timeout / cancel), tear down
	// the container by name instead of killing the client's process group.
	var tornDown atomic.Bool
	var oomKilled atomic.Bool
	grace := contracts.GracePeriodOrDefault(req.GracePeriod)
	// Detached context for teardown so it runs even when ctx is already done.
	teardownCtx, teardownCancel := context.WithTimeout(context.Background(), grace+10*time.Second)
	defer teardownCancel()
	go func() {
		<-execCtx.Done()
		tornDown.Store(true)
		handle.Teardown(teardownCtx, grace)
	}()

	// Monitor output ceiling concurrently.
	ceilingDone := make(chan struct{}, 1)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-execCtx.Done():
				return
			case <-ceilingDone:
				return
			case <-ticker.C:
				if stdoutBuf.exceededCeiling() || stderrBuf.exceededCeiling() {
					oomKilled.Store(true)
					handle.Teardown(teardownCtx, grace)
					return
				}
			}
		}
	}()

	err = cmd.Wait()
	close(ceilingDone)
	res := contracts.NewCommandResult(stdoutBuf.String(), stderrBuf.String(), err, time.Since(start), tornDown.Load())
	if oomKilled.Load() {
		res.OOMKilled = true
		res.TornDown = true
		res.TimedOut = false
		res.ExitCode = -1
	}
	res.StdoutBytes = int64(stdoutBuf.Len())
	res.StderrBytes = int64(stderrBuf.Len())
	return res, nil
}

// Note: newCommandResult was promoted to contracts.NewCommandResult in Phase 1.

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
	absSlash := filepath.ToSlash(abs)
	if !strings.HasPrefix(absSlash, r.workspaceSlash) {
		return "", fmt.Errorf("workdir %s outside workspace %s", abs, r.workspace)
	}
	rel, err := filepath.Rel(r.workspace, abs)
	if err != nil {
		return "", err
	}
	containerPath := "/workspace"
	if rel != "." {
		containerPath = filepath.ToSlash(filepath.Join(containerPath, rel))
	}
	return containerPath, nil
}
