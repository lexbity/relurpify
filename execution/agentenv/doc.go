/*
Package agentenv owns workspace lifecycle values for agents.

It consumes app-composed security, registration, and capability products, then
assembles the execution-owned prompt registry, telemetry handles, and agent
context used by application entrypoints and named agent packages.

Boot-layer verb taxonomy (design decision 8):

	Open*        Session lifecycle         OpenWorkspace
	Bootstrap*   Agent runtime assembly    BootstrapAgentRuntime
	Build*       Single leaf component     BuildPromptRegistry, etc.

No two functions share a verb-noun pair. App packages build authorization,
sandbox, and capability products in app/envcomposition, then pass those
products to OpenWorkspace. agentenv does not import the authorization
implementation package, construct sandbox runners, or build capability bundles
when the app-composed CapabilityProduct is supplied.

Scope mechanism (design decision 9):

	WorkspaceScope is a field on WorkspaceConfig, not a positional parameter.
	ScopeFull builds all optional layers. ScopeEmbeddedAgent builds only the
	agent execution layer (no LLM backend, no knowledge, no services, no telemetry sink).
	A zero-valued scope is promoted to ScopeFull by OpenWorkspace.
*/
package agentenv
