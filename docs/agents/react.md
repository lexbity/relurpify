# ReAct Agent

## Synopsis

ReAct is the general-purpose reasoning loop used for open-ended work. It
alternates between model reasoning, tool execution, and observation gathering
until the runtime reaches a final answer or hits its iteration limit.

This is the most familiar agent shape in the repo and the one other paradigms
often embed or delegate to. When people think of an "agent" deciding what to
do next based on tool results, they are usually describing some version of the
ReAct loop.

## How It Works

1. A think step asks the model what to do next.
2. An act step turns that decision into a tool invocation or a completion
   decision.
3. An observe step records tool output and folds it back into runtime context.
4. A decision layer routes the workflow back into another iteration or into
   completion.
5. Recovery logic handles malformed model output or failed tool calls.

This is the most flexible generic runtime and the default fallback for many
named agents.

That flexibility comes with tradeoffs. ReAct is excellent when the workflow
cannot be known ahead of time, but it is less predictable than a fixed pipeline
or a method-driven HTN decomposition. It is best used when iterative
exploration is part of the problem rather than an implementation accident.

## Runtime Behaviour

- ReAct initialises a capability registry if one is not already provided and
  defaults to a bounded iteration count when none is configured.
- Prompt construction is rebuilt from compact state on each iteration rather
  than replaying an unbounded transcript, which keeps the runtime usable with
  smaller local models.
- Capability metadata is inserted into prompts so the model can choose from the
  currently admitted tools.
- Retrieval, search, and index manager surfaces can be attached so the runtime
  can enrich prompts with workspace context.
- Optional checkpointing can persist loop state through `CheckpointPath`.
- ReAct is the right baseline when the workflow shape is not known in advance
  and the task requires iterative exploration.

Operationally, ReAct lives or dies on context discipline. The implementation is
therefore optimized around rebuilding a compact prompt each cycle from shared
state, summaries, and selected context rather than naively replaying an ever-
growing transcript. That is especially important for the local-model-focused
environment this project targets.
