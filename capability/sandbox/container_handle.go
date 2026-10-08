package sandbox

import (
	"context"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ContainerOwner labels are applied to every managed sandbox container so its
// lifecycle can be owned deterministically: reaped at boot when the owner is
// dead, or force-removed by the supervising process on teardown (SBH-1 D-11).
const (
	LabelManaged   = "relurpify.managed"
	LabelWorkspace = "relurpify.workspace"
	LabelPID       = "relurpify.pid"
	LabelCreated   = "relurpify.created"
)

// ContainerHandle holds the identity and owner labels of one managed sandbox
// container and provides the deterministic teardown path. Created before every
// Run; owned by the watchdog goroutine (timeout/context-cancel) and the
// deferred cleanup so no container outlives its supervisor by design.
type ContainerHandle struct {
	Name   string            // deterministic name, e.g. "relurpify-<wsHash8>-<rand6>"
	Labels map[string]string // owner labels (relurpify.*)
	binary string            // docker CLI path

	once sync.Once // idempotent teardown: the first caller wins
}

// NewContainerHandle creates a handle for the given name, owner labels, and
// docker CLI binary. An empty binary defaults to "docker".
func NewContainerHandle(name string, labels map[string]string, binary string) *ContainerHandle {
	if strings.TrimSpace(binary) == "" {
		binary = "docker"
	}
	return &ContainerHandle{
		Name:   name,
		Labels: labels,
		binary: binary,
	}
}

// Teardown force-stops and removes the container. Idempotent: only the first
// caller actually tears down; later calls are no-ops.
//
// Semantics (docker CLI): `stop -t <grace>` (bounded by grace+5s) then
// `rm -f` (bounded by 5s). `rm -f` runs even when stop fails — the removal
// half is the guaranteed bound on container lifetime. Both errors are logged,
// never panicked (a nonexistent container fails harmlessly).
func (h *ContainerHandle) Teardown(ctx context.Context, grace time.Duration) {
	if h == nil || strings.TrimSpace(h.Name) == "" {
		return
	}
	h.once.Do(func() {
		h.teardown(ctx, grace)
	})
}

func (h *ContainerHandle) teardown(ctx context.Context, grace time.Duration) {
	binaryPath, err := exec.LookPath(h.binary)
	if err != nil {
		binaryPath = h.binary // let the OS resolve or fail with a clear error
	}
	if grace < 0 {
		grace = 0
	}

	// docker stop -t <grace> sends SIGTERM, waits up to grace, then SIGKILLs.
	stopCtx, cancel := context.WithTimeout(ctx, grace+5*time.Second)
	defer cancel()
	stopSeconds := strconv.FormatInt(int64(grace/time.Second), 10)
	if err := exec.CommandContext(stopCtx, binaryPath, "stop", "-t", stopSeconds, h.Name).Run(); err != nil {
		log.Printf("sandbox: docker stop %s failed: %v", h.Name, err)
	}

	// Force-remove is the guaranteed cleanup; a stopped/exited container is
	// removed without error.
	rmCtx, cancel2 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel2()
	if err := exec.CommandContext(rmCtx, binaryPath, "rm", "-f", h.Name).Run(); err != nil {
		log.Printf("sandbox: docker rm -f %s failed: %v", h.Name, err)
	}
}
