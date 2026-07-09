package sandbox

import (
	"fmt"
	"strings"
)

// NewSandboxRuntimeForBackend resolves a backend name to a SandboxRuntime.
func NewSandboxRuntimeForBackend(backend string, cfg SandboxConfig, image, workspace string) (SandboxRuntime, error) {
	b := strings.ToLower(strings.TrimSpace(backend))
	if b == "" {
		b = "gvisor"
	}
	if !IsSupportedSandboxBackend(b) {
		supported := strings.Join(SupportedSandboxBackends(), ", ")
		return nil, fmt.Errorf("unsupported sandbox backend %q (supported: %s)", backend, supported)
	}
	switch b {
	case "gvisor":
		return NewSandboxRuntime(cfg), nil
	default:
		return nil, fmt.Errorf("unreachable: unsupported sandbox backend %q", b)
	}
}
