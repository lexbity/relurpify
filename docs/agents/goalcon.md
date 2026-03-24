# GoalCon Agent

## Synopsis

GoalCon is a deterministic goal-planning runtime built around backward
chaining. It models work as goal satisfaction, searches for a plan that can
establish the desired conditions, and then executes the resulting primitive
steps.

This runtime is most natural when the problem domain can be expressed in terms
of predicates, operators, preconditions, and effects. Instead of asking the
model to improvise a workflow from scratch, GoalCon asks a more constrained
question: what sequence of valid operators would make the goal true from the
current state?

## How It Works

1. The runtime classifies the incoming instruction into a goal condition, using
   an LLM-backed classifier with a deterministic operator registry.
2. It creates a world state from the configured initial conditions.
3. A backward-chaining solver searches for a plan that can satisfy the goal
   using registered operators.
4. The resulting plan is executed step by step through a delegated
   `graph.WorkflowExecutor`.
5. Execution metrics and provenance are recorded for later inspection.

GoalCon is best when the problem can be expressed in terms of explicit
preconditions, effects, and a finite operator library.

That makes it especially good for domains where correctness depends on
following well-defined transformations or preapproved procedures. It is less
appropriate for exploratory work where the space of possible actions cannot be
encoded cleanly as operators.

## Runtime Behaviour

- If no operator registry is supplied, the runtime constructs a default one.
- Classifier configuration, metrics recording, and audit trail support are
  initialised automatically during `Initialize`.
- The runtime stores goal, plan, unsatisfied predicates, and search depth under
  `goalcon.*` keys in shared state.
- Plan execution uses `graph.PlanExecutor`, with completed step ids tracked in
  `plan.completed_steps`.
- If no plan steps are needed, GoalCon delegates directly to its leaf executor.
- A provenance summary is attached to the final result and can also be written
  to the configured memory store for audit or quality analysis.

The audit and metrics surfaces are part of the runtime, not just add-ons. A
GoalCon run is intended to produce a traceable explanation of which operators
were selected, how deep the search went, and how successful those operators
were over time. That makes it suitable for workflows where repeatability and
post-run analysis matter.
