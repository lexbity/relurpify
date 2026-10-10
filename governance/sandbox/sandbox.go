// Package sandbox owns the backend-neutral sandbox policy vocabulary:
// the security intent (SandboxPolicy), the runtime knobs (SandboxConfig),
// and the network rule (NetworkRule) they are built from, plus their
// validation. Governance is the owner because these structs are the
// contract between authored policy and enforcement; backends (capability,
// platform) implement the enforcement ports against these types.
package sandbox

import (
	"errors"
	"fmt"
	"strings"
)

// NetworkRule defines network access rules for sandbox policies.
type NetworkRule struct {
	Direction   string // "ingress" or "egress"
	Protocol    string // "tcp", "udp", etc.
	Host        string
	Port        int
	Description string
}

// SandboxPolicy captures the backend-neutral security intent to apply to a sandbox runtime.
// Fields are universal unless a backend explicitly rejects them via
// ValidatePolicy.
type SandboxPolicy struct {
	NetworkRules    []NetworkRule
	ReadOnlyRoot    bool
	ProtectedPaths  []string
	NoNewPrivileges bool
	SeccompProfile  string
	AllowedEnvKeys  []string
	DeniedEnvKeys   []string
}

// SandboxConfig exposes runtime knobs for a sandbox backend.
type SandboxConfig struct {
	RunscPath        string
	ContainerRuntime string // docker or containerd
	Platform         string // ptrace or kvm
	NetworkIsolation bool
	ReadOnlyRoot     bool
	SeccompProfile   string
}

// Validate ensures universal policy invariants hold before backend-specific
// capability checks run.
func (p SandboxPolicy) Validate() error {
	allowed := make(map[string]struct{}, len(p.AllowedEnvKeys))
	for _, key := range p.AllowedEnvKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			return errors.New("allowed env key required")
		}
		if _, ok := allowed[key]; ok {
			return fmt.Errorf("duplicate allowed env key %q", key)
		}
		allowed[key] = struct{}{}
	}
	for _, key := range p.DeniedEnvKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			return errors.New("denied env key required")
		}
		if _, ok := allowed[key]; ok {
			return fmt.Errorf("env key %q cannot be both allowed and denied", key)
		}
	}
	for i, rule := range p.NetworkRules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("network rule %d: %w", i, err)
		}
	}
	for i, path := range p.ProtectedPaths {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("protected path %d required", i)
		}
	}
	return nil
}

// Validate checks that a network rule is structurally sound.
func (r NetworkRule) Validate() error {
	if strings.TrimSpace(r.Direction) == "" {
		return errors.New("direction required")
	}
	switch strings.ToLower(strings.TrimSpace(r.Direction)) {
	case "egress", "ingress":
	default:
		return fmt.Errorf("unsupported direction %q", r.Direction)
	}
	if strings.TrimSpace(r.Protocol) == "" {
		return errors.New("protocol required")
	}
	if r.Port < 0 {
		return fmt.Errorf("invalid port %d", r.Port)
	}
	return nil
}
