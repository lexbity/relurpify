package react

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
)

func defaultIterationsForMode(mode string) int {
	switch strings.ToLower(mode) {
	case "code", "tdd":
		return 16
	case "debug":
		return 20
	case "review":
		return 12
	default:
		return 8
	}
}

func (a *ReActAgent) initializePhase(state *core.Context, task *core.Task) {
	if state == nil {
		return
	}
	// NEW: Inherit parent state from HTN steps to preserve workspace context
	// This prevents React from re-discovering files already explored by previous steps
	if task != nil && task.Context != nil {
		if parentStateRaw, ok := task.Context["parent_state"]; ok {
			if parentState, ok := parentStateRaw.(*core.Context); ok && parentState != nil {
				state.Merge(parentState)
				a.debugf("initialized phase with parent state from HTN step")
			}
		}
	}
	if phase := state.GetString("react.phase"); phase != "" {
		return
	}
	phase := contextmgrPhaseExplore
	text := taskInstructionText(task)
	if task != nil && task.Context != nil {
		if _, ok := task.Context["current_step"]; ok {
			if strings.Contains(text, "verify") || strings.Contains(text, "test") || strings.Contains(text, "build") {
				phase = contextmgrPhaseVerify
			}
		}
	}
	if !taskNeedsEditing(task) && taskRequiresVerification(task) && len(explicitlyRequestedToolNames(task)) > 0 {
		phase = contextmgrPhaseVerify
	}
	if strings.EqualFold(a.Mode, "debug") && (strings.Contains(text, "test") || strings.Contains(text, "build") || strings.Contains(text, "lint") || strings.Contains(text, "cargo")) {
		phase = contextmgrPhaseVerify
	}
	if strings.EqualFold(a.Mode, "docs") {
		phase = contextmgrPhaseEdit
	}
	state.Set("react.phase", phase)
}

func (a *ReActAgent) availableToolsForPhase(state *core.Context, task *core.Task) []core.Tool {
	catalog := a.executionCapabilityCatalog()
	if catalog == nil && a.Tools == nil {
		return nil
	}
	phase := contextmgrPhaseExplore
	if state != nil {
		if current := state.GetString("react.phase"); current != "" {
			phase = current
		}
	}
	var filtered []core.Tool
	tools := executionCallableTools(a.Tools, catalog)
	for _, tool := range tools {
		if toolAllowedForPhase(tool, phase, task) || a.recoveryToolAllowed(state, task, tool.Name()) {
			if !a.toolAllowedBySkillConfig(task, phase, tool.Name()) {
				continue
			}
			if !a.toolAllowedByExecutionContext(state, task, phase, tool) {
				continue
			}
			filtered = append(filtered, tool)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name() < filtered[j].Name() })
	return filtered
}

func (a *ReActAgent) executionCapabilityCatalog() *capability.ExecutionCapabilityCatalogSnapshot {
	if a == nil {
		return nil
	}
	if a.executionCatalog != nil {
		return a.executionCatalog
	}
	if a.Tools == nil {
		return nil
	}
	return a.Tools.CaptureExecutionCatalogSnapshot()
}

func (a *ReActAgent) executionPolicySnapshot() *core.PolicySnapshot {
	if catalog := a.executionCapabilityCatalog(); catalog != nil {
		return catalog.PolicySnapshot()
	}
	if a == nil || a.Tools == nil {
		return nil
	}
	return a.Tools.CapturePolicySnapshot()
}

func (a *ReActAgent) executionCapabilityDescriptor(idOrName string) (core.CapabilityDescriptor, bool) {
	if catalog := a.executionCapabilityCatalog(); catalog != nil {
		if entry, ok := catalog.GetCapability(idOrName); ok {
			return entry.Descriptor, true
		}
	}
	if a == nil || a.Tools == nil {
		return core.CapabilityDescriptor{}, false
	}
	return a.Tools.GetCapability(idOrName)
}

func executionCallableTools(registry *capability.Registry, catalog *capability.ExecutionCapabilityCatalogSnapshot) []core.Tool {
	if catalog != nil {
		return catalog.ModelCallableTools()
	}
	if registry == nil {
		return nil
	}
	return registry.ModelCallableTools()
}

func (a *ReActAgent) toolAllowedByExecutionContext(state *core.Context, task *core.Task, phase string, tool core.Tool) bool {
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
	if hasEditObservation(state) {
		return true
	}
	if tool.Name() == "file_read" && repeatedReadTarget(state) != "" {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(tool.Name()))
	if strings.Contains(name, "rustfmt") || strings.Contains(name, "format") || strings.Contains(name, "fmt") {
		return false
	}
	if taskNeedsEditing(task) && hasFailureFromState(state) && verificationLikeTool(tool) {
		return false
	}
	return true
}

func (a *ReActAgent) recoveryToolAllowed(state *core.Context, task *core.Task, toolName string) bool {
	if state == nil || !hasFailureFromState(state) {
		return false
	}
	for _, probe := range a.recoveryProbeTools(task) {
		if strings.EqualFold(strings.TrimSpace(probe), toolName) {
			return true
		}
	}
	return false
}

func (a *ReActAgent) toolAllowedBySkillConfig(task *core.Task, phase, toolName string) bool {
	resolved := a.resolvedSkillPolicy(task)
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

func (a *ReActAgent) resolvedSkillPolicy(task *core.Task) frameworkskills.ResolvedSkillPolicy {
	return frameworkskills.ResolveEffectiveSkillPolicy(task, a.effectiveAgentSpec(task), a.Tools).Policy
}

func (a *ReActAgent) recoveryProbeTools(task *core.Task) []string {
	resolved := a.resolvedSkillPolicy(task)
	return append([]string{}, resolved.RecoveryProbeCapabilities...)
}

func (a *ReActAgent) verificationSuccessTools(task *core.Task) []string {
	resolved := a.resolvedSkillPolicy(task)
	return append([]string{}, resolved.VerificationSuccessCapabilities...)
}

func (a *ReActAgent) effectiveAgentSpec(task *core.Task) *core.AgentRuntimeSpec {
	if a == nil || a.Config == nil {
		return frameworkskills.EffectiveAgentSpec(task, nil)
	}
	return frameworkskills.EffectiveAgentSpec(task, a.Config.AgentSpec)
}

func toolAllowedForPhase(tool core.Tool, phase string, task *core.Task) bool {
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
		if hasTag(core.TagDestructive) {
			return true
		}
		if hasTag(core.TagExecute) {
			return isLanguageExecutionTool(name, task)
		}
		if hasTag(core.TagReadOnly) {
			return strings.HasPrefix(name, "file_") || strings.HasPrefix(name, "ast_") || strings.HasPrefix(name, "lsp_") || strings.Contains(name, "grep")
		}
		return name == "exec_run_code"
	case contextmgrPhaseVerify:
		if hasTag(core.TagExecute) {
			return true
		}
		return strings.Contains(name, "rustfmt") || strings.Contains(name, "format") || strings.HasPrefix(name, "file_read")
	default:
		if hasTag(core.TagReadOnly) {
			return true
		}
		if hasTag(core.TagExecute) {
			return strings.EqualFold(taskMode(task), "debug") && isLanguageExecutionTool(name, task)
		}
		return strings.HasPrefix(name, "ast_") || strings.HasPrefix(name, "lsp_") || strings.Contains(name, "grep")
	}
}

func isLanguageExecutionTool(name string, task *core.Task) bool {
	name = strings.ToLower(name)
	if _, ok := explicitlyRequestedToolNames(task)[name]; ok {
		return true
	}
	if strings.Contains(name, "cargo") || strings.Contains(name, "rustfmt") {
		return true
	}
	if strings.Contains(name, "sqlite") {
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

func taskMode(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(task.Context["mode"]))
}
