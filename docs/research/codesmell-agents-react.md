# Codesmells: `agents/react`

## Observations

- `react.go` is a large orchestration surface that handles initialization, graph construction, phase selection, context policy wiring, checkpointing, and result post-processing. The behavior is understandable in isolation, but the file is difficult to audit because the execution flow fans out through many hidden helper branches.
- `react_observe_node.go` contains multiple failure-analysis and recovery heuristics with overlapping responsibilities. Some helpers are clearly testable, but the repeated inference logic makes it easy for path duplication to creep in.
- `prompt_context.go` accumulates formatting, extraction, and warning-generation helpers in one place. This is convenient for prompt assembly, but it also creates several small transformation paths that are easy to drift apart.

## Tradeoff Note

- The monolithic shape appears intentional because the ReAct agent has to coordinate graph execution, tool permissions, memory, summarization, and recovery policy in one runtime. The main risk is maintainability and testability, not that the package is incorrectly structured for its purpose.
