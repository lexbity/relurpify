# Test Coverage Remediation Plan

## Objective
Raise the in-scope Go codebase to roughly:

- `~80%` overall statement coverage, excluding `app/relurpish/**` and `platform/shell/**`
- `~90%` coverage for most in-scope packages

Current baseline from a full `go test ./... -coverprofile` run:

- Repo-wide coverage: `57.0%`
- Coverage excluding `app/relurpish/**` and `platform/shell/**`: `58.6%`

The gap is not evenly distributed. A small number of large packages dominate the missing statements, so the plan is to work from highest-leverage infrastructure outward rather than chase package count.

## Scope And Constraints

- In scope: all Go packages except the explicitly excluded rework surfaces below.
- Out of scope for coverage targets right now:
  - `app/relurpish/**`
  - `platform/shell/**`
- Codesmell notes are required as work proceeds.
  - Write one file per package or area.
  - Use `docs/research/codesmell-<package-or-area>.md`.
  - Record only execution-path duplication, maintainability risk, and testability issues.
  - If a design is intentional because another package depends on it, note that as a tradeoff instead of a defect.

## Prioritization Criteria

1. Prefer packages with both high statement volume and low coverage.
2. Prefer packages that are core dependencies of many other packages.
3. Prefer test additions over production refactors unless the code is blocking testability.
4. Prefer deterministic unit tests before broad integration tests.
5. Keep the work aligned with existing architecture boundaries. Do not collapse package ownership just to increase coverage.

## Phase 1: `framework/core` Stabilization

Why first:

- It is central to agent state, context management, budget accounting, compression, and spec validation.
- It is a dependency of many agent/runtime layers, so every improvement here compounds.
- The package has many dark helper paths that are testable without changing behavior.

Target areas:

- `framework/core/context.go`
- `framework/core/context_history.go`
- `framework/core/context_budget.go`
- `framework/core/compressed_context.go`
- `framework/core/agent_spec_overlay.go`
- `framework/core/agent_spec_merge.go`
- `framework/core/capability_policy_eval.go`
- `framework/core/capability_types.go`

Test strategy:

- Add focused unit tests for state snapshotting, deep-copy semantics, history trimming, conflict logging, and LLM prompt rendering.
- Add budget tests that exercise borrowing, compression, listener notifications, and legacy token accounting.
- Add strategy tests that cover compression parsing, prompt construction, and error branches.
- Add validation and merge tests for the helper paths that still have zero or low coverage.

Expected outcome:

- Move `framework/core` materially upward from the current `58.2%`.
- Reduce the number of zero-coverage helper functions that are currently concentrated in the core package.
- Establish the test helper patterns that later phases can reuse.

Known codesmell notes to log during this phase:

- `framework/core/context.go` is intentionally broad because it serves the blackboard and graph execution model, but it bundles state, history, compression, and dirty tracking into one type.
- `framework/core/context_budget.go` mixes the newer allocation engine with legacy accounting helpers, which creates duplicated reasoning paths.
- `framework/core/agent_spec.go` and overlay helpers are large, repetitive validation/merge surfaces that are difficult to reason about as one monolith.

## Phase 2: Shared Policy And Middleware Layer

Priority packages:

- `framework/capability`
- `framework/authorization`
- `framework/middleware/fmp`
- `framework/contextmgr`
- `framework/memory`
- `framework/memory/db`

Why next:

- These packages encode policy, routing, mediation, and memory retention rules that are used by multiple agents and apps.
- They currently have substantial uncovered branches in helper methods, validation rules, and state transitions.

Test strategy:

- Add matrix-style tests for policy selection, selector normalization, permission evaluation, and insertion decisions.
- Add concurrency and listener-focused tests only where they prove a correctness edge.
- Add persistence tests that verify store contracts, not just happy-path writes.
- Add state transition tests for memory and context-management flows that are currently only partially covered.

Expected outcome:

- Bring the shared infrastructure closer to the `80%+` band and eliminate several low-coverage hot spots that affect downstream packages.

## Phase 3: Runtime Surfaces For Named Agents

Priority packages:

- `named/euclo/**`
- `named/rex/**`
- `agents/htn/**`
- `agents/react`
- `agents/rewoo`
- `agents/relurpic`

Why next:

- These packages define the runtime behavior of the main agent systems.
- Many of them already have pockets of strong coverage, but the large orchestrators, state machines, and transition code still have significant gaps.

Test strategy:

- Add transition-focused tests for runtime controllers, dispatchers, orchestrators, and recovery paths.
- Add contract tests for state handoffs and environment-shaping logic.
- Add scenario tests only where the unit surface is too intertwined to isolate without losing signal.

Expected outcome:

- Push most runtime subpackages into the `80-90%` range and reduce the size of the uncovered execution graph.

## Phase 4: Application, Persistence, And Integration Boundaries

Priority packages:

- `app/nexus/**`
- `app/nexus/db`
- `app/nexus/admin`
- `framework/graphdb`
- `framework/retrieval`
- `archaeo/**`
- `ayenitd`
- `platform/browser`
- `platform/lsp`
- `platform/lang/*`
- `platform/fs`

Why last:

- These areas are larger integration boundaries and often depend on the lower-level packages above.
- They benefit from the more focused helper patterns created in earlier phases.

Test strategy:

- Add repository-backed store tests for persistence layers.
- Add adapter tests for server entry points and protocol translation.
- Add domain scenario tests where the business logic is too cross-cutting for narrow unit tests.

Expected outcome:

- Raise the remaining non-excluded packages toward the target without destabilizing the lower layers.

## Phase 5: Coverage Sweep And Guardrails

Goal:

- Close the remaining largest uncovered packages after the high-value remediation work above.

Actions:

- Re-run the full coverage profile and rank packages by uncovered statements, not just percentage.
- Add targeted tests to the next two or three largest gaps.
- Document any deliberately deferred packages or rework surfaces.
- If a package has changed shape enough during the work, refresh its codesmell note.

Exit criteria:

- In-scope coverage is roughly `80%` overall.
- Most packages are at or near `90%`.
- The remaining exceptions are consciously documented and limited to areas with known redesign pressure.
