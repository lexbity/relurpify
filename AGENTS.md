# Agents

Relurpify ships five agent types, each suited to different tasks.

| Agent | Key | Best for |
|-------|-----|----------|
| **CodingAgent** | `coding` | General-purpose code work (read, write, debug, explain, plan) |
| **PlannerAgent** | `planner` | Producing structured plans before execution |
| **ReActAgent** | `react` | Open-ended reasoning with iterative tool use |
| **ReflectionAgent** | `reflection` | Self-correcting tasks where output quality matters |
| **EternalAgent** | `eternal` | Long-running autonomous loops |

CodingAgent supports five modes via `spec.agent.mode` in the manifest:
`code` · `architect` · `ask` · `debug` · `docs`

Language-specific coding agent manifests are typically copied into `relurpify_cfg/agents/` from shared templates. Once copied, the workspace versions are authoritative.

For full details see [docs/agents.md](docs/agents.md).

