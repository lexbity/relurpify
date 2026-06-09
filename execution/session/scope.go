package session

// WorkspaceScope declares which optional feature layers are assembled during
// OpenWorkspace. Security and capability assembly are always unconditional.
type WorkspaceScope struct {
	LLMBackend     bool
	Knowledge      bool
	Services       bool
	TelemetrySinks bool
}

// ScopeFull builds every optional layer.
var ScopeFull = WorkspaceScope{
	LLMBackend:     true,
	Knowledge:      true,
	Services:       true,
	TelemetrySinks: true,
}

// ScopeEmbeddedAgent builds only security + capabilities.
var ScopeEmbeddedAgent = WorkspaceScope{}
