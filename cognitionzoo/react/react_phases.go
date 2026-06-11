package react

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	capresult "codeburg.org/lexbit/relurpify/capability/result"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/policyresolve"
)

const (
	contextmgrPhaseExplore = "explore"
	contextmgrPhaseVerify  = "verify"
	contextmgrPhaseEdit    = "edit"
)


func (a *ReActAgent) availableToolsForPhase(ctx context.Context, env *contextdata.Envelope, task *execution.Task) []ports.Tool {
	catalog := a.executionCapabilityCatalog(ctx)
	if catalog == nil && a.Tools == nil {
		return nil
	}
	phase := contextmgrPhaseExplore
	if env != nil {
		if current := envGetString(env, "react.phase"); current != "" {
			phase = current
		}
	}
	var filtered []ports.Tool
	tools := executionCallableTools(ctx, a.Tools, catalog)
	for _, tool := range tools {
		if toolAllowedForPhase(tool, phase, task) || a.recoveryToolAllowed(env, task, tool.Name()) {
			if !a.toolAllowedBySkillConfig(task, phase, tool.Name()) {
				continue
			}
			if !a.toolAllowedByExecutionContext(env, task, phase, tool) {
				continue
			}
			filtered = append(filtered, tool)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name() < filtered[j].Name() })
	return filtered
}

func (a *ReActAgent) executionCapabilityCatalog(ctx context.Context) *capability.ExecutionCapabilityCatalogSnapshot {
	if a == nil {
		return nil
	}
	if a.executionCatalog != nil {
		return a.executionCatalog
	}
	if a.Tools == nil {
		return nil
	}
	return a.Tools.CaptureExecutionCatalogSnapshot(ctx)
}

func (a *ReActAgent) executionPolicySnapshot(ctx context.Context) *capresult.PolicySnapshot {
	if catalog := a.executionCapabilityCatalog(ctx); catalog != nil {
		return catalog.PolicySnapshot()
	}
	if a == nil || a.Tools == nil {
		return nil
	}
	return a.Tools.CapturePolicySnapshot()
}

func (a *ReActAgent) executionCapabilityDescriptor(ctx context.Context, idOrName string) (descriptor.CapabilityDescriptor, bool) {
	if catalog := a.executionCapabilityCatalog(ctx); catalog != nil {
		if entry, ok := catalog.GetCapability(idOrName); ok {
			return entry.Descriptor, true
		}
	}
	if a == nil || a.Tools == nil {
		return descriptor.CapabilityDescriptor{}, false
	}
	return a.Tools.GetCapability(idOrName)
}

func executionCallableTools(ctx context.Context, registry *capability.CapabilityRegistry, catalog *capability.ExecutionCapabilityCatalogSnapshot) []ports.Tool {
	if catalog != nil {
		return catalog.ModelCallableTools(ctx)
	}
	if registry == nil {
		return nil
	}
	return registry.ModelCallableTools(ctx)
}

func (a *ReActAgent) toolAllowedByExecutionContext(env *contextdata.Envelope, task *execution.Task, phase string, tool ports.Tool) bool {
	if tool == nil {
		return false
	}
	if strings.EqualFold(a.Mode, "docs") {
		name := strings.ToLower(strings.TrimSpace(tool.Name()))
		if name == "file_write" || name == "file_create" || name == "file_delete" {
			return false
		}
	}
	if requested := explicitlyRequestedToolNames(task); len(requested) > 0 && !taskNeedsEditing(task) && phase != contextmgrPhaseEdit {
		if _, ok := requested[strings.ToLower(strings.TrimSpace(tool.Name()))]; !ok {
			return false
		}
	}
	if requested := explicitlyRequestedToolNames(task); len(requested) > 0 && verificationLikeTool(tool) {
		if _, ok := requested[strings.ToLower(strings.TrimSpace(tool.Name()))]; !ok {
			return false
		}
	}
	if phase != contextmgrPhaseEdit {
		return true
	}
	if hasEditObservation(env) {
		return true
	}
	if tool.Name() == "file_read" && repeatedReadTarget(env) != "" {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(tool.Name()))
	if strings.Contains(name, "rustfmt") || strings.Contains(name, "format") || strings.Contains(name, "fmt") {
		return false
	}
	if taskNeedsEditing(task) && hasFailureFromState(env) && verificationLikeTool(tool) {
		return false
	}
	return true
}

func (a *ReActAgent) recoveryToolAllowed(env *contextdata.Envelope, task *execution.Task, toolName string) bool {
	if env == nil || !hasFailureFromState(env) {
		return false
	}
	for _, probe := range a.recoveryProbeTools() {
		if strings.EqualFold(strings.TrimSpace(probe), toolName) {
			return true
		}
	}
	return false
}

func (a *ReActAgent) toolAllowedBySkillConfig(task *execution.Task, phase, toolName string) bool {
	resolved := a.resolvedAgentPolicy()
	if len(resolved.PhaseCapabilities) == 0 {
		return true
	}
	allowed, ok := resolved.PhaseCapabilities[phase]
	if !ok || len(allowed) == 0 {
		return true
	}
	for _, entry := range allowed {
		if strings.EqualFold(strings.TrimSpace(entry), toolName) {
			return true
		}
	}
	return false
}

func (a *ReActAgent) resolvedAgentPolicy() policy.ResolvedAgentPolicy {
	if a == nil || a.Config == nil || a.Config.AgentSpec == nil {
		return policy.ResolvedAgentPolicy{}
	}
	reg := capability.NewPolicyResolveRegistry(a.Tools)
	cfg := agentspec.ToPolicyResolveOrchConfig(a.Config.AgentSpec.Orchestration)
	return policyresolve.ResolveAgentPolicy(reg, cfg)
}

func (a *ReActAgent) recoveryProbeTools() []string {
	resolved := a.resolvedAgentPolicy()
	return append([]string{}, resolved.RecoveryProbeCapabilities...)
}

func (a *ReActAgent) verificationSuccessTools() []string {
	resolved := a.resolvedAgentPolicy()
	return append([]string{}, resolved.VerificationSuccessCapabilities...)
}

func toolAllowedForPhase(tool ports.Tool, phase string, task *execution.Task) bool {
	if tool == nil {
		return false
	}
	name := tool.Name()
	tags := tool.Tags()
	if len(tags) == 0 {
		return true
	}
	hasTag := func(target string) bool {
		for _, tag := range tags {
			if tag == target {
				return true
			}
		}
		return false
	}
	switch phase {
	case contextmgrPhaseEdit:
		if hasTag(toolcapabilities.TagDestructive) {
			return true
		}
		if hasTag(toolcapabilities.TagExecute) {
			return isLanguageExecutionTool(name, task)
		}
		if hasTag(toolcapabilities.TagReadOnly) {
			return strings.HasPrefix(name, "file_") || strings.HasPrefix(name, "ast_") || strings.HasPrefix(name, "lsp_") || strings.Contains(name, "grep")
		}
		return name == "exec_run_code"
	case contextmgrPhaseVerify:
		if hasTag(toolcapabilities.TagExecute) {
			return true
		}
		return strings.Contains(name, "rustfmt") || strings.Contains(name, "format") || strings.HasPrefix(name, "file_read")
	default:
		if hasTag(toolcapabilities.TagReadOnly) {
			return true
		}
		if hasTag(toolcapabilities.TagExecute) {
			return strings.EqualFold(taskMode(task), "debug") && isLanguageExecutionTool(name, task)
		}
		return strings.HasPrefix(name, "ast_") || strings.HasPrefix(name, "lsp_") || strings.Contains(name, "grep")
	}
}

func isLanguageExecutionTool(name string, task *execution.Task) bool {
	name = strings.ToLower(name)
	if _, ok := explicitlyRequestedToolNames(task)[name]; ok {
		return true
	}
	if strings.Contains(name, "cargo") || strings.Contains(name, "rustfmt") {
		return true
	}
	if strings.Contains(name, "test") || strings.Contains(name, "build") || strings.Contains(name, "lint") || strings.Contains(name, "format") || strings.Contains(name, "check") {
		return true
	}
	if strings.Contains(name, "exec_run_code") {
		return true
	}
	text := ""
	if task != nil {
		text = strings.ToLower(task.Instruction)
	}
	return strings.Contains(text, "test") || strings.Contains(text, "build") || strings.Contains(text, "lint")
}

func taskMode(task *execution.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(task.Context["mode"]))
}
