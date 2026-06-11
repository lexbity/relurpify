package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// ContainerHandle holds the identity of a sandbox container and provides a
// deterministic teardown path. Created before every Run and used by the
// context-cancellation goroutine to clean up the workload container.
type ContainerHandle struct {
	Name    string // deterministic container name, e.g. "relurpify-sandbox-<rand>"
	Runtime string // "docker" | "runsc"
	binary  string // path to docker/containerd/runsc CLI
}

// NewContainerHandle creates a handle for the given runtime.
func NewContainerHandle(name, runtime, binary string) *ContainerHandle {
	return &ContainerHandle{
		Name:    name,
		Runtime: runtime,
		binary:  binary,
	}
}

// Teardown force-stops and removes the container.
//
// Semantics:
//  1. SIGTERM (docker: "stop -t <grace>"; runsc: "kill TERM")
//  2. Wait grace
//  3. SIGKILL / force-remove (docker: "rm -f"; runsc: "delete --force")
//
// Idempotent — safe to call multiple times.
func (h *ContainerHandle) Teardown(ctx context.Context, grace time.Duration) {
	if h == nil || h.Name == "" {
		return
	}
	switch h.Runtime {
	case "docker":
		h.teardownDocker(ctx, grace)
	case "runsc":
		h.teardownRunsc(ctx, grace)
	}
}

func (h *ContainerHandle) teardownDocker(ctx context.Context, grace time.Duration) {
	if h.binary == "" {
		h.binary = "docker"
	}
	binaryPath, err := exec.LookPath(h.binary)
	if err != nil {
		binaryPath = h.binary
	}
	// docker stop -t <grace_secs> sends SIGTERM, waits up to grace, then SIGKILLs.
	_, cancel := context.WithTimeout(ctx, grace+5*time.Second)
	defer cancel()
	_ = (&exec.Cmd{
		Path: binaryPath,
		Args: []string{
			binaryPath,
			"stop", "-t", fmt.Sprintf("%.0f", grace.Seconds()), h.Name,
		},
	}).Run()

	// Force-remove in case --rm wasn't set or didn't clean up.
	_, cancel2 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel2()
	_ = (&exec.Cmd{
		Path: binaryPath,
		Args: []string{binaryPath, "rm", "-f", h.Name},
	}).Run()
}

func (h *ContainerHandle) teardownRunsc(ctx context.Context, grace time.Duration) {
	if h.binary == "" {
		h.binary = "runsc"
	}
	binaryPath, err := exec.LookPath(h.binary)
	if err != nil {
		binaryPath = h.binary
	}
	// runsc kill <id> TERM
	_, cancel := context.WithTimeout(ctx, grace+2*time.Second)
	defer cancel()
	_ = (&exec.Cmd{
		Path: binaryPath,
		Args: []string{binaryPath, "kill", h.Name, "TERM"},
	}).Run()

	// Give the process a chance to flush, then force-delete.
	time.Sleep(grace)

	_, cancel3 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel3()
	_ = (&exec.Cmd{
		Path: binaryPath,
		Args: []string{binaryPath, "delete", "--force", h.Name},
	}).Run()
}
