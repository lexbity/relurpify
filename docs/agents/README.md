# Agents

## Synopsis

This section documents the generic execution paradigms implemented under
`agents/`. These are the reusable runtime patterns that named agents and
workspace manifests can build on top of.

The `coding` agent name is intentionally not documented here as a generic
paradigm. In the current runtime it resolves to the named Euclo coding runtime,
which composes and constrains these generic patterns for day-to-day code work.

The important distinction is that these documents are about execution models,
not end-user presets. A manifest or named agent might expose a familiar name
such as `coding`, `rex`, or a workspace-specific agent profile, but underneath
it still needs a concrete control scheme. The files in this section explain
those control schemes directly: what kind of reasoning they perform, how they
sequence work, and what they persist while running.

## Named Agents

Named agents are top-level specialized agents in `named/` that own their
complete control scheme. They compose generic paradigms rather than
reimplementing them. See [framework/layering](../framework/layering.md) for the
full four-layer architecture.

| Agent | Domain | Documentation |
|-------|--------|--------------|
| **Euclo** | Primary coding orchestrator — classifies tasks, selects modes, gates phases | [euclo.md](euclo.md) |
| **Rex** | Nexus-managed distributed runtime with delegate routing | — |
| **Eternal** | Long-lived background execution | [eternal.md](eternal.md) |
| **TestFu** | Agent test automation | — |

## Generic Agent Runtimes

| Agent | Purpose |
|-------|---------|
| Architect | Plan once, then execute each step with a focused executor |
| Blackboard | Shared-state specialist dispatch driven by evolving task state |
| Chainer | Deterministic linear chain of isolated LLM stages |
| Eternal | Continuous or long-lived looped execution |
| GoalCon | Deterministic backward-chaining goal planning |
| HTN | Hierarchical decomposition using a method library |
| Pipeline | Typed multi-stage execution using pipeline contracts |
| Planner | Explicit plan, execute, verify, summarize workflow |
| ReAct | Thought, action, observation loop for open-ended work |
| Reflection | Delegate execution followed by iterative self-review |
| ReWOO | Plan once, execute tools mechanically, synthesize once |

## How Agents Fit Into Relurpify

The framework layer owns manifests, config resolution, capability admission,
permission policy, and skill resolution. The agent layer owns concrete
execution behaviour once a runtime has already been selected and wired with an
`agentenv.AgentEnvironment`.

That separation matters operationally. When you change a manifest, you are
usually changing which runtime is chosen, which capabilities are admitted, or
what prompt and policy surfaces are available to it. When you change code under
`agents/`, you are changing the actual execution semantics: whether the model
plans first, loops iteratively, dispatches specialists, decomposes tasks into
recipes, or runs a fixed stage pipeline.

At runtime, most generic agents receive some combination of:

- a language model
- a capability registry
- memory or workflow persistence surfaces
- optional retrieval and search services
- a shared `core.Context` for state exchange

Those dependencies are then combined into a specific execution pattern.

Most runtime documents in this section follow the same structure:

- `Synopsis`: what the paradigm is for and why you would choose it
- `How It Works`: the logical flow of execution
- `Runtime Behaviour`: the implementation-level behavior that matters in
  practice, such as persistence, checkpoints, context layout, and resume rules
