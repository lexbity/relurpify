# ReWOO Agent

## Synopsis

ReWOO is a small-model-friendly plan-once runtime. It minimizes model calls by
doing all high-level reasoning up front, executing the resulting tool plan
mechanically, and then calling the model again only once to synthesize the
final answer.

The core idea is to separate reasoning from execution more aggressively than
ReAct does. Rather than repeatedly thinking after each tool call, ReWOO commits
to a plan, carries it out, and only returns to the model once the tool outputs
have been collected.

## How It Works

1. A planning phase uses a single model call to produce a structured ReWOO
   plan.
2. The execution phase walks that plan and invokes tools without further model
   reasoning during the intermediate steps.
3. Intermediate outputs are captured and bound to step references for later
   use.
4. A final synthesis phase uses one more model call to turn the plan outputs
   into the user-facing result.
5. Optional recovery and replanning helpers can assist when execution diverges
   from the original plan.

This pattern is intended for environments where model context and iterative
reasoning budget are limited.

That makes it a strong fit for smaller local models or workflows where you want
low variance across runs. The tradeoff is that a bad initial plan can constrain
the rest of the workflow more than it would in a loop that reasons after every
observation.

## Runtime Behaviour

- ReWOO initialises shared runtime surfaces such as capability registry,
  context policy, workflow state, and permission defaults before execution.
- The workflow is represented as a graph with explicit plan, step, checkpoint,
  aggregate, and synthesis nodes.
- Workflow state can be persisted through a dedicated checkpoint store so the
  runtime can resume after interruption.
- Retrieval and telemetry hooks can be attached to enrich planning and
  execution while keeping the high-level reasoning pattern fixed.
- Because tools are executed mechanically after planning, failures tend to be
  easier to localise than in a free-form ReAct loop.
- ReWOO is most useful when you want low-variance execution with very few LLM
  calls.

Operationally, ReWOO sits between a pipeline and a free-form agent loop. It
still lets the model decide the high-level plan, but once execution begins the
workflow becomes much more predictable and easier to replay, checkpoint, and
inspect.
