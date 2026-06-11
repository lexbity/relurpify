package dockersandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// spillWriter replaces the old capped-buffer pattern. It records all bytes
// written and signals when the stream exceeds the configured ceiling so the
// runner can tear down the container.
type spillWriter struct {
	preview  bytes.Buffer
	total    int64
	ceiling  int64
	exceeded atomic.Bool
}

func newSpillWriter(ceiling int64) *spillWriter {
	if ceiling <= 0 {
		ceiling = 32 * 1024 * 1024
	}
	return &spillWriter{ceiling: ceiling}
}

func (w *spillWriter) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	n := len(p)
	newTotal := w.total + int64(n)
	if newTotal > w.ceiling {
		w.exceeded.Store(true)
		w.total = newTotal
		return n, nil
	}
	w.total = newTotal
	_, _ = w.preview.Write(p)
	return n, nil
}

func (w *spillWriter) exceededCeiling() bool {
	return w != nil && w.exceeded.Load()
}

func (w *spillWriter) String() string {
	if w == nil {
		return ""
	}
	return w.preview.String()
}

func (w *spillWriter) Len() int {
	if w == nil {
		return 0
	}
	return int(w.total)
}

var _ io.Writer = (*spillWriter)(nil)

// randSuffix returns a short hex string for container naming.
func randSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Runner executes commands through Docker using the backend's active policy.
type Runner struct {
	backend *Backend
}

// NewRunner constructs a Docker command runner.
func NewRunner(backend *Backend) (*Runner, error) {
	if backend == nil {
		return nil, errors.New("docker backend required")
	}
	if strings.TrimSpace(backend.config.Workspace) == "" {
		return nil, errors.New("docker backend workspace required")
	}
	return &Runner{backend: backend}, nil
}

// Run executes the command via `docker run`.
func (r *Runner) Run(ctx context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	if r == nil || r.backend == nil {
		return nil, errors.New("docker runner missing backend")
	}
	if len(req.Args) == 0 {
		return nil, errors.New("command arguments required")
	}
	containerWorkdir, err := r.containerWorkdir(req.Workdir)
	if err != nil {
		return nil, err
	}

	// Container identity for lifecycle management.
	containerName := "relurpify-docker-" + randSuffix()
	dockerPath := r.backend.config.DockerPath
	if dockerPath == "" {
		dockerPath = "docker"
	}

	resolvedDockerPath, err := exec.LookPath(dockerPath)
	if err != nil {
		return nil, fmt.Errorf("docker not found: %w", err)
	}

	policy := r.backend.Policy()
	args := []string{"run", "--rm", "--name", containerName, "-v", fmt.Sprintf("%s:/workspace", r.backend.config.Workspace), "-w", containerWorkdir}
	if policy.ReadOnlyRoot {
		args = append(args, "--read-only")
	}
	if policy.NoNewPrivileges {
		args = append(args, "--security-opt", "no-new-privileges")
	}
	if strings.TrimSpace(policy.SeccompProfile) != "" {
		args = append(args, "--security-opt", "seccomp="+policy.SeccompProfile)
	}
	// Always isolate: the docker backend has no packet-level egress filtering and
	// rejects granular NetworkRules at policy validation, so isolation must never
	// be dropped on the presence of rules (SF-3).
	args = append(args, "--network", "none")
	for _, mount := range r.protectedMounts(policy.ProtectedPaths) {
		args = append(args, "-v", mount)
	}
	// Resource limits from CommandRequest (defaults applied from contracts).
	args = append(args, "--memory", strconv.FormatInt(sandbox.MemoryBytesOrDefault(req.MemoryBytes), 10))
	args = append(args, "--pids-limit", strconv.FormatInt(sandbox.PidsLimitOrDefault(req.PidsLimit), 10))
	args = append(args, "--cpus", strconv.FormatFloat(sandbox.CPUsOrDefault(req.CPUs), 'f', -1, 64))
	for _, env := range req.Env {
		if env == "" {
			continue
		}
		args = append(args, "-e", env)
	}
	image := strings.TrimSpace(r.backend.config.Image)
	if image == "" {
		image = "ghcr.io/lexcodex/relurpify/runtime:0.4.1"
	}
	// When a digest is configured, pin the image reference to that digest
	// to prevent mutable tag attacks. If the image already includes a
	// digest suffix we respect it; otherwise append @digest.
	if digest := strings.TrimSpace(r.backend.config.ImageDigest); digest != "" {
		if strings.Contains(image, "@") {
			image = strings.SplitN(image, "@", 2)[0] + "@" + digest
		} else {
			image = image + "@" + digest
		}
	}
	args = append(args, image)
	args = append(args, req.Args...)
	execCtx, cancel := context.WithCancel(ctx)
	if req.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()
	start := time.Now()
	cmd := &exec.Cmd{
		Path: resolvedDockerPath,
		Args: append([]string{resolvedDockerPath}, args...),
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	ceiling := sandbox.OutputCeilingOrDefault(req.OutputCeiling)
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

	// Teardown goroutine: on context cancellation (timeout / cancel), stop and
	// remove the container by name instead of killing the client's process group.
	var tornDown atomic.Bool
	var oomKilled atomic.Bool
	grace := sandbox.GracePeriodOrDefault(req.GracePeriod)
	teardownCtx, teardownCancel := context.WithTimeout(context.Background(), grace+10*time.Second)
	defer teardownCancel()
	go func() {
		<-execCtx.Done()
		tornDown.Store(true)
		// docker stop -t <grace> sends SIGTERM, waits up to grace, then SIGKILLs.
		_, stopCancel := context.WithTimeout(teardownCtx, grace+5*time.Second)
		defer stopCancel()
		_ = (&exec.Cmd{
			Path: resolvedDockerPath,
			Args: []string{resolvedDockerPath, "stop", "-t", fmt.Sprintf("%.0f", grace.Seconds()), containerName},
		}).Run()
		// Force-remove in case --rm didn't clean up.
		_, rmCancel := context.WithTimeout(teardownCtx, 5*time.Second)
		defer rmCancel()
		_ = (&exec.Cmd{
			Path: resolvedDockerPath,
			Args: []string{resolvedDockerPath, "rm", "-f", containerName},
		}).Run()
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
					// docker stop -t 0 force-kills immediately, then rm -f
					_, stopCancel2 := context.WithTimeout(teardownCtx, 5*time.Second)
					defer stopCancel2()
					_ = (&exec.Cmd{
						Path: resolvedDockerPath,
						Args: []string{resolvedDockerPath, "stop", "-t", "0", containerName},
					}).Run()
					_, rmCancel2 := context.WithTimeout(teardownCtx, 5*time.Second)
					defer rmCancel2()
					_ = (&exec.Cmd{
						Path: resolvedDockerPath,
						Args: []string{resolvedDockerPath, "rm", "-f", containerName},
					}).Run()
					return
				}
			}
		}
	}()

	err = cmd.Wait()
	close(ceilingDone)
	res := sandbox.NewCommandResult(stdoutBuf.String(), stderrBuf.String(), err, time.Since(start), tornDown.Load())
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

func (r *Runner) protectedMounts(paths []string) []string {
	if r == nil || r.backend == nil || len(paths) == 0 {
		return nil
	}
	workspace := r.backend.config.Workspace
	seen := make(map[string]struct{}, len(paths))
	mounts := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		containerPath := filepath.ToSlash(filepath.Join("/workspace", rel))
		mounts = append(mounts, fmt.Sprintf("%s:%s:ro", path, containerPath))
		seen[path] = struct{}{}
	}
	return mounts
}

func (r *Runner) containerWorkdir(workdir string) (string, error) {
	workspace := r.backend.config.Workspace
	if strings.TrimSpace(workdir) == "" {
		return "/workspace", nil
	}
	abs := workdir
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(workspace, workdir)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(workspace, abs)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("workdir %s outside workspace %s", abs, workspace)
	}
	if rel == "." {
		return "/workspace", nil
	}
	return filepath.ToSlash(filepath.Join("/workspace", rel)), nil
}
