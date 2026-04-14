# Euclo

Euclo is the named coding runtime built on top of `/framework`, `/archaeo`, and `/agents`.

The code is organized around four Euclo-owned layers.

## Directory Structure

### `core`

Stable Euclo definitions shared across the runtime:
- mode and artifact aliases
- capability and runtime IDs
- cross-package type aliases and small contracts

### `runtime`

Runtime ownership and lifecycle:
- `UnitOfWork`
- policy, classification, and contract state
- context and restore lifecycle
- transitions and continuity
- runtime reporting
- session and behavior dispatch

Current runtime subpackages:
- `runtime/work`
- `runtime/policy`
- `runtime/context`
- `runtime/restore`
- `runtime/reporting`
- `runtime/transitions`
- `runtime/archaeomem`
- `runtime/orchestrate`

The root `runtime` package still holds shared runtime types and some cross-cutting helpers used by those subpackages.

`runtime/orchestrate` is internally split into:
- profile execution coordination
- recovery strategy handling
- interactive execution and transitions
- observability and persistence recording

### `execution`

Euclo-side execution substrate over `/agents`.

This layer owns:
- paradigm adapters for `react`, `planner`, `htn`, `reflection`, `architect`, `pipeline`, `chainer`, `rewoo`, and `blackboard`
- named recipe execution used by Euclo relurpic behavior
- shared behavior trace, artifact merge, routine execution, and verification helpers

Boundary:
- `/agents` owns the generic paradigms
- `named/euclo/execution` owns Euclo’s recipe-level use of those paradigms

### `relurpicabilities`

Euclo-owned relurpic capability catalog and behavior implementation.

Mode groups:
- `relurpicabilities/chat`
- `relurpicabilities/debug`
- `relurpicabilities/archaeology`
- `relurpicabilities/local`

Ownership split:
- `chat`, `debug`, and `archaeology` own primary mode behavior
- `local` holds reusable subordinate relurpic capabilities that can stay separate while also being invoked under a primary owner

## Primary Relurpic Owners

Primary behavior ownership lives in:
- `relurpicabilities/chat/behavior.go`
- `relurpicabilities/debug/behavior.go`
- `relurpicabilities/archaeology/behavior.go`

Current primary owners:
- `euclo:chat.ask`
- `euclo:chat.inspect`
- `euclo:chat.implement`
- `euclo:debug.investigate-repair`
- `euclo:archaeology.explore`
- `euclo:archaeology.compile-plan`
- `euclo:archaeology.implement-plan`

These owners:
- execute supporting routines explicitly
- compose `/agents` paradigms through `named/euclo/execution`
- record recipe and subordinate-capability execution in the Euclo behavior trace
- publish runtime and reporting state through the runtime layer

## Supporting And Local Relurpic Capabilities

Supporting relurpic capabilities are implemented as explicit routines plus reusable local capabilities.

Examples:
- chat local review and targeted verification routines
- debug root-cause, localization, flaw-surface, and verification-repair routines
- archaeology pattern, prospective, convergence, and coherence routines
- reusable local capabilities such as:
  - `euclo:design.alternatives`
  - `euclo:trace.analyze`
  - `euclo:refactor.api_compatible`
  - `euclo:review.implement_if_safe`
  - migration and artifact-transform capabilities

Those local capabilities remain separate where that separation is useful, but they run through the same recipe layer and can be composed under a primary owner.

Examples of local capability substrate use:
- `euclo:migration.execute` now runs staged execution through `execution/pipe` over `/agents/pipeline`
- trace and regression investigation use `execution/blackboard`
- `euclo:debug.investigate-repair` uses `execution/pipe` for deterministic investigation-summary and repair-readiness postpasses
- `euclo:chat.inspect` uses `execution/chainer` for deterministic inspect-summary and compatibility-summary postpasses
- `euclo:chat.implement` now attempts staged Architect execution for broad cross-cutting work before falling back to the standard implement flow
- `euclo:archaeology.implement-plan` now attempts plan-bound execution through `execution/rewoo` before falling back to manual step execution
- API-compatible refactor and review-guided implementation use the shared recipe layer

## Behavior Dispatch And Execution

`runtime/dispatch/dispatcher.go` dispatches from `UnitOfWork` to the correct primary relurpic owner.

It is not where the behavior is implemented.

The ownership model is:
- `runtime` selects and manages work
- `relurpicabilities/*` owns concrete Euclo behavior
- `execution/*` runs named recipes over `/agents`

The main behavior trace is carried in `euclo.relurpic_behavior_trace` and now records:
- supporting routine execution
- executed recipe IDs
- specialized subordinate capability IDs
- behavior path metadata

## External Ownership Boundary

- `/framework` owns capability primitives, policy resolution, sandbox and permission enforcement, provider interfaces, and shared runtime substrate.
- `/archaeo` owns memory, provenance, living-plan state, and knowledge relationships.
- `/agents` owns generic execution paradigms.
- `named/euclo` owns coding-runtime orchestration and Euclo-specific relurpic behavior.

## Current State

Euclo now has:
- explicit supporting routines
- concrete chat/debug/archaeology primary owners
- archaeology exploration, compile, and implement behavior that no longer collapse into thin workflow wrappers
- recipe-level execution reporting that reflects the actual behavior path
- assurance-aware runtime reporting with separate `result_class` and `assurance_class`
- executed-verification enforcement for fresh edits
- bounded failed-verification repair
- a real TDD red/green/refactor lifecycle
- semantic review gating for automatic mutation
- verification-plan and waiver artifacts in final reporting
- orchestrate seams that separate profile execution, recovery, interactive flow, and observability

## Current Runtime Guarantees

For normal mutating flows, Euclo now guarantees:

- fresh edits are not proven by fallback or reused verification evidence
- semantic review blocks automatic mutation when critical findings are present
- verification plans and executed checks are surfaced explicitly
- failed verification enters bounded repair rather than soft success
- TDD completion requires explicit red and green evidence
- final reporting includes both lifecycle outcome and assurance level

Those guarantees are enforced in runtime/assurance/reporting code and covered by
unit, behavior, and runtime-surface tests under `named/euclo/...`.

## Remaining Work

The main remaining work is now narrower:

- continue expanding higher-level agenttest/live scenario coverage
- keep sorting shared runtime helpers into the subpackages
- collapse any remaining helper wrappers that no longer buy clarity
- finish documentation and reporting cleanup where older wording remains
