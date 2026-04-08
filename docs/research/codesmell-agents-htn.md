# Codesmells: `agents/htn`

## Observations

- `htn_agent.go` does a lot of work in one execution flow: decomposing the task, resuming checkpoints, publishing runtime state, dispatching primitive steps, persisting step outcomes, and updating workflow artifacts. The flow is deliberate, but the branching around checkpointing and persistence is dense and easy to misread.
- The checkpoint-compaction helpers (`compactHTNCheckpoint`, `compactHTNCheckpointMap`, `compactHTNCheckpointState`) are small but repetitive. They encode the same shape conversion in multiple forms, which is a maintainability risk if the checkpoint schema changes.
- `recordingPrimitiveAgent` is a wrapper with mirrored forwarding methods and its own persistence side effects. That duplication is mostly structural, but it makes behavior harder to test because the side effects are split across wrapper layers.

## Tradeoff Note

- The design is probably intentional because HTN needs to bridge a planning surface with primitive execution and workflow persistence. The package is not obviously over-abstracted; it is just carrying several responsibilities in one coordination layer.
