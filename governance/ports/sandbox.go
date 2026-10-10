// Package ports defines consumer-owned interfaces for cross-domain
// communication. SandboxRuntime is the governance-owned port for
// sandbox backends; platform/sandbox implements it. The policy and
// config vocabulary it speaks is owned by governance/sandbox.
package ports

import (
	"context"

	"codeburg.org/lexbit/relurpify/governance/sandbox"
)

// SandboxRuntime describes a sandbox runtime with policy methods.
type SandboxRuntime interface {
	Verify(ctx context.Context) error
	ValidatePolicy(policy sandbox.SandboxPolicy) error
	ApplyPolicy(ctx context.Context, policy sandbox.SandboxPolicy) error
	Policy() sandbox.SandboxPolicy
	RunConfig() sandbox.SandboxConfig
	Name() string
}

// SandboxSelector chooses a sandbox backend implementation.
type SandboxSelector interface {
	SelectBackend(ctx context.Context, backend string, cfg sandbox.SandboxConfig, image, workspace string) (SandboxRuntime, error)
	SupportedBackends() []string
}
