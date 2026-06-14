package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/provider"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/identity"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

var ErrSessionNotManaged = errors.New("provider session not managed")

const providerKindMetadataKey = "provider_kind"

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
	Descriptor() provider.ProviderDescriptor
}

// SessionManagedProvider supports forced shutdown of individual live provider sessions.
type SessionManagedProvider interface {
	RuntimeProvider
	CloseSession(ctx context.Context, sessionID string) error
}

type runtimeProviderRecord struct {
	provider RuntimeProvider
	desc     provider.ProviderDescriptor
}

// RegisterBuiltinProviders installs builtin runtime-managed providers declared by the agent spec.
func RegisterBuiltinProviders(ctx context.Context, rt *Runtime) error {
	if rt == nil || rt.AgentWorkspace() == nil || rt.AgentWorkspace().AgentSpec == nil {
		return nil
	}
	for _, providerSpec := range rt.AgentWorkspace().AgentSpec.Providers {
		log.Printf("runtime provider config unsupported: id=%s kind=%s target=%s", providerSpec.ID, providerSpec.Kind, providerSpec.Target)
		rt.emitProviderLifecycleEvent(providerSpec.ID, "", "provider_config_unsupported", "runtime provider config unsupported", map[string]any{
			providerKindMetadataKey: string(providerSpec.Kind),
			"provider_target":       providerSpec.Target,
			"activation_scope":      providerSpec.ActivationScope,
			"recoverability":        string(providerSpec.Recoverability),
		})
	}
	return nil
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
	r.emitProviderLifecycleEvent(providerDescriptor(provider).ID, "", "provider_admitted", "", map[string]any{
		providerKindMetadataKey: string(providerDescriptor(provider).Kind),
	})
	return nil
}

func (r *Runtime) authorizeProviderActivation(ctx context.Context, desc provider.ProviderDescriptor) error {
	if r == nil {
		return fmt.Errorf("runtime unavailable")
	}
	if r.AgentWorkspace().Registration != nil && r.AgentWorkspace().Registration.Policy != nil {
		if r.registration == nil || r.registration.Permissions == nil {
			return fmt.Errorf("provider %s activation requires approval but permission manager is missing", desc.ID)
		}
		metadata := map[string]string{
			"provider_id":           desc.ID,
			providerKindMetadataKey: string(desc.Kind),
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
			Manager: r.registration.Permissions,
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
		if r.registration == nil || r.registration.Permissions == nil {
			return fmt.Errorf("provider %s activation requires approval but permission manager is missing", desc.ID)
		}
		metadata := map[string]string{
			"provider_id":           desc.ID,
			providerKindMetadataKey: string(desc.Kind),
		}
		if desc.Security.Origin != "" {
			metadata["provider_origin"] = string(desc.Security.Origin)
		}
		return r.registration.Permissions.RequireApproval(ctx, r.registration.ID, permissions.PermissionDescriptor{
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
	if r.Tools != nil {
		r.Tools.RevokeProvider(providerID, reason)
	}
	record, ok := r.removeProviderRecord(providerID)
	if !ok {
		r.emitProviderLifecycleEvent(providerID, "", "provider_quarantined", reason, map[string]any{})
		return nil
	}
	err := record.provider.Close()
	r.emitProviderLifecycleEvent(providerID, "", "provider_quarantined", reason, map[string]any{
		providerKindMetadataKey: string(record.desc.Kind),
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
			r.emitProviderLifecycleEvent(record.desc.ID, sessionID, "session_revoked", reason, map[string]any{
				providerKindMetadataKey: string(record.desc.Kind),
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

func (r *Runtime) CaptureProviderSnapshots(ctx context.Context) ([]provider.ProviderSnapshot, []provider.ProviderSessionSnapshot, error) {
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

func providerDescriptor(runtimeProvider RuntimeProvider) provider.ProviderDescriptor {
	if described, ok := runtimeProvider.(DescribedRuntimeProvider); ok {
		return described.Descriptor()
	}
	return provider.ProviderDescriptor{}
}

func (r *Runtime) emitProviderLifecycleEvent(providerID, sessionID, event, reason string, metadata map[string]any) {
	if r == nil || r.AgentWorkspace().Telemetry == nil {
		return
	}
	if metadata == nil {
		metadata = make(map[string]any)
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
