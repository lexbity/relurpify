package dockersandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/sandbox"
)

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
func (r *Runner) Run(ctx context.Context, req sandbox.CommandRequest) (string, string, error) {
	if r == nil || r.backend == nil {
		return "", "", errors.New("docker runner missing backend")
	}
	if len(req.Args) == 0 {
		return "", "", errors.New("command arguments required")
	}
	containerWorkdir, err := r.containerWorkdir(req.Workdir)
	if err != nil {
		return "", "", err
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
	if len(policy.NetworkRules) == 0 {
		args = append(args, "--network", "none")
	}
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
		image = "ghcr.io/relurpify/runtime:latest"
	}
	args = append(args, image)
	args = append(args, req.Args...)
	execCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(execCtx, r.backend.config.DockerPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if req.Input != "" {
		cmd.Stdin = strings.NewReader(req.Input)
	}
	if err := cmd.Run(); err != nil {
		return stdout.String(), stderr.String(), err
	}
	return stdout.String(), stderr.String(), nil
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
