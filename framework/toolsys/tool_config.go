package toolsys

// ToolMatrixOverride selectively overrides tool matrix booleans.
type ToolMatrixOverride struct {
	FileRead       *bool
	FileWrite      *bool
	FileEdit       *bool
	BashExecute    *bool
	LSPQuery       *bool
	SearchCodebase *bool
	WebSearch      *bool
}

// ToolPolicyOverlay describes a layer of tool matrix + policy changes.
type ToolPolicyOverlay struct {
	MatrixOverride *ToolMatrixOverride
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
			matrix = overlay.MatrixOverride.apply(matrix)
		}
		for name, policy := range overlay.Policies {
			policies[name] = policy
		}
	}
	return matrix, policies
}

func (o *ToolMatrixOverride) apply(base AgentToolMatrix) AgentToolMatrix {
	if o == nil {
		return base
	}
	if o.FileRead != nil {
		base.FileRead = *o.FileRead
	}
	if o.FileWrite != nil {
		base.FileWrite = *o.FileWrite
	}
	if o.FileEdit != nil {
		base.FileEdit = *o.FileEdit
	}
	if o.BashExecute != nil {
		base.BashExecute = *o.BashExecute
	}
	if o.LSPQuery != nil {
		base.LSPQuery = *o.LSPQuery
	}
	if o.SearchCodebase != nil {
		base.SearchCodebase = *o.SearchCodebase
	}
	if o.WebSearch != nil {
		base.WebSearch = *o.WebSearch
	}
	return base
}

// ApplyToolConfig merges overlays and applies the resulting policies/matrix.
func ApplyToolConfig(registry *ToolRegistry, baseMatrix AgentToolMatrix, basePolicies map[string]ToolPolicy, overlays ...ToolPolicyOverlay) {
	if registry == nil {
		return
	}
	matrix, policies := MergeToolConfig(baseMatrix, basePolicies, overlays...)

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
