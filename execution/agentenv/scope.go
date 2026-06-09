package agentenv

// WorkspaceScope declares which optional feature layers are assembled during
// OpenWorkspace. Security and capability assembly are always unconditional;
// only the listed layers can be subtracted via a non-full scope.
//
// A zero-value WorkspaceScope defaults to ScopeFull
// when Scope is not set on WorkspaceConfig (see zeroScopeDefaultsToFull).
type WorkspaceScope struct {
	LLMBackend     bool // build inference backend + instrumented model
	Knowledge      bool // knowledge store, retriever, compiler, stream trigger
	Services       bool // ServiceManager + scheduler + event log
	TelemetrySinks bool // JSON file telemetry sink (logger always on)
}

// ScopeFull builds every optional layer. This is the default when
// WorkspaceConfig.Scope is zero-valued.
var ScopeFull = WorkspaceScope{
	LLMBackend:     true,
	Knowledge:      true,
	Services:       true,
	TelemetrySinks: true,
}

// ScopeEmbeddedAgent builds only security + capabilities. No LLM backend,
// no knowledge, no services, no telemetry sink — just the verified and
// authorized runner backed by a full capability registry.
var ScopeEmbeddedAgent = WorkspaceScope{}
