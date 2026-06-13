package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"
	capresult "codeburg.org/lexbit/relurpify/capability/result"
	runtime "codeburg.org/lexbit/relurpify/capability/runtime"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	AgentId_capability_registry_wrappers = "agent_id"
	Capability_capability_registry_wrappers = "capability"
	Capabilityscompleted_capability_registry_wrappers = "capability %s completed"
	CapabilityId_capability_registry_wrappers = "capability_id"
	DurationMs_capability_registry_wrappers = "duration_ms"
	Error_capability_registry_wrappers = "error"
	Kind_capability_registry_wrappers = "kind"
	RuntimeFamily_capability_registry_wrappers = "runtime_family"
)


// wrapTool decorates a tool with the instrumentation wrapper.
func (r *CapabilityRegistry) wrapTool(tool ports.Tool) ports.Tool {
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

func (r *CapabilityRegistry) wrapCapabilityHandler(handler handler.CapabilityHandler) handler.CapabilityHandler {
	if handler == nil {
		return nil
	}
	desc := descriptor.NormalizeCapabilityDescriptor(handler.Descriptor(context.Background(), nil))
	return r.wrapCapabilityHandlerPrepared(handler, desc, buildDescriptorProfile(desc))
}

func (r *CapabilityRegistry) wrapCapabilityHandlerPrepared(handler handler.CapabilityHandler, desc descriptor.CapabilityDescriptor, profile descriptorProfile) handler.CapabilityHandler {
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
	ports.Tool
	registry *CapabilityRegistry
}

type instrumentCapabilityHandler struct {
	handler    handler.CapabilityHandler
	registry   *CapabilityRegistry
	descriptor descriptor.CapabilityDescriptor
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

func (h instrumentCapabilityHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	if h.descriptor.ID != "" {
		return h.descriptor
	}
	if h.handler == nil {
		return descriptor.CapabilityDescriptor{}
	}
	return descriptor.NormalizeCapabilityDescriptor(h.handler.Descriptor(ctx, env))
}

func (h instrumentCapabilityHandler) Availability(ctx context.Context, env ports.State) descriptor.AvailabilitySpec {
	if aware, ok := h.handler.(handler.AvailabilityAwareCapabilityHandler); ok {
		return aware.Availability(ctx, env)
	}
	return descriptor.AvailabilitySpec{Available: true}
}

func (h instrumentCapabilityHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	invocable, ok := h.handler.(handler.InvocableCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability handler unavailable")
	}
	desc := h.descriptor
	if desc.ID == "" {
		desc = h.Descriptor(ctx, env)
	}
	var workingData map[string]any
	if env != nil {
		workingData = env.Snapshot()
	}
	approvalBinding := capresult.ApprovalBindingFromCapability(desc, workingData, args)
	approvalMetadata := map[string]string(nil)
	if approvalBinding != nil {
		approvalMetadata = approvalBinding.PermissionMetadata()
	}
	stateSnapshot := h.runtimeState()
	if err := ValidateAndCoerce(args, desc.InputSchema, nil); err != nil {
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
		if schemaErr := ValidateValueAgainstSchema(result.Data, desc.OutputSchema); schemaErr != nil {
			err = fmt.Errorf("capability %s blocked: output schema invalid: %w", desc.ID, schemaErr)
			result.Success = false
			result.Error = err.Error()
		}
	}
	if err == nil && stateSnapshot.safety != nil {
		if safetyErr := stateSnapshot.safety.RecordResult(desc, result); safetyErr != nil {
			err = safetyErr
			if result == nil {
				result = &ports.ToolResult{Success: false, Error: safetyErr.Error()}
			} else {
				result.Success = false
				result.Error = safetyErr.Error()
			}
		}
	}
	if result != nil {
		if result.Metadata == nil {
			result.Metadata = map[string]any{}
		}
		result.Metadata["capability_descriptor"] = desc
		if approvalBinding != nil {
			result.Metadata["approval_binding"] = approvalBinding
		}
		result.Metadata["insertion_decision"] = capresult.DefaultInsertionDecision(desc, capresult.ContentDispositionRaw)
	}
	emitCapabilityResultTelemetry(stateSnapshot.telemetry, desc, stateSnapshot.agentID, result, err, time.Since(startedAt))
	return result, err
}

func (h instrumentCapabilityHandler) RenderPrompt(ctx context.Context, env ports.State, args map[string]any) (*handler.PromptRenderResult, error) {
	promptHandler, ok := h.handler.(handler.PromptCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("prompt handler unavailable")
	}
	desc := h.descriptor
	if desc.ID == "" {
		desc = h.Descriptor(ctx, env)
	}
	stateSnapshot := h.runtimeState()
	if err := ValidateAndCoerce(args, desc.InputSchema, nil); err != nil {
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

func (h instrumentCapabilityHandler) ReadResource(ctx context.Context, env ports.State) (*handler.ResourceReadResult, error) {
	resourceHandler, ok := h.handler.(handler.ResourceCapabilityHandler)
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

func requestCapabilityApproval(ctx context.Context, desc descriptor.CapabilityDescriptor, stateSnapshot executionRuntimeState, metadata map[string]string, reason string) error {
	if stateSnapshot.manager == nil {
		return fmt.Errorf("capability %s blocked: approval required but permission manager missing", desc.ID)
	}
	return stateSnapshot.manager.RequireApproval(ctx, stateSnapshot.agentID, permissions.PermissionDescriptor{
		Type:         permissions.PermissionTypeHITL,
		Action:       fmt.Sprintf("capability:%s", desc.ID),
		Resource:     stateSnapshot.agentID,
		Metadata:     metadata,
		RequiresHITL: true,
	}, reason, policy.GrantScopeOneTime, policy.RiskLevelMedium, 0)
}

func enforceDescriptorExecutionPolicies(ctx context.Context, desc descriptor.CapabilityDescriptor, stateSnapshot executionRuntimeState, approvalMetadata map[string]string) error {
	return enforceDescriptorExecutionPoliciesWithProfile(ctx, desc, buildDescriptorProfile(desc), stateSnapshot, approvalMetadata)
}

func enforceDescriptorExecutionPoliciesWithProfile(ctx context.Context, desc descriptor.CapabilityDescriptor, profile descriptorProfile, stateSnapshot executionRuntimeState, approvalMetadata map[string]string) error {
	if desc.Kind == agentspec.CapabilityKindTool && desc.RuntimeFamily == agentspec.CapabilityRuntimeFamilyLocalTool && stateSnapshot.policy.agentSpec != nil {
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
func (t *instrumentedTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	desc := descriptor.ToolDescriptor(ctx, t.Tool)
	approvalBinding := capresult.ApprovalBindingFromCapability(desc, nil, args)
	approvalMetadata := map[string]string(nil)
	if approvalBinding != nil {
		approvalMetadata = approvalBinding.PermissionMetadata()
	}
	stateSnapshot := t.runtimeState()
	if err := ValidateAndCoerce(args, desc.InputSchema, t.Parameters()); err != nil {
		return nil, fmt.Errorf("tool %s blocked: input schema invalid: %w", t.Name(), err)
	}
	if err := enforceDescriptorExecutionPolicies(ctx, desc, stateSnapshot, approvalMetadata); err != nil {
		return nil, normalizeToolExecutionPolicyError(t.Name(), err)
	}
	if stateSnapshot.manager != nil {
		if err := stateSnapshot.manager.AuthorizeTool(ctx, stateSnapshot.agentID, t.Tool, args); err != nil {
			var denied *permissions.PermissionDeniedError
			if errors.As(err, &denied) {
				return nil, fmt.Errorf("tool %s blocked: %w", t.Name(), err)
			}
			return nil, err
		}
	}
	if stateSnapshot.safety != nil {
		if err := stateSnapshot.safety.CheckBeforeExecution(desc); err != nil {
			return nil, err
		}
	}
	traceCtx, _ := fwtelemetry.TraceContextFromContext(ctx)
	if stateSnapshot.telemetry != nil {
		spanAttrs := buildSpanAttrs(desc, t.Tool)
		meta := map[string]any{
			"tool":       t.Name(),
			AgentId_capability_registry_wrappers:   stateSnapshot.agentID,
			"args":       summarizeArgs(args),
			"span_attrs": spanAttrs,
		}
		if traceCtx.TraceID != "" {
			meta["trace_id"] = traceCtx.TraceID
			meta["span_id"] = traceCtx.SpanID
		}
		stateSnapshot.telemetry.Emit(fwtelemetry.Event{
			Type:      fwtelemetry.EventToolCall,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("tool %s invoked", t.Name()),
			Metadata:  redactTelemetryMetadata(stateSnapshot.safety, meta),
		})
	}
	startedAt := time.Now().UTC()
	result, err := t.Tool.Execute(ctx, args)
	if err == nil && result != nil && desc.OutputSchema != nil {
		if schemaErr := ValidateValueAgainstSchema(result.Data, desc.OutputSchema); schemaErr != nil {
			err = fmt.Errorf("tool %s blocked: output schema invalid: %w", t.Name(), schemaErr)
			result.Success = false
			result.Error = err.Error()
		}
	}
	if err == nil && stateSnapshot.safety != nil {
		if safetyErr := stateSnapshot.safety.RecordResult(desc, result); safetyErr != nil {
			err = safetyErr
			if result == nil {
				result = &ports.ToolResult{Success: false, Error: safetyErr.Error()}
			} else {
				result.Success = false
				result.Error = safetyErr.Error()
			}
		}
	}
	if result != nil {
		if result.Metadata == nil {
			result.Metadata = map[string]any{}
		}
		result.Metadata["capability_descriptor"] = desc
		if approvalBinding != nil {
			result.Metadata["approval_binding"] = approvalBinding
		}
		result.Metadata["insertion_decision"] = capresult.DefaultInsertionDecision(desc, capresult.ContentDispositionRaw)
	}
	if err != nil {
		var denied *permissions.PermissionDeniedError
		if errors.As(err, &denied) {
			err = fmt.Errorf("tool %s blocked: %w", t.Name(), err)
		}
	}
	if stateSnapshot.telemetry != nil {
		spanAttrs := buildSpanAttrs(desc, t.Tool)
		metadata := map[string]any{
			"tool":       t.Name(),
			AgentId_capability_registry_wrappers:   stateSnapshot.agentID,
			"span_attrs": spanAttrs,
		}
		if traceCtx.TraceID != "" {
			metadata["trace_id"] = traceCtx.TraceID
			metadata["span_id"] = traceCtx.SpanID
		}
		if result != nil {
			metadata["success"] = result.Success
			metadata["exit_code"] = extractExitCode(result)
			metadata["stdout_bytes"] = extractStdoutBytes(result)
			metadata["artifact_ref"] = extractArtifactRef(result)
			if result.Error != "" {
				metadata["tool_error"] = result.Error
			}
		}
		if err != nil {
			metadata[Error_capability_registry_wrappers] = err.Error()
		}
		metadata[DurationMs_capability_registry_wrappers] = time.Since(startedAt).Milliseconds()
		stateSnapshot.telemetry.Emit(fwtelemetry.Event{
			Type:      fwtelemetry.EventToolResult,
			Timestamp: time.Now().UTC(),
			Message:   fmt.Sprintf("tool %s completed", t.Name()),
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

func redactTelemetryMetadata(controller *runtime.RuntimeSafetyController, metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	if controller != nil {
		spec := controller.SnapshotSpec()
		if spec != nil && !spec.RedactionEnabled() {
			return metadata
		}
	}
	return runtime.RedactMetadataMap(metadata)
}

func emitCapabilitySecurityEvent(telemetry fwtelemetry.Telemetry, event string, desc descriptor.CapabilityDescriptor, exposure agentspec.CapabilityExposure, reason string) {
	if telemetry == nil || desc.ID == "" {
		return
	}
	metadata := map[string]any{
		"security_event": event,
		CapabilityId_capability_registry_wrappers:  desc.ID,
		Capability_capability_registry_wrappers:     desc.Name,
		Kind_capability_registry_wrappers:           string(desc.Kind),
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
	telemetry.Emit(fwtelemetry.Event{
		Type:      fwtelemetry.EventStateChange,
		Timestamp: time.Now().UTC(),
		Message:   strings.ReplaceAll(event, "_", " "),
		Metadata:  runtime.RedactMetadataMap(metadata),
	})
}

func unwrapTool(tool ports.Tool) ports.Tool {
	if wrapped, ok := tool.(*instrumentedTool); ok {
		return wrapped.Tool
	}
	return tool
}

func unwrapCapabilityHandler(handler handler.CapabilityHandler) handler.CapabilityHandler {
	if wrapped, ok := handler.(instrumentCapabilityHandler); ok {
		return wrapped.handler
	}
	return handler
}

type legacyToolHandler struct {
	tool ports.Tool
}

func (h legacyToolHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.ToolDescriptor(ctx, unwrapTool(h.tool))
}

func (h legacyToolHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	if h.tool == nil {
		return nil, fmt.Errorf("tool handler unavailable")
	}
	return h.tool.Execute(ctx, args)
}

func (h legacyToolHandler) Availability(ctx context.Context, env ports.State) descriptor.AvailabilitySpec {
	if h.tool == nil {
		return descriptor.AvailabilitySpec{Available: false, Reason: "tool handler unavailable"}
	}
	if !h.tool.IsAvailable(ctx) {
		return descriptor.AvailabilitySpec{Available: false, Reason: "tool unavailable"}
	}
	return descriptor.AvailabilitySpec{Available: true}
}

func emitCapabilityInvocationTelemetry(telemetry fwtelemetry.Telemetry, desc descriptor.CapabilityDescriptor, agentID string, args map[string]any) {
	if telemetry == nil {
		return
	}
	telemetry.Emit(fwtelemetry.Event{
		Type:      fwtelemetry.EventCapabilityCall,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf("capability %s invoked", desc.Name),
		Metadata: redactTelemetryMetadata(nil, map[string]any{
			CapabilityId_capability_registry_wrappers:  desc.ID,
			Capability_capability_registry_wrappers:     desc.Name,
			Kind_capability_registry_wrappers:           string(desc.Kind),
			RuntimeFamily_capability_registry_wrappers: string(desc.RuntimeFamily),
			AgentId_capability_registry_wrappers:       agentID,
			"args":           summarizeArgs(args),
		}),
	})
}

func emitCapabilityResultTelemetry(telemetry fwtelemetry.Telemetry, desc descriptor.CapabilityDescriptor, agentID string, result *ports.ToolResult, err error, duration time.Duration) {
	if telemetry == nil {
		return
	}
	metadata := map[string]any{
		CapabilityId_capability_registry_wrappers:  desc.ID,
		Capability_capability_registry_wrappers:     desc.Name,
		Kind_capability_registry_wrappers:           string(desc.Kind),
		RuntimeFamily_capability_registry_wrappers: string(desc.RuntimeFamily),
		AgentId_capability_registry_wrappers:       agentID,
	}
	if result != nil {
		metadata["success"] = result.Success
		if result.Error != "" {
			metadata["capability_error"] = result.Error
		}
	}
	if err != nil {
		metadata[Error_capability_registry_wrappers] = err.Error()
	}
	metadata[DurationMs_capability_registry_wrappers] = duration.Milliseconds()
	telemetry.Emit(fwtelemetry.Event{
		Type:      fwtelemetry.EventCapabilityResult,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf(Capabilityscompleted_capability_registry_wrappers, desc.Name),
		Metadata:  redactTelemetryMetadata(nil, metadata),
	})
}

func emitPromptCapabilityResultTelemetry(telemetry fwtelemetry.Telemetry, desc descriptor.CapabilityDescriptor, agentID string, result *handler.PromptRenderResult, err error, duration time.Duration) {
	if telemetry == nil {
		return
	}
	metadata := map[string]any{
		CapabilityId_capability_registry_wrappers:  desc.ID,
		Capability_capability_registry_wrappers:     desc.Name,
		Kind_capability_registry_wrappers:           string(desc.Kind),
		RuntimeFamily_capability_registry_wrappers: string(desc.RuntimeFamily),
		AgentId_capability_registry_wrappers:       agentID,
	}
	if result != nil {
		metadata["message_count"] = len(result.Messages)
	}
	if err != nil {
		metadata[Error_capability_registry_wrappers] = err.Error()
	}
	metadata[DurationMs_capability_registry_wrappers] = duration.Milliseconds()
	telemetry.Emit(fwtelemetry.Event{
		Type:      fwtelemetry.EventCapabilityResult,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf(Capabilityscompleted_capability_registry_wrappers, desc.Name),
		Metadata:  redactTelemetryMetadata(nil, metadata),
	})
}

func emitResourceCapabilityResultTelemetry(telemetry fwtelemetry.Telemetry, desc descriptor.CapabilityDescriptor, agentID string, result *handler.ResourceReadResult, err error, duration time.Duration) {
	if telemetry == nil {
		return
	}
	metadata := map[string]any{
		CapabilityId_capability_registry_wrappers:  desc.ID,
		Capability_capability_registry_wrappers:     desc.Name,
		Kind_capability_registry_wrappers:           string(desc.Kind),
		RuntimeFamily_capability_registry_wrappers: string(desc.RuntimeFamily),
		AgentId_capability_registry_wrappers:       agentID,
	}
	if result != nil {
		metadata["content_count"] = len(result.Contents)
	}
	if err != nil {
		metadata[Error_capability_registry_wrappers] = err.Error()
	}
	metadata[DurationMs_capability_registry_wrappers] = duration.Milliseconds()
	telemetry.Emit(fwtelemetry.Event{
		Type:      fwtelemetry.EventCapabilityResult,
		Timestamp: time.Now().UTC(),
		Message:   fmt.Sprintf(Capabilityscompleted_capability_registry_wrappers, desc.Name),
		Metadata:  redactTelemetryMetadata(nil, metadata),
	})
}

func summarizeArgs(args map[string]any) any {
	if len(args) == 0 {
		return nil
	}
	return runtime.RedactMetadataMap(args)
}

// buildSpanAttrs extracts span-compatible attributes from the capability
// descriptor and the tool implementation.
func buildSpanAttrs(desc descriptor.CapabilityDescriptor, tool ports.Tool) map[string]any {
	attrs := map[string]any{
		"tool.name":   tool.Name(),
		"tool.family": tool.Category(),
	}
	if desc.TrustClass != "" {
		attrs["trust_class"] = string(desc.TrustClass)
	}
	for i, ec := range desc.EffectClasses {
		var key string
		if i == 0 {
			key = "effect_class"
		} else {
			key = fmt.Sprintf("effect_class_%d", i)
		}
		attrs[key] = string(ec)
	}
	return attrs
}

func extractExitCode(result *ports.ToolResult) int {
	if result == nil || result.Data == nil {
		return 0
	}
	if ec, ok := result.Data["exit_code"]; ok {
		switch v := ec.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

func extractStdoutBytes(result *ports.ToolResult) int64 {
	if result == nil || result.Data == nil {
		return 0
	}
	if sb, ok := result.Data["stdout_bytes"]; ok {
		switch v := sb.(type) {
		case int64:
			return v
		case float64:
			return int64(v)
		case int:
			return int64(v)
		}
	}
	return 0
}

func extractArtifactRef(result *ports.ToolResult) string {
	if result == nil || result.Data == nil {
		return ""
	}
	if ref, ok := result.Data["stdout_ref"]; ok {
		if s, ok := ref.(string); ok {
			return s
		}
	}
	return ""
}
