# Agents

## Synopsis

An agent is the reasoning layer between your instruction and the tools that act on your code. It decides what to do, in what order, and when to stop. Different agents implement different reasoning strategies — choosing the right one for a task affects both quality and speed.

---

## Why Multiple Agent Types

A single agent type is not optimal for all tasks. Answering a question about code requires different reasoning than planning a refactor, which requires different reasoning than iteratively debugging a failing test. Relurpify ships seven agent types, each tuned for a different pattern:

| Agent | Strategy | Best for |
|-------|----------|----------|
| **CodingAgent** | Euclo runtime (mode/profile routed) | General-purpose day-to-day work |
| **ArchitectAgent** | Plan → step-by-step ReAct with recovery | Multi-step tasks that benefit from an upfront plan |
| **PipelineAgent** | Deterministic typed stages | Structured workflows with declared input/output contracts |
| **PlannerAgent** | Plan generation only | Thinking through a task before acting |
| **ReActAgent** | Thought → Action → Observation loop | Open-ended exploration and tool-heavy tasks |
| **ReflectionAgent** | ReAct + self-critique pass | Tasks where output quality is more important than speed |
| **EternalAgent** | Infinite loop with configurable pacing | Background monitoring or continuous autonomous work |

---

## How Agents Work

### CodingAgent Rollout

`coding` now resolves to the Euclo runtime by default. Euclo is the canonical
coding runtime layer that selects mode, execution profile, retrieval policy,
capability-family routing, verification policy, and proof surfaces before
delegating to generic paradigms such as ReAct, Planner, or Reflection.

For legacy manifests that still declare `implementation: react` under a
`coding` manifest name, the runtime migrates them to Euclo by default during
contract/bootstrap resolution.

Compatibility control:

- set `RELURPIFY_CODING_RUNTIME_COMPAT=legacy-react` to preserve the old
  `coding -> react` behavior for migration/debugging
- omit it, or set `RELURPIFY_CODING_RUNTIME_COMPAT=euclo`, to keep Euclo as
  the default runtime

### The ReAct Loop

Most agents are built on the ReAct (Reason + Act) pattern. At each step the model:

1. **Thinks** — reasons about the current state and what to do next
2. **Acts** — emits a tool call (or decides it's done)
3. **Observes** — receives the tool result and adds it to context
4. **Repeats** until the model produces a final answer with no tool calls

This loop runs inside the graph runtime. Each thought-act-observe cycle is a pass through LLM → Tool → Observation nodes.

Tokens from the LLM are streamed to the TUI as they arrive — you see the agent's reasoning in real time, not just the final answer.

### How Context is Managed

As the loop runs, the context grows: messages, tool results, file contents. Relurpify is tuned for small local models, so the agent does not depend on replaying a long raw transcript. Instead it rebuilds each iteration from compact state: the current step, compressed history, summarized tool outputs, and progressively loaded file or symbol context. The context manager tracks token usage against the model's budget and downgrades lower-priority items to summaries when it runs tight.

### How Tool Calls are Authorised

Before any tool executes, the permission manager checks it against the manifest's declared permissions. The outcome is one of three things: the call proceeds, it is blocked with an error, or it is paused and you are asked (HITL). The agent does not proceed until you respond. See [Permission Model](permission-model.md) for the full details.

---

## Skill System

Skills are a shared policy and guidance layer that shape how agents use runtime
capabilities. They can also contribute prompt and resource capabilities, but
only inside the manifest-defined security envelope.

In the current model, skills can:

- prioritize or narrow capability usage
- contribute prompt capabilities
- contribute resource capabilities backed by contained workspace files
- widen `allowed_capabilities` through skill-declared selectors before final admission
- steer phase-specific behavior
- define verification expectations
- suggest recovery probes when work fails
- provide planning and review hints

Skills cannot:

- bypass manifest permissions
- bypass runtime policy checks or sandboxing

This means skills can change what is visible and callable inside the runtime,
but they still act inside the existing security envelope. They can influence
behavior and contribute admitted prompt/resource surfaces, but they never grant
authority outside manifest-enforced policy.

### Capability selectors

Skills select capabilities through ordered selectors rather than fixed hardcoded
tool lists. A selector can target an exact capability name or match by tags,
with optional exclusions.

Example shape:

```yaml
capability: "go_test"
tags: ["lang:go", "test"]
exclude_tags: ["destructive"]
```

Selector resolution follows these rules:

1. selectors never grant access on their own
2. resolution only considers capabilities already registered and allowed
3. exact capability names win over tag-based matches
4. tag selectors require all listed tags and reject excluded tags

This lets a skill prefer capability families such as tests, linters, formatters,
or language-specific tooling without hardwiring a single tool name into every
agent manifest.

### Phase capability selectors

Skills can define ordered `phase_capability_selectors` so agents know which
capabilities are preferred during different phases of work.

Typical uses include:

- discovery and probing before edits
- editing and refactoring during code changes
- verification after changes
- review or inspection before final output

Because these are selectors rather than grants, they work across different
agent implementations and different workspace capability registries while still
respecting local policy.

### Verification success criteria

Skills can declare `verification.success_capability_selectors` to identify what
counts as a successful verification pass for a task.

This is useful when "verification" should mean more than "run any test." A
language skill can, for example, prefer:

- language-native test runners
- build or compile checks
- lint or static analysis families
- domain-specific health checks

Agents can use these selectors to decide which verification actions matter most
and whether a task has met the skill's expected completion bar.

### Recovery probes

Skills can declare `recovery.failure_probe_capability_selectors` as an ordered
set of probes to run after a failure.

These probes help agents diagnose problems consistently. Common examples are:

- rerunning a focused test
- gathering compiler output
- checking formatting or lint state
- running read-only inspection commands that narrow the fault

Because the probes are ordered, a skill can encode a preferred debugging path
instead of leaving every failure to ad hoc tool choice.

### Planning hints

Skills can also carry planning guidance that helps agents structure work before
they start changing files.

Planning hints may include:

- required discovery or probe steps before editing
- preferred edit capability families
- preferred verification capability families
- reusable step templates for common task shapes

This is especially useful for coding and architect-style flows where the agent
needs to decide whether it has enough context to edit safely and what
verification path should follow.

### Runtime behavior

At runtime, skill policy is resolved against the current capability registry.
Agents consume the resolved result for:

- phase disclosure and tool preference
- verification success matching
- recovery probe ordering
- planning guidance
- review hints

The important constraint is unchanged: the resolved skill policy can only
filter, prioritize, or organize capabilities that the framework has already
admitted and allowed.

---

## ArchitectAgent

ArchitectAgent implements plan-then-execute: it uses PlannerAgent to produce a
multi-step plan, then drives ReActAgent through each step sequentially,
persisting workflow state after every step so interrupted work can resume.

Two capability registries are used:
- **PlannerTools** (read-only) — used during planning; prevents side effects
  while the plan is being developed.
- **ExecutorTools** (full) — used during execution of each plan step.

When a step fails, the agent attempts to diagnose the root cause and recover
before marking the step as failed. Workflow state is persisted to
`SQLiteWorkflowStateStore` so a crash or timeout during a long plan does not
lose completed work.

```bash
relurpish --agent architect
```

---

## PipelineAgent

PipelineAgent executes a deterministic sequence of typed pipeline stages
declared via `framework/pipeline` contracts. Each stage has a `ContractDescriptor`
naming its input key, output key, schema version, and retry policy.

Use PipelineAgent when you need the agent's reasoning process to follow a
fixed structure — for example, the coding stage sequence:
Explore → Analyze → Plan → Code → Verify.

Stage results are persisted after each stage, so interrupted pipelines resume
from the last completed stage rather than starting over.

---

## PlannerAgent

Produces a structured plan — a list of steps with descriptions and expected outcomes — without executing any of them. Use it when you want to review what will happen before an agent touches your code.

The planner has a read-only tool scope: it can read files and search code but cannot write, execute, or call git. Its output is a `Plan` object stored in the shared context.

PlannerAgent supports explicit graph-level system nodes via `core.Config` flags:
- `UseExplicitCheckpointNodes` — inserts `CheckpointNode` steps after plan, execute, verify, and summarize phases instead of using callback-based checkpointing.
- `UseStructuredPersistence` — includes a `PersistenceWriterNode` at completion that writes the plan summary to declarative memory.

```bash
relurpish chat --agent planner
```

---

## ReActAgent

The direct ReAct implementation. Where CodingAgent wraps ReAct with mode-specific prompt decoration and tool scoping, ReActAgent is the lower-level reason/act loop. It rebuilds a compact prompt per iteration, summarizes tool outputs immediately, and exposes only phase-appropriate tools so small models do not waste context on irrelevant state.

ReActAgent supports the same system node flags as PlannerAgent:
- `UseExplicitCheckpointNodes` — inserts a `CheckpointNode` before completion.
- `UseDeclarativeRetrieval` — prepends a `RetrieveDeclarativeMemoryNode` that hydrates relevant project knowledge before the first think step.
- `UseStructuredPersistence` — includes a `PersistenceWriterNode` that persists the final output summary to declarative memory.

Useful for exploratory tasks where you want the model to reason freely across the full tool set.

---

## ReflectionAgent

Wraps another agent (typically ReAct) and adds a self-critique pass after the inner agent produces its result. The model is prompted to review its own output, identify issues, and refine. This runs as a second LLM call.

Use ReflectionAgent when output quality matters more than latency — code reviews, documentation, plans.

---

## EternalAgent

A loop agent that keeps running until you stop it. Each cycle executes an instruction, waits a configurable duration, then repeats. Optional HITL checkpoints at cycle boundaries let you review and redirect before the next iteration.

```bash
relurpish chat --agent eternal
```

Intended for background work: continuous refactoring, watching a test suite, maintaining a living document.

---

## Agent Registry

At startup, the agent registry scans `relurpify_cfg/agents/` for workspace-owned manifest YAML files, validates them, and makes them available by name. The TUI Settings pane (press `4`) lists all discovered agents and lets you switch between them.

To see all available agents:

```bash
go run ./cmd/dev-agent agents list
```

---

## Picking the Right Agent

A few rules of thumb:

- **Day-to-day coding**: `coding-go` (or your language variant) in default mode
- **"What would it take to..."**: `planner` — get a plan first, decide whether to execute
- **Answering questions without side effects**: `coding` in `ask` mode, or `react`
- **Refactoring with quality review**: `reflection`
- **Autonomous background task**: `eternal`

When in doubt, start with the language-specific coding agent. The manifest controls the implementation, permissions, and defaults; the task decides which CodingAgent execution mode runs.

---

## See Also

- [Configuration](configuration.md) — full manifest schema for agent settings
- [Permission Model](permission-model.md) — how tool access is controlled per agent
- [TUI](Relurpish_TUI.md) — switching agents at runtime via the Settings pane
