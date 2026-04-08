# Codesmell: `named/rex/reconcile`

Observed while expanding test coverage in reconciliation helpers.

## Notes

- `reconcile.go` combines in-memory reconciliation state, protected-write fencing, and outbox history in a single package.
- `FMPBackedReconciler` layers ownership-ground-truth lookups on top of the base reconciler, which is pragmatic but introduces multiple nested decision paths.
- `attemptRetryable` and `ValidateProtectedWrite` are small, but they sit inside a file that already handles several separate retry/fencing concepts.

## Risk

- Maintainability risk: the file is the local home for too many related-but-distinct state machines.
- Testability risk: retryability depends on both local reconcile state and optional FMP attempt views, so tests need to control several layers at once.

## Tradeoff

- The combined file shape is understandable because rex needs a single reconciliation boundary that can stay usable even when FMP integration is not available.

