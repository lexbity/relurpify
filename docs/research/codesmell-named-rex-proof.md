# Codesmell: `named/rex/proof`

Observed while expanding coverage in proof and completion gating.

## Notes

- `proof.go` combines proof-surface synthesis, verification-evidence normalization, and completion-gate evaluation in one file.
- `EvaluateCompletion` threads policy, evidence, and state mutation together, which makes the control flow hard to reason about at a glance.
- The evidence helpers are individually small, but the file contains enough fallback behavior that branch coverage is easy to miss without deliberate tests.

## Risk

- Maintainability risk: proof generation and completion gating evolve together here, so changes to one can subtly affect the other.
- Testability risk: state-driven fallbacks are numerous, especially around manual verification and absent evidence.

## Tradeoff

- This coupling is reasonable because rex needs a canonical proof surface and completion gate at the same boundary where execution finishes.

