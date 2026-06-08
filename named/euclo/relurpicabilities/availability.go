package relurpicabilities

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

type relurpicCapabilitySpec struct {
	Handler       handler.InvocableCapabilityHandler
	RequiredTools []string
}

type availabilityWrappedInvocableHandler struct {
	handler    handler.InvocableCapabilityHandler
	descriptor descriptor.CapabilityDescriptor
}

func (h availabilityWrappedInvocableHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.NormalizeCapabilityDescriptor(h.descriptor)
}

func (h availabilityWrappedInvocableHandler) Invoke(ctx context.Context, env ports.State, args map[string]interface{}) (*ports.ToolResult, error) {
	if h.handler == nil {
		return nil, fmt.Errorf("capability handler unavailable")
	}
	return h.handler.Invoke(ctx, env, args)
}

func (h availabilityWrappedInvocableHandler) Availability(ctx context.Context, env *contextdata.Envelope) descriptor.AvailabilitySpec {
	return h.descriptor.Availability
}

func (h availabilityWrappedInvocableHandler) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
	if aware, ok := h.handler.(registry.PermissionAware); ok {
		aware.SetPermissionManager(manager, agentID)
	}
}

func (h availabilityWrappedInvocableHandler) SetAgentSpec(spec *agentspec.AgentRuntimeSpec, agentID string) {
	if aware, ok := h.handler.(registry.AgentSpecAware); ok {
		aware.SetAgentSpec(spec, agentID)
	}
}

func (h availabilityWrappedInvocableHandler) SetSandboxScope(scope *permissions.FileScopePolicy) {
	if aware, ok := h.handler.(registry.SandboxScopeAware); ok {
		aware.SetSandboxScope(scope)
	}
}

func computeAvailability(reg *registry.CapabilityRegistry, requiredTools []string) descriptor.AvailabilitySpec {
	if len(requiredTools) == 0 {
		return descriptor.AvailabilitySpec{Available: true}
	}
	if reg == nil {
		return descriptor.AvailabilitySpec{Available: false, Reason: fmt.Sprintf("tool dependency missing: %s", requiredTools[0])}
	}
	for _, name := range requiredTools {
		toolName := strings.TrimSpace(name)
		if toolName == "" {
			continue
		}
		desc, ok := reg.GetCapability(toolName)
		if !ok {
			return descriptor.AvailabilitySpec{Available: false, Reason: fmt.Sprintf("tool dependency missing: %s", toolName)}
		}
		if reg.EffectiveExposure(desc) != agentspec.CapabilityExposureCallable {
			return descriptor.AvailabilitySpec{Available: false, Reason: fmt.Sprintf("tool dependency missing: %s (not callable)", toolName)}
		}
	}
	return descriptor.AvailabilitySpec{Available: true}
}

func registerRelurpicCapability(reg *registry.CapabilityRegistry, spec relurpicCapabilitySpec) error {
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
