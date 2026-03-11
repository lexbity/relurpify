# Relurpify Documentation

## User Docs

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | System overview, mental model, component map |
| [Installation](installation.md) | Prerequisites, global install, workspace setup |
| [Workspace Layout](workspace-layout.md) | Canonical `relurpify_cfg/` contract, ownership rules, and test run layout |
| [Configuration](configuration.md) | Workspace config, agent manifests, skills |
| [Agents](agents.md) | Agent types, modes, when to use each |
| [MCP Architecture](mcp.md) | Capability model, provider lifecycle, security, and MCP client/server design |
| [Nexus Admin API](nexus-admin-api.md) | Canonical Nexus admin service and MCP adapter boundary |
| [Permission Model](permission-model.md) | How the security contract is enforced |
| [TUI](Relurpish_TUI.md) | relurpish interface — panes, keybindings, workflow |
| [Testing](testing.md) | Unit tests, agenttest, recording and replay |
| [External State Store Spec](external-state-store-spec.md) | Workflow persistence rework, step-scoped execution, replay/resume design |
| [relurpify_cfg Plan](relurpify_cfg-plan.md) | Target directory model for workspace state, templates, tests, and tool split |
| [relurpify_cfg Engineering Spec](relurpify_cfg-engineering-spec.md) | Phased implementation plan for layout, doctor flow, tests, and CLI changes |

## Developer Docs

For building on top of the framework — custom agents, custom tools, embedded usage.

| Document | Description |
|----------|-------------|
| [Graph Runtime](dev/graph.md) | State machine workflow engine internals |
| [Custom Tools](dev/tools.md) | Tool interface, built-in tools, writing new ones |
| [Context Budget](dev/context-budget.md) | Token budget management and pruning strategies |
| [Custom Agents](dev/custom-agents.md) | Implementing the Agent interface, library usage |
