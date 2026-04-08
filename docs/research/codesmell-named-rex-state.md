# Codesmell: `named/rex/state`

Observed while expanding test coverage in workflow identity and recovery helpers.

## Notes

- `state.go` handles identity derivation, runtime-surface resolution, recovery scanning, and workflow/run seeding in one place.
- `EnsureWorkflowRun` and `RecoveryBoot` each mix policy decisions with durable-store operations.
- The fallback helpers are compact, but the package still serves as a coordination point between envelope normalization and workflow persistence.

## Risk

- Maintainability risk: the file owns multiple responsibilities that tend to change at different times.
- Testability risk: recovery behavior depends on store shape, workflow status, and fallback identity rules, so narrow unit tests are easy to miss without explicit coverage.

## Tradeoff

- This consolidation is reasonable because rex needs a canonical place for workflow identity and recovery bootstrapping before any execution family runs.

