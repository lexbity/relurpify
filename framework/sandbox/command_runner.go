package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/lexcodex/relurpify/framework/manifest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CommandRequest captures process execution metadata routed through a sandbox.
type CommandRequest struct {
	Workdir string
	Args    []string
	Env     []string
	Input   string
	Timeout time.Duration
}

// CommandRunner describes a primitive capable of executing commands in a sandbox.
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (stdout string, stderr string, err error)
}

// SandboxCommandRunner launches commands via the configured gVisor runtime.
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

// NewSandboxCommandRunner wires the manifest/runtime metadata into a runner.
func NewSandboxCommandRunner(manifest *manifest.AgentManifest, runtime SandboxRuntime, workspace string) (*SandboxCommandRunner, error) {
	if manifest == nil {
		return nil, errors.New("manifest required")
	}
	if runtime == nil {
		return nil, errors.New("sandbox runtime required")
	}
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
		image:           manifest.Spec.Image,
		workspace:       absWorkspace,
		workspaceSlash:  filepath.ToSlash(absWorkspace),
		user:            manifest.Spec.Security.RunAsUser,
		readOnlyRoot:    manifest.Spec.Security.ReadOnlyRoot,
		noNewPrivileges: manifest.Spec.Security.NoNewPrivileges,
	}, nil
}

// Run executes the requested command inside the sandboxed container runtime.
func (r *SandboxCommandRunner) Run(ctx context.Context, req CommandRequest) (string, string, error) {
	if r == nil {
		return "", "", errors.New("sandbox command runner missing")
	}
	if len(req.Args) == 0 {
		return "", "", errors.New("command arguments required")
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
		return "", "", err
	}
	args := []string{"run", "--rm", "--runtime", runtimeName, "-v", fmt.Sprintf("%s:/workspace", r.workspace), "-w", containerWorkdir}
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
	// Network isolation: use --network none when requested and no specific
	// egress rules are declared (rules are enforced by PermissionManager instead).
	policy := r.rt.Policy()
	if r.config.NetworkIsolation && len(policy.NetworkRules) == 0 {
		args = append(args, "--network", "none")
	}
	for _, env := range req.Env {
		if env == "" {
			continue
		}
		args = append(args, "-e", env)
	}
	image := r.image
	if strings.TrimSpace(image) == "" {
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
	cmd := exec.CommandContext(execCtx, runtimeBinary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if req.Input != "" {
		cmd.Stdin = strings.NewReader(req.Input)
	}
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
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
func (r *SandboxCommandRunner) containerWorkdir(workdir string) (string, error) {
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
