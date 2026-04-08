# Codesmell Notes: `framework/contextmgr`

Captured while adding phase-2 coverage around context selection and pruning.

## Observations

### Strategy duplication

- Execution-path duplication: `AggressiveStrategy`, `ConservativeStrategy`, and `AdaptiveStrategy` each reimplement similar decision shapes for `SelectContext`, `ShouldCompress`, `DetermineDetailLevel`, and `ShouldExpandContext`.
- Maintainability risk: the three strategy personalities drift easily because the same concept is encoded three times with slightly different thresholds.
- Testability issue: the behavior is table-driven but spread across multiple types, so a missing branch can hide in one strategy even when the others are well tested.

### Context policy orchestration

- Maintainability risk: `ContextPolicy` ties together budget enforcement, progressive loading, summary generation, signal handling, and graph-memory publication logic.
- Testability issue: the policy depends on many collaborators, so narrow tests are easy to write only for helper functions, while higher-level behavior is harder to isolate.
- Intentional tradeoff: this package is acting as a control surface for repo-wide context behavior, so some breadth is expected.

### Pruning helpers

- Execution-path duplication: pruning/compression strategies encode overlapping prioritization heuristics in separate forms.
- Maintainability risk: selection logic is correct in small tests, but the scoring and ordering rules are complex enough that a future change could alter behavior subtly.

## Notes

- Most of the complexity here is justified by the need to support multiple context-loading personalities.
- If the package is ever refactored, the first candidates are the repeated strategy thresholds and scoring rules, not the helper functions that are already small and stable.
