# Codesmell: `named/rex` root agent

Observed while expanding coverage in the root rex agent.

## Notes

- `agent.go` owns environment initialization, route resolution, runtime projection, managed-adapter construction, reconciliation helpers, and persistence hooks.
- `Execute` is the main orchestration path, but it now depends on several lower-level helper functions that are spread across the file.
- `persistProof` and `persistContextExpansion` are straightforward on their own, but the file mixes them with execution, adaptation, and workspace discovery logic.

## Risk

- Maintainability risk: this is the broadest orchestration surface in the rex package, so changes to one behavior can easily affect several others.
- Testability risk: the file spans both high-level execution and low-level persistence details, which makes it easy to miss edge-case branches without targeted unit tests.

## Tradeoff

- The breadth is understandable because the root agent is the integration point that ties together routing, proof, runtime, and reconciliation for rex.

