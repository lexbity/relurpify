# Architect Agent

## Synopsis

Architect is a plan-then-execute runtime for multi-step work where the system
benefits from an explicit upfront plan and isolated execution of each step. It
is designed to keep planning broad and execution focused.

Compared with a free-form agent loop, Architect trades some flexibility for
better structure. It is useful when a task has enough moving parts that you
want an explicit artifact describing the intended sequence of work, but you do
not want the executor carrying the full planning context into every step.
Instead, the runtime turns one large task into a controlled workflow of smaller
step tasks.

## How It Works

1. The runtime produces a structured plan once, using an embedded
   `PlannerAgent`.
2. Each plan step is converted into a smaller task with a focused instruction,
   expected outcome, and optional verification guidance.
3. Steps are executed sequentially through an embedded `ReActAgent`.
4. After every completed step, the shared state is updated with progress and a
   short step summary.
5. If a step fails, the plan executor can attempt diagnosis and recovery before
   marking the workflow as failed.

Architect separates planning tools from execution tools. The planning side is
typically narrowed to read-only capabilities so the agent can reason about the
workspace without making changes during plan formation.

In practice that separation reduces a common failure mode in autonomous coding
systems: the model begins editing before it has finished understanding the task.
Architect makes planning a distinct phase and lets execution focus on one step
at a time, which improves inspectability and makes step-level recovery more
coherent.

## Runtime Behaviour

- The runtime sets `architect.mode=plan_execute` and records task metadata such
  as the task id, type, and instruction in shared state.
- Planning is handled by a `PlannerAgent`; execution is handled by a fresh
  `ReActAgent` configured in `code` mode.
- The executor runs each step with a compact per-step context instead of
  replaying the full plan conversation on every iteration.
- Completed step ids are tracked in `architect.completed_steps` and mirrored to
  `plan.completed_steps` for compatibility with shared plan execution helpers.
- When `WorkflowStatePath` is configured, workflow state is persisted so long
  runs can resume after interruption.
- When only `CheckpointPath` is configured, the delegated ReAct executor can
  still checkpoint its own step-level execution.
- This runtime is a strong fit for refactors, migrations, and other tasks where
  ordering and recovery matter more than raw speed.

Operationally, Architect behaves like a coordinator more than a single-agent
loop. The planner owns plan shape, the executor owns step completion, and the
workflow state binds them together. That makes it easier to inspect where a run
failed: at plan quality, at a specific execution step, or during verification
of a step outcome.
