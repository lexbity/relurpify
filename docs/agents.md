# Agents

## Synopsis

An agent is the reasoning layer between your instruction and the tools that act on your code. It decides what to do, in what order, and when to stop. Different agents implement different reasoning strategies — choosing the right one for a task affects both quality and speed.

---

## Why Multiple Agent Types

A single agent type is not optimal for all tasks. Answering a question about code requires different reasoning than planning a refactor, which requires different reasoning than iteratively debugging a failing test. Relurpify ships five agent types, each tuned for a different pattern:

| Agent | Strategy | Best for |
|-------|----------|----------|
| **CodingAgent** | Adaptive (delegates by mode) | General-purpose day-to-day work |
| **PlannerAgent** | Plan generation only | Thinking through a task before acting |
| **ReActAgent** | Thought → Action → Observation loop | Open-ended exploration and tool-heavy tasks |
| **ReflectionAgent** | ReAct + self-critique pass | Tasks where output quality is more important than speed |
| **EternalAgent** | Infinite loop with configurable pacing | Background monitoring or continuous autonomous work |

---

## How Agents Work

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

## CodingAgent

The primary agent for interactive development work. It delegates to specialised strategies based on `mode`:

| Mode | Strategy | What it focuses on |
|------|----------|--------------------|
| `code` | ReAct | Reading, editing, and creating files; running tests and builds |
| `architect` | Candidate select → Plan → step-scoped ReAct | Chooses an approach for branchy tasks, then executes one step at a time with compact per-step context |
| `ask` | ReAct (cautious) | Answers questions without modifying files |
| `debug` | ReAct | Diagnostic focus: runs tests, reads stack traces, searches code |
| `docs` | ReAct (write-focused) | Writes or updates documentation files |

The mode is set in the manifest under `spec.agent.mode`. In `architect` mode the caller can also request resume-from-checkpoint via task context so interrupted long plans continue from the latest saved step. Language-specific manifests are provided for Go, Rust, Python, Node.js, and SQLite in `relurpify_cfg/agents/`. These differ in their skill stacks, declared executables, and system prompts.

**Selecting a language-specific agent:**

```bash
relurpish chat --agent coding-go
relurpish chat --agent coding-rust
```

---

## PlannerAgent

Produces a structured plan — a list of steps with descriptions and expected outcomes — without executing any of them. Use it when you want to review what will happen before an agent touches your code.

The planner has a read-only tool scope: it can read files and search code but cannot write, execute, or call git. Its output is a `Plan` object stored in the shared context.

```bash
relurpish chat --agent planner
```

---

## ReActAgent

The direct ReAct implementation. Where CodingAgent wraps ReAct with mode-specific prompt decoration and tool scoping, ReActAgent is the lower-level reason/act loop. It rebuilds a compact prompt per iteration, summarizes tool outputs immediately, and exposes only phase-appropriate tools so small models do not waste context on irrelevant state.

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

At startup, the agent registry scans `relurpify_cfg/agents/` for manifest YAML files, validates them, and makes them available by name. The TUI Settings pane (press `4`) lists all discovered agents and lets you switch between them.

To see all available agents:

```bash
go run ./cmd/coding-agent agents list
```

---

## Picking the Right Agent

A few rules of thumb:

- **Day-to-day coding**: `coding-go` (or your language variant) in default mode
- **"What would it take to..."**: `planner` — get a plan first, decide whether to execute
- **Answering questions without side effects**: `coding` in `ask` mode, or `react`
- **Refactoring with quality review**: `reflection`
- **Autonomous background task**: `eternal`

When in doubt, start with the language-specific coding agent. Its mode can be changed in the manifest or overridden in the Settings pane.

---

## See Also

- [Configuration](configuration.md) — full manifest schema for agent settings
- [Permission Model](permission-model.md) — how tool access is controlled per agent
- [TUI](tui.md) — switching agents at runtime via the Settings pane
