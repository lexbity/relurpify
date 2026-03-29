# Plan 09: Archaeo GraphQL Server

## Status

Proposed rewrite of the GraphQL server engineering specification and phased
implementation plan.

This version replaces the earlier stub-oriented plan. It reflects the current
`archaeo` runtime after the runtime-boundary, provenance, request, deferred,
decision, convergence, and external-fulfillment work.

## Summary

`app/archaeo-graphql-server` should become a thin GraphQL transport over the
current `archaeo` control plane.

The server should expose:

- archaeology domain records and projections as queryable GraphQL types
- domain mutations that manipulate archaeology state directly
- request lifecycle mutations needed by euclo and relurpic capability runtimes
- workspace-scoped and workflow-scoped views without forcing in-process clients
  through GraphQL

The server should not:

- reimplement archaeology lifecycle rules
- become a scheduler for archaeology requests
- own provider execution behavior
- replace direct bindings where in-process access is more appropriate

## Architectural Position

The layering remains:

1. `framework/*`
2. `archaeo/*`
3. `agents/*`
4. `named/*`
5. `app/*`

Within that layering:

- `archaeo/*` owns archaeology state, events, runtime semantics, projections,
  bindings, and request application rules
- `app/archaeo-graphql-server/*` owns transport, schema, execution, and client
  boundary concerns

GraphQL is an app-level API surface over `archaeo`, not a replacement for the
runtime itself.

## Current Runtime Reality

The GraphQL plan must now reflect these `archaeo` surfaces as first-class:

- workflow-scoped archaeology state:
  - exploration sessions and snapshots
  - workflow projections and timelines
  - learning interactions
  - tensions
  - plan versions and lineage
  - requests
  - provenance and coherence
  - mutation history
- workspace-scoped archaeology state:
  - deferred draft records
  - convergence review/resolution records
  - current convergence projection
  - decision trails
- request execution boundary:
  - archaeology request creation and status transitions
  - external claim / renew / release / apply / fail / invalidate / supersede
  - euclo and relurpic capability execution living outside the GraphQL server

The GraphQL server should expose these constructs faithfully rather than hiding
them behind an older, narrower “exploration + plan + tension” schema.

## Goals

The server must:

- expose `archaeo` constructs directly and coherently
- preserve a thin transport boundary over runtime services
- support both workflow-scoped and workspace-scoped access patterns
- support direct mutation of archaeology state where the domain already permits
  it
- support external request fulfillment flows used by euclo and relurpic
- remain compatible with future realtime/subscription delivery

## Non-Goals

This plan does not:

- make GraphQL the only transport
- force euclo to use GraphQL for in-process access
- introduce scheduling semantics into `archaeo`
- redefine archaeology domain rules
- require subscriptions in the first implementation phase

Direct bindings remain valid for:

- euclo in-process execution
- relurpish in-process UX/runtime integration
- tests and benchmarks that need low-overhead direct access

## Core Design Principles

### 1. Direct archaeology API, not an app-specific abstraction layer

The server should expose the runtime constructs that actually exist:

- requests
- deferred drafts
- convergence records
- decisions
- provenance
- workflow projections

It should not reshape everything into generic “dashboard” payloads on day one.

### 2. Workflow scope and workspace scope are both first-class

The schema must support:

- workflow-centric reads for execution/history/provenance
- workspace-centric reads for deferred drafts, convergence, and decision state

### 3. Request fulfillment is part of the API surface

GraphQL should not only expose archaeology reads and end-user mutations.
It should also expose request lifecycle mutations used by euclo or relurpic
capability runtimes when GraphQL is the chosen integration path.

### 4. Comments are interaction surfaces, not the authoritative state

Decision and convergence records may reference comment IDs, but the structured
archaeology records remain authoritative.

### 5. Subscriptions come after the core query/mutation model is stable

The first implementation should prioritize correct query and mutation coverage.
Realtime delivery is a later phase unless a concrete client requires it sooner.

## Server Boundary Strategy

The server should primarily depend on `archaeo` runtime services and bindings.

Recommended preference order:

1. `archaeo/bindings/*` where an operation already exists cleanly
2. direct `archaeo/*` services for domain records and projections
3. never duplicate runtime logic in the GraphQL layer

This differs from the earlier plan: the GraphQL server should not be forced to
route primarily through `archaeo/bindings/relurpish` if that makes the API less
faithful to the actual runtime.

## GraphQL Surface

The schema should be organized around five groups of top-level operations.

### 1. Workflow Queries

These queries expose workflow-scoped archaeology state:

- `workflowProjection(workflowId: ID!)`
- `timeline(workflowId: ID!)`
- `mutationHistory(workflowId: ID!)`
- `requestHistory(workflowId: ID!)`
- `provenance(workflowId: ID!)`
- `coherence(workflowId: ID!)`
- `learningQueue(workflowId: ID!)`
- `tensions(workflowId: ID!)`
- `tensionSummary(workflowId: ID!)`
- `activePlanVersion(workflowId: ID!)`
- `planLineage(workflowId: ID!)`
- `comparePlanVersions(workflowId: ID!, left: Int!, right: Int!)`

### 2. Exploration Queries

- `activeExploration(workspaceId: ID!)`
- `explorationView(explorationId: ID!)`
- `explorationByWorkflow(workflowId: ID!)`

### 3. Workspace Queries

These are now first-class and should be explicit:

- `deferredDrafts(workspaceId: ID!)`
- `currentConvergence(workspaceId: ID!)`
- `convergenceHistory(workspaceId: ID!)`
- `decisionTrail(workspaceId: ID!)`

Summary and pagination variants should be introduced where needed once real
clients start consuming full history.

### 4. Request Queries

- `pendingRequests(workflowId: ID!)`
- `request(requestId: ID!, workflowId: ID!)`

If needed later:

- `requestsByWorkspace(workspaceId: ID!)`
- `requestsByStatus(...)`

### 5. Supporting Queries

- `workflowPhase(workflowId: ID!)`
- `workspaceSummary(workspaceId: ID!)`

The `workspaceSummary` query should be intentionally compact and built from
existing summary/current-state records, not from a synthetic app-owned model.

## Mutation Surface

Mutations should be grouped by domain concern.

### 1. Archaeology Lifecycle Mutations

- `prepareLivingPlan(workflowId: ID!, input: PrepareLivingPlanInput!)`
- `markExplorationStale(explorationId: ID!, reason: String!)`
- `refreshExplorationSnapshot(explorationId: ID!, input: RefreshExplorationInput!)`

These should call existing archaeology services and return domain-shaped payloads.

### 2. Learning / Tension / Plan Mutations

- `resolveLearningInteraction(...)`
- `deferLearningInteraction(...)`
- `resolveTension(...)`
- `updateTensionStatus(...)`
- `activatePlanVersion(...)`
- `markPlanVersionStale(...)`
- `archivePlanVersion(...)`

### 3. Request Lifecycle Mutations

These are now critical:

- `dispatchRequest(...)`
- `claimRequest(...)`
- `renewRequestClaim(...)`
- `releaseRequestClaim(...)`
- `applyRequestFulfillment(...)`
- `failRequest(...)`
- `invalidateRequest(...)`
- `supersedeRequest(...)`

These mutations should be present even if some callers still use direct
bindings, because they define the GraphQL server’s contract with external
fulfillment runtimes.

### 4. Deferred / Convergence / Decision Mutations

- `createOrUpdateDeferredDraft(...)`
- `finalizeDeferredDraft(...)`
- `createConvergenceRecord(...)`
- `resolveConvergenceRecord(...)`
- `createDecisionRecord(...)`
- `resolveDecisionRecord(...)`

These should accept comment refs and metadata, but comments themselves remain in
the existing comment store.

## Type System Shape

The schema should favor domain-shaped object types over generic maps.

Core types should include:

- `WorkflowProjection`
- `TimelineEvent`
- `MutationHistoryProjection`
- `RequestHistoryProjection`
- `ProvenanceProjection`
- `CoherenceProjection`
- `ExplorationSession`
- `ExplorationSnapshot`
- `LearningInteraction`
- `Tension`
- `TensionSummary`
- `PlanVersion`
- `PlanLineageProjection`
- `RequestRecord`
- `DeferredDraftRecord`
- `ConvergenceRecord`
- `WorkspaceConvergenceProjection`
- `DecisionRecord`

GraphQL payloads may omit low-value internal fields, but should not invent a
parallel conceptual model unless there is a clear client-facing reason.

## Transport Model

The server should still be split into three internal layers.

### 1. Runtime / bootstrap

Responsible for:

- configuration
- store/runtime construction
- resolver dependencies
- subscription manager wiring when subscriptions are added

Suggested files:

- `app/archaeo-graphql-server/runtime.go`
- `app/archaeo-graphql-server/config.go`
- `app/archaeo-graphql-server/bootstrap.go`

### 2. Schema / resolvers

Responsible for:

- schema definition
- resolver registration
- argument validation
- GraphQL-to-runtime mapping

Suggested files:

- `app/archaeo-graphql-server/schema.go`
- `app/archaeo-graphql-server/resolvers_query.go`
- `app/archaeo-graphql-server/resolvers_mutation.go`
- `app/archaeo-graphql-server/types.go`

Add `resolvers_subscription.go` only when subscriptions are implemented.

### 3. HTTP transport

Responsible for:

- HTTP endpoint wiring
- request/response encoding
- auth propagation into context
- request-scoped tracing/logging

Suggested files:

- `app/archaeo-graphql-server/http.go`
- `app/archaeo-graphql-server/middleware.go`

Add websocket transport only when subscriptions are introduced.

## GraphQL Engine Choice

The server should use a real GraphQL engine with:

- schema validation
- typed resolver registration
- field selection
- testable execution behavior

Selection criteria:

- minimal hidden codegen/runtime behavior
- profiler-friendly resolver execution
- good query validation and error reporting
- easy integration with Go service dependencies

Avoid pushing domain logic into generated code or large generated layers that
make performance and correctness difficult to inspect.

## Pagination And Cost Controls

The initial server should ship with bounded list shapes for history-heavy reads.

Required controls:

- max list limits
- pagination for request history, timeline, decisions, convergence history, and
  deferred drafts if needed
- basic query complexity / depth limits

Do not wait for subscriptions to add these protections.

## Auth / Authz Position

The first pass may remain local or workspace-scoped, but the server should keep
clear hooks for:

- workspace access checks
- mutation authorization
- request-claim authorization

Auth policy should remain outside `archaeo` domain logic.

## euclo And Relurpic Integration Position

GraphQL is not the only euclo integration path.

Expected usage split:

- euclo may still use direct bindings in-process
- euclo may use GraphQL for remote or transport-constrained integration
- relurpic capability runtimes may fulfill archaeology requests either through
  direct services/bindings or through GraphQL request lifecycle mutations

The schema should therefore expose request lifecycle mutations cleanly, but the
server should not assume it is the only fulfillment path.

## Phased Implementation Plan

### Phase 1: Replace The Stub With A Real GraphQL Runtime

Implement:

- choose and wire a real GraphQL engine
- build executable schema
- replace string-matching handler logic
- preserve existing smoke-test behavior where possible

Deliverables:

- real query parsing and execution
- typed query and mutation resolver registration
- request-scoped runtime bootstrap

Tests:

- GraphQL HTTP contract tests
- schema validation tests
- error-path tests

### Phase 2: Implement Core Workflow And Exploration Queries

Implement:

- workflow projection
- timeline
- mutation history
- request history
- provenance
- coherence
- exploration view
- active exploration
- plan lineage / active version
- tensions / tension summary

Tests:

- resolver tests against seeded archaeology fixtures
- response-shape tests

### Phase 3: Implement Workspace-Scoped Queries

Implement:

- deferred drafts by workspace
- current convergence projection by workspace
- convergence history by workspace
- decision trail by workspace
- workspace summary query

Tests:

- workspace query resolver tests
- pagination/limit tests where needed

### Phase 4: Implement Domain Mutation Surface

Implement:

- learning/tension/plan mutations
- deferred/convergence/decision mutations
- archaeology refresh / exploration mutations

Tests:

- mutation contract tests
- idempotency tests where appropriate
- comment-ref round-trip tests

### Phase 5: Implement Request Lifecycle Mutations

Implement:

- claim / renew / release
- apply fulfillment
- fail / invalidate / supersede
- request lookup and pending reads shaped for external executors

This phase is the main GraphQL boundary for euclo/relurpic remote fulfillment.

Tests:

- competing claim behavior
- stale fulfillment behavior
- decision-record creation for stale/partial results

### Phase 6: Add Query Limits, Auth Hooks, And Error Model Hardening

Implement:

- pagination / limits / depth controls
- auth/authz hook points
- stable GraphQL error payload shape

Tests:

- unauthorized mutation/query tests
- cost-control tests
- bounded history-read tests

### Phase 7: Add Subscriptions If Needed

Implement only if a real client requires it:

- workflow projection updates
- request status updates
- current convergence updates
- decision/deferred updates

Subscriptions should be driven by existing event/projection changes, not by new
GraphQL-owned runtime state.

Tests:

- subscription delivery tests
- reconnect/resubscribe behavior
- bounded fanout tests

## Test Plan

Run continuously during implementation:

- `go test ./app/archaeo-graphql-server/...`
- `go test ./archaeo/... ./named/euclo/... ./app/relurpish/... ./agents/relurpic/...`

Add:

- schema contract tests
- resolver tests with seeded archaeology fixtures
- request lifecycle GraphQL tests
- workspace-scoped query tests
- transport error-model tests

Add benchmarks for:

- workflow projection query execution
- provenance query execution
- workspace convergence query execution
- request history query execution

## Acceptance Criteria

- the GraphQL server exposes the current `archaeo` runtime faithfully
- workflow-scoped and workspace-scoped archaeology reads are both supported
- request lifecycle mutations are available for remote fulfillment paths
- comment-linked decision/deferred/convergence records are exposed as structured
  archaeology state
- the GraphQL server remains a transport layer, not a second archaeology runtime
- direct bindings remain viable for in-process euclo and relurpish integration

## Future Revisions

The API surface will likely need targeted revisions once:

- GraphQL clients exercise history-heavy reads at scale
- euclo begins fulfilling archaeology requests through transport rather than
  direct bindings
- relurpic capability runtimes clarify which higher-level orchestration helpers
  are worth exposing through GraphQL

That revision should happen after the first working server pass, not before.
