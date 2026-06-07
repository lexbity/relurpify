/*
Package agentenv owns runtime composition and workspace lifecycle for agents.

It assembles loaded config into live workspace state, capability registries,
telemetry, sandbox wiring, and the executable agent environment used by the
application entrypoints and named agent packages.

Boot-layer verb taxonomy (design decision 8):

	Open*        Session lifecycle         OpenWorkspace
	Bootstrap*   Secured runtime assembly  BootstrapAgentRuntime
	build*       Foundation (unexported)   buildSecuredRuntime
	Build*       Single leaf component     BuildBuiltinCapabilityBundle, BuildPromptRegistry, etc.

No two functions share a verb-noun pair. Every entry point traverses
OpenWorkspace, which calls BootstrapAgentRuntime, which calls buildSecuredRuntime.
The security foundation (buildSecuredRuntime) is the only producer of
*sandbox.AuthorizedRunner, and BuildBuiltinCapabilityBundle is the only consumer.

Scope mechanism (design decision 9):

	WorkspaceScope is a field on WorkspaceConfig, not a positional parameter.
	ScopeFull builds all optional layers. ScopeEmbeddedAgent builds only security
	and capabilities (no LLM backend, no knowledge, no services, no telemetry sink).
	A zero-valued scope is promoted to ScopeFull by OpenWorkspace for backward
	compatibility.
*/
package agentenv
