# Planner Agent

## Synopsis

Planner is an explicit plan-first runtime. It asks the model for a structured
plan, executes tool-backed work against that plan, verifies the outcome, and
then summarizes the result. It is both a useful runtime and a reference
implementation for building other multi-step agents.

Planner is simpler than Architect and less exploratory than ReAct. It keeps the
full workflow inside one explicit graph and is useful when you want planning,
execution, verification, and summarisation to remain visible as distinct phases
of a single run.

## How It Works

1. The planner node prompts the model for a machine-readable plan.
2. The execution node walks the plan and invokes the relevant tools or
   delegated actions.
3. A verification node checks whether the plan outcome appears complete.
4. A summarisation node condenses the plan, execution results, and verification
   outcome into final state.
5. Optional persistence nodes can write summary artifacts and workflow records.

Unlike Architect, Planner owns the entire workflow itself rather than handing
step execution to a separate specialised runtime.

That makes Planner easier to study and easier to reuse as a baseline runtime.
If you are trying to understand how multi-step agent workflows are assembled in
Relurpify, Planner is one of the clearest concrete examples.

## Runtime Behaviour

- Planner runs as an explicit graph with plan, execute, verify, summarize, and
  optional persistence/checkpoint nodes.
- If a capability registry is available, it is attached to the graph as the
  capability catalog for inspection and execution.
- The runtime stores the generated plan under `planner.plan` and writes related
  outputs such as `planner.results`, `planner.summary`, and
  `planner.skipped_tools`.
- Checkpointing can be enabled either through explicit graph checkpoint nodes or
  through file-backed checkpoint saving at execution time.
- Structured persistence can write planner summaries and related artifacts into
  runtime and workflow stores when those surfaces are available.
- Planner is especially useful when a task needs an explicit, inspectable plan
  artifact rather than an implicit model-controlled loop.

In operational terms, Planner gives you a clean midpoint between loose agentic
reasoning and fully fixed pipelines. It allows model-generated task structure,
but once that structure exists the rest of the run follows an explicit graph
with visible execution and verification boundaries.
