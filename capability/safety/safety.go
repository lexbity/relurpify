// Package safety holds the runtime safety budget vocabulary for capability
// execution. It is a dependency-free leaf so that both the capability root
// package and capability/agentspec can depend on a single definition without
// forming an import cycle between them.
//
// This package is a stepping-stone: once the agent-manifest (agentspec
// AgentRuntimeSpec) is dropped, agentspec no longer needs RuntimeSafetySpec and
// this type folds back into the capability root package.
package safety

import "fmt"

// RuntimeSafetySpec defines runtime budget limits for capability execution.
// Tags carry both yaml (manifest/config load) and json (runtime snapshots).
type RuntimeSafetySpec struct {
	MaxCallsPerCapability     int   `yaml:"max_calls_per_capability,omitempty" json:"max_calls_per_capability,omitempty"`
	MaxCallsPerProvider       int   `yaml:"max_calls_per_provider,omitempty" json:"max_calls_per_provider,omitempty"`
	MaxBytesPerSession        int   `yaml:"max_bytes_per_session,omitempty" json:"max_bytes_per_session,omitempty"`
	MaxOutputTokensSession    int   `yaml:"max_output_tokens_per_session,omitempty" json:"max_output_tokens_per_session,omitempty"`
	MaxSubprocessesPerSession int   `yaml:"max_subprocesses_per_session,omitempty" json:"max_subprocesses_per_session,omitempty"`
	MaxNetworkRequestsSession int   `yaml:"max_network_requests_per_session,omitempty" json:"max_network_requests_per_session,omitempty"`
	RedactSensitiveMetadata   *bool `yaml:"redact_sensitive_metadata,omitempty" json:"redact_sensitive_metadata,omitempty"`
}

func (s RuntimeSafetySpec) Validate() error {
	for name, value := range map[string]int{
		"max_calls_per_capability":         s.MaxCallsPerCapability,
		"max_calls_per_provider":           s.MaxCallsPerProvider,
		"max_bytes_per_session":            s.MaxBytesPerSession,
		"max_output_tokens_session":        s.MaxOutputTokensSession,
		"max_subprocesses_per_session":     s.MaxSubprocessesPerSession,
		"max_network_requests_per_session": s.MaxNetworkRequestsSession,
	} {
		if value < 0 {
			return fmt.Errorf("%s must be >= 0", name)
		}
	}
	return nil
}

func (s RuntimeSafetySpec) RedactionEnabled() bool {
	if s.RedactSensitiveMetadata == nil {
		return true
	}
	return *s.RedactSensitiveMetadata
}
