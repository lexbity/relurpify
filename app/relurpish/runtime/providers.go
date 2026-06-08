package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	capability "codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/identity"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

var ErrSessionNotManaged = errors.New("provider session not managed")

// ManagedProvider is the minimal lifecycle surface for long-lived runtime services.
type ManagedProvider interface {
	Close() error
}

// RuntimeProvider can attach tools or state to a runtime and will be closed
// when the runtime shuts down.
type RuntimeProvider interface {
	ManagedProvider
	Initialize(ctx context.Context, rt *Runtime) error
}

// DescribedRuntimeProvider exposes framework-owned provider metadata so runtime
// policy can gate activation before initialization.
type DescribedRuntimeProvider interface {
	RuntimeProvider
	Descriptor() capability.ProviderDescriptor
}

// SessionManagedProvider supports forced shutdown of individual live provider sessions.
type SessionManagedProvider interface {
	RuntimeProvider
	CloseSession(ctx context.Context, sessionID string) error
}

type runtimeProviderHealthReporter interface {
	HealthSnapshot(ctx context.Context) (capability.ProviderHealthSnapshot, error)
}

type runtimeProviderSessionLister interface {
	ListSessions(ctx context.Context) ([]capability.ProviderSession, error)
}

type runtimeProviderRecord struct {
	provider RuntimeProvider
	desc     capability.ProviderDescriptor
}

// RegisterBuiltinProviders installs builtin runtime-managed providers declared by the agent spec.
func RegisterBuiltinProviders(ctx context.Context, rt *Runtime) error {
	for _, providerCfg := range mergeConfiguredProviders(rt.AgentWorkspace().AgentSpec) {
		provider, err := providerFromConfig(providerCfg)
		if err != nil {
			return err
		}
		if provider == nil {
			continue
		}
		if err := rt.RegisterProvider(ctx, provider); err != nil {
			return err
		}
	}
	return nil
}

func mergeConfiguredProviders(spec *agentspec.AgentRuntimeSpec) []capability.ProviderConfig {
	if spec == nil || len(spec.Providers) == 0 {
		return nil
	}
	out := make([]capability.ProviderConfig, len(spec.Providers))
	for i, provider := range spec.Providers {
		out[i] = capability.ProviderConfig{
			ID:              provider.ID,
			Kind:            agentspec.ProviderKind(provider.Kind),
			Enabled:         provider.Enabled,
			Target:          provider.Target,
			ActivationScope: provider.ActivationScope,
			TrustBaseline:   agentspec.TrustClass(provider.TrustBaseline),
			Recoverability:  policy.RecoverabilityMode(provider.Recoverability),
		}
		if len(provider.Config) > 0 {
			out[i].Config = make(map[string]any, len(provider.Config))
			for key, value := range provider.Config {
				out[i].Config[key] = value
			}
		}
	}
	return out
}

// RegisterProvider initializes a provider against the runtime and records it
// for deterministic shutdown.
func (r *Runtime) RegisterProvider(ctx context.Context, provider RuntimeProvider) error {
	if r == nil {
		return fmt.Errorf("runtime unavailable")
	}
	if provider == nil {
		return fmt.Errorf("provider required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if described, ok := provider.(DescribedRuntimeProvider); ok {
		desc := described.Descriptor()
		if desc.ID != "" || desc.Kind != "" {
			if err := desc.Validate(); err != nil {
				return err
			}
			if err := r.authorizeProviderActivation(ctx, desc); err != nil {
				return err
			}
		}
	}
	if err := provider.Initialize(ctx, r); err != nil {
		return err
	}
	r.providersMu.Lock()
	r.providers = append(r.providers, runtimeProviderRecord{provider: provider, desc: providerDescriptor(provider)})
	r.providersMu.Unlock()
	r.emitProviderLifecycleEvent(providerDescriptor(provider).ID, "", "provider_admitted", "", map[string]interface{}{
		"provider_kind": string(providerDescriptor(provider).Kind),
	})
	return nil
}

func (r *Runtime) authorizeProviderActivation(ctx context.Context, desc capability.ProviderDescriptor) error {
	if r == nil {
		return fmt.Errorf("runtime unavailable")
	}
	if r.AgentWorkspace().Registration != nil && r.AgentWorkspace().Registration.Policy != nil {
		metadata := map[string]string{
			"provider_id":   desc.ID,
			"provider_kind": string(desc.Kind),
		}
		if desc.Security.Origin != "" {
			metadata["provider_origin"] = string(desc.Security.Origin)
		}
		_, err := fauthorization.EnforcePolicyRequest(ctx, r.AgentWorkspace().Registration.Policy, policy.PolicyRequest{
			Target:         policy.PolicyTargetProvider,
			Actor:          identity.EventActor{Kind: "agent", ID: r.AgentWorkspace().Registration.ID},
			CapabilityID:   "provider:" + desc.ID + ":activate",
			CapabilityName: "provider:" + desc.ID + ":activate",
			ProviderKind:   string(desc.Kind),
			ProviderOrigin: string(desc.Security.Origin),
			TrustClass:     string(desc.TrustBaseline),
		}, fauthorization.ApprovalRequest{
			AgentID: r.AgentWorkspace().Registration.ID,
			Manager: r.AgentWorkspace().Registration.Permissions,
			Permission: permissions.PermissionDescriptor{
				Type:         permissions.PermissionTypeCapability,
				Action:       fmt.Sprintf("provider:%s:activate", desc.ID),
				Resource:     desc.ID,
				Metadata:     metadata,
				RequiresHITL: true,
			},
			Justification:      fmt.Sprintf("activate provider %s", desc.ID),
			Scope:              policy.GrantScopeSession,
			Risk:               policy.RiskLevelMedium,
			MissingManagerErr:  fmt.Sprintf("provider %s activation requires approval but permission manager is missing", desc.ID),
			DenyReasonFallback: fmt.Sprintf("provider %s activation denied by policy", desc.ID),
		})
		if err != nil {
			return err
		}
		return nil
	}
	level := agentspec.AgentPermissionAllow
	if desc.Security.Origin == agentspec.ProviderOriginRemote {
		level = agentspec.AgentPermissionAsk
	}
	if desc.Kind == agentspec.ProviderKindBuiltin || desc.Kind == agentspec.ProviderKindAgentRuntime {
		level = agentspec.AgentPermissionAllow
	}
	if r.AgentWorkspace().AgentSpec != nil && r.AgentWorkspace().AgentSpec.ProviderPolicies != nil {
		if policy, ok := r.AgentWorkspace().AgentSpec.ProviderPolicies[desc.ID]; ok && policy.Activate != "" {
			level = policy.Activate
		}
	}
	switch level {
	case agentspec.AgentPermissionAllow, "":
		return nil
	case agentspec.AgentPermissionDeny:
		return fmt.Errorf("provider %s activation denied by policy", desc.ID)
	case agentspec.AgentPermissionAsk:
		if r.AgentWorkspace().Registration == nil || r.AgentWorkspace().Registration.Permissions == nil {
			return fmt.Errorf("provider %s activation requires approval but permission manager is missing", desc.ID)
		}
		metadata := map[string]string{
			"provider_id":   desc.ID,
			"provider_kind": string(desc.Kind),
		}
		if desc.Security.Origin != "" {
			metadata["provider_origin"] = string(desc.Security.Origin)
		}
		return r.AgentWorkspace().Registration.Permissions.RequireApproval(ctx, r.AgentWorkspace().Registration.ID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeCapability,
			Action:       fmt.Sprintf("provider:%s:activate", desc.ID),
			Resource:     desc.ID,
			Metadata:     metadata,
			RequiresHITL: true,
		}, fmt.Sprintf("activate provider %s", desc.ID), policy.GrantScopeSession, policy.RiskLevelMedium, 0)
	default:
		return fmt.Errorf("provider %s activation policy %s invalid", desc.ID, level)
	}
}

func (r *Runtime) QuarantineProvider(ctx context.Context, providerID, reason string) error {
	if r == nil {
		return fmt.Errorf("runtime unavailable")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return fmt.Errorf("provider id required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r.Tools != nil {
		r.Tools.RevokeProvider(providerID, reason)
	}
	record, ok := r.removeProviderRecord(providerID)
	if !ok {
		r.emitProviderLifecycleEvent(providerID, "", "provider_quarantined", reason, map[string]interface{}{})
		return nil
	}
	err := record.provider.Close()
	r.emitProviderLifecycleEvent(providerID, "", "provider_quarantined", reason, map[string]interface{}{
		"provider_kind": string(record.desc.Kind),
	})
	return err
}

func (r *Runtime) RevokeSession(ctx context.Context, sessionID, reason string) error {
	if r == nil {
		return fmt.Errorf("runtime unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r.Tools != nil {
		r.Tools.RevokeSession(sessionID, reason)
	}
	r.providersMu.Lock()
	records := append([]runtimeProviderRecord(nil), r.providers...)
	r.providersMu.Unlock()
	for _, record := range records {
		managed, ok := record.provider.(SessionManagedProvider)
		if !ok {
			continue
		}
		err := managed.CloseSession(ctx, sessionID)
		switch {
		case err == nil:
			r.emitProviderLifecycleEvent(record.desc.ID, sessionID, "session_revoked", reason, map[string]interface{}{
				"provider_kind": string(record.desc.Kind),
			})
			return nil
		case errors.Is(err, ErrSessionNotManaged):
			continue
		default:
			return err
		}
	}
	r.emitProviderLifecycleEvent("", sessionID, "session_revoked", reason, nil)
	return nil
}

func (r *Runtime) CaptureProviderSnapshots(ctx context.Context) ([]capability.ProviderSnapshot, []capability.ProviderSessionSnapshot, error) {
	return nil, nil, nil
}

func (r *Runtime) PersistProviderSnapshots(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	return nil
}

func (r *Runtime) registeredProviders() []RuntimeProvider {
	if r == nil {
		return nil
	}
	r.providersMu.Lock()
	defer r.providersMu.Unlock()
	providers := make([]RuntimeProvider, 0, len(r.providers))
	for _, record := range r.providers {
		providers = append(providers, record.provider)
	}
	r.providers = nil
	return providers
}

func (r *Runtime) removeProviderRecord(providerID string) (runtimeProviderRecord, bool) {
	r.providersMu.Lock()
	defer r.providersMu.Unlock()
	for idx, record := range r.providers {
		if record.desc.ID != providerID {
			continue
		}
		r.providers = append(r.providers[:idx], r.providers[idx+1:]...)
		return record, true
	}
	return runtimeProviderRecord{}, false
}

func providerDescriptor(provider RuntimeProvider) capability.ProviderDescriptor {
	if described, ok := provider.(DescribedRuntimeProvider); ok {
		return described.Descriptor()
	}
	return capability.ProviderDescriptor{}
}

func (r *Runtime) emitProviderLifecycleEvent(providerID, sessionID, event, reason string, metadata map[string]interface{}) {
	if r == nil || r.AgentWorkspace().Telemetry == nil {
		return
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["provider_event"] = event
	if providerID != "" {
		metadata["provider_id"] = providerID
	}
	if sessionID != "" {
		metadata["session_id"] = sessionID
	}
	if reason != "" {
		metadata["reason"] = reason
	}
	r.AgentWorkspace().Telemetry.Emit(telemetry.Event{
		Type:      telemetry.EventStateChange,
		Timestamp: time.Now().UTC(),
		Message:   strings.ReplaceAll(event, "_", " "),
		Metadata:  metadata,
	})
}
