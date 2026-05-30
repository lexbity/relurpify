package dockersandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// stdoutLimit and stderrLimit are the default caps applied to docker command
// output when CommandRequest.MaxOutputBytes is 0. These match the defaults in
// the local command runner.
const dockerDefaultOutputLimit int64 = 256 * 1024

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
func (r *Runner) Run(ctx context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
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
	policy := r.backend.Policy()
	args := []string{"run", "--rm", "-v", fmt.Sprintf("%s:/workspace", r.backend.config.Workspace), "-w", containerWorkdir}
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
	cmd := exec.Command(r.backend.config.DockerPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	limit := req.MaxOutputBytes
	if limit <= 0 {
		limit = dockerDefaultOutputLimit
	}
	stdoutBuf := &cappedBuffer{limit: limit}
	stderrBuf := &cappedBuffer{limit: limit}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	if req.Input != "" {
		cmd.Stdin = strings.NewReader(req.Input)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	pid := cmd.Process.Pid
	go func() {
		<-execCtx.Done()
		if pgid, err := syscall.Getpgid(pid); err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}()
	err = cmd.Wait()
	return contracts.NewCommandResult(stdoutBuf.String(), stderrBuf.String(), err, time.Since(start)), nil
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

// cappedBuffer wraps bytes.Buffer with a write limit. Writes and reads beyond
// the limit are silently discarded. Both Write and ReadFrom are overridden
// to enforce the cap.
type cappedBuffer struct {
	bytes.Buffer
	limit int64
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.limit <= 0 {
		return c.Buffer.Write(p)
	}
	remaining := c.limit - int64(c.Buffer.Len())
	if remaining <= 0 {
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	return c.Buffer.Write(p)
}

func (c *cappedBuffer) ReadFrom(r io.Reader) (int64, error) {
	if c.limit <= 0 {
		return c.Buffer.ReadFrom(r)
	}
	remaining := c.limit - int64(c.Buffer.Len())
	if remaining <= 0 {
		_, err := io.Copy(io.Discard, r)
		return 0, err
	}
	return c.Buffer.ReadFrom(io.LimitReader(r, remaining))
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
