package agentspec

// AgentDefinition defines the configuration for a single agent.
type AgentDefinition struct {
	Name        string           `yaml:"name" json:"name"`
	Description string           `yaml:"description,omitempty" json:"description,omitempty"`
	Spec        AgentRuntimeSpec `yaml:"spec" json:"spec"`
}

// AgentSpecOverlaysForName returns the overlay derived from a named agent definition.
func AgentSpecOverlaysForName(name string, defs map[string]*AgentDefinition) []AgentSpecOverlay {
	if defs == nil {
		return nil
	}
	def, ok := defs[name]
	if !ok || def == nil {
		return nil
	}
	return []AgentSpecOverlay{AgentSpecOverlayFromSpec(&def.Spec)}
}
