// Package runtime orchestrates capability providers and agent execution for
// the relurpish TUI.
//
// # Runtime
//
// Runtime is the central coordinator. It opens the workspace configuration,
// registers capability providers, wires the permission manager, and launches
// agents in response to user instructions from the TUI.
//
// # Providers
//
// Multiple provider types can be active simultaneously:
//
//   - Builtin tools: filesystem, shell, git, language tools, and AST tools
//     registered directly from the platform packages.
//   - MCP client (mcp_provider.go): connects to external MCP servers declared
//     in the workspace configuration and imports their capabilities.
//   - Nexus node (nexus_provider.go, nexus_node.go): connects to the Nexus
//     gateway and exposes capabilities from registered remote nodes.
//   - Background delegation (background_delegation_provider.go): routes tasks
//     marked for background execution to Nexus-managed agent instances.
//   - Browser (browser_provider.go): registers browser automation tools when
//     a WebDriver backend is configured.
//
// # Delegation
//
// delegations.go manages the lifecycle of capability delegations — bounded
// grants that allow the agent to forward specific capability calls to a remote
// node or background instance. nexus_session_dispatcher.go routes delegated
// calls through the Nexus session layer.
package runtime
