// Package ports defines consumer-owned interfaces for cross-domain
// communication. These are defined by the consumer (governance) so that
// capability never needs to import governance for type information.
package ports

import (
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// DescriptorView is the governance-owned view of a capability descriptor.
// governance/policy and governance/authorization evaluate policy against
// this interface instead of importing capability directly.
type DescriptorView interface {
	CapabilityID() string
	CapabilityName() string
	CapabilityKind() string // e.g. "tool", "prompt", "resource"
	RuntimeFamily() string  // e.g. "local-tool", "provider", "relurpic"
	Description() string
	Version() string
	Category() string
	Tags() []string
	TrustClass() string // e.g. "builtin-trusted", "workspace-trusted"
	RiskClasses() []risk.RiskClass
	EffectClasses() []classification.EffectClass
	SourceProviderID() string
	SourceScope() string // e.g. "builtin", "workspace", "provider", "remote"
	SourceSessionID() string

	// Coordination metadata
	CoordinationRole() string // e.g. "planner", "executor"
	CoordinationTarget() bool
	CoordinationTaskTypes() []string
	CoordinationExecutionModes() []string // e.g. "sync", "session-backed"
	CoordinationLongRunning() int32       // 0=unset, 1=enabled, 2=disabled
	CoordinationDirectInsertionAllowed() int32
	CoordinationMaxDepth() int
	CoordinationMaxRuntimeSeconds() int
}

// SpecView is the governance-owned view of an AgentRuntimeSpec for policy
// compilation. capability/agentspec satisfies this interface.
type SpecView interface {
	GetAllowedCapabilities() []CapabilitySelectorView
	GetToolExecutionPolicy() map[string]ToolPolicyView
	GetCapabilityPolicies() []CapabilityPolicyView
	GetSessionPolicies() []SessionPolicyView
	GetProviderPolicies() map[string]ProviderPolicyView
	GetGlobalPolicies() map[string]string
	GetBrowser() BrowserSpecView
	GetOrchestration() OrchestrationConfigView
}

type CapabilitySelectorView struct {
	ID                          string
	Name                        string
	Kind                        string
	RuntimeFamilies             []string
	Tags                        []string
	ExcludeTags                 []string
	SourceScopes                []string
	TrustClasses                []string
	RiskClasses                 []string
	EffectClasses               []string
	CoordinationTaskTypes       []string
	CoordinationRoles           []string
	CoordinationExecModes       []string
	CoordinationLongRunning     int32
	CoordinationDirectInsertion int32
}

type ToolPolicyView struct {
	Execute string // "allow", "deny", "ask", ""
}

type CapabilityPolicyView struct {
	Selector CapabilitySelectorView
	Execute  string
}

type SessionPolicyView struct {
	ID          string
	Name        string
	Priority    int
	Enabled     bool
	Selector    SessionSelectorView
	Effect      string
	Approvers   []string
	ApprovalTTL string
	Reason      string
}

type SessionSelectorView struct {
	Partitions                []string
	ChannelIDs                []string
	Scopes                    []string
	TrustClasses              []string
	Operations                []string
	ActorKinds                []string
	ActorIDs                  []string
	ExternalProvider          []string
	AuthOnly                  *bool
	RequireOwnership          *bool
	RequireDelegation         *bool
	RequireExternalBinding    *bool
	RequireResolvedExternal   *bool
	RequireRestrictedExternal *bool
}

type ProviderPolicyView struct {
	Activate             string
	DefaultTrust         string
	AllowCredentialShare bool
}

type BrowserSpecView struct {
	Enabled         bool
	DefaultBackend  string
	AllowedBackends []string
	Actions         map[string]string
}

type OrchestrationConfigView struct {
	PhaseCapabilities        map[string][]string
	PhaseCapabilitySelectors map[string][]CapabilitySelectorView
}
