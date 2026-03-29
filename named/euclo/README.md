# Euclo

Euclo is the named coding runtime built on top of `/agents`, `/archaeo`, and `/framework`.

It is organized around a few explicit layers:

- `core`
  Stable Euclo-owned contracts, IDs, mode/profile descriptors, artifact helpers, and relurpic capability metadata that other Euclo packages share.

- `runtime`
  Execution lifecycle, `UnitOfWork` assembly, policy resolution, context/restore/reporting state, transitions, and orchestration support.

  Current subfolders:
  - `runtime/work`: `UnitOfWork`, compiled execution, envelopes, lifecycle, deferrals, and edit execution support.
  - `runtime/policy`: classification, routing, verification/security policy, and shared-context policy summaries.
  - `runtime/context`: context-management runtime and context lifecycle support.
  - `runtime/restore`: workflow/runtime surfaces plus provider/session snapshot persistence and restore.
  - `runtime/reporting`: observability, chat/debug runtime reporting, and final-report helpers.
  - `runtime/transitions`: `UnitOfWork` transition state and history.
  - `runtime/archaeomem`: Archaeo-associated semantic inputs, archaeology runtime state, and semantic reasoning helpers.
  - `runtime/orchestrate`: controller, recovery, and session-dispatch glue.

  The root `runtime` package still holds shared types plus a small number of cross-cutting runtime helpers that have not yet been split further. It is no longer the place where relurpic behavior lives.

- `execution`
  Euclo-side execution-paradigm adapters over `/agents` primitives such as React, Planner, HTN, and Reflection. These are substrate-level runners, not Euclo behavior definitions.

- `relurpicabilities`
  Euclo-owned relurpic capability definitions and concrete mode-scoped behaviors.

  Current mode groups:
  - `relurpicabilities/chat`
  - `relurpicabilities/debug`
  - `relurpicabilities/archaeology`

  This package owns:
  - capability IDs and descriptors
  - supporting-capability relationships
  - mode-scoped concrete relurpic behavior
  - the primary behavior implementations for:
    - `chat.ask`
    - `chat.inspect`
    - `chat.implement`
    - `debug.investigate`
    - `archaeology.explore`
    - `archaeology.compile-plan`
    - `archaeology.implement-plan`

## Relurpic Capability Behavior

Euclo’s concrete relurpic behavior is implemented in the mode packages:

- `relurpicabilities/chat/behavior.go`
- `relurpicabilities/debug/behavior.go`
- `relurpicabilities/archaeology/behavior.go`

The separation is:
- `relurpicabilities/*` defines and implements Euclo’s mode-owned relurpic behavior.
- `runtime/orchestrate/behavior_service.go` is only the dispatcher that resolves a `UnitOfWork` primary owner to the correct mode behavior.
- `execution/*` provides reusable execution-paradigm adapters and shared behavior helpers over `/agents`.

That keeps capability vocabulary, behavior ownership, and paradigm execution separate.

## Directory Intent

The intended ownership model is:

- `/framework` owns capability primitives, policy, sandbox/security enforcement, provider interfaces, and shared runtime substrate.
- `/archaeo` owns memory, provenance, living-plan state, and knowledge relationships.
- `/agents` owns generic execution paradigms.
- `named/euclo` owns coding-runtime orchestration, modal behavior, `UnitOfWork`, and Euclo-specific relurpic capabilities.

## Migration Note

This tree replaces the old Euclo package layout. The runtime split is underway; several files have already been moved into `policy`, `reporting`, and `restore`, and the remaining root `runtime` files are the next cleanup target rather than the intended long-term shape.
