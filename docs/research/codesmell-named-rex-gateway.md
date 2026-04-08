# Codesmells: `named/rex/gateway`

## Observations

- `gateway.go` mixes identity derivation, event classification, workflow lookup, and signal validation in one adapter surface. The helpers are small, but the file is doing both routing and state-verification work.
- `validateSignalEvent` carries multiple concerns at once: workflow existence checks, run status checks, event-type-specific validation, and trust gating. That is practical, but it makes the behavior easy to under-test if a new branch is added later.

## Tradeoff Note

- The shape is likely intentional because rex needs a single deterministic gateway between external events and workflow execution. The main risk is branch duplication and maintainability, not the overall ownership boundary.
