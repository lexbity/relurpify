# Glossary

This glossary defines the project terms that recur across the README, docs, and
package comments.

- `relurpify`: the repository and the overall agent framework.
- `relurpish`: the end-user Bubble Tea TUI shipped in `app/relurpish`.
- `dev-agent-cli`: the internal CLI for agent-test workflows and scaffolding.
- `relurplint`: the workspace lint binary for config, prompts, and recipes.
- `relurpify_cfg/`: the checked-in workspace configuration tree and tool
  manifests used by the runtime.
- `.relurpify_state/`: generated runtime state, logs, telemetry, and workspace
  artifacts. Treat it as output, not source.
- `userconfig`: the configuration load boundary. In practice this is where
  workspace files and environment overrides are resolved.
- `Euclo`: the coding-agent workflow surfaced through `relurpish`.
- `cognitionzoo`: the reasoning-paradigm package family used by the agent
  runtime.
- `ayenitd`: an internal service lifecycle and bootstrap package. It is not a
  standalone user-facing binary.
- `contextstream`: the in-memory stream layer for workspace context changes and
  indexing triggers.
- `jobs`: the job abstraction and persistence layer used for background work.
- `graphdb`: the graph-backed persistence layer used by context and lifecycle
  repositories.
- `Badger`: the embedded key-value store used by the job store and graph
  persistence paths.
- `offline`: the deterministic scripted model backend used for CI and plumbing
  checks.
- `nexus`: an older runtime name that is presently shelved in the runtime code
  and should be treated as stale unless reintroduced by a verified change.

When a term is used in a doc or comment but does not appear here, prefer adding
it here instead of repeating an ad hoc definition.
