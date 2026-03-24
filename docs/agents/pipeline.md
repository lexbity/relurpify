# Pipeline Agent

## Synopsis

Pipeline is a deterministic typed-stage runtime built on
`framework/pipeline`. Each stage declares a contract, receives explicit inputs,
and produces structured output for the next stage. It is the most rigid of the
generic runtimes.

That rigidity is the point. Pipeline is for workflows where the order of
operations is already known and where correctness depends on stage boundaries
being explicit. It is closer to a workflow engine than to a free-form agent.

## How It Works

1. The runtime resolves stages from a static list, a stage builder, or a stage
   factory.
2. A pipeline runner executes the stages in order.
3. Each stage reads declared inputs, performs its work, and emits typed output.
4. Stage results are collected and can be persisted for recovery.
5. The final result contains the full stage history and the last decoded stage
   output.

Pipeline is a good fit for workflows that already have a known stage model,
such as Explore -> Analyze -> Plan -> Code -> Verify.

Because each stage is contract-driven, Pipeline works well when teams want the
runtime to enforce stable hand-offs between stages instead of depending on
prompt conventions alone. It is a strong choice for repeatable internal
workflows, codified delivery paths, and resumable staged processing.

## Runtime Behaviour

- `Execute` requires both a task and a configured language model.
- Workflow persistence can be enabled through `WorkflowStatePath`; when
  configured, the runtime creates or resumes workflow and run records.
- Retrieval can be hydrated from the workflow store and merged into task
  context before the stage sequence starts.
- When persistence is enabled, the runner checkpoints after each stage and can
  resume from `ResumeCheckpointID`.
- Capability invocation for model-callable tools is provided through the
  configured capability registry.
- `BuildGraph` returns a visual representation of the stage order only; it is
  not the executable path for real pipeline runs.

Operationally, Pipeline gives you the clearest failure boundaries of any
generic runtime. A run fails at a specific stage with a specific contract and
specific decoded output. That makes it straightforward to debug, retry, or
replace individual stages without redesigning the entire workflow.
