# Eternal Agent

## Synopsis

Eternal is a long-running looped execution runtime intended for continuous or
background-style work. It repeatedly prompts the model, feeds the previous
response back into the next cycle, and stops only on cancellation or configured
cycle limits.

It is not a planning runtime and it is not primarily a code-editing runtime. It
is closer to a continuous simulation or ambient reasoning loop. That makes it
useful for experiments, autonomous background processes, long-lived monitoring
style workflows, or any case where the value comes from continued iteration
rather than from finishing a bounded task quickly.

## How It Works

1. The runtime starts from the task instruction, or a default bootstrap prompt
   if no instruction is provided.
2. It prepends a fixed system prompt and calls the model in a loop.
3. The generated response becomes the seed prompt for the next cycle.
4. Output can be streamed live through a callback supplied in task context.
5. The loop continues until the context is cancelled or the configured cycle
   budget is exhausted.

This runtime is intentionally simple. It does not plan, decompose, or use a
specialised graph workflow for each turn.

That simplicity is part of the contract. Eternal is not trying to emulate the
more structured recovery and workflow semantics of Planner, HTN, or Pipeline.
It exists to keep a process alive, generate output incrementally, and respond
to runtime cancellation and pacing controls.

## Runtime Behaviour

- Safe defaults are conservative: `Infinite` is disabled by default and
  `MaxCycles` defaults to a single cycle unless overridden.
- `eternal.infinite`, `eternal.max_cycles`, and `eternal.sleep` can be supplied
  through `task.Context` to control loop behaviour at runtime.
- `MaxTokensPerCycle` limits each generation call, while `ResetDuration`
  defines when the runtime should reset its internal cycle window.
- If `task.Context["stream_callback"]` contains a compatible callback, tokens
  are streamed as they are generated.
- Generated responses are appended to interaction history in the shared state.
- `BuildGraph` is only a minimal placeholder for visualisation; the real
  behaviour lives in the execution loop itself.

Operationally, the main thing to understand is that Eternal is replay-sensitive
and context-light. It does not attempt to preserve a complex resumable workflow
graph across iterations. If you need robust resumability or explicit
side-effect tracking, a more structured runtime is the better operational
choice.
