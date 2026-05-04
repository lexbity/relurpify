# Framework

## Synopsis

`framework/` is the shared runtime foundation used by agents, app entry points,
and platform code. It owns the canonical contracts for manifests, agent
specs, capabilities, authorization, sandboxing, workflow execution, context
assembly, memory, persistence, retrieval, search, telemetry, and workspace
analysis.

At a technical level, the framework is where the runtime's enforcement
surface is defined. It resolves the effective agent spec, compiles context and
context policy, admits capabilities, enforces permissions, and provides the
execution envelope used by graph-based agents. Higher layers consume these
contracts; they do not redefine them.

The current tree does not split this area into separate `pipeline/`,
`middleware/`, or `contextmgr/` packages. Those responsibilities are spread
across packages such as `agentgraph`, `contextdata`, `contextstream`,
`contextpolicy`, `compiler`, `capability`, `authorization`, and `sandbox`.

The dependency rule is simple:

- `framework/` defines enforcement-critical runtime contracts
- `agents/` and `named/` consume those contracts
- `framework/` must not import `agents/`

That boundary is checked by `scripts/check-framework-boundaries.sh`.

For the higher-level architecture and layering rules, see
[architecture.md](architecture.md) and [layering.md](layering.md).

---

## Package Map

### Contracts and composition

| Package | Role |
|--------|------|
| `core` | Shared runtime types and compatibility aliases for tasks, tools, providers, policies, capabilities, permissions, and state boundaries. |
| `agentspec` | Agent-spec models, overlay merge logic, selector evaluation, and effective runtime-spec composition. |
| `manifest` | Parses agent and skill manifests, resolves workspace paths under `relurpify_cfg/`, and expands skill references. |
| `contextpolicy` | Compiles manifest and skill context policy into a runtime `ContextPolicyBundle`. |
| `contextbudget` | Live token accounting, budget snapshots, and session reset advisories for compiler-aware LLM calls. |
| `agentenv` | Defines `WorkspaceEnvironment`, the composition-root object produced by `ayenitd.Open()`. |
| `capability` | Capability registry, dispatch, tool formatting, provenance wrapping, and runtime-family handling. |
| `authorization` | Allow/Ask/Deny policy enforcement, HITL handling, command authorization, and delegation checks. |
| `sandbox` | Command-execution policy and backend-neutral runner abstractions, including filesystem scoping helpers. |

### Execution and context

| Package | Role |
|--------|------|
| `agentgraph` | Deterministic workflow runtime with node contracts, preflight, checkpointing, branch merging, and system nodes. |
| `contextdata` | Execution envelope for working memory, streamed context, and retrieval references. |
| `contextstream` | Compiler-triggered context streaming requests, triggers, and job primitives. |
| `compiler` | Live context assembly, caching, replay, audit, and event-driven invalidation. |
| `contextmetric` | Artifact-budget and context-budget telemetry helpers. |

### State and persistence

| Package | Role |
|--------|------|
| `memory` | Durable workflow state, runtime memory, checkpoint snapshots, message/vector stores, and hydrators. |
| `persistence` | Schema and adapter boundaries for persisted runtime data. |
| `graphdb` | Embedded graph engine for durable traversal-oriented storage. |
| `agentlifecycle` | Workflow, run, delegation, lineage, and runtime lifecycle records. |
| `event` | Shared event log used across the runtime and audit surfaces. |
| `telemetry` | Local structured audit trail and event-log bridge. |
| `jobs` | Job boundaries, worker lifecycle, and scheduler-facing types. |

### Workspace intelligence

| Package | Role |
|--------|------|
| `ast` | Go and Markdown parsing plus workspace symbol indexing. |
| `search` | File glob and content search helpers. |
| `retrieval` | Ranking and retrieval utilities for knowledge selection. |
| `knowledge` | Semantic chunk substrate, freshness, invalidation, and graph helpers. |
| `summarization` | Code and prose summarizers used during context compaction. |
| `ingestion` | Workspace scanning, file parsing, and typed ingestion pipeline for files, LLM output, and tool results. |
| `templates` | Prompt-template resolution and caching. |
| `skills` | Skill manifest expansion, selector resolution, and skill capability registration. |
| `patterns` | Durable pattern, comment, and gap storage types. |
| `perfstats` | Lightweight counters for framework-level runtime behavior. |

---

## Selected Packages

### `core`

`core` is the shared contract layer for the rest of the framework. It
contains the common runtime types used across graph execution, capabilities,
permissions, tasks, tools, and provider metadata.

Important detail: `core` is not the only place where domain types live. Many
framework concerns are now owned by more specific packages such as
`agentspec`, `manifest`, `contextpolicy`, `agentgraph`, `memory`, and
`compiler`.

Technical notes:

- Most cross-package compatibility aliases live here so older call sites can
  continue compiling while the ownership model moves to more specific
  packages.
- State-boundary types, capability selectors, policy classes, and task/context
  shapes are defined here because they are used by multiple subsystems.
- `core` is intentionally broad, but it should be treated as a contract layer
  rather than a place for behavior-heavy logic.

### `agentspec`

`framework/agentspec` composes the effective agent runtime spec from manifests,
skill contributions, overlays, and runtime overrides. It is the package to use
when you need the resolved agent configuration rather than a raw manifest.

Technical notes:

- `AgentDefinition` and `AgentRuntimeSpec` are the primary entry points for
  loading and validating agent configuration.
- Runtime behavior is normalized before execution: tool-calling intent,
  capability selectors, permission levels, provider policies, runtime safety,
  browser/LSP/search flags, and composition metadata are all resolved into one
  effective spec.
- This package owns the compatibility layer for merging agent definitions and
  runtime overlays without forcing callers to manually reconcile all of the
  input sources.
- It is the right layer when you need the final runtime intent, not the raw
  manifest text.

### `manifest`

`framework/manifest` is responsible for the on-disk agent and skill manifests
under `relurpify_cfg/`. It also owns the canonical workspace path layout used
to locate `config.yaml`, `agent.yaml`, `skills/`, `logs/`, `telemetry/`,
`sessions/`, `memory/`, and the other runtime data files.

Technical notes:

- The package validates the manifest version and shape before the runtime uses
  any of the declared settings.
- It resolves workspace-relative paths into the canonical `relurpify_cfg/`
  structure so runtime code can stay path-agnostic.
- Skill manifests are not just metadata; they are resolved into concrete
  runtime contributions that later participate in capability and policy
  admission.
- This package is about declaration and resolution, not execution.

### `contextpolicy`

`framework/contextpolicy` compiles the context policy section from the manifest
and any resolved skills into a runtime bundle. That bundle captures policy
defaults, rankers, scanners, summarizers, quota and rate-limit settings, and
skill contributions.

Technical notes:

- The compiled bundle is the runtime-facing object used by context assembly
  and context admission.
- Manifest defaults are applied first, then skill contributions are merged in,
  so the final policy reflects both the agent definition and resolved skill
  surface.
- The package carries the knobs that control budget behavior, trust handling,
  degraded content handling, and substitution policy when context is short on
  space.
- It exists to make context assembly deterministic and inspectable.

### `capability`

`framework/capability` owns the registry for already-admitted capabilities.
It handles local tools, prompt capabilities, resource capabilities, and
provider-backed or higher-order runtime families through the same registry and
policy machinery.

Technical notes:

- The registry is the runtime lookup point for capabilities an agent may
  actually invoke.
- Dispatch is gated by policy, trust, effect class, and provenance; the
  registry wraps outcomes in a result envelope so the caller can preserve
  insertion and audit metadata.
- Tool formatting lives here because transport backends need a normalized tool
  schema, even though the capability itself may originate elsewhere.
- Runtime families are distinct from capability kinds, which lets the system
  treat local tools, prompts, resources, and opinionated higher-order flows
  consistently.

### `authorization`

`framework/authorization` enforces the runtime policy model. It decides
whether a requested action is allowed, needs human approval, or is denied,
but it does not itself launch processes or provide OS-level isolation.

Technical notes:

- This package implements the Allow / Ask / Deny boundary used by tool calls
  and other privileged actions.
- It is responsible for HITL request flow, command authorization, and
  delegation checks, but the actual command execution remains in `sandbox`.
- The enforcement logic consumes the effective runtime contract rather than
  the raw manifest so runtime overlays and admitted capability state are
  reflected correctly.

### `sandbox`

`framework/sandbox` abstracts command execution. It defines the backend-neutral
policy contract and the filesystem-scoping helpers used by sandbox-aware tools.

Technical notes:

- The package is intentionally backend-neutral so the same policy surface can
  be applied to local execution and sandboxed execution paths.
- It owns the pre-execution checks that keep protected filesystem roots out of
  reach before a host command is launched.
- Higher layers decide which backend is active; `sandbox` defines the policy
  and enforcement mechanics.

### `agentgraph`

`framework/agentgraph` is the deterministic workflow runtime used by agents.
Nodes declare execution contracts, preflight validates required capabilities,
checkpointing records transition-boundary state, and parallel branches merge
only explicit state changes back into the parent context.

Technical notes:

- `WorkflowExecutor` is the execution contract that higher-level agents
  implement.
- `NodeContract` captures side-effect class, idempotency, placement,
  checkpoint policy, recoverability, trust/risk ceilings, and the allowed
  state boundary for a node.
- Default contracts vary by node type, which lets the runtime enforce different
  replay and state-access rules for tool nodes, human nodes, terminal nodes,
  and streaming or observation nodes.
- Checkpoints are transition-boundary records. Resume starts at the next node,
  which prevents re-running completed single-shot steps.
- Branch merges are explicit and conservative: only branch-local state writes
  that do not conflict are merged back.
- The package also exposes the compiler-trigger interface used when graph
  execution needs fresh assembled context.

### `contextdata`

`framework/contextdata` defines the execution envelope threaded through graph
nodes. It carries working memory, streamed context, and retrieval results
without duplicating the underlying data.

Technical notes:

- The envelope is the shared in-flight execution state for graph nodes.
- It separates mutable working memory from streamed context and retrieval
  references so the runtime can keep provenance and mutation rules explicit.
- Branch clone and merge semantics are defined around the envelope, not around
  ad hoc maps, so the runtime can control which changes are safe to propagate.
- This package exists to make context flow cheap to copy and precise to merge.

### `contextstream`

`framework/contextstream` models compiler-triggered context streaming. It
provides the request and job primitives used when execution asks the compiler
to assemble or refresh context.

Technical notes:

- This package is about orchestration, not assembly.
- It gives graph execution a way to request a refreshed or filtered context
  view from the compiler without directly coupling nodes to compiler internals.
- Keeping the trigger/job types separate from the assembled envelope makes the
  control plane explicit.

### `compiler`

`framework/compiler` assembles runtime context from retrieval, knowledge, and
policy state. It caches compilation results, emits replay records, and applies
event-driven invalidation.

Technical notes:

- Compilation is staged: ranker admission, scatter, fusion, trust filtering,
  freshness filtering, budget fitting, and result emission.
- The compiler keeps enough metadata to replay a prior assembly and compare it
  against current state.
- Budget pressure can trigger substitution of chunks with summaries so the
  runtime can stay within the configured token window.
- Caching is keyed by the query, manifest, policy bundle, and event-log
  sequence so the compiled result remains deterministic and inspectable.
- This package is the bridge between retrieval, knowledge storage, and the
  execution envelope.

### `memory`

`framework/memory` owns durable workflow state and runtime memory. It
includes the stores and adapters used for checkpoints, declarative and
procedural memory, message history, vectors, and workflow records.

Technical notes:

- The package distinguishes between transient working memory and durable
  runtime records.
- Declarative memory is for facts, decisions, constraints, and similar
  long-lived knowledge.
- Procedural memory is for reusable routines and capability compositions.
- Hydrators convert retrieved memory records back into either a raw state map
  or an execution envelope, which keeps the graph runtime decoupled from the
  underlying store implementation.
- Checkpoint storage is treated as a first-class runtime concern so interrupted
  execution can resume without replaying the entire workflow.

### `persistence`

`framework/persistence` defines the adapter boundary for persisted runtime
data. The domain packages depend on these narrow interfaces instead of talking
to a specific backend directly.

Technical notes:

- This package defines schema and adapter boundaries, not domain semantics.
- It exists to keep compiler, lifecycle, and memory packages backend-agnostic
  while still allowing durable storage to be implemented elsewhere.
- The shape of the adapter is intentionally narrow so the storage backend can
  evolve without forcing the domain packages to absorb backend details.

### `ast`, `search`, `retrieval`, `knowledge`

These packages work together to let agents find and rank workspace content
without loading entire files into context. `ast` indexes symbols, `search`
locates files and content, `retrieval` ranks candidates, and `knowledge`
provides the durable semantic chunk substrate.

Technical notes:

- `ast` is the symbol-oriented entry point for language-aware workspace
  navigation.
- `search` is the coarse file and content locator that helps narrow the working
  set before more expensive analysis runs.
- `retrieval` ranks and filters candidate chunks so the compiler can assemble a
  bounded context window.
- `knowledge` stores the semantic artifacts that survive across runs and feed
  retrieval and compilation.

### `summarization`, `ingestion`, `templates`, `skills`

These packages support prompt construction and context reduction. `ingestion`
scans and parses workspace content, `summarization` compresses large artifacts,
`templates` resolves prompt templates, and `skills` expands skill manifests
into capabilities and resource contributions.

Technical notes:

- `ingestion` converts workspace files into typed inputs for downstream
  analysis and chunking.
- `summarization` exists so large artifacts can be downsampled without losing
  the structure the runtime needs to keep working.
- `templates` is the prompt assembly layer for reusable text inputs.
- `skills` is the bridge between skill manifests and runtime-admitted
  capabilities, including prompt and resource contributions.

---

## Runtime Flow

The packages are designed to be used in a predictable sequence:

1. `manifest` loads the declared agent and skill inputs.
2. `agentspec` resolves those inputs into one effective runtime spec.
3. `contextpolicy` compiles the policy bundle that controls context
   admission and budget behavior.
4. `capability` admits the callable surface the agent may use.
5. `authorization` and `sandbox` enforce the action boundary for privileged
   operations.
6. `agentgraph` executes the workflow and requests compiled context as
   needed.
7. `compiler`, `retrieval`, `knowledge`, `ast`, `search`, and `memory`
   cooperate to assemble, store, and refresh the working set.

That flow is the reason the package boundaries matter: the runtime stays
inspectable only if contract resolution, admission, execution, and persistence
remain separate.

## Practical Reading Order

If you are trying to understand the framework from the code outward, read the
packages in this order:

1. `core`
2. `agentspec`
3. `manifest`
4. `contextpolicy`
5. `capability`
6. `authorization`
7. `sandbox`
8. `contextdata`
9. `agentgraph`
10. `compiler`
11. `memory`

That sequence follows the flow from shared contracts, to policy, to execution,
to durable state.
