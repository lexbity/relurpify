# Documentation Index

> **Audience**: everyone; this page routes you and defines the vocabulary.
> **Purpose**: one-line descriptions of every doc, the core glossary, and reading paths.
> **Prerequisites**: none.

## The doc set

| Doc | Description | Audience |
|---|---|---|
| [../README.md](../README.md) | Project pitch, features, install, quick start | Everyone |
| [ARCHITECTURE.md](ARCHITECTURE.md) | System shape, enforced layering rules, boot and turn lifecycle, state locations, decision records | Framework developers |
| [GETTING_STARTED.md](GETTING_STARTED.md) | Prerequisites, build, workspace init, model configuration, environment variables, troubleshooting | New users and developers |
| [RELURPISH.md](RELURPISH.md) | Daily use of the terminal UI: tabs, commands, approvals, postures, state inspection | Users of the coding agent |
| [RECIPE_DSL.md](RECIPE_DSL.md) | Authoring thoughtrecipes: triggers, tool scopes, steps, captures, families | Agent and workflow authors |
| [PROMPT_DSL.md](PROMPT_DSL.md) | Prompt file format, front matter, the prompt registry, overrides | Agent and workflow authors |
| [CONFIGURATION.md](CONFIGURATION.md) | The `relurpify_cfg/` tree: workspace settings, providers, profiles, security policies, tool manifests, validation rules | Users and framework developers |
| [../CONTRIBUTING.md](../CONTRIBUTING.md) | DCO sign-off, architecture gates, coverage floor, commit style, release process | Contributors |
| [TESTING.md](TESTING.md) | Test layers, agent suites, execution profiles, tapes, conformance, coverage | Contributors |
| [../CHANGELOG.md](../CHANGELOG.md) | Release history (Keep a Changelog) | Everyone |
| [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) | Community rules and enforcement contact | Everyone |

## Reading paths

- "I want to use the coding agent": [GETTING_STARTED](GETTING_STARTED.md) then [RELURPISH](RELURPISH.md).
- "I want to author workflows": [RECIPE_DSL](RECIPE_DSL.md) and [PROMPT_DSL](PROMPT_DSL.md).
- "I want to work on the framework": [ARCHITECTURE](ARCHITECTURE.md), then [CONTRIBUTING](../CONTRIBUTING.md) and [TESTING](TESTING.md).
- "Something broke": [GETTING_STARTED troubleshooting](GETTING_STARTED.md#troubleshooting), then [RELURPISH](RELURPISH.md).

## Glossary

These terms are used with exactly these meanings in every doc.

| Term | Meaning |
|---|---|
| Relurpify | This framework; the repository. |
| relurpish | The terminal UI binary; also runs `doctor` and `status`. |
| Euclo | The built-in coding agent that relurpish runs. |
| workspace | The project directory the agent works in; the agent acts only inside it. |
| `relurpify_cfg/` | The workspace configuration tree; materialized once by doctor, then owned by you. |
| `.relurpify_state/` | The runtime state directory: logs, telemetry, audit chain, jobs, memory, artifacts. Output only, never source. |
| capability | Anything the model may do; declared by a descriptor with trust class, effect classes, and schemas. |
| sandbox | Container-isolated command execution; gVisor or Docker backend. |
| HITL | Human-in-the-loop: an approval request that pauses a run for your decision. |
| thoughtrecipe | A declarative `.erpe` workflow that Euclo routes to and executes; "recipe" for short. |
| intent family | One of nine keyword-classified instruction groups Euclo routes on (debug, review, …). |
| paradigm | A reusable reasoning strategy (ReAct, planner, blackboard, ReWOO, …) that recipes can delegate to. |
| knowledge chunk | The memory atom: content plus provenance, freshness, and typed links into the graph store. |
| context compiler | The deterministic stage that assembles what the model sees each turn under a token budget. |
| envelope | The per-task state carrier that flows through the agent graph. |

Secondary terms are defined on first use: **doctor** (readiness check), **relurplint** (workspace linter), **dev-agent** (agent test CLI), **relurpify-runner** (background job runner), **tape** (LLM record/replay fixture), **agenttest** (YAML-driven end-to-end suite), **selection posture** (deterministic or model_assisted intent disambiguation).
