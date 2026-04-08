# Codesmell: `named/rex/runtime`

Observed while expanding test coverage in the runtime manager.

## Notes

- `runtime.go` mixes queue management, worker execution, recovery scanning, and health reporting in a single manager type.
- `executeItem` is tiny, but it sits inside a loop that already owns scheduling, queue depth accounting, and recovery state.
- `scanRecoveries` couples the runtime health model directly to workflow-store recovery boot logic.

## Risk

- Maintainability risk: the manager owns several orthogonal concerns, which makes the execution path harder to reason about as features accumulate.
- Testability risk: queue and recovery behavior interact through shared mutable state, so tests need to be deliberate about which path they’re exercising.

## Tradeoff

- The manager-centric shape is practical because rex needs one runtime coordination object that can manage health, recovery, and worker dispatch together.

