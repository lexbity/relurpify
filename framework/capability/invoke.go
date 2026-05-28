package capability

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// InvokeCapability executes an invocable capability by capability ID or public name.
func (r *CapabilityRegistry) InvokeCapability(ctx context.Context, state *contextdata.Envelope, idOrName string, args map[string]interface{}) (*contracts.ToolResult, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	if r.delegate != nil {
		if r.toolIDAllowlist != nil {
			desc, ok := r.delegate.GetCapability(idOrName)
			if !ok {
				return nil, fmt.Errorf("capability %s not found", idOrName)
			}
			if !r.isAllowlisted(desc.ID) {
				return nil, fmt.Errorf("capability %s is not permitted in this context", idOrName)
			}
		}
		return r.delegate.InvokeCapability(ctx, state, idOrName, args)
	}
	if sessionHandler, ok := LookupSessionCapability(state, idOrName); ok {
		return sessionHandler.Invoke(ctx, state, args)
	}
	entry, err := r.prepareCapabilityInvocation(ctx, state, idOrName, args)
	if err != nil {
		return nil, err
	}
	invocable, ok := entry.handler.(core.InvocableCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability %s is not invocable", entry.descriptor.ID)
	}
	// Redact sensitive argument values before any logging or telemetry.
	if entry.legacyTool != nil {
		_ = contracts.RedactArgs(args, entry.legacyTool.Parameters())
	}
	startTime := time.Now()
	var result *contracts.ToolResult
	result, err = recoverToolPanic(func() (*contracts.ToolResult, error) {
		return invocable.Invoke(ctx, state, args)
	})
	callDuration := time.Since(startTime)
	if postErr := r.runPostchecks(entry.descriptor, result); postErr != nil {
		if result == nil {
			result = &contracts.ToolResult{Success: false, Error: postErr.Error()}
		} else {
			result.Success = false
			result.Error = postErr.Error()
		}
		if err == nil {
			err = postErr
		}
	}
	if r.metrics != nil {
		r.metrics.RecordCall(
			err == nil && result != nil && result.Success,
			callDuration,
			result != nil && result.Truncated,
		)
	}
	return result, err
}

// InvokeCapabilityBackground submits a long-running capability invocation to
// the framework job queue and returns a handle the caller can use to track the
// job. The capability handler must implement core.BackgroundCapabilityHandler;
// if it does not, this method returns an error — callers that want synchronous
// execution should use InvokeCapability instead.
//
// The handler is responsible for building the jobs.JobSpec and calling
// env.JobSubmitter.Submit from inside InvokeBackground. The registry provides
// only lookup, admission, and postchecks — it does not own the JobSpec shape.
func (r *CapabilityRegistry) InvokeCapabilityBackground(ctx context.Context, state *contextdata.Envelope, idOrName string, args map[string]interface{}) (*core.BackgroundInvocationHandle, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	if r.delegate != nil {
		if r.toolIDAllowlist != nil {
			desc, ok := r.delegate.GetCapability(idOrName)
			if !ok {
				return nil, fmt.Errorf("capability %s not found", idOrName)
			}
			if !r.isAllowlisted(desc.ID) {
				return nil, fmt.Errorf("capability %s is not permitted in this context", idOrName)
			}
		}
		return r.delegate.InvokeCapabilityBackground(ctx, state, idOrName, args)
	}
	entry, err := r.prepareCapabilityInvocation(ctx, state, idOrName, args)
	if err != nil {
		return nil, err
	}
	bgHandler, ok := entry.handler.(core.BackgroundCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability %s does not support background invocation", entry.descriptor.ID)
	}
	return bgHandler.InvokeBackground(ctx, state, args)
}

// recoverToolPanic wraps a tool invocation and converts any panic into a
// ToolResult error. The panic value and stack trace are logged but NOT
// returned to the caller in the ToolResult.Error field to avoid leaking
// internal implementation details to the LLM.
func recoverToolPanic(fn func() (*contracts.ToolResult, error)) (res *contracts.ToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("tool panic recovered: %v\n%s", r, debug.Stack())
			res = &contracts.ToolResult{Success: false, Error: "tool panicked — see server logs"}
			err = nil
		}
	}()
	return fn()
}

func (r *CapabilityRegistry) prepareCapabilityInvocation(ctx context.Context, state *contextdata.Envelope, idOrName string, args map[string]interface{}) (*capabilityEntry, error) {
	entry, err := r.capabilityEntry(idOrName)
	if err != nil {
		return nil, err
	}
	if aware, ok := entry.handler.(core.AvailabilityAwareCapabilityHandler); ok {
		if availability := aware.Availability(ctx, state); !availability.Available {
			reason := strings.TrimSpace(availability.Reason)
			if reason == "" {
				reason = "capability unavailable"
			}
			return nil, fmt.Errorf("capability %s blocked: %s", entry.descriptor.ID, reason)
		}
	}
	if err := r.enforceCapabilityPolicy(ctx, entry); err != nil {
		return nil, err
	}
	if err := r.runPrechecks(entry.descriptor, args); err != nil {
		var doomErr *DoomLoopError
		if errors.As(err, &doomErr) {
			if r.metrics != nil {
				r.metrics.RecordDoomLoop()
			}
			proceed, guideErr := r.handleDoomLoopGuidance(ctx, *doomErr)
			if guideErr != nil {
				return nil, fmt.Errorf("capability %s blocked: %w", entry.descriptor.ID, guideErr)
			}
			if proceed {
				return entry, nil
			}
		}
		return nil, fmt.Errorf("capability %s blocked: %w", entry.descriptor.ID, err)
	}
	return entry, nil
}

func (r *CapabilityRegistry) enforceCapabilityPolicy(ctx context.Context, entry *capabilityEntry) error {
	desc := entry.descriptor
	r.mu.RLock()
	policyEngine := r.policyEngine
	agentID := r.registeredAgentID
	manager := r.permissionManager
	r.mu.RUnlock()
	_, err := authorization.EnforcePolicyRequest(ctx, policyEngine, core.PolicyRequest{
		Target:         core.PolicyTargetCapability,
		Actor:          identity.EventActor{Kind: "agent", ID: agentID},
		CapabilityID:   desc.ID,
		CapabilityName: desc.Name,
		CapabilityKind: desc.Kind,
		RuntimeFamily:  desc.RuntimeFamily,
		ProviderKind:   providerKindForDescriptor(desc),
		TrustClass:     desc.TrustClass,
		RiskClasses:    desc.RiskClasses,
		EffectClasses:  desc.EffectClasses,
	}, authorization.ApprovalRequest{
		AgentID: agentID,
		Manager: manager,
		Permission: contracts.PermissionDescriptor{
			Type:         contracts.PermissionTypeCapability,
			Action:       "capability:" + desc.Name,
			Resource:     desc.ID,
			RequiresHITL: true,
		},
		Justification:      "capability policy approval",
		Scope:              authorization.GrantScopeSession,
		Risk:               authorization.RiskLevelMedium,
		MissingManagerErr:  "approval required but permission manager unavailable",
		DenyReasonFallback: "denied by policy",
	})
	if err != nil {
		return fmt.Errorf("capability %s blocked: %w", desc.ID, err)
	}
	return nil
}

func (r *CapabilityRegistry) capabilityEntry(idOrName string) (*capabilityEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	idOrName = r.normalizeToolNameLocked(idOrName)
	if entry, ok := r.entries[idOrName]; ok {
		return entry, nil
	}
	if ids := r.capabilityNameIndex[normalizeComparable(idOrName)]; len(ids) > 0 {
		for _, id := range ids {
			if entry, ok := r.entries[id]; ok {
				return entry, nil
			}
		}
	}
	return nil, fmt.Errorf("capability %s not found", idOrName)
}

// CapabilityAvailable reports whether a registered capability is currently available for invocation.
func (r *CapabilityRegistry) CapabilityAvailable(ctx context.Context, state *contextdata.Envelope, idOrName string) bool {
	if r == nil {
		return false
	}
	entry, err := r.capabilityEntry(idOrName)
	if err != nil || entry == nil {
		return false
	}
	aware, ok := entry.handler.(core.AvailabilityAwareCapabilityHandler)
	if !ok {
		return true
	}
	return aware.Availability(ctx, state).Available
}

// InvocableCapabilities returns non-hidden capability descriptors with an invocable runtime handler.
func (r *CapabilityRegistry) InvocableCapabilities() []core.CapabilityDescriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]core.CapabilityDescriptor, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry == nil || entry.handler == nil {
			continue
		}
		if _, ok := entry.handler.(core.InvocableCapabilityHandler); !ok {
			continue
		}
		if r.effectiveExposureLocked(entry.descriptor) == core.CapabilityExposureHidden {
			continue
		}
		res = append(res, entry.descriptor)
	}
	return res
}

func providerKindForDescriptor(desc core.CapabilityDescriptor) core.ProviderKind {
	switch desc.Source.Scope {
	case agentspec.CapabilityScopeProvider, agentspec.CapabilityScopeRemote:
		return core.ProviderKindNodeDevice
	default:
		return core.ProviderKindBuiltin
	}
}
