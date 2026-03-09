package capability

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// wrapTool decorates a tool with the instrumentation wrapper.
func (r *CapabilityRegistry) wrapTool(tool Tool) Tool {
	if tool == nil {
		return nil
	}
	if existing, ok := tool.(*instrumentedTool); ok {
		existing.manager = r.permissionManager
		existing.agentID = r.registeredAgentID
		existing.telemetry = r.telemetry
		existing.policy = r.toolPolicies[existing.Tool.Name()]
		existing.capabilityPolicies = append([]core.CapabilityPolicy{}, r.capabilityPolicies...)
		existing.hasPolicy = r.agentSpec != nil
		existing.globalPolicies = r.globalPolicies
		existing.safety = r.safety
		return existing
	}
	return &instrumentedTool{
		Tool:               tool,
		manager:            r.permissionManager,
		agentID:            r.registeredAgentID,
		telemetry:          r.telemetry,
		policy:             r.toolPolicies[tool.Name()],
		capabilityPolicies: append([]core.CapabilityPolicy{}, r.capabilityPolicies...),
		hasPolicy:          r.agentSpec != nil,
		globalPolicies:     r.globalPolicies,
		safety:             r.safety,
	}
}

func (r *CapabilityRegistry) wrapCapabilityHandler(handler core.CapabilityHandler) core.CapabilityHandler {
	if handler == nil {
		return nil
	}
	if aware, ok := handler.(AgentSpecAware); ok && r.agentSpec != nil {
		aware.SetAgentSpec(r.agentSpec, r.registeredAgentID)
	}
	if existing, ok := handler.(instrumentCapabilityHandler); ok {
		existing.registeredAgentID = r.registeredAgentID
		existing.manager = r.permissionManager
		existing.toolPolicies = r.toolPolicies
		existing.capabilityPolicies = append([]core.CapabilityPolicy{}, r.capabilityPolicies...)
		existing.globalPolicies = cloneGlobalPolicies(r.globalPolicies)
		existing.telemetry = r.telemetry
		existing.safety = r.safety
		return existing
	}
	return instrumentCapabilityHandler{
		handler:            handler,
		registeredAgentID:  r.registeredAgentID,
		manager:            r.permissionManager,
		toolPolicies:       r.toolPolicies,
		capabilityPolicies: append([]core.CapabilityPolicy{}, r.capabilityPolicies...),
		globalPolicies:     cloneGlobalPolicies(r.globalPolicies),
		telemetry:          r.telemetry,
		safety:             r.safety,
	}
}

type instrumentedTool struct {
	Tool
	manager            *PermissionManager
	agentID            string
	telemetry          Telemetry
	policy             ToolPolicy
	capabilityPolicies []core.CapabilityPolicy
	hasPolicy          bool
	globalPolicies     map[string]AgentPermissionLevel
	safety             *runtimeSafetyController
}

type capabilityToolShim struct {
	registry   *CapabilityRegistry
	descriptor core.CapabilityDescriptor
}

func (t capabilityToolShim) Name() string {
	if name := strings.TrimSpace(t.descriptor.Name); name != "" {
		return name
	}
	return strings.TrimSpace(t.descriptor.ID)
}

func (t capabilityToolShim) Description() string { return t.descriptor.Description }
func (t capabilityToolShim) Category() string    { return t.descriptor.Category }
func (t capabilityToolShim) Tags() []string      { return append([]string{}, t.descriptor.Tags...) }

func (t capabilityToolShim) Parameters() []core.ToolParameter {
	return toolParametersFromSchema(t.descriptor.InputSchema)
}

func (t capabilityToolShim) Execute(ctx context.Context, state *Context, args map[string]interface{}) (*core.ToolResult, error) {
	if t.registry == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	return t.registry.InvokeCapability(ctx, state, t.Name(), args)
}

func (t capabilityToolShim) IsAvailable(ctx context.Context, state *Context) bool {
	if t.registry == nil {
		return false
	}
	return t.registry.CapabilityAvailable(ctx, state, t.Name())
}

func (t capabilityToolShim) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{}}
}

type instrumentCapabilityHandler struct {
	handler            core.CapabilityHandler
	registeredAgentID  string
	manager            *PermissionManager
	toolPolicies       map[string]ToolPolicy
	capabilityPolicies []core.CapabilityPolicy
	globalPolicies     map[string]AgentPermissionLevel
	telemetry          Telemetry
	safety             *runtimeSafetyController
}

func toolParametersFromSchema(schema *core.Schema) []core.ToolParameter {
	if schema == nil || schema.Type != "object" || len(schema.Properties) == 0 {
		return nil
	}
	required := make(map[string]struct{}, len(schema.Required))
	for _, name := range schema.Required {
		required[strings.TrimSpace(name)] = struct{}{}
	}
	out := make([]core.ToolParameter, 0, len(schema.Properties))
	for name, prop := range schema.Properties {
		param := core.ToolParameter{
			Name:        name,
			Description: prop.Description,
			Default:     prop.Default,
		}
		param.Type = strings.TrimSpace(prop.Type)
		if param.Type == "" {
			param.Type = "string"
		}
		_, param.Required = required[name]
		out = append(out, param)
	}
	return out
}

func (h instrumentCapabilityHandler) Descriptor(ctx context.Context, state *Context) core.CapabilityDescriptor {
	if h.handler == nil {
		return core.CapabilityDescriptor{}
	}
	return core.NormalizeCapabilityDescriptor(h.handler.Descriptor(ctx, state))
}

func (h instrumentCapabilityHandler) Availability(ctx context.Context, state *Context) core.AvailabilitySpec {
	if aware, ok := h.handler.(core.AvailabilityAwareCapabilityHandler); ok {
		return aware.Availability(ctx, state)
	}
	return core.AvailabilitySpec{Available: true}
}

func (h instrumentCapabilityHandler) Invoke(ctx context.Context, state *Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	invocable, ok := h.handler.(core.InvocableCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability handler unavailable")
	}
	desc := h.Descriptor(ctx, state)
	approvalBinding := core.ApprovalBindingFromCapability(desc, state, args)
	approvalMetadata := map[string]string(nil)
	if approvalBinding != nil {
		approvalMetadata = approvalBinding.PermissionMetadata()
	}
	if h.safety != nil {
		if err := h.safety.CheckBeforeExecution(desc); err != nil {
			return nil, err
		}
	}
	if err := core.ValidateValueAgainstSchema(args, desc.InputSchema); err != nil {
		return nil, fmt.Errorf("capability %s blocked: input schema invalid: %w", desc.ID, err)
	}
	if desc.Kind == core.CapabilityKindTool && desc.RuntimeFamily == core.CapabilityRuntimeFamilyLocalTool && len(h.toolPolicies) > 0 {
		if policy, ok := h.toolPolicies[desc.Name]; ok {
			switch policy.Execute {
			case AgentPermissionDeny:
				return nil, fmt.Errorf("capability %s blocked: execution denied by tool policy", desc.ID)
			case AgentPermissionAsk:
				return nil, h.requireApproval(ctx, desc, approvalMetadata, "tool execution approval")
			}
		}
	}
	if len(h.capabilityPolicies) > 0 {
		effective := effectiveCapabilityPolicyForDescriptor(desc, h.capabilityPolicies)
		switch effective {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("capability %s blocked: execution denied by capability selector policy", desc.ID)
		case AgentPermissionAsk:
			return nil, h.requireApproval(ctx, desc, approvalMetadata, "capability selector policy approval")
		}
	}
	if len(h.globalPolicies) > 0 {
		effective := effectiveClassPolicyForDescriptor(desc, h.globalPolicies)
		switch effective {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("capability %s blocked: execution denied by capability policy", desc.ID)
		case AgentPermissionAsk:
			return nil, h.requireApproval(ctx, desc, approvalMetadata, "capability class policy approval")
		}
	}
	emitCapabilityInvocationTelemetry(h.telemetry, desc, h.registeredAgentID, args)
	result, err := invocable.Invoke(ctx, state, args)
	if err == nil && result != nil && desc.OutputSchema != nil {
		if schemaErr := core.ValidateValueAgainstSchema(result.Data, desc.OutputSchema); schemaErr != nil {
			err = fmt.Errorf("capability %s blocked: output schema invalid: %w", desc.ID, schemaErr)
			result.Success = false
			result.Error = err.Error()
		}
	}
	if err == nil && h.safety != nil {
		if safetyErr := h.safety.RecordResult(desc, result); safetyErr != nil {
			err = safetyErr
			if result == nil {
				result = &core.ToolResult{Success: false, Error: safetyErr.Error()}
			} else {
				result.Success = false
				result.Error = safetyErr.Error()
			}
		}
	}
	if result != nil {
		if result.Metadata == nil {
			result.Metadata = map[string]interface{}{}
		}
		result.Metadata["capability_descriptor"] = desc
		if approvalBinding != nil {
			result.Metadata["approval_binding"] = approvalBinding
		}
		result.Metadata["insertion_decision"] = core.DefaultInsertionDecision(desc, core.ContentDispositionRaw)
	}
	emitCapabilityResultTelemetry(h.telemetry, desc, h.registeredAgentID, result, err)
	return result, err
}

func (h instrumentCapabilityHandler) RenderPrompt(ctx context.Context, state *Context, args map[string]interface{}) (*core.PromptRenderResult, error) {
	promptHandler, ok := h.handler.(core.PromptCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("prompt handler unavailable")
	}
	desc := h.Descriptor(ctx, state)
	if h.safety != nil {
		if err := h.safety.CheckBeforeExecution(desc); err != nil {
			return nil, err
		}
	}
	if err := core.ValidateValueAgainstSchema(args, desc.InputSchema); err != nil {
		return nil, fmt.Errorf("capability %s blocked: input schema invalid: %w", desc.ID, err)
	}
	if len(h.capabilityPolicies) > 0 {
		effective := effectiveCapabilityPolicyForDescriptor(desc, h.capabilityPolicies)
		switch effective {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("capability %s blocked: execution denied by capability selector policy", desc.ID)
		case AgentPermissionAsk:
			return nil, h.requireApproval(ctx, desc, nil, "capability selector policy approval")
		}
	}
	if len(h.globalPolicies) > 0 {
		effective := effectiveClassPolicyForDescriptor(desc, h.globalPolicies)
		switch effective {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("capability %s blocked: execution denied by capability policy", desc.ID)
		case AgentPermissionAsk:
			return nil, h.requireApproval(ctx, desc, nil, "capability class policy approval")
		}
	}
	emitCapabilityInvocationTelemetry(h.telemetry, desc, h.registeredAgentID, args)
	result, err := promptHandler.RenderPrompt(ctx, state, args)
	emitPromptCapabilityResultTelemetry(h.telemetry, desc, h.registeredAgentID, result, err)
	return result, err
}

func (h instrumentCapabilityHandler) ReadResource(ctx context.Context, state *Context) (*core.ResourceReadResult, error) {
	resourceHandler, ok := h.handler.(core.ResourceCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("resource handler unavailable")
	}
	desc := h.Descriptor(ctx, state)
	if h.safety != nil {
		if err := h.safety.CheckBeforeExecution(desc); err != nil {
			return nil, err
		}
	}
	if len(h.capabilityPolicies) > 0 {
		effective := effectiveCapabilityPolicyForDescriptor(desc, h.capabilityPolicies)
		switch effective {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("capability %s blocked: execution denied by capability selector policy", desc.ID)
		case AgentPermissionAsk:
			return nil, h.requireApproval(ctx, desc, nil, "capability selector policy approval")
		}
	}
	if len(h.globalPolicies) > 0 {
		effective := effectiveClassPolicyForDescriptor(desc, h.globalPolicies)
		switch effective {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("capability %s blocked: execution denied by capability policy", desc.ID)
		case AgentPermissionAsk:
			return nil, h.requireApproval(ctx, desc, nil, "capability class policy approval")
		}
	}
	emitCapabilityInvocationTelemetry(h.telemetry, desc, h.registeredAgentID, nil)
	result, err := resourceHandler.ReadResource(ctx, state)
	emitResourceCapabilityResultTelemetry(h.telemetry, desc, h.registeredAgentID, result, err)
	return result, err
}

func (h instrumentCapabilityHandler) requireApproval(ctx context.Context, desc core.CapabilityDescriptor, metadata map[string]string, reason string) error {
	if h.manager == nil {
		return fmt.Errorf("capability %s blocked: approval required but permission manager missing", desc.ID)
	}
	return h.manager.RequireApproval(ctx, h.registeredAgentID, PermissionDescriptor{
		Type:         PermissionTypeHITL,
		Action:       fmt.Sprintf("capability:%s", desc.ID),
		Resource:     h.registeredAgentID,
		Metadata:     metadata,
		RequiresHITL: true,
	}, reason, GrantScopeOneTime, RiskLevelMedium, 0)
}

// Execute authorizes the wrapped tool before delegating to the original implementation.
func (t *instrumentedTool) Execute(ctx context.Context, state *Context, args map[string]interface{}) (*ToolResult, error) {
	desc := core.ToolDescriptor(ctx, state, t.Tool)
	approvalBinding := core.ApprovalBindingFromCapability(desc, state, args)
	approvalMetadata := map[string]string(nil)
	if approvalBinding != nil {
		approvalMetadata = approvalBinding.PermissionMetadata()
	}
	if t.safety != nil {
		if err := t.safety.CheckBeforeExecution(desc); err != nil {
			return nil, err
		}
	}
	if err := core.ValidateValueAgainstSchema(args, desc.InputSchema); err != nil {
		return nil, fmt.Errorf("tool %s blocked: input schema invalid: %w", t.Tool.Name(), err)
	}
	if t.hasPolicy {
		switch t.policy.Execute {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("tool %s blocked: execution denied by policy", t.Tool.Name())
		case AgentPermissionAsk:
			if t.manager == nil {
				return nil, fmt.Errorf("tool %s blocked: approval required but permission manager missing", t.Tool.Name())
			}
			if err := t.manager.RequireApproval(ctx, t.agentID, PermissionDescriptor{
				Type:         PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", t.Tool.Name()),
				Resource:     t.agentID,
				Metadata:     approvalMetadata,
				RequiresHITL: true,
			}, "tool execution approval", GrantScopeOneTime, RiskLevelMedium, 0); err != nil {
				return nil, err
			}
		}
	}
	if len(t.capabilityPolicies) > 0 {
		effective := effectiveCapabilityPolicy(t.Tool, t.capabilityPolicies)
		switch effective {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("tool %s blocked: execution denied by capability selector policy", t.Tool.Name())
		case AgentPermissionAsk:
			if t.manager == nil {
				return nil, fmt.Errorf("tool %s blocked: approval required but permission manager missing", t.Tool.Name())
			}
			if err := t.manager.RequireApproval(ctx, t.agentID, PermissionDescriptor{
				Type:         PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", t.Tool.Name()),
				Resource:     t.agentID,
				Metadata:     approvalMetadata,
				RequiresHITL: true,
			}, "capability selector policy approval", GrantScopeOneTime, RiskLevelMedium, 0); err != nil {
				return nil, err
			}
		}
	}
	if len(t.globalPolicies) > 0 {
		effective := effectiveClassPolicy(t.Tool, t.globalPolicies)
		switch effective {
		case AgentPermissionDeny:
			return nil, fmt.Errorf("tool %s blocked: execution denied by capability policy", t.Tool.Name())
		case AgentPermissionAsk:
			if t.manager == nil {
				return nil, fmt.Errorf("tool %s blocked: approval required but permission manager missing", t.Tool.Name())
			}
			if err := t.manager.RequireApproval(ctx, t.agentID, PermissionDescriptor{
				Type:         PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", t.Tool.Name()),
				Resource:     t.agentID,
				Metadata:     approvalMetadata,
				RequiresHITL: true,
			}, "capability class policy approval", GrantScopeOneTime, RiskLevelMedium, 0); err != nil {
				return nil, err
			}
		}
	}
	if t.manager != nil {
		if err := t.manager.AuthorizeTool(ctx, t.agentID, t.Tool, args); err != nil {
			var denied *PermissionDeniedError
			if errors.As(err, &denied) {
				return nil, fmt.Errorf("tool %s blocked: %w", t.Tool.Name(), err)
			}
			return nil, err
		}
	}
	if t.telemetry != nil {
		t.telemetry.Emit(Event{
			Type:      EventToolCall,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("tool %s invoked", t.Tool.Name()),
			Metadata: redactTelemetryMetadata(t.safety, map[string]interface{}{
				"tool":     t.Tool.Name(),
				"agent_id": t.agentID,
				"args":     summarizeArgs(args),
			}),
		})
	}
	result, err := t.Tool.Execute(ctx, state, args)
	if err == nil && result != nil && desc.OutputSchema != nil {
		if schemaErr := core.ValidateValueAgainstSchema(result.Data, desc.OutputSchema); schemaErr != nil {
			err = fmt.Errorf("tool %s blocked: output schema invalid: %w", t.Tool.Name(), schemaErr)
			result.Success = false
			result.Error = err.Error()
		}
	}
	if err == nil && t.safety != nil {
		if safetyErr := t.safety.RecordResult(desc, result); safetyErr != nil {
			err = safetyErr
			if result == nil {
				result = &ToolResult{Success: false, Error: safetyErr.Error()}
			} else {
				result.Success = false
				result.Error = safetyErr.Error()
			}
		}
	}
	if result != nil {
		if result.Metadata == nil {
			result.Metadata = map[string]interface{}{}
		}
		result.Metadata["capability_descriptor"] = desc
		if approvalBinding != nil {
			result.Metadata["approval_binding"] = approvalBinding
		}
		result.Metadata["insertion_decision"] = core.DefaultInsertionDecision(desc, core.ContentDispositionRaw)
	}
	if err != nil {
		var denied *PermissionDeniedError
		if errors.As(err, &denied) {
			err = fmt.Errorf("tool %s blocked: %w", t.Tool.Name(), err)
		}
	}
	if t.telemetry != nil {
		metadata := map[string]interface{}{
			"tool":     t.Tool.Name(),
			"agent_id": t.agentID,
		}
		if result != nil {
			metadata["success"] = result.Success
			if result.Error != "" {
				metadata["tool_error"] = result.Error
			}
		}
		if err != nil {
			metadata["error"] = err.Error()
		}
		t.telemetry.Emit(Event{
			Type:      EventToolResult,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("tool %s completed", t.Tool.Name()),
			Metadata:  redactTelemetryMetadata(t.safety, metadata),
		})
	}
	return result, err
}

func redactTelemetryMetadata(controller *runtimeSafetyController, metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	if controller != nil {
		spec := controller.SnapshotSpec()
		if spec != nil && !spec.RedactionEnabled() {
			return metadata
		}
	}
	return core.RedactMetadataMap(metadata)
}

func emitCapabilitySecurityEvent(telemetry Telemetry, event string, desc core.CapabilityDescriptor, exposure core.CapabilityExposure, reason string) {
	if telemetry == nil || desc.ID == "" {
		return
	}
	metadata := map[string]interface{}{
		"security_event": event,
		"capability_id":  desc.ID,
		"capability":     desc.Name,
		"kind":           string(desc.Kind),
		"scope":          string(desc.Source.Scope),
		"trust_class":    string(desc.TrustClass),
		"exposure":       string(exposure),
	}
	if desc.Source.ProviderID != "" {
		metadata["provider_id"] = desc.Source.ProviderID
	}
	if desc.Source.SessionID != "" {
		metadata["session_id"] = desc.Source.SessionID
	}
	if reason != "" {
		metadata["reason"] = reason
	}
	telemetry.Emit(Event{
		Type:      core.EventStateChange,
		Timestamp: time.Now().UTC(),
		Message:   strings.ReplaceAll(event, "_", " "),
		Metadata:  core.RedactMetadataMap(metadata),
	})
}

func unwrapTool(tool Tool) Tool {
	if wrapped, ok := tool.(*instrumentedTool); ok {
		return wrapped.Tool
	}
	return tool
}

func unwrapCapabilityHandler(handler core.CapabilityHandler) core.CapabilityHandler {
	if wrapped, ok := handler.(instrumentCapabilityHandler); ok {
		return wrapped.handler
	}
	return handler
}

type legacyToolHandler struct {
	tool Tool
}

func (h legacyToolHandler) Descriptor(ctx context.Context, state *Context) core.CapabilityDescriptor {
	return core.ToolDescriptor(ctx, state, unwrapTool(h.tool))
}

func (h legacyToolHandler) Invoke(ctx context.Context, state *Context, args map[string]interface{}) (*ToolResult, error) {
	if h.tool == nil {
		return nil, fmt.Errorf("tool handler unavailable")
	}
	return h.tool.Execute(ctx, state, args)
}

func (h legacyToolHandler) Availability(ctx context.Context, state *Context) core.AvailabilitySpec {
	if h.tool == nil {
		return core.AvailabilitySpec{Available: false, Reason: "tool handler unavailable"}
	}
	if !h.tool.IsAvailable(ctx, state) {
		return core.AvailabilitySpec{Available: false, Reason: "tool unavailable"}
	}
	return core.AvailabilitySpec{Available: true}
}

func emitCapabilityInvocationTelemetry(telemetry Telemetry, desc core.CapabilityDescriptor, agentID string, args map[string]interface{}) {
	if telemetry == nil {
		return
	}
	telemetry.Emit(Event{
		Type:      EventCapabilityCall,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("capability %s invoked", desc.Name),
		Metadata: redactTelemetryMetadata(nil, map[string]interface{}{
			"capability_id":  desc.ID,
			"capability":     desc.Name,
			"kind":           string(desc.Kind),
			"runtime_family": string(desc.RuntimeFamily),
			"agent_id":       agentID,
			"args":           summarizeArgs(args),
		}),
	})
}

func emitCapabilityResultTelemetry(telemetry Telemetry, desc core.CapabilityDescriptor, agentID string, result *core.CapabilityExecutionResult, err error) {
	if telemetry == nil {
		return
	}
	metadata := map[string]interface{}{
		"capability_id":  desc.ID,
		"capability":     desc.Name,
		"kind":           string(desc.Kind),
		"runtime_family": string(desc.RuntimeFamily),
		"agent_id":       agentID,
	}
	if result != nil {
		metadata["success"] = result.Success
		if result.Error != "" {
			metadata["capability_error"] = result.Error
		}
	}
	if err != nil {
		metadata["error"] = err.Error()
	}
	telemetry.Emit(Event{
		Type:      EventCapabilityResult,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("capability %s completed", desc.Name),
		Metadata:  redactTelemetryMetadata(nil, metadata),
	})
}

func emitPromptCapabilityResultTelemetry(telemetry Telemetry, desc core.CapabilityDescriptor, agentID string, result *core.PromptRenderResult, err error) {
	if telemetry == nil {
		return
	}
	metadata := map[string]interface{}{
		"capability_id":  desc.ID,
		"capability":     desc.Name,
		"kind":           string(desc.Kind),
		"runtime_family": string(desc.RuntimeFamily),
		"agent_id":       agentID,
	}
	if result != nil {
		metadata["message_count"] = len(result.Messages)
	}
	if err != nil {
		metadata["error"] = err.Error()
	}
	telemetry.Emit(Event{
		Type:      EventCapabilityResult,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("capability %s completed", desc.Name),
		Metadata:  redactTelemetryMetadata(nil, metadata),
	})
}

func emitResourceCapabilityResultTelemetry(telemetry Telemetry, desc core.CapabilityDescriptor, agentID string, result *core.ResourceReadResult, err error) {
	if telemetry == nil {
		return
	}
	metadata := map[string]interface{}{
		"capability_id":  desc.ID,
		"capability":     desc.Name,
		"kind":           string(desc.Kind),
		"runtime_family": string(desc.RuntimeFamily),
		"agent_id":       agentID,
	}
	if result != nil {
		metadata["content_count"] = len(result.Contents)
	}
	if err != nil {
		metadata["error"] = err.Error()
	}
	telemetry.Emit(Event{
		Type:      EventCapabilityResult,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("capability %s completed", desc.Name),
		Metadata:  redactTelemetryMetadata(nil, metadata),
	})
}

func summarizeArgs(args map[string]interface{}) interface{} {
	if len(args) == 0 {
		return nil
	}
	return core.RedactMetadataMap(args)
}
