# Layering Rules

## Synopsis

Relurpify has three relevant ownership layers for runtime composition:

- `framework/` owns enforcement-critical schemas, resolution, and policy surfaces
- `agents/` owns concrete agent implementations and post-contract runtime behavior
- `app/` owns product/runtime bootstrap and user-facing assembly

The key rule is dependency direction, not just package placement.

## Boundary Rules

`framework/` may own:

- manifest schemas and loaders
- workspace/global config schemas and loaders
- effective contract resolution
- skill resolution when it changes the sandbox envelope or admitted capability set
- capability selector admission
- policy compilation and authorization inputs

`framework/` must not import:

- `github.com/lexcodex/relurpify/agents`
- `github.com/lexcodex/relurpify/agents/...`

`agents/` may own:

- concrete agent implementations
- runtime defaults that do not change the sandbox envelope
- agent-specific planning, prompting, and execution behavior
- adapters that consume framework-native contracts

`app/` may own:

- runtime/bootstrap wiring
- convenience adapters for CLI/TUI/server entry points
- temporary migration shims at the product edge

## Practical Test

Before placing logic in `framework/`, ask:

- Does this determine the final sandbox envelope?
- Does this determine the final admitted capability set?
- Does this determine the compiled policy surface?

If the answer is yes, it belongs in `framework/`.

If the answer is no, it likely belongs in `agents/` or `app/`.

## Current Refactor Rule

For the framework/agents boundary cleanup, the immediate invariant is:

- no package under `framework/` may import `agents`

This repository enforces that invariant with `scripts/check-framework-boundaries.sh`.

It also enforces a migration rule for application and tests:

- `app/` and `testsuite/` must not call deprecated `agents` wrappers for
  framework-owned config, contract, or skill-resolution logic

That rule is enforced with `scripts/check-deprecated-agent-wrappers.sh`.
