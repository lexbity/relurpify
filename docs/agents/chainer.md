# Chainer Agent

## Synopsis

Chainer is a deterministic linear runtime made of isolated LLM links. Each link
reads only its declared inputs and writes exactly one output key back into
shared state, which makes the workflow easier to reason about and test.

It is deliberately narrower than a general agent loop. Chainer assumes that the
workflow designer already knows the high-level sequence of operations and wants
each step to be small, explicit, and insulated from unrelated context. That
makes it useful for structured prompt pipelines, transformation passes, and
repeatable review flows.

## How It Works

1. The runtime resolves a `Chain` either from a static definition or from a
   task-specific builder.
2. The chain is validated before execution to ensure link contracts are sound.
3. Each link runs in sequence and emits a single output value into shared
   state.
4. Downstream links consume only the keys they explicitly declare.
5. The final result aggregates executed link count and all link outputs that
   were written to shared state.

Chainer is intentionally stricter than ReAct. It is built for predictable
multi-step prompt pipelines rather than open-ended tool-heavy reasoning.

The isolation rule is the key design choice. A link does not implicitly inherit
all prior context; it receives only what its declared inputs expose. That keeps
prompt growth under control and reduces accidental coupling between stages.

## Runtime Behaviour

- Without a checkpoint store, Chainer uses its legacy custom runner.
- When `CheckpointStore` is configured, it upgrades to a
  `framework/pipeline.Runner` so the chain can resume from the last completed
  link.
- Optional budget tracking and compression listeners can react to context size
  pressure during long chains.
- Optional telemetry event recording captures resume events, per-link
  execution, and summary information for later inspection.
- The runtime stores the task instruction in `__chainer_instruction` so adapted
  pipeline stages can build prompts consistently.
- On success it records `chainer.links_executed` and returns the collected link
  outputs in the final result payload.

In practice Chainer behaves more like a typed workflow engine than like an
autonomous agent. If you need exploration, tool-choice reasoning, or dynamic
branching, another paradigm is usually a better fit. If you need stable,
inspectable prompt choreography, Chainer is often the simpler choice.
