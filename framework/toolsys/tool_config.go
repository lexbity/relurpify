package toolsys

import "github.com/lexcodex/relurpify/framework/core"

// ToolPolicyOverlay describes a layer of tool matrix + policy changes.
type ToolPolicyOverlay struct {
	MatrixOverride *core.ToolMatrixOverride
	Policies       map[string]ToolPolicy
}

// MergeToolConfig combines the base matrix/policies with any overlays.
func MergeToolConfig(baseMatrix AgentToolMatrix, basePolicies map[string]ToolPolicy, overlays ...ToolPolicyOverlay) (AgentToolMatrix, map[string]ToolPolicy) {
	matrix := baseMatrix
	policies := make(map[string]ToolPolicy, len(basePolicies))
	for name, policy := range basePolicies {
		policies[name] = policy
	}
	for _, overlay := range overlays {
		if overlay.MatrixOverride != nil {
			matrix = applyToolMatrixOverride(matrix, overlay.MatrixOverride)
		}
		for name, policy := range overlay.Policies {
			policies[name] = policy
		}
	}
	return matrix, policies
}

func applyToolMatrixOverride(base AgentToolMatrix, override *core.ToolMatrixOverride) AgentToolMatrix {
	if override == nil {
		return base
	}
	if override.FileRead != nil {
		base.FileRead = *override.FileRead
	}
	if override.FileWrite != nil {
		base.FileWrite = *override.FileWrite
	}
	if override.FileEdit != nil {
		base.FileEdit = *override.FileEdit
	}
	if override.BashExecute != nil {
		base.BashExecute = *override.BashExecute
	}
	if override.LSPQuery != nil {
		base.LSPQuery = *override.LSPQuery
	}
	if override.SearchCodebase != nil {
		base.SearchCodebase = *override.SearchCodebase
	}
	if override.WebSearch != nil {
		base.WebSearch = *override.WebSearch
	}
	return base
}

// ApplyToolConfig merges overlays and applies the resulting policies/matrix.
func ApplyToolConfig(registry *ToolRegistry, baseMatrix AgentToolMatrix, basePolicies map[string]ToolPolicy, overlays ...ToolPolicyOverlay) {
	if registry == nil {
		return
	}
	matrix, policies := MergeToolConfig(baseMatrix, basePolicies, overlays...)

	registry.setToolMatrix(matrix)
	registry.mu.Lock()
	registry.toolPolicies = make(map[string]ToolPolicy, len(policies))
	for name, policy := range policies {
		registry.toolPolicies[name] = policy
	}
	for name, pol := range registry.toolPolicies {
		if pol.Visible != nil && !*pol.Visible {
			delete(registry.tools, name)
		}
	}
	for name, tool := range registry.tools {
		var inner Tool = tool
		if instrumented, ok := tool.(*instrumentedTool); ok {
			inner = instrumented.Tool
			instrumented.policy = registry.toolPolicies[inner.Name()]
			instrumented.hasPolicy = registry.agentSpec != nil
		}
		registry.tools[name] = registry.wrapTool(inner)
	}
	registry.mu.Unlock()

	RestrictToolRegistryByMatrix(registry, matrix)
}
