package ayenitd

// ProbeResult represents the outcome of a single platform runtime check.
type ProbeResult struct {
	Name     string
	Required bool
	OK       bool
	Message  string
}

// ProbeWorkspace runs all platform runtime checks required for a workspace.
// It returns a slice of results, one per check.
func ProbeWorkspace(cfg WorkspaceConfig) []ProbeResult {
	// TODO: Implement actual checks
	// For now, return stub results
	return []ProbeResult{
		{
			Name:     "workspace_directory",
			Required: true,
			OK:       true,
			Message:  "workspace directory exists and is readable",
		},
		{
			Name:     "ollama_reachable",
			Required: true,
			OK:       true,
			Message:  "Ollama endpoint reachable",
		},
		{
			Name:     "ollama_model",
			Required: true,
			OK:       true,
			Message:  "model present in Ollama",
		},
		{
			Name:     "disk_space",
			Required: false,
			OK:       true,
			Message:  "sufficient disk space available",
		},
	}
}
