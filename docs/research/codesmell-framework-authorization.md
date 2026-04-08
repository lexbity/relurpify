# Codesmell Notes: `framework/authorization`

Captured while extending coverage across permissions, policy translation, and HITL flow.

## Observations

### Permission manager breadth

- Maintainability risk: `PermissionManager` owns filesystem, executable, network, capability, IPC, task grants, and HITL approval logic in one type.
- Testability issue: the class mixes stateful caches, policy translation, and runtime enforcement hooks, which makes each code path harder to reason about independently.
- Execution-path duplication: several checks follow the same pattern of lookup, default-policy fallback, optional HITL escalation, and audit logging.

### HITL lifecycle

- Maintainability risk: the broker manages request registration, waiting, approval, denial, timeouts, and event broadcasting in one place.
- Testability issue: the async approval flow is correct but easy to break because it depends on the interaction between goroutines, timers, and request bookkeeping.

### Policy translation

- Execution-path duplication: policy compilation and policy-request fallback logic both encode trust/risk/provider/session branching with similar but not identical rules.
- Maintainability risk: the package has a high surface area of small helper functions, so changes to policy semantics can be easy to miss unless tests are very targeted.

## Notes

- These are mostly deliberate tradeoffs: authorization is a cross-cutting subsystem, so it naturally centralizes policy decisions.
- The biggest risk is not one function, but divergence between the manager, the policy engine, and the HITL broker.
