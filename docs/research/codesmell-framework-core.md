# Codesmell Notes: `framework/core`

Captured while raising test coverage in `framework/core`.

## Observations

### `framework/core/context.go`

- Execution-path duplication: the type owns state, variables, knowledge, history, compression state, dirty tracking, and snapshotting.
- Maintainability risk: the context object is central to many workflows, so changes in one concern can affect several unrelated behaviors.
- Testability issue: the public API is broad, and the internal locking/copy-on-write behavior makes state transitions hard to isolate.
- Intentional design tradeoff: this shape exists because `agents/blackboard` and graph execution need a single shared blackboard-like abstraction.

### `framework/core/context_budget.go`

- Execution-path duplication: the file mixes the newer allocation/budget engine with legacy token accounting helpers.
- Maintainability risk: the current API surface forces readers to reason about two budget models at once.
- Testability issue: the same package owns policy setup, allocation, compression, listeners, and legacy compatibility behavior.

### `framework/core/agent_spec.go` and overlay helpers

- Execution-path duplication: validation and merge logic repeat similar selector/policy checks across many branches.
- Maintainability risk: the file is large enough that small behavior changes require careful tracing through multiple helper layers.
- Testability issue: the logic is highly data-driven, so it benefits from table coverage but is expensive to understand without dedicated tests.

## Notes

- These are not all defects. Some are deliberate tradeoffs made to keep the framework compatible with multiple agent families and runtime surfaces.
- If a later refactor becomes necessary, the first candidates should be the most repetitive merge/validation helpers, not the context object itself.
