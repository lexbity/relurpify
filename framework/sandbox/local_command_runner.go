package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/runtimeenv"
)

// LocalCommandRunner executes commands directly on the host machine while still
// honoring the workspace boundary enforced by permissions/tooling.
type LocalCommandRunner struct {
	workspace  string
	hostEnv    []string
	allowedEnv []string
	extraEnv   []string
}

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
	execCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(execCtx, req.Args[0], req.Args[1:]...)
	cmd.Dir = dir
	extraEnv := append(append([]string(nil), r.extraEnv...), req.Env...)
	cmd.Env = assembleSubprocessEnv(r.hostEnv, r.allowedEnv, extraEnv)
	if req.Input != "" {
		cmd.Stdin = strings.NewReader(req.Input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
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

// Ensure LocalCommandRunner satisfies the interface.
var _ CommandRunner = (*LocalCommandRunner)(nil)

// DefaultTimeout is a small helper for callers that want an easy constant.
const DefaultTimeout = 30 * time.Second
