package capability

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// wrapTool decorates a tool with the instrumentation wrapper.
func (r *CapabilityRegistry) wrapTool(tool contracts.Tool) contracts.Tool {
	if tool == nil {
		return nil
	}
	if existing, ok := tool.(*instrumentedTool); ok {
		existing.registry = r
		return existing
	}
	return &instrumentedTool{
		Tool:     tool,
		registry: r,
	}
}

func (r *CapabilityRegistry) wrapCapabilityHandler(handler core.CapabilityHandler) core.CapabilityHandler {
	if handler == nil {
		return nil
	}
	desc := core.NormalizeCapabilityDescriptor(handler.Descriptor(context.Background(), nil))
	return r.wrapCapabilityHandlerPrepared(handler, desc, buildDescriptorProfile(desc))
}

func (r *CapabilityRegistry) wrapCapabilityHandlerPrepared(handler core.CapabilityHandler, desc core.CapabilityDescriptor, profile descriptorProfile) core.CapabilityHandler {
	if handler == nil {
		return nil
	}
	if aware, ok := handler.(PermissionAware); ok && r.permissionManager != nil {
		aware.SetPermissionManager(r.permissionManager, r.registeredAgentID)
	}
	if aware, ok := handler.(AgentSpecAware); ok && r.agentSpec != nil {
		aware.SetAgentSpec(r.agentSpec, r.registeredAgentID)
	}
	if aware, ok := handler.(SandboxScopeAware); ok && r.sandboxScope != nil {
		aware.SetSandboxScope(r.sandboxScope)
	}
	if existing, ok := handler.(instrumentCapabilityHandler); ok {
		existing.registry = r
		existing.descriptor = desc
		existing.profile = profile
		return existing
	}
	return instrumentCapabilityHandler{
		handler:    handler,
		registry:   r,
		descriptor: desc,
		profile:    profile,
	}
}

type instrumentedTool struct {
	contracts.Tool
	registry *CapabilityRegistry
}

type instrumentCapabilityHandler struct {
	handler    core.CapabilityHandler
	registry   *CapabilityRegistry
	descriptor core.CapabilityDescriptor
	profile    descriptorProfile
}

func (t *instrumentedTool) runtimeState() executionRuntimeState {
	if t == nil {
		return executionRuntimeState{policy: &compiledRuntimePolicy{}}
	}
	if t.registry == nil {
		return executionRuntimeState{policy: &compiledRuntimePolicy{}}
	}
	return t.registry.executionRuntimeState()
}

func (h instrumentCapabilityHandler) runtimeState() executionRuntimeState {
	if h.registry == nil {
		return executionRuntimeState{policy: &compiledRuntimePolicy{}}
	}
	return h.registry.executionRuntimeState()
}

func toolParametersFromSchema(schema *core.Schema) []contracts.ToolParameter {
	if schema == nil || schema.Type != "object" || len(schema.Properties) == 0 {
		return nil
	}
	required := make(map[string]struct{}, len(schema.Required))
	for _, name := range schema.Required {
		required[strings.TrimSpace(name)] = struct{}{}
	}
	out := make([]contracts.ToolParameter, 0, len(schema.Properties))
	for name, prop := range schema.Properties {
		param := contracts.ToolParameter{
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

func (h instrumentCapabilityHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	if h.descriptor.ID != "" {
		return h.descriptor
	}
	if h.handler == nil {
		return core.CapabilityDescriptor{}
	}
	return core.NormalizeCapabilityDescriptor(h.handler.Descriptor(ctx, env))
}

func (h instrumentCapabilityHandler) Availability(ctx context.Context, env *contextdata.Envelope) core.AvailabilitySpec {
	if aware, ok := h.handler.(core.AvailabilityAwareCapabilityHandler); ok {
		return aware.Availability(ctx, env)
	}
	return core.AvailabilitySpec{Available: true}
}

func (h instrumentCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	invocable, ok := h.handler.(core.InvocableCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability handler unavailable")
	}
	desc := h.descriptor
	if desc.ID == "" {
		desc = h.Descriptor(ctx, env)
	}
	var workingData map[string]interface{}
	if env != nil {
		workingData = env.WorkingData
	}
	approvalBinding := core.ApprovalBindingFromCapability(desc, workingData, args)
	approvalMetadata := map[string]string(nil)
	if approvalBinding != nil {
		approvalMetadata = approvalBinding.PermissionMetadata()
	}
	stateSnapshot := h.runtimeState()
	if err := core.ValidateValueAgainstSchema(args, desc.InputSchema); err != nil {
		return nil, fmt.Errorf("capability %s blocked: input schema invalid: %w", desc.ID, err)
	}
	if err := enforceDescriptorExecutionPoliciesWithProfile(ctx, desc, h.profile, stateSnapshot, approvalMetadata); err != nil {
		return nil, err
	}
	if stateSnapshot.safety != nil {
		if err := stateSnapshot.safety.CheckBeforeExecution(desc); err != nil {
			return nil, err
		}
	}
	emitCapabilityInvocationTelemetry(stateSnapshot.telemetry, desc, stateSnapshot.agentID, args)
	startedAt := time.Now().UTC()
	result, err := invocable.Invoke(ctx, env, args)
	if err == nil && result != nil && desc.OutputSchema != nil {
		if schemaErr := core.ValidateValueAgainstSchema(result.Data, desc.OutputSchema); schemaErr != nil {
			err = fmt.Errorf("capability %s blocked: output schema invalid: %w", desc.ID, schemaErr)
			result.Success = false
			result.Error = err.Error()
		}
	}
	if err == nil && stateSnapshot.safety != nil {
		if safetyErr := stateSnapshot.safety.RecordResult(desc, result); safetyErr != nil {
			err = safetyErr
			if result == nil {
				result = &contracts.ToolResult{Success: false, Error: safetyErr.Error()}
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
	emitCapabilityResultTelemetry(stateSnapshot.telemetry, desc, stateSnapshot.agentID, result, err, time.Since(startedAt))
	return result, err
}

func (h instrumentCapabilityHandler) RenderPrompt(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.PromptRenderResult, error) {
	promptHandler, ok := h.handler.(core.PromptCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("prompt handler unavailable")
	}
	desc := h.descriptor
	if desc.ID == "" {
		desc = h.Descriptor(ctx, env)
	}
	stateSnapshot := h.runtimeState()
	if err := core.ValidateValueAgainstSchema(args, desc.InputSchema); err != nil {
		return nil, fmt.Errorf("capability %s blocked: input schema invalid: %w", desc.ID, err)
	}
	if err := enforceDescriptorExecutionPoliciesWithProfile(ctx, desc, h.profile, stateSnapshot, nil); err != nil {
		return nil, err
	}
	if stateSnapshot.safety != nil {
		if err := stateSnapshot.safety.CheckBeforeExecution(desc); err != nil {
			return nil, err
		}
	}
	emitCapabilityInvocationTelemetry(stateSnapshot.telemetry, desc, stateSnapshot.agentID, args)
	startedAt := time.Now().UTC()
	result, err := promptHandler.RenderPrompt(ctx, env, args)
	emitPromptCapabilityResultTelemetry(stateSnapshot.telemetry, desc, stateSnapshot.agentID, result, err, time.Since(startedAt))
	return result, err
}

func (h instrumentCapabilityHandler) ReadResource(ctx context.Context, env *contextdata.Envelope) (*core.ResourceReadResult, error) {
	resourceHandler, ok := h.handler.(core.ResourceCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("resource handler unavailable")
	}
	desc := h.descriptor
	if desc.ID == "" {
		desc = h.Descriptor(ctx, env)
	}
	stateSnapshot := h.runtimeState()
	if err := enforceDescriptorExecutionPoliciesWithProfile(ctx, desc, h.profile, stateSnapshot, nil); err != nil {
		return nil, err
	}
	if stateSnapshot.safety != nil {
		if err := stateSnapshot.safety.CheckBeforeExecution(desc); err != nil {
			return nil, err
		}
	}
	emitCapabilityInvocationTelemetry(stateSnapshot.telemetry, desc, stateSnapshot.agentID, nil)
	startedAt := time.Now().UTC()
	result, err := resourceHandler.ReadResource(ctx, env)
	emitResourceCapabilityResultTelemetry(stateSnapshot.telemetry, desc, stateSnapshot.agentID, result, err, time.Since(startedAt))
	return result, err
}

func requestCapabilityApproval(ctx context.Context, desc core.CapabilityDescriptor, stateSnapshot executionRuntimeState, metadata map[string]string, reason string) error {
	if stateSnapshot.manager == nil {
		return fmt.Errorf("capability %s blocked: approval required but permission manager missing", desc.ID)
	}
	return stateSnapshot.manager.RequireApproval(ctx, stateSnapshot.agentID, contracts.PermissionDescriptor{
		Type:         contracts.PermissionTypeHITL,
		Action:       fmt.Sprintf("capability:%s", desc.ID),
		Resource:     stateSnapshot.agentID,
		Metadata:     metadata,
		RequiresHITL: true,
	}, reason, authorization.GrantScopeOneTime, authorization.RiskLevelMedium, 0)
}

func enforceDescriptorExecutionPolicies(ctx context.Context, desc core.CapabilityDescriptor, stateSnapshot executionRuntimeState, approvalMetadata map[string]string) error {
	return enforceDescriptorExecutionPoliciesWithProfile(ctx, desc, buildDescriptorProfile(desc), stateSnapshot, approvalMetadata)
}

func enforceDescriptorExecutionPoliciesWithProfile(ctx context.Context, desc core.CapabilityDescriptor, profile descriptorProfile, stateSnapshot executionRuntimeState, approvalMetadata map[string]string) error {
	if desc.Kind == core.CapabilityKindTool && desc.RuntimeFamily == core.CapabilityRuntimeFamilyLocalTool && stateSnapshot.policy.agentSpec != nil {
		switch stateSnapshot.policy.toolPolicies[desc.Name].Execute {
		case agentspec.AgentPermissionDeny:
			return fmt.Errorf("capability %s blocked: execution denied by tool policy", desc.ID)
		case agentspec.AgentPermissionAsk:
			return requestCapabilityApproval(ctx, desc, stateSnapshot, approvalMetadata, "tool execution approval")
		}
	}
	if len(stateSnapshot.policy.compiledCapabilityPolicies) > 0 {
		effective := effectiveCompiledCapabilityPolicyForProfile(profile, stateSnapshot.policy.compiledCapabilityPolicies)
		switch effective {
		case agentspec.AgentPermissionDeny:
			return fmt.Errorf("capability %s blocked: execution denied by capability selector policy", desc.ID)
		case agentspec.AgentPermissionAsk:
			return requestCapabilityApproval(ctx, desc, stateSnapshot, approvalMetadata, "capability selector policy approval")
		}
	}
	if len(stateSnapshot.policy.globalPolicies) > 0 {
		effective := effectiveClassPolicyForProfile(profile, stateSnapshot.policy.globalPolicies)
		switch effective {
		case agentspec.AgentPermissionDeny:
			return fmt.Errorf("capability %s blocked: execution denied by capability policy", desc.ID)
		case agentspec.AgentPermissionAsk:
			return requestCapabilityApproval(ctx, desc, stateSnapshot, approvalMetadata, "capability class policy approval")
		}
	}
	return nil
}

// Execute authorizes the wrapped tool before delegating to the original implementation.
func (t *instrumentedTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	desc := core.ToolDescriptor(ctx, t.Tool)
	approvalBinding := core.ApprovalBindingFromCapability(desc, nil, args)
	approvalMetadata := map[string]string(nil)
	if approvalBinding != nil {
		approvalMetadata = approvalBinding.PermissionMetadata()
	}
	stateSnapshot := t.runtimeState()
	if err := core.ValidateValueAgainstSchema(args, desc.InputSchema); err != nil {
		return nil, fmt.Errorf("tool %s blocked: input schema invalid: %w", t.Tool.Name(), err)
	}
	if err := enforceDescriptorExecutionPolicies(ctx, desc, stateSnapshot, approvalMetadata); err != nil {
		return nil, normalizeToolExecutionPolicyError(t.Tool.Name(), err)
	}
	if stateSnapshot.manager != nil {
		if err := stateSnapshot.manager.AuthorizeTool(ctx, stateSnapshot.agentID, t.Tool, args); err != nil {
			var denied *contracts.PermissionDeniedError
			if errors.As(err, &denied) {
				return nil, fmt.Errorf("tool %s blocked: %w", t.Tool.Name(), err)
			}
			return nil, err
		}
	}
	if stateSnapshot.safety != nil {
		if err := stateSnapshot.safety.CheckBeforeExecution(desc); err != nil {
			return nil, err
		}
	}
	if stateSnapshot.telemetry != nil {
		stateSnapshot.telemetry.Emit(core.Event{
			Type:      core.EventToolCall,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("tool %s invoked", t.Tool.Name()),
			Metadata: redactTelemetryMetadata(stateSnapshot.safety, map[string]interface{}{
				"tool":     t.Tool.Name(),
				"agent_id": stateSnapshot.agentID,
				"args":     summarizeArgs(args),
			}),
		})
	}
	startedAt := time.Now().UTC()
	result, err := t.Tool.Execute(ctx, args)
	if err == nil && result != nil && desc.OutputSchema != nil {
		if schemaErr := core.ValidateValueAgainstSchema(result.Data, desc.OutputSchema); schemaErr != nil {
			err = fmt.Errorf("tool %s blocked: output schema invalid: %w", t.Tool.Name(), schemaErr)
			result.Success = false
			result.Error = err.Error()
		}
	}
	if err == nil && stateSnapshot.safety != nil {
		if safetyErr := stateSnapshot.safety.RecordResult(desc, result); safetyErr != nil {
			err = safetyErr
			if result == nil {
				result = &contracts.ToolResult{Success: false, Error: safetyErr.Error()}
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
		var denied *contracts.PermissionDeniedError
		if errors.As(err, &denied) {
			err = fmt.Errorf("tool %s blocked: %w", t.Tool.Name(), err)
		}
	}
	if stateSnapshot.telemetry != nil {
		metadata := map[string]interface{}{
			"tool":     t.Tool.Name(),
			"agent_id": stateSnapshot.agentID,
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
		metadata["duration_ms"] = time.Since(startedAt).Milliseconds()
		stateSnapshot.telemetry.Emit(core.Event{
			Type:      core.EventToolResult,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("tool %s completed", t.Tool.Name()),
			Metadata:  redactTelemetryMetadata(stateSnapshot.safety, metadata),
		})
	}
	return result, err
}

func normalizeToolExecutionPolicyError(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("tool %s blocked: %s", name, strings.TrimPrefix(err.Error(), "capability tool:"+name+" blocked: "))
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

func emitCapabilitySecurityEvent(telemetry core.Telemetry, event string, desc core.CapabilityDescriptor, exposure core.CapabilityExposure, reason string) {
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
	telemetry.Emit(core.Event{
		Type:      core.EventStateChange,
		Timestamp: time.Now().UTC(),
		Message:   strings.ReplaceAll(event, "_", " "),
		Metadata:  core.RedactMetadataMap(metadata),
	})
}

func unwrapTool(tool contracts.Tool) contracts.Tool {
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
	tool contracts.Tool
}

func (h legacyToolHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.ToolDescriptor(ctx, unwrapTool(h.tool))
}

func (h legacyToolHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.ToolResult, error) {
	if h.tool == nil {
		return nil, fmt.Errorf("tool handler unavailable")
	}
	return h.tool.Execute(ctx, args)
}

func (h legacyToolHandler) Availability(ctx context.Context, env *contextdata.Envelope) core.AvailabilitySpec {
	if h.tool == nil {
		return core.AvailabilitySpec{Available: false, Reason: "tool handler unavailable"}
	}
	if !h.tool.IsAvailable(ctx) {
		return core.AvailabilitySpec{Available: false, Reason: "tool unavailable"}
	}
	return core.AvailabilitySpec{Available: true}
}

func emitCapabilityInvocationTelemetry(telemetry core.Telemetry, desc core.CapabilityDescriptor, agentID string, args map[string]interface{}) {
	if telemetry == nil {
		return
	}
	telemetry.Emit(core.Event{
		Type:      core.EventCapabilityCall,
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

func emitCapabilityResultTelemetry(telemetry core.Telemetry, desc core.CapabilityDescriptor, agentID string, result *contracts.CapabilityExecutionResult, err error, duration time.Duration) {
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
	metadata["duration_ms"] = duration.Milliseconds()
	telemetry.Emit(core.Event{
		Type:      core.EventCapabilityResult,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("capability %s completed", desc.Name),
		Metadata:  redactTelemetryMetadata(nil, metadata),
	})
}

func emitPromptCapabilityResultTelemetry(telemetry core.Telemetry, desc core.CapabilityDescriptor, agentID string, result *core.PromptRenderResult, err error, duration time.Duration) {
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
	metadata["duration_ms"] = duration.Milliseconds()
	telemetry.Emit(core.Event{
		Type:      core.EventCapabilityResult,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("capability %s completed", desc.Name),
		Metadata:  redactTelemetryMetadata(nil, metadata),
	})
}

func emitResourceCapabilityResultTelemetry(telemetry core.Telemetry, desc core.CapabilityDescriptor, agentID string, result *core.ResourceReadResult, err error, duration time.Duration) {
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
	metadata["duration_ms"] = duration.Milliseconds()
	telemetry.Emit(core.Event{
		Type:      core.EventCapabilityResult,
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
