package envcomposition

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	govsandbox "codeburg.org/lexbit/relurpify/governance/sandbox"
)

// NewSandboxBackendFactory returns a SandboxBackendFactory that creates
// sandbox runtimes from the capability/sandbox implementations. With one
// policy vocabulary (governance/sandbox) the capability runtimes satisfy
// the governance port directly — there is nothing left to adapt.
func NewSandboxBackendFactory() fauthorization.SandboxBackendFactory {
	return func(ctx context.Context, backend string, cfg govsandbox.SandboxConfig, image, workspace string) (governanceports.SandboxRuntime, error) {
		b := strings.ToLower(strings.TrimSpace(backend))
		if b == "" {
			b = "gvisor"
		}
		if !sandbox.IsSupportedSandboxBackend(b) {
			supported := strings.Join(sandbox.SupportedSandboxBackends(), ", ")
			return nil, fmt.Errorf("unsupported sandbox backend %q (supported: %s)", backend, supported)
		}
		sboxCfg := cfg
		switch b {
		case "gvisor":
			return sandbox.NewSandboxRuntime(sboxCfg), nil
		case "docker":
			return sandbox.NewDockerSandboxBackend(sboxCfg), nil
		default:
			return nil, fmt.Errorf("unreachable: unsupported sandbox backend %q", b)
		}
	}
}
