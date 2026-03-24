# HTN Agent

## Synopsis

HTN is a hierarchical task network runtime. It decomposes work using an
explicit method library, then executes the primitive leaf tasks through another
workflow executor. The language model does not decide the structure of the
workflow; it only handles the focused leaf work.

This makes HTN one of the most disciplined paradigms in the repo. The runtime
assumes that decomposition knowledge can be encoded ahead of time in methods
and that execution quality improves when the model is constrained to narrow
leaf tasks instead of being asked to invent the entire procedure during a run.

## How It Works

1. The runtime resolves or constructs a method library.
2. The incoming task is classified into an HTN task type if it does not
   already declare one.
3. A matching method is selected from the method library.
4. The selected method is decomposed into a concrete plan of primitive steps.
5. Those primitive steps are executed sequentially through the configured
   primitive executor, usually another generic agent runtime.

If no method matches, HTN can delegate directly to the primitive executor
without decomposition.

That fallback matters operationally because it lets HTN behave gracefully in
mixed environments. You can gradually add methods for the task shapes you want
to formalize without making the runtime unusable for everything else.

## Runtime Behaviour

- HTN initialises its method library and primitive executor during
  `Initialize`.
- Shared workflow surfaces are resolved from memory, and a SQLite-backed
  workflow store can be opened from `CheckpointPath` when needed.
- Retrieval can be hydrated into the task before decomposition so methods and
  primitive steps start with richer workspace context.
- Plan execution uses `graph.PlanExecutor`, with completed step ids recorded in
  `plan.completed_steps` for checkpoint resume support.
- When workflow persistence is enabled, the runtime stores checkpoints, run
  status, operator outcomes, and HTN-specific execution metadata.
- `BuildGraph` is intentionally minimal because actual HTN behaviour depends on
  runtime decomposition, not on a static fixed graph.

In practice HTN gives you a way to encode engineering recipes in code rather
than leaving them implicit in prompts. The more mature the method library
becomes, the less workflow structure depends on model improvisation and the
more predictable decomposition and recovery become.
