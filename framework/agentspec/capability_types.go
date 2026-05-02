package agentspec

import (
	"fmt"
	"strings"
)

type CapabilityKind string

const (
	CapabilityKindTool         CapabilityKind = "tool"
	CapabilityKindPrompt       CapabilityKind = "prompt"
	CapabilityKindResource     CapabilityKind = "resource"
	CapabilityKindSession      CapabilityKind = "session"
	CapabilityKindSubscription CapabilityKind = "subscription"
)

type CapabilityScope string

const (
	CapabilityScopeBuiltin   CapabilityScope = "builtin"
	CapabilityScopeWorkspace CapabilityScope = "workspace"
	CapabilityScopeProvider  CapabilityScope = "provider"
	CapabilityScopeRemote    CapabilityScope = "remote"
)

type CapabilityRuntimeFamily string

const (
	CapabilityRuntimeFamilyLocalTool CapabilityRuntimeFamily = "local-tool"
	CapabilityRuntimeFamilyProvider  CapabilityRuntimeFamily = "provider"
	CapabilityRuntimeFamilyRelurpic  CapabilityRuntimeFamily = "relurpic"
)

type CapabilitySource struct {
	ProviderID string          `json:"provider_id,omitempty" yaml:"provider_id,omitempty"`
	Scope      CapabilityScope `json:"scope,omitempty" yaml:"scope,omitempty"`
	SessionID  string          `json:"session_id,omitempty" yaml:"session_id,omitempty"`
}

type CoordinationTargetMetadata struct {
	Target                 bool                        `json:"target,omitempty" yaml:"target,omitempty"`
	Role                   CoordinationRole            `json:"role,omitempty" yaml:"role,omitempty"`
	TaskTypes              []string                    `json:"task_types,omitempty" yaml:"task_types,omitempty"`
	ExecutionModes         []CoordinationExecutionMode `json:"execution_modes,omitempty" yaml:"execution_modes,omitempty"`
	LongRunning            bool                        `json:"long_running,omitempty" yaml:"long_running,omitempty"`
	MaxDepth               int                         `json:"max_depth,omitempty" yaml:"max_depth,omitempty"`
	MaxRuntimeSeconds      int                         `json:"max_runtime_seconds,omitempty" yaml:"max_runtime_seconds,omitempty"`
	DirectInsertionAllowed bool                        `json:"direct_insertion_allowed,omitempty" yaml:"direct_insertion_allowed,omitempty"`
}

type CapabilityDescriptor struct {
	ID            string                      `json:"id" yaml:"id"`
	Kind          CapabilityKind              `json:"kind" yaml:"kind"`
	RuntimeFamily CapabilityRuntimeFamily     `json:"runtime_family,omitempty" yaml:"runtime_family,omitempty"`
	Name          string                      `json:"name" yaml:"name"`
	Version       string                      `json:"version,omitempty" yaml:"version,omitempty"`
	Description   string                      `json:"description,omitempty" yaml:"description,omitempty"`
	Category      string                      `json:"category,omitempty" yaml:"category,omitempty"`
	Tags          []string                    `json:"tags,omitempty" yaml:"tags,omitempty"`
	Source        CapabilitySource            `json:"source,omitempty" yaml:"source,omitempty"`
	TrustClass    TrustClass                  `json:"trust_class,omitempty" yaml:"trust_class,omitempty"`
	RiskClasses   []RiskClass                 `json:"risk_classes,omitempty" yaml:"risk_classes,omitempty"`
	EffectClasses []EffectClass               `json:"effect_classes,omitempty" yaml:"effect_classes,omitempty"`
	Coordination  *CoordinationTargetMetadata `json:"coordination,omitempty" yaml:"coordination,omitempty"`
	Annotations   map[string]any              `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type TrustClass string

const (
	TrustClassBuiltinTrusted         TrustClass = "builtin-trusted"
	TrustClassWorkspaceTrusted       TrustClass = "workspace-trusted"
	TrustClassLLMGenerated           TrustClass = "llm-generated"
	TrustClassToolResult             TrustClass = "tool-result"
	TrustClassProviderLocalUntrusted TrustClass = "provider-local-untrusted"
	TrustClassRemoteDeclared         TrustClass = "remote-declared-untrusted"
	TrustClassRemoteApproved         TrustClass = "remote-approved"
)

type RiskClass string

const (
	RiskClassReadOnly     RiskClass = "read-only"
	RiskClassDestructive  RiskClass = "destructive"
	RiskClassExecute      RiskClass = "execute"
	RiskClassNetwork      RiskClass = "network"
	RiskClassCredentialed RiskClass = "credentialed"
	RiskClassExfiltration RiskClass = "exfiltration-sensitive"
	RiskClassSessioned    RiskClass = "sessioned"
)

type EffectClass string

const (
	EffectClassFilesystemMutation EffectClass = "filesystem-mutation"
	EffectClassProcessSpawn       EffectClass = "process-spawn"
	EffectClassNetworkEgress      EffectClass = "network-egress"
	EffectClassCredentialUse      EffectClass = "credential-use"
	EffectClassExternalState      EffectClass = "external-state-change"
	EffectClassSessionCreation    EffectClass = "long-lived-session-creation"
	EffectClassContextInsertion   EffectClass = "model-context-insertion"
)

type CoordinationRole string

const (
	CoordinationRolePlanner         CoordinationRole = "planner"
	CoordinationRoleArchitect       CoordinationRole = "architect"
	CoordinationRoleReviewer        CoordinationRole = "reviewer"
	CoordinationRoleVerifier        CoordinationRole = "verifier"
	CoordinationRoleExecutor        CoordinationRole = "executor"
	CoordinationRoleDomainPack      CoordinationRole = "domain-pack"
	CoordinationRoleBackgroundAgent CoordinationRole = "background-agent"
)

type CoordinationExecutionMode string

const (
	CoordinationExecutionModeSync            CoordinationExecutionMode = "sync"
	CoordinationExecutionModeSessionBacked   CoordinationExecutionMode = "session-backed"
	CoordinationExecutionModeBackgroundAgent CoordinationExecutionMode = "background-service"
)

type InsertionAction string

const (
	InsertionActionDirect       InsertionAction = "direct"
	InsertionActionSummarized   InsertionAction = "summarized"
	InsertionActionMetadataOnly InsertionAction = "metadata-only"
	InsertionActionHITLRequired InsertionAction = "hitl-required"
	InsertionActionDenied       InsertionAction = "denied"
)

type SessionScope string

const (
	SessionScopeMain           SessionScope = "main"
	SessionScopePerChannelPeer SessionScope = "per-channel-peer"
	SessionScopePerThread      SessionScope = "per-thread"
)

type SessionOperation string

const (
	SessionOperationAttach  SessionOperation = "attach"
	SessionOperationSend    SessionOperation = "send"
	SessionOperationInvoke  SessionOperation = "invoke"
	SessionOperationResume  SessionOperation = "resume"
	SessionOperationInspect SessionOperation = "inspect"
	SessionOperationClose   SessionOperation = "close"
)

type ExternalProvider string

const (
	ExternalProviderDiscord  ExternalProvider = "discord"
	ExternalProviderTelegram ExternalProvider = "telegram"
	ExternalProviderWebchat  ExternalProvider = "webchat"
	ExternalProviderNexus    ExternalProvider = "nexus"
)

type SessionSelector struct {
	Partitions                []string           `yaml:"partitions,omitempty" json:"partitions,omitempty"`
	ChannelIDs                []string           `yaml:"channel_ids,omitempty" json:"channel_ids,omitempty"`
	Scopes                    []SessionScope     `yaml:"scopes,omitempty" json:"scopes,omitempty"`
	TrustClasses              []TrustClass       `yaml:"trust_classes,omitempty" json:"trust_classes,omitempty"`
	Operations                []SessionOperation `yaml:"operations,omitempty" json:"operations,omitempty"`
	ActorKinds                []string           `yaml:"actor_kinds,omitempty" json:"actor_kinds,omitempty"`
	ActorIDs                  []string           `yaml:"actor_ids,omitempty" json:"actor_ids,omitempty"`
	ExternalProviders         []ExternalProvider `yaml:"external_providers,omitempty" json:"external_providers,omitempty"`
	RequireOwnership          *bool              `yaml:"require_ownership,omitempty" json:"require_ownership,omitempty"`
	RequireDelegation         *bool              `yaml:"require_delegation,omitempty" json:"require_delegation,omitempty"`
	RequireExternalBinding    *bool              `yaml:"require_external_binding,omitempty" json:"require_external_binding,omitempty"`
	RequireResolvedExternal   *bool              `yaml:"require_resolved_external,omitempty" json:"require_resolved_external,omitempty"`
	RequireRestrictedExternal *bool              `yaml:"require_restricted_external,omitempty" json:"require_restricted_external,omitempty"`
	AuthenticatedOnly         *bool              `yaml:"authenticated_only,omitempty" json:"authenticated_only,omitempty"`
}

type SessionPolicy struct {
	ID          string               `yaml:"id" json:"id"`
	Name        string               `yaml:"name" json:"name"`
	Priority    int                  `yaml:"priority,omitempty" json:"priority,omitempty"`
	Enabled     bool                 `yaml:"enabled" json:"enabled"`
	Selector    SessionSelector      `yaml:"selector" json:"selector"`
	Effect      AgentPermissionLevel `yaml:"effect" json:"effect"`
	Approvers   []string             `yaml:"approvers,omitempty" json:"approvers,omitempty"`
	ApprovalTTL string               `yaml:"approval_ttl,omitempty" json:"approval_ttl,omitempty"`
	Reason      string               `yaml:"reason,omitempty" json:"reason,omitempty"`
}

type ProviderKind string

const (
	ProviderKindBuiltin      ProviderKind = "builtin"
	ProviderKindPlugin       ProviderKind = "plugin"
	ProviderKindMCPClient    ProviderKind = "mcp-client"
	ProviderKindMCPServer    ProviderKind = "mcp-server"
	ProviderKindAgentRuntime ProviderKind = "agent-runtime"
	ProviderKindLSP          ProviderKind = "lsp"
	ProviderKindNodeDevice   ProviderKind = "node-device"
)

type RecoverabilityMode string

const (
	RecoverabilityEphemeral        RecoverabilityMode = "ephemeral"
	RecoverabilityInProcess        RecoverabilityMode = "recoverable-in-process"
	RecoverabilityPersistedRestore RecoverabilityMode = "recoverable-from-persisted-state"
)

type ProviderConfig struct {
	ID              string             `json:"id" yaml:"id"`
	Kind            ProviderKind       `json:"kind" yaml:"kind"`
	Enabled         bool               `json:"enabled" yaml:"enabled"`
	Target          string             `json:"target,omitempty" yaml:"target,omitempty"`
	ActivationScope string             `json:"activation_scope,omitempty" yaml:"activation_scope,omitempty"`
	TrustBaseline   TrustClass         `json:"trust_baseline,omitempty" yaml:"trust_baseline,omitempty"`
	Recoverability  RecoverabilityMode `json:"recoverability,omitempty" yaml:"recoverability,omitempty"`
	Config          map[string]any     `json:"config,omitempty" yaml:"config,omitempty"`
}

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

func ValidateSessionPolicy(policy SessionPolicy) error {
	if strings.TrimSpace(policy.ID) == "" {
		return fmt.Errorf("id required")
	}
	if strings.TrimSpace(policy.Name) == "" {
		return fmt.Errorf("name required")
	}
	if err := ValidateSessionSelector(policy.Selector); err != nil {
		return fmt.Errorf("selector invalid: %w", err)
	}
	switch policy.Effect {
	case AgentPermissionAllow, AgentPermissionAsk, AgentPermissionDeny:
	default:
		return fmt.Errorf("effect=%s invalid", policy.Effect)
	}
	for _, approver := range policy.Approvers {
		if strings.TrimSpace(approver) == "" {
			return fmt.Errorf("approvers contains empty approver")
		}
	}
	return nil
}

func ValidateSessionSelector(selector SessionSelector) error {
	if len(selector.Partitions) == 0 &&
		len(selector.ChannelIDs) == 0 &&
		len(selector.Scopes) == 0 &&
		len(selector.TrustClasses) == 0 &&
		len(selector.Operations) == 0 &&
		len(selector.ActorKinds) == 0 &&
		len(selector.ActorIDs) == 0 &&
		len(selector.ExternalProviders) == 0 &&
		selector.RequireOwnership == nil &&
		selector.RequireDelegation == nil &&
		selector.RequireExternalBinding == nil &&
		selector.RequireResolvedExternal == nil &&
		selector.RequireRestrictedExternal == nil &&
		selector.AuthenticatedOnly == nil {
		return fmt.Errorf("at least one selector field required")
	}
	for _, partition := range selector.Partitions {
		if strings.TrimSpace(partition) == "" {
			return fmt.Errorf("partitions contains empty partition")
		}
	}
	for _, channelID := range selector.ChannelIDs {
		if strings.TrimSpace(channelID) == "" {
			return fmt.Errorf("channel_ids contains empty channel id")
		}
	}
	for _, scope := range selector.Scopes {
		switch scope {
		case SessionScopeMain, SessionScopePerChannelPeer, SessionScopePerThread:
		default:
			return fmt.Errorf("scope %s invalid", scope)
		}
	}
	for _, operation := range selector.Operations {
		switch operation {
		case SessionOperationAttach, SessionOperationSend, SessionOperationInvoke, SessionOperationResume, SessionOperationInspect, SessionOperationClose:
		default:
			return fmt.Errorf("operation %s invalid", operation)
		}
	}
	for _, provider := range selector.ExternalProviders {
		switch provider {
		case ExternalProviderDiscord, ExternalProviderTelegram, ExternalProviderWebchat, ExternalProviderNexus:
		default:
			return fmt.Errorf("external provider %s invalid", provider)
		}
	}
	return nil
}

func (c ProviderConfig) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("provider id required")
	}
	switch c.Kind {
	case ProviderKindBuiltin, ProviderKindPlugin, ProviderKindMCPClient, ProviderKindMCPServer, ProviderKindAgentRuntime, ProviderKindLSP, ProviderKindNodeDevice:
	default:
		return fmt.Errorf("provider kind %s invalid", c.Kind)
	}
	switch c.Recoverability {
	case "", RecoverabilityEphemeral, RecoverabilityInProcess, RecoverabilityPersistedRestore:
	default:
		return fmt.Errorf("recoverability mode %s invalid", c.Recoverability)
	}
	return nil
}
