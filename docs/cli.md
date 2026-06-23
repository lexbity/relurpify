# CLI

This document covers the three user-facing binaries that currently matter for local work.

The binary build command was verified in this workspace. The runtime command
examples below are documented invocation shapes, not all re-run here.

## `relurpish`

End-user Bubble Tea TUI for the Relurpify runtime.

Root flags:

```text
--workspace <dir>
--inference-endpoint <url>
--inference-model <name>
--inference-provider <ollama|lmstudio|offline>
--offline
--sandbox-backend <gvisor|docker>
--runsc <path>
--container-runtime <name>
--sandbox-platform <hint>
```

Subcommands:

- `doctor` checks local readiness and can materialize starter config.
- `status` boots the runtime and opens the TUI status view.
- `chat` boots the runtime and opens the main chat flow.

`doctor` flags:

```text
--fix
--yes
--set-provider <name>
```

Notes:

- `--offline` is sugar for the offline inference backend.
- `doctor --fix` can write starter workspace configuration into `relurpify_cfg/`.
- `status` and `chat` both boot the runtime before entering the TUI.

## `dev-agent-cli`

Internal development CLI for agent tests and related workflows.

Root flag:

```text
--workspace <dir>
```

Available subcommands:

- `agenttest run`
- `agenttest promote`
- `agenttest report`
- `agenttest rerecord`

`agenttest run` flags that are most useful in practice:

```text
--suite <path>
--agent <name>
--case <name>
--tag <tag>
--tier smoke|stable|live-flaky|quarantined
--profile live|record|replay|developer-live|ci-live|ci-replay
--strict
--include-quarantined
--out <dir>
--sandbox
--timeout <duration>
--bootstrap-timeout <duration>
--skip-ast-index
--max-retries <n>
--model <name>
--endpoint <url>
--max-iterations <n>
--debug-llm
--debug-agent
--backend-reset none|model|server
--backend-bin <path>
--backend-service <name>
--backend-reset-between
--backend-reset-on <regex>
```

`agenttest promote` flags:

```text
--suite <path>
--agent <name>
--run <dir>
--case <name>
--all
```

`agenttest report` and `agenttest rerecord` use `--suite` and `--agent`.

## `relurplint`

Workspace validation binary for configuration, tool manifests, recipes, and prompts.

Flags:

```text
--check all|config|tools|recipes|prompts|comma,separated,list
--workspace <dir>
--format text|json
```

Checks:

- `config`
- `tools`
- `recipes`
- `prompts`

`--check all` runs every registered check.
