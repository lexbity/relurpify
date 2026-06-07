package capability

import (
	"context"
	"fmt"
	"strings"

	agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

type ProviderKind = agentspec.ProviderKind

const (
	ProviderKindBuiltin      ProviderKind = agentspec.ProviderKindBuiltin
	ProviderKindPlugin       ProviderKind = agentspec.ProviderKindPlugin
	ProviderKindMCPClient    ProviderKind = agentspec.ProviderKindMCPClient
	ProviderKindMCPServer    ProviderKind = agentspec.ProviderKindMCPServer
	ProviderKindAgentRuntime ProviderKind = agentspec.ProviderKindAgentRuntime
	ProviderKindLSP          ProviderKind = agentspec.ProviderKindLSP
	ProviderKindNodeDevice   ProviderKind = agentspec.ProviderKindNodeDevice
)

type RecoverabilityMode = policy.RecoverabilityMode

const (
	RecoverabilityEphemeral        RecoverabilityMode = policy.RecoverabilityEphemeral
	RecoverabilityInProcess        RecoverabilityMode = policy.RecoverabilityInProcess
	RecoverabilityPersistedRestore RecoverabilityMode = policy.RecoverabilityPersistedRestore
)

type ProviderDescriptor struct {
	ID                 string                  `json:"id"`
	Kind               ProviderKind            `json:"kind"`
	ConfiguredSource   string                  `json:"configured_source,omitempty"`
	ActivationScope    string                  `json:"activation_scope,omitempty"`
	TrustBaseline      agentspec.TrustClass    `json:"trust_baseline,omitempty"`
	RecoverabilityMode RecoverabilityMode      `json:"recoverability_mode,omitempty"`
	SupportsHealth     bool                    `json:"supports_health,omitempty"`
	Security           ProviderSecurityProfile `json:"security,omitempty"`
}

type ProviderConfig struct {
	ID              string               `json:"id"`
	Kind            ProviderKind         `json:"kind"`
	Enabled         bool                 `json:"enabled"`
	Target          string               `json:"target,omitempty"`
	ActivationScope string               `json:"activation_scope,omitempty"`
	TrustBaseline   agentspec.TrustClass `json:"trust_baseline,omitempty"`
	Recoverability  RecoverabilityMode   `json:"recoverability,omitempty"`
	Config          map[string]any       `json:"config,omitempty"`
}

type ProviderOriginKind = agentspec.ProviderOriginKind

const (
	ProviderOriginLocal  ProviderOriginKind = agentspec.ProviderOriginLocal
	ProviderOriginRemote ProviderOriginKind = agentspec.ProviderOriginRemote
)

type ProviderSecurityProfile struct {
	Origin                     ProviderOriginKind `json:"origin,omitempty"`
	HoldsCredentials           bool               `json:"holds_credentials,omitempty"`
	CredentialDomains          []string           `json:"credential_domains,omitempty"`
	SafeForDirectInsertion     bool               `json:"safe_for_direct_insertion,omitempty"`
	RequiresFrameworkMediation bool               `json:"requires_framework_mediation,omitempty"`
}

type ProviderSession struct {
	ID             string                 `json:"id"`
	ProviderID     string                 `json:"provider_id"`
	CapabilityIDs  []string               `json:"capability_ids,omitempty"`
	WorkflowID     string                 `json:"workflow_id,omitempty"`
	TaskID         string                 `json:"task_id,omitempty"`
	TrustClass     agentspec.TrustClass   `json:"trust_class,omitempty"`
	Recoverability RecoverabilityMode     `json:"recoverability,omitempty"`
	CreatedAt      string                 `json:"created_at,omitempty"`
	LastActivityAt string                 `json:"last_activity_at,omitempty"`
	Health         string                 `json:"health,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type ProviderHealthSnapshot struct {
	Status   string                 `json:"status,omitempty"`
	Message  string                 `json:"message,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type ProviderSnapshot struct {
	ProviderID      string                 `json:"provider_id"`
	Recoverability  RecoverabilityMode     `json:"recoverability,omitempty"`
	Descriptor      ProviderDescriptor     `json:"descriptor"`
	Health          ProviderHealthSnapshot `json:"health,omitempty"`
	CapabilityIDs   []string               `json:"capability_ids,omitempty"`
	WorkflowID      string                 `json:"workflow_id,omitempty"`
	TaskID          string                 `json:"task_id,omitempty"`
	Metadata        map[string]any         `json:"metadata,omitempty"`
	State           any                    `json:"state,omitempty"`
	CapturedAt      string                 `json:"captured_at,omitempty"`
	LastRecoveryErr string                 `json:"last_recovery_error,omitempty"`
}

type ProviderSessionSnapshot struct {
	Session         ProviderSession `json:"session"`
	State           any             `json:"state,omitempty"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
	CapturedAt      string          `json:"captured_at,omitempty"`
	LastRecoveryErr string          `json:"last_recovery_error,omitempty"`
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
	case "", agentspec.TrustClassBuiltinTrusted, agentspec.TrustClassWorkspaceTrusted, agentspec.TrustClassLLMGenerated, agentspec.TrustClassToolResult, agentspec.TrustClassProviderLocalUntrusted, agentspec.TrustClassRemoteDeclared, agentspec.TrustClassRemoteApproved:
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
	case "", agentspec.TrustClassBuiltinTrusted, agentspec.TrustClassWorkspaceTrusted, agentspec.TrustClassLLMGenerated, agentspec.TrustClassToolResult, agentspec.TrustClassProviderLocalUntrusted, agentspec.TrustClassRemoteDeclared, agentspec.TrustClassRemoteApproved:
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
	desc.RuntimeFamily = agentspec.CapabilityRuntimeFamilyProvider
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
	if desc.Kind != agentspec.CapabilityKindTool {
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

func normalizeProviderCapabilityScope(scope agentspec.CapabilityScope, provider ProviderDescriptor) agentspec.CapabilityScope {
	switch provider.Security.Origin {
	case ProviderOriginRemote:
		return agentspec.CapabilityScopeRemote
	case ProviderOriginLocal:
		if scope == agentspec.CapabilityScopeRemote {
			return agentspec.CapabilityScopeRemote
		}
		return agentspec.CapabilityScopeProvider
	default:
		if scope != "" {
			return scope
		}
		return agentspec.CapabilityScopeProvider
	}
}

func providerCapabilityTrustBaseline(provider ProviderDescriptor, policy agentspec.ProviderPolicy, scope agentspec.CapabilityScope) agentspec.TrustClass {
	if policy.DefaultTrust != "" {
		return policy.DefaultTrust
	}
	if provider.TrustBaseline != "" {
		return provider.TrustBaseline
	}
	switch scope {
	case agentspec.CapabilityScopeRemote:
		return agentspec.TrustClassRemoteDeclared
	case agentspec.CapabilityScopeWorkspace:
		return agentspec.TrustClassWorkspaceTrusted
	case agentspec.CapabilityScopeBuiltin:
		return agentspec.TrustClassBuiltinTrusted
	default:
		return agentspec.TrustClassProviderLocalUntrusted
	}
}

func moreRestrictiveTrustClass(left, right agentspec.TrustClass) agentspec.TrustClass {
	if trustClassRank(left) >= trustClassRank(right) {
		return left
	}
	return right
}

func trustClassRank(class agentspec.TrustClass) int {
	switch class {
	case agentspec.TrustClassBuiltinTrusted, agentspec.TrustClassWorkspaceTrusted:
		return 0
	case agentspec.TrustClassLLMGenerated, agentspec.TrustClassToolResult, agentspec.TrustClassRemoteApproved:
		return 1
	case agentspec.TrustClassProviderLocalUntrusted:
		return 2
	case agentspec.TrustClassRemoteDeclared:
		return 3
	default:
		return 3
	}
}
