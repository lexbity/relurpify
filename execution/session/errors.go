package session

import "errors"

var (
	// ErrSecurityUnavailable is returned when the security controller
	// cannot be used because no security/permission provider is configured.
	ErrSecurityUnavailable = errors.New("security: not available in this workspace session")

	// ErrKnowledgeUnavailable is returned when the knowledge controller
	// cannot be used because no knowledge/index provider is configured.
	ErrKnowledgeUnavailable = errors.New("knowledge: not available in this workspace session")

	// ErrCapabilityUnavailable is returned when the capability controller
	// cannot be used because no capability registry is configured.
	ErrCapabilityUnavailable = errors.New("capability: not available in this workspace session")

	// ErrNamedAgentUnavailable is returned when the named agent controller
	// cannot be used because no named agent provider is configured.
	ErrNamedAgentUnavailable = errors.New("named agent: not available in this workspace session")
)
