package core

import (
	"context"
	"fmt"
	"strings"

	agentspec "codeburg.org/lexbit/relurpify/framework/agentspec"
)

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

type ProviderDescriptor struct {
	ID                 string                  `json:"id" yaml:"id"`
	Kind               ProviderKind            `json:"kind" yaml:"kind"`
	ConfiguredSource   string                  `json:"configured_source,omitempty" yaml:"configured_source,omitempty"`
	ActivationScope    string                  `json:"activation_scope,omitempty" yaml:"activation_scope,omitempty"`
	TrustBaseline      TrustClass              `json:"trust_baseline,omitempty" yaml:"trust_baseline,omitempty"`
	RecoverabilityMode RecoverabilityMode      `json:"recoverability_mode,omitempty" yaml:"recoverability_mode,omitempty"`
	SupportsHealth     bool                    `json:"supports_health,omitempty" yaml:"supports_health,omitempty"`
	Security           ProviderSecurityProfile `json:"security,omitempty" yaml:"security,omitempty"`
}

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

type ProviderOriginKind string

const (
	ProviderOriginLocal  ProviderOriginKind = "local"
	ProviderOriginRemote ProviderOriginKind = "remote"
)

type ProviderSecurityProfile struct {
	Origin                     ProviderOriginKind `json:"origin,omitempty" yaml:"origin,omitempty"`
	HoldsCredentials           bool               `json:"holds_credentials,omitempty" yaml:"holds_credentials,omitempty"`
	CredentialDomains          []string           `json:"credential_domains,omitempty" yaml:"credential_domains,omitempty"`
	SafeForDirectInsertion     bool               `json:"safe_for_direct_insertion,omitempty" yaml:"safe_for_direct_insertion,omitempty"`
	RequiresFrameworkMediation bool               `json:"requires_framework_mediation,omitempty" yaml:"requires_framework_mediation,omitempty"`
}

type ProviderSession struct {
	ID             string                 `json:"id" yaml:"id"`
	ProviderID     string                 `json:"provider_id" yaml:"provider_id"`
	CapabilityIDs  []string               `json:"capability_ids,omitempty" yaml:"capability_ids,omitempty"`
	WorkflowID     string                 `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	TaskID         string                 `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	TrustClass     TrustClass             `json:"trust_class,omitempty" yaml:"trust_class,omitempty"`
	Recoverability RecoverabilityMode     `json:"recoverability,omitempty" yaml:"recoverability,omitempty"`
	CreatedAt      string                 `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	LastActivityAt string                 `json:"last_activity_at,omitempty" yaml:"last_activity_at,omitempty"`
	Health         string                 `json:"health,omitempty" yaml:"health,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type ProviderHealthSnapshot struct {
	Status   string                 `json:"status,omitempty" yaml:"status,omitempty"`
	Message  string                 `json:"message,omitempty" yaml:"message,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type ProviderSnapshot struct {
	ProviderID      string                 `json:"provider_id" yaml:"provider_id"`
	Recoverability  RecoverabilityMode     `json:"recoverability,omitempty" yaml:"recoverability,omitempty"`
	Descriptor      ProviderDescriptor     `json:"descriptor" yaml:"descriptor"`
	Health          ProviderHealthSnapshot `json:"health,omitempty" yaml:"health,omitempty"`
	CapabilityIDs   []string               `json:"capability_ids,omitempty" yaml:"capability_ids,omitempty"`
	WorkflowID      string                 `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	TaskID          string                 `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	Metadata        map[string]any         `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	State           any                    `json:"state,omitempty" yaml:"state,omitempty"`
	CapturedAt      string                 `json:"captured_at,omitempty" yaml:"captured_at,omitempty"`
	LastRecoveryErr string                 `json:"last_recovery_error,omitempty" yaml:"last_recovery_error,omitempty"`
}

type ProviderSessionSnapshot struct {
	Session         ProviderSession `json:"session" yaml:"session"`
	State           any             `json:"state,omitempty" yaml:"state,omitempty"`
	Metadata        map[string]any  `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CapturedAt      string          `json:"captured_at,omitempty" yaml:"captured_at,omitempty"`
	LastRecoveryErr string          `json:"last_recovery_error,omitempty" yaml:"last_recovery_error,omitempty"`
}

type CapabilityRegistrar interface {
	RegisterCapability(descriptor CapabilityDescriptor) error
}

type Provider interface {
	Descriptor() ProviderDescriptor
	Initialize(ctx context.Context, runtime ProviderRuntime) error
	RegisterCapabilities(ctx context.Context, registrar CapabilityRegistrar) error
	ListSessions(ctx context.Context) ([]ProviderSession, error)
	HealthSnapshot(ctx context.Context) (ProviderHealthSnapshot, error)
	Close(ctx context.Context) error
}

type ProviderRuntime interface {
	State() map[string]interface{}
}

type ProviderSnapshotter interface {
	SnapshotProvider(ctx context.Context) (*ProviderSnapshot, error)
}

type ProviderSessionSnapshotter interface {
	SnapshotSessions(ctx context.Context) ([]ProviderSessionSnapshot, error)
}

type ProviderRestorer interface {
	RestoreProvider(ctx context.Context, snapshot ProviderSnapshot) error
}

type ProviderSessionRestorer interface {
	RestoreSession(ctx context.Context, snapshot ProviderSessionSnapshot) error
}

func (d ProviderDescriptor) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("provider id required")
	}
	switch d.Kind {
	case ProviderKindBuiltin, ProviderKindPlugin, ProviderKindMCPClient, ProviderKindMCPServer, ProviderKindAgentRuntime, ProviderKindLSP, ProviderKindNodeDevice:
	default:
		return fmt.Errorf("provider kind %s invalid", d.Kind)
	}
	switch d.TrustBaseline {
	case "", TrustClassBuiltinTrusted, TrustClassWorkspaceTrusted, TrustClassLLMGenerated, TrustClassToolResult, TrustClassProviderLocalUntrusted, TrustClassRemoteDeclared, TrustClassRemoteApproved:
	default:
		return fmt.Errorf("trust baseline %s invalid", d.TrustBaseline)
	}
	switch d.RecoverabilityMode {
	case "", RecoverabilityEphemeral, RecoverabilityInProcess, RecoverabilityPersistedRestore:
	default:
		return fmt.Errorf("recoverability mode %s invalid", d.RecoverabilityMode)
	}
	if err := d.Security.Validate(); err != nil {
		return fmt.Errorf("provider security invalid: %w", err)
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
	switch c.TrustBaseline {
	case "", TrustClassBuiltinTrusted, TrustClassWorkspaceTrusted, TrustClassLLMGenerated, TrustClassToolResult, TrustClassProviderLocalUntrusted, TrustClassRemoteDeclared, TrustClassRemoteApproved:
	default:
		return fmt.Errorf("trust baseline %s invalid", c.TrustBaseline)
	}
	switch c.Recoverability {
	case "", RecoverabilityEphemeral, RecoverabilityInProcess, RecoverabilityPersistedRestore:
	default:
		return fmt.Errorf("recoverability mode %s invalid", c.Recoverability)
	}
	return nil
}

func (s ProviderSnapshot) Validate() error {
	if strings.TrimSpace(s.ProviderID) == "" {
		return fmt.Errorf("provider id required")
	}
	if err := s.Descriptor.Validate(); err != nil {
		return fmt.Errorf("descriptor invalid: %w", err)
	}
	if s.Descriptor.ID != s.ProviderID {
		return fmt.Errorf("descriptor provider id %s does not match snapshot provider id %s", s.Descriptor.ID, s.ProviderID)
	}
	switch s.Recoverability {
	case "", RecoverabilityEphemeral, RecoverabilityInProcess, RecoverabilityPersistedRestore:
	default:
		return fmt.Errorf("recoverability mode %s invalid", s.Recoverability)
	}
	return nil
}

func (s ProviderSessionSnapshot) Validate() error {
	if strings.TrimSpace(s.Session.ID) == "" {
		return fmt.Errorf("session id required")
	}
	if strings.TrimSpace(s.Session.ProviderID) == "" {
		return fmt.Errorf("provider id required")
	}
	switch s.Session.Recoverability {
	case "", RecoverabilityEphemeral, RecoverabilityInProcess, RecoverabilityPersistedRestore:
	default:
		return fmt.Errorf("recoverability mode %s invalid", s.Session.Recoverability)
	}
	return nil
}

func (p ProviderSecurityProfile) Validate() error {
	switch p.Origin {
	case "", ProviderOriginLocal, ProviderOriginRemote:
	default:
		return fmt.Errorf("origin %s invalid", p.Origin)
	}
	for _, domain := range p.CredentialDomains {
		if strings.TrimSpace(domain) == "" {
			return fmt.Errorf("credential_domains contains empty value")
		}
	}
	if len(p.CredentialDomains) > 0 && !p.HoldsCredentials {
		return fmt.Errorf("credential_domains requires holds_credentials=true")
	}
	return nil
}

// NormalizeProviderCapability applies provider-owned admission defaults to a
// capability descriptor before it enters the shared registry.
func NormalizeProviderCapability(desc CapabilityDescriptor, provider ProviderDescriptor, policy agentspec.ProviderPolicy) (CapabilityDescriptor, error) {
	if err := provider.Validate(); err != nil {
		return CapabilityDescriptor{}, fmt.Errorf("provider invalid: %w", err)
	}
	if err := agentspec.ValidateProviderPolicy(policy); err != nil {
		return CapabilityDescriptor{}, fmt.Errorf("provider policy invalid: %w", err)
	}
	desc = NormalizeCapabilityDescriptor(desc)
	if strings.TrimSpace(desc.ID) == "" {
		return CapabilityDescriptor{}, fmt.Errorf("capability id required")
	}
	if desc.Source.ProviderID != "" && desc.Source.ProviderID != provider.ID {
		return CapabilityDescriptor{}, fmt.Errorf("capability %s provider %s does not match provider %s", desc.ID, desc.Source.ProviderID, provider.ID)
	}
	desc.Source.ProviderID = provider.ID
	desc.Source.Scope = normalizeProviderCapabilityScope(desc.Source.Scope, provider)
	desc.RuntimeFamily = CapabilityRuntimeFamilyProvider
	baseline := providerCapabilityTrustBaseline(provider, policy, desc.Source.Scope)
	if desc.TrustClass == "" {
		desc.TrustClass = baseline
	} else {
		desc.TrustClass = moreRestrictiveTrustClass(desc.TrustClass, baseline)
	}
	if provider.Security.Origin == ProviderOriginRemote {
		desc = normalizeRemoteCapabilityDescriptor(desc, provider)
	}
	return desc, nil
}

func normalizeRemoteCapabilityDescriptor(desc CapabilityDescriptor, provider ProviderDescriptor) CapabilityDescriptor {
	desc.RiskClasses = nil
	if desc.Kind != CapabilityKindTool {
		desc.EffectClasses = nil
	}
	if desc.Annotations == nil {
		desc.Annotations = map[string]any{}
	}
	desc.Annotations["remote_metadata_advisory"] = true
	desc.Annotations["requires_insertion_policy"] = true
	desc.Annotations["admitted_by_provider"] = provider.ID
	if strings.TrimSpace(desc.Description) == "" {
		desc.Description = fmt.Sprintf("remote %s capability admitted via provider %s", desc.Kind, provider.ID)
	}
	return desc
}

func normalizeProviderCapabilityScope(scope CapabilityScope, provider ProviderDescriptor) CapabilityScope {
	switch provider.Security.Origin {
	case ProviderOriginRemote:
		return CapabilityScopeRemote
	case ProviderOriginLocal:
		if scope == CapabilityScopeRemote {
			return CapabilityScopeRemote
		}
		return CapabilityScopeProvider
	default:
		if scope != "" {
			return scope
		}
		return CapabilityScopeProvider
	}
}

func providerCapabilityTrustBaseline(provider ProviderDescriptor, policy agentspec.ProviderPolicy, scope CapabilityScope) TrustClass {
	if policy.DefaultTrust != "" {
		return policy.DefaultTrust
	}
	if provider.TrustBaseline != "" {
		return provider.TrustBaseline
	}
	switch scope {
	case CapabilityScopeRemote:
		return TrustClassRemoteDeclared
	case CapabilityScopeWorkspace:
		return TrustClassWorkspaceTrusted
	case CapabilityScopeBuiltin:
		return TrustClassBuiltinTrusted
	default:
		return TrustClassProviderLocalUntrusted
	}
}

func moreRestrictiveTrustClass(left, right TrustClass) TrustClass {
	if trustClassRank(left) >= trustClassRank(right) {
		return left
	}
	return right
}

func trustClassRank(class TrustClass) int {
	switch class {
	case TrustClassBuiltinTrusted, TrustClassWorkspaceTrusted:
		return 0
	case TrustClassLLMGenerated, TrustClassToolResult, TrustClassRemoteApproved:
		return 1
	case TrustClassProviderLocalUntrusted:
		return 2
	case TrustClassRemoteDeclared:
		return 3
	default:
		return 3
	}
}
