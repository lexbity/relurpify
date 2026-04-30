package relurpicabilities

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

type relurpicCapabilitySpec struct {
	Handler       core.InvocableCapabilityHandler
	RequiredTools []string
}

type availabilityWrappedInvocableHandler struct {
	handler    core.InvocableCapabilityHandler
	descriptor core.CapabilityDescriptor
}

func (h availabilityWrappedInvocableHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.NormalizeCapabilityDescriptor(h.descriptor)
}

func (h availabilityWrappedInvocableHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	if h.handler == nil {
		return nil, fmt.Errorf("capability handler unavailable")
	}
	return h.handler.Invoke(ctx, env, args)
}

func (h availabilityWrappedInvocableHandler) Availability(ctx context.Context, env *contextdata.Envelope) core.AvailabilitySpec {
	return h.descriptor.Availability
}

func (h availabilityWrappedInvocableHandler) SetPermissionManager(manager *capability.PermissionManager, agentID string) {
	if aware, ok := h.handler.(capability.PermissionAware); ok {
		aware.SetPermissionManager(manager, agentID)
	}
}

func (h availabilityWrappedInvocableHandler) SetAgentSpec(spec *capability.AgentRuntimeSpec, agentID string) {
	if aware, ok := h.handler.(capability.AgentSpecAware); ok {
		aware.SetAgentSpec(spec, agentID)
	}
}

func (h availabilityWrappedInvocableHandler) SetSandboxScope(scope *sandbox.FileScopePolicy) {
	if aware, ok := h.handler.(capability.SandboxScopeAware); ok {
		aware.SetSandboxScope(scope)
	}
}

func computeAvailability(reg *capability.Registry, requiredTools []string) core.AvailabilitySpec {
	if len(requiredTools) == 0 {
		return core.AvailabilitySpec{Available: true}
	}
	if reg == nil {
		return core.AvailabilitySpec{Available: false, Reason: fmt.Sprintf("tool dependency missing: %s", requiredTools[0])}
	}
	for _, name := range requiredTools {
		toolName := strings.TrimSpace(name)
		if toolName == "" {
			continue
		}
		desc, ok := reg.GetCapability(toolName)
		if !ok {
			return core.AvailabilitySpec{Available: false, Reason: fmt.Sprintf("tool dependency missing: %s", toolName)}
		}
		if reg.EffectiveExposure(desc) != core.CapabilityExposureCallable {
			return core.AvailabilitySpec{Available: false, Reason: fmt.Sprintf("tool dependency missing: %s (not callable)", toolName)}
		}
	}
	return core.AvailabilitySpec{Available: true}
}

func registerRelurpicCapability(reg *capability.Registry, spec relurpicCapabilitySpec) error {
	if reg == nil {
		return fmt.Errorf("capability registry is nil")
	}
	if spec.Handler == nil {
		return fmt.Errorf("relurpic capability handler is nil")
	}
	desc := spec.Handler.Descriptor(context.Background(), nil)
	desc.Availability = computeAvailability(reg, spec.RequiredTools)
	wrapped := availabilityWrappedInvocableHandler{
		handler:    spec.Handler,
		descriptor: desc,
	}
	return reg.RegisterInvocableCapability(wrapped)
}
