# TUI And API Surface Engineering Specification

## Status

Largely implemented for the current slice, with follow-on UX refinement still pending

## Goal

Define how Relurpify exposes capability-oriented runtime state to users and
external clients once the capability/runtime, provider/session, MCP, and agent
coordination work is in place.

This specification should describe how runtime-owned structures become
inspectable and manageable without inventing a separate presentation-only
model.

## Scope

This specification covers:

- capability inspection surfaces
- provider and session visibility
- workflow, delegation, and resource inspection
- structured result rendering
- approval and HITL visibility
- HTTP API exposure of capability/runtime metadata

This specification does not define:

- low-level transport or protocol mechanics
- detailed schema internals
- final visual design
- policy bypasses or out-of-band execution paths

## Relationship To The Other Specifications

This document is downstream of:

- [`1_capability-model-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/1_capability-model-spec.md)
- [`3_provider-runtime-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/3_provider-runtime-spec.md)
- [`4_mcp-core-integration-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/4_mcp-core-integration-spec.md)
- [`5_agent-coordination-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/5_agent-coordination-spec.md)

The TUI/API surface should therefore assume:

- capabilities are the primary inspectable unit
- providers and sessions are runtime-owned inspectable state
- workflow resources and delegations are first-class inspectable objects
- trust, exposure, insertion, provenance, and recoverability metadata matter at
  the presentation layer
- TUI and HTTP API should project the same runtime model rather than divergent
  bespoke ones
- MCP protocol endpoints remain MCP-native, while Relurpify's own HTTP/TUI
  surfaces expose MCP-backed runtime state through normal capability/provider/session
  inspection rather than a separate MCP admin protocol

## Current State Review

Relurpify already has:

- a TUI with panes for tasks, tools, settings, and chat
- runtime-family-aware capability inspection in
  [`app/relurpish/tui/runtime_adapter.go`](/home/lex/Public/Relurpify/app/relurpish/tui/runtime_adapter.go)
- workflow inspection through the TUI adapter, including delegations,
  transitions, promoted artifacts, linked resources, providers, and provider
  sessions in
  [`app/relurpish/tui/runtime_adapter.go`](/home/lex/Public/Relurpify/app/relurpish/tui/runtime_adapter.go)
- command-driven HITL inspection in
  [`app/relurpish/tui/commands.go`](/home/lex/Public/Relurpify/app/relurpish/tui/commands.go)
- an HTTP API server for task execution and workflow inspection in
  [`server/api.go`](/home/lex/Public/Relurpify/server/api.go)

Current gaps:

- approval UX still needs richer interactive flows beyond the current structured inspection surfaces
- prompt/resource browsing and workflow-resource navigation need further polish
- live provider/session inspection is available through Tasks-pane detail views, but not yet a broader management surface
- external API schemas may still evolve as capability/provider metadata continues to stabilize

## Architectural Review Of The Codebase Changes Thus Far

The codebase now has more inspection support than this specification originally
assumed.

What already exists:

- runtime-family-aware capability inspection
- local-tool inspection with current policy exposure
- workflow inspection through the TUI adapter
- persisted provider and provider-session inspection through workflow records
- delegation history, transitions, promoted artifacts, and linked-resource inspection
- command-driven HITL approval inspection

What still does not exist cleanly:

- richer approval action flows beyond inspection and review
- dedicated provider/session management actions beyond inspection
- a fully stabilized external schema for long-term third-party clients

This specification should therefore separate:

- already-implemented runtime/TUI baseline
- missing HTTP API parity
- future richer rendering and management surfaces

## Review Findings

The main engineering conclusions from reviewing the current codebase are:

1. The runtime adapter is already ahead of the visible TUI.
   [`app/relurpish/tui/runtime_adapter.go`](/home/lex/Public/Relurpify/app/relurpish/tui/runtime_adapter.go)
   already exposes capability inspection and rich workflow inspection, but the
   TUI mostly reaches that data through slash-command output rather than
   dedicated Tasks-pane inspection views.

2. The Tasks pane is the correct first integration point.
   [`app/relurpish/tui/pane_tasks.go`](/home/lex/Public/Relurpify/app/relurpish/tui/pane_tasks.go)
   already owns task selection and is the natural place to attach modal/detail
   inspection for capabilities, providers, sessions, workflows, and approvals.

3. The HTTP API is behind the TUI/runtime baseline.
   [`server/api.go`](/home/lex/Public/Relurpify/server/api.go) currently exposes
   task execution plus basic workflow inspection, but it does not yet expose
   capability/provider/session catalogs, delegation inspection endpoints, or a
   unified approval inspection shape.

4. The inspection model should stay read-only first.
   The framework now has enough state to inspect, but mutation controls for
   providers, sessions, prompts, or resources would create avoidable complexity
   before the inspection shapes settle.

5. Approval inspection should be treated as part of the same inspection model,
   not as a separate special-case subsystem.
   The existing HITL path is command-driven, but the spec now correctly pushes
   toward a unified approval schema and Tasks-pane drill-down.

## Design Principles

- Users should be able to see what capabilities exist, where they came from,
  what runtime family they belong to, and how trusted they are.
- Structured results should be rendered as structured data rather than flattened
  logs where possible.
- Approval surfaces should show capability source, trust class, risk class, and
  target session/resource context.
- External APIs should expose enough metadata for orchestration and debugging
  without bypassing policy.
- The first UI/API milestone is inspectability, not full remote management.
- UI and API surfaces should be built on the capability/provider/session model
  used by execution and policy, not on duplicated legacy views.
- Live runtime state and persisted workflow snapshots should be related but not
  conflated.

## Current Implementation Foundation

The current implementation already provides the substrate needed to begin this
specification in earnest:

- the TUI has a Tasks pane and workflow/HITL command paths in
  [`app/relurpish/tui/pane_tasks.go`](/home/lex/Public/Relurpify/app/relurpish/tui/pane_tasks.go)
  and [`app/relurpish/tui/commands.go`](/home/lex/Public/Relurpify/app/relurpish/tui/commands.go)
- the runtime adapter already exposes capability and workflow inspection models
- workflow persistence already stores delegations, artifacts, provider
  snapshots, and provider-session snapshots
- the runtime already emits delegation-oriented audit and telemetry signals
- MCP-backed capabilities and provider sessions already flow through the normal
  runtime inspection path

This means the next implementation work is not blocked by framework plumbing.
It is primarily UI/API surface work on top of existing runtime state.

## TUI Opportunities

### Capability inspection

The TUI already exposes capability metadata through the runtime adapter, but it
still needs a first-class inspection surface rather than indirect command output
or internal-only adapter methods.

The intended initial placement is:

- modal and detail views launched from the Tasks pane
- drill-down from active workflow, task, provider, and session context
- no separate global inspection workspace as the first milestone

The capability view should show:

- capability ID
- kind
- name
- runtime family
- source provider or session
- trust class
- exposure mode
- risk classes
- schema summary
- availability state
- session affinity where relevant
- coordination metadata where relevant

Phase 1 should prefer a read-only capability catalog before adding mutate or
manage actions.

### Session and provider views

The TUI now has workflow-linked provider/session visibility, but not yet a
dedicated live provider/session surface.

Add visibility for:

- active providers
- provider health
- active sessions
- session scope and age
- recoverability state
- configured vs active state
- source configuration summary
- last error or degraded reason
- negotiated MCP/session metadata where relevant
- delegation linkage for background or remote delegated work

The provider/session view should be explicitly split between:

- live runtime inspection
- persisted workflow snapshots

Those are related but not the same thing, and the UI should not flatten them.

### Workflow and delegation inspection

The TUI already has a workflow inspection path and should expand it rather than
introducing a separate coordination-only browser.

Workflow inspection should show:

- workflow steps and recent events
- delegations and delegation transitions
- promoted workflow artifacts
- linked workflow resources
- provider and provider-session linkage
- trust and insertion outcomes for delegated results

Until prompt/resource browsing lands, linked resources should remain:

- summary-visible
- referenceable by ID/URI
- inspectable as metadata and summary text only

They should not yet imply full navigation or open/browse behavior.

### Structured result rendering

Render:

- text blocks
- JSON objects and trees
- resource links and workflow-resource references
- embedded resources
- structured errors
- delegation result summaries
- capability result provenance and insertion disposition

Unknown block types should degrade gracefully to metadata plus a raw summary.

### Approval and HITL UI

Approval surfaces should show:

- capability source
- runtime family
- trust class
- risk class
- target resource/session
- why approval is required
- whether the approval is for admission, execution, insertion, or provider/session operation

The current `/hitl` inspection path is a useful baseline, but the long-term UI
should become more structured than command output.

The approval model should use one unified approval schema with typed kind
fields rather than separate approval object families.

Suggested kinds:

- `execution`
- `insertion`
- `admission`
- `provider_operation`

These approvals should all be visible through HITL inspection from the start,
even if richer dedicated dialogs land later.

## API Opportunities

The HTTP API should eventually expose:

- capability catalog
- provider catalog
- session inspection
- prompt listing and retrieval
- resource listing and retrieval
- workflow resources with provenance
- delegation inspection for workflow-scoped coordination state
- provider/session snapshot inspection tied to workflows
- unified approval inspection with typed approval kinds

The API should not expose unrestricted execution paths outside framework policy.

The API should also keep a clean boundary around MCP:

- MCP protocol endpoints remain protocol-defined and unchanged by this spec
- Relurpify HTTP endpoints expose imported/exported MCP-backed runtime state as
  ordinary capability/provider/session inspection data
- no separate MCP-specific inspection protocol should be introduced unless a
  later operational requirement justifies it

The HTTP API should also avoid a single universal resource envelope for all
inspectable object types.

Instead it should use:

- a shared metadata subset across inspectable resources
- resource-specific payload shapes for capabilities, providers, sessions,
  delegations, approvals, and other resource families

This is required because capability and provider schemas are still likely to
evolve, and the API should not over-normalize them too early.

## Initial API Surface

The initial API surface on the replacement architecture should add read-only
endpoints equivalent to:

- `GET /api/capabilities`
- `GET /api/capabilities/{id}`
- `GET /api/providers`
- `GET /api/providers/{id}`
- `GET /api/sessions`
- `GET /api/sessions/{id}`
- `GET /api/workflows/{id}/delegations`
- `GET /api/workflows/{id}/artifacts`
- `GET /api/workflows/{id}/providers`
- `GET /api/workflows/{id}/sessions`
- `GET /api/approvals`
- `GET /api/approvals/{id}`

These endpoints should return trust, risk, source, availability, runtime-family,
recoverability, and session metadata, but should not create new execution
bypasses.

Approval responses should use one unified schema with a typed `kind` field so
clients can handle execution, insertion, admission, and provider-operation
approvals through a single model.

More generally, API responses should expose:

- stable shared metadata fields where appropriate
- resource-specific payload sections that can evolve independently

Clients should branch on resource kind/type rather than assuming a single
normalized body shape for all inspectable resources.

The existing workflow endpoints in [`server/api.go`](/home/lex/Public/Relurpify/server/api.go)
should be treated as the starting point, not replaced outright. The new work
should expand them toward parity with the richer TUI/runtime inspection model.

## Initial TUI Surface

The initial TUI surface on the replacement architecture should add:

- Tasks-pane capability/provider/session modal and detail views
- richer approval dialog metadata
- workflow inspection sections for delegations, artifacts, resources, and provider/session linkage

This should land before prompt/resource browsing or session mutation controls.

## Structured Rendering Rules

Rendering should branch on content-block type rather than flattening everything
into text.

Minimum renderers:

- text
- JSON object/tree
- resource link/reference
- embedded resource preview
- structured error

Unknown block types should degrade gracefully to metadata plus raw summary.

## Replacement Phases

### Phase 1

- add API-readable capability/provider/session metadata
- add minimal TUI inspection panes
- add unified approval inspection with typed kinds

Acceptance:

- TUI can inspect capabilities and live providers/sessions from Tasks-pane modal/detail views without digging into logs
- HTTP API can enumerate capabilities/providers/sessions read-only
- HTTP API can enumerate approvals through one unified typed schema
- runtime family, trust, exposure, and recoverability are visible
- workflow-linked provider/session state is inspectable over HTTP as well as in the TUI
- shared metadata is stable while resource-specific payload schemas remain independently evolvable

### Phase 2

- add structured result rendering
- add richer approval metadata

Acceptance:

- result rendering respects content type and provenance instead of flattening to text
- approval surfaces explain source, trust, risk, scope, and why approval is required
- delegation and workflow-resource outputs are rendered intentionally

### Phase 3

- add prompt/resource browsing and management
- add workflow resource navigation

Acceptance:

- users can browse prompt and resource catalogs from both TUI and API surfaces
- workflow resources can be navigated by ID/URI and linked back to delegations/results
- future management actions remain policy-gated and do not bypass runtime controls

## Initial Implementation Plan

Implementation should proceed in slices that keep the TUI and HTTP API aligned
without forcing them to land simultaneously.

### Slice 1: Tasks-Pane Inspection Model

- add Tasks-pane modal/detail state for inspecting:
  - workflows
  - delegations
  - capabilities
  - providers
  - sessions
  - approvals
- wire selection and drill-down behavior onto the existing Tasks pane rather
  than creating a new global inspection pane
- reuse the existing runtime adapter models as the backing data source

Acceptance:

- users can open inspection views from the Tasks pane
- workflow/delegation/provider/session/capability inspection no longer depends
  on slash-command text output alone
- linked workflow resources remain summary-only

### Slice 2: Runtime Adapter And UI Model Cleanup

- add any missing runtime-adapter methods required for:
  - live provider inspection
  - live session inspection
  - unified approval listing/detail
- normalize TUI-side view models so capability/provider/session/delegation and
  approval inspection all use shared metadata conventions with resource-specific
  bodies
- keep prompt/resource browsing out of scope for this slice

Acceptance:

- the TUI has stable internal view models for all Phase 1 inspection targets
- approval inspection uses one typed approval model
- live runtime state and persisted workflow snapshots are clearly separated

### Slice 3: HTTP API Inspection Parity

- extend [`server/api.go`](/home/lex/Public/Relurpify/server/api.go) with
  read-only endpoints for:
  - capabilities
  - providers
  - sessions
  - workflow delegations
  - workflow artifacts
  - workflow provider snapshots
  - workflow session snapshots
  - approvals
- keep the HTTP API on a shared metadata subset plus resource-specific payloads
- keep MCP protocol endpoints unchanged

Acceptance:

- HTTP inspection covers the same main resource families as the TUI
- approval endpoints use the unified typed approval schema
- capability/provider/session payloads are independently evolvable

### Slice 4: Structured Rendering And Approval UX

- improve TUI rendering for:
  - JSON payloads
  - provenance/insertion metadata
  - delegation result summaries
  - workflow-resource summaries
- replace purely textual approval inspection with richer structured detail views
- ensure all approval kinds are visible through the same HITL-oriented surface

Acceptance:

- structured outputs are intentionally rendered instead of flattened
- approval views explain source, trust, risk, scope, and approval kind
- execution, insertion, admission, and provider-operation approvals are all inspectable

### Slice 5: Prompt/Resource Browsing And Resource Navigation

- add prompt/resource catalog inspection after the Phase 1-2 inspection surfaces
  are stable
- add workflow-resource navigation by ID/URI
- connect prompt/resource inspection to MCP-backed imports and exports where relevant

Acceptance:

- users can browse prompts/resources from both TUI and HTTP API surfaces
- linked workflow resources move from summary-only to navigable inspection
- management actions remain policy-gated and read-only inspection remains the default

## Suggested Execution Order

1. implement Tasks-pane modal/detail inspection state
2. add any missing runtime-adapter and approval-model support
3. extend the HTTP API to reach TUI inspection parity
4. improve structured rendering and approval UX
5. add prompt/resource browsing and workflow-resource navigation

## Acceptance

This specification is complete when:

- users can inspect capability origin, runtime family, trust, and exposure
- provider/session state is visible in both live and workflow-linked contexts
- delegations and workflow resources are inspectable through TUI and API surfaces
- structured outputs are handled intentionally
- external API design follows the capability-oriented architecture
- UI and API surfaces do not depend on separate legacy tool-only inspection models
