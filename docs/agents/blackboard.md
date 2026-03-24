# Blackboard Agent

## Synopsis

Blackboard is a data-driven coordination runtime. Instead of following a fixed
execution order, it maintains a shared blackboard and lets specialised
knowledge sources decide what should run next based on the current state of the
work.

This is the least linear of the generic paradigms. Rather than encoding a plan
or stage sequence up front, Blackboard assumes that the shape of the work will
emerge as facts accumulate. It is therefore best suited to tasks where
investigation, planning, execution, and verification need to influence one
another continuously.

## How It Works

1. The runtime creates or resumes a shared blackboard state for the task.
2. A controller evaluates knowledge-source activation conditions each cycle.
3. The most relevant specialist is dispatched to read from and write to the
   blackboard.
4. Newly written facts, hypotheses, plans, or failures can activate different
   specialists on the next cycle.
5. The controller continues until the task is complete, blocked, or reaches a
   terminal summarisation step.

The built-in knowledge-source lifecycle follows an engineering-oriented flow:
Explorer, Analyzer, Planner, Review, Executor, FailureTriage, Verifier, and
Summarizer.

That lifecycle is not a rigid stage pipeline. Those specialists exist as
knowledge sources that can become eligible at different times depending on what
is already known, what is blocked, and what still needs verification. The same
specialist may run multiple times across a workflow if the evolving state makes
it relevant again.

## Runtime Behaviour

- Blackboard runs on the graph runtime rather than a hidden package-private
  loop, so controller cycles, checkpoints, persistence, and telemetry are
  represented through framework execution surfaces.
- The shared `core.Context` carries blackboard state, controller decisions, and
  any persisted workflow metadata needed for recovery.
- Built-in knowledge sources can be combined with custom ones, or replaced
  entirely if a workflow needs a different specialist set.
- Capability usage is mediated through the capability registry passed into the
  runtime; knowledge sources do not bypass framework permission checks.
- Retrieval and structured persistence can be attached to the workflow so
  specialist decisions have access to hydrated context and resumable state.
- Blackboard is useful when the correct next step depends on intermediate
  discoveries rather than on a predetermined stage order.

From an operator perspective, Blackboard gives you a more inspectable version
of dynamic specialist dispatch. The blackboard state becomes the authoritative
record of why a particular specialist ran, what it contributed, and what new
conditions it created for the next controller cycle.
