package sandbox

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

	"codeburg.org/lexbit/relurpify/framework/runtimeenv"
)

// defaultMaxOutputBytes is the default soft limit applied to subprocess
// stdout/stderr when CommandRequest.MaxOutputBytes is 0. Beyond this threshold
// the content is truncated and the caller is informed via the error string
// that truncation occurred. Memory safety is provided by io.LimitReader.
const defaultMaxOutputBytes = 256 * 1024

// LocalCommandRunner executes commands directly on the host machine while still
// honoring the workspace boundary enforced by permissions/tooling.
type LocalCommandRunner struct {
	workspace  string
	hostEnv    []string
	allowedEnv []string
	extraEnv   []string
}

// defaultSubprocessEnvAllowlist controls which host environment variables are
// passed through to subprocesses. Dangerously subvertable variables such as
// LD_PRELOAD, LD_LIBRARY_PATH, BASH_ENV, IFS, and PROMPT_COMMAND are excluded
// to prevent subprocess injection and privilege escalation.
var defaultSubprocessEnvAllowlist = []string{"HOME", "USER", "PATH", "GOPATH", "GOROOT", "GOMODCACHE"}

func NewLocalCommandRunner(workspace string, allowedEnv, extraEnv []string) *LocalCommandRunner {
	abs := workspace
	if abs == "" {
		abs = "."
	}
	if resolved, err := filepath.Abs(abs); err == nil {
		abs = resolved
	}
	if len(allowedEnv) == 0 {
		allowedEnv = defaultSubprocessEnvAllowlist
	}
	return &LocalCommandRunner{
		workspace:  filepath.Clean(abs),
		hostEnv:    runtimeenv.Capture(),
		allowedEnv: append([]string(nil), allowedEnv...),
		extraEnv:   append([]string(nil), extraEnv...),
	}
}

func (r *LocalCommandRunner) Run(ctx context.Context, req CommandRequest) (string, string, error) {
	if r == nil {
		return "", "", errors.New("local command runner missing")
	}
	if len(req.Args) == 0 {
		return "", "", errors.New("command arguments required")
	}
	dir, err := r.resolveWorkdir(req.Workdir)
	if err != nil {
		return "", "", err
	}
	execCtx, cancel := context.WithCancel(ctx)
	if req.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	cmd := exec.Command(req.Args[0], req.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = dir
	extraEnv := append(append([]string(nil), r.extraEnv...), req.Env...)
	cmd.Env = assembleSubprocessEnv(r.hostEnv, r.allowedEnv, extraEnv)
	if req.Input != "" {
		cmd.Stdin = strings.NewReader(req.Input)
	}
	limit := req.MaxOutputBytes
	if limit <= 0 {
		limit = defaultMaxOutputBytes
	}
	stdoutBuf := &cappedBuffer{limit: limit}
	stderrBuf := &cappedBuffer{limit: limit}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("start: %w", err)
	}
	pid := cmd.Process.Pid
	go func() {
		<-execCtx.Done()
		// Kill the entire process group, terminating any child processes
		// that the command may have spawned (build servers, background
		// tasks, test runners, etc.).
		if pgid, err := syscall.Getpgid(pid); err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}()
	err = cmd.Wait()
	return stdoutBuf.String(), stderrBuf.String(), err
}

func (r *LocalCommandRunner) resolveWorkdir(workdir string) (string, error) {
	if workdir == "" {
		return r.workspace, nil
	}
	abs := workdir
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(r.workspace, workdir)
	}
	abs = filepath.Clean(abs)
	workspaceSlash := filepath.ToSlash(r.workspace)
	absSlash := filepath.ToSlash(abs)
	if !strings.HasPrefix(absSlash, workspaceSlash) {
		return "", fmt.Errorf("workdir %s outside workspace %s", abs, r.workspace)
	}
	return abs, nil
}

// cappedBuffer wraps bytes.Buffer with a write limit. Writes and reads beyond
// the limit are silently discarded, providing memory safety against runaway
// processes. Both Write and ReadFrom are overridden to enforce the cap;
// the embedded bytes.Buffer.ReadFrom would otherwise read unlimited data.
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

// ReadFrom overrides bytes.Buffer.ReadFrom to enforce the output cap.
// The embedded ReadFrom reads the entire source into the buffer, which
// defeats the capped write limit. This implementation copies at most
// (limit - current length) bytes, then discards the remainder.
func (c *cappedBuffer) ReadFrom(r io.Reader) (int64, error) {
	if c.limit <= 0 {
		return c.Buffer.ReadFrom(r)
	}
	remaining := c.limit - int64(c.Buffer.Len())
	if remaining <= 0 {
		// Buffer is already full. Drain the reader to unblock the
		// producer but discard everything.
		_, err := io.Copy(io.Discard, r)
		return 0, err
	}
	return c.Buffer.ReadFrom(io.LimitReader(r, remaining))
}

// Ensure LocalCommandRunner satisfies the interface.
var _ CommandRunner = (*LocalCommandRunner)(nil)

// DefaultTimeout is a small helper for callers that want an easy constant.
const DefaultTimeout = 30 * time.Second
