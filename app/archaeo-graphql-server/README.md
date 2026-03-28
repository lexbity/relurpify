# archaeo-graphql-server

`app/archaeo-graphql-server` is the GraphQL transport for the `archaeo`
control plane.

It exposes current archaeology constructs directly:
- workflow-scoped reads such as workflow projection, timeline, mutation
  history, request history, provenance, coherence, plan lineage, and tensions
- workspace-scoped reads such as deferred drafts, convergence history/current
  projection, and decision trails
- archaeology domain mutations
- request lifecycle mutations for external euclo / relurpic fulfillment
- polling-backed subscriptions over the same core surfaces

The server does not own archaeology semantics. Domain behavior stays in
`archaeo/*`.

## Design

The package is intentionally transport-thin:
- [http.go](/home/lex/Public/Relurpify/app/archaeo-graphql-server/http.go)
  wires HTTP to the GraphQL runtime
- [schema.go](/home/lex/Public/Relurpify/app/archaeo-graphql-server/schema.go)
  defines the SDL
- [runtime.go](/home/lex/Public/Relurpify/app/archaeo-graphql-server/runtime.go)
  adapts the existing `archaeo` services and bindings
- [resolvers_query.go](/home/lex/Public/Relurpify/app/archaeo-graphql-server/resolvers_query.go),
  [resolvers_mutation.go](/home/lex/Public/Relurpify/app/archaeo-graphql-server/resolvers_mutation.go),
  and
  [resolvers_subscription.go](/home/lex/Public/Relurpify/app/archaeo-graphql-server/resolvers_subscription.go)
  expose that runtime through GraphQL

The engine is `github.com/graph-gophers/graphql-go`.

## Transport Shape

Inputs are typed GraphQL input objects.

Outputs currently use a custom `Map` scalar defined in
[types.go](/home/lex/Public/Relurpify/app/archaeo-graphql-server/types.go).
That was chosen because the current `archaeo` runtime returns a large set of
concrete Go structs and `graph-gophers/graphql-go` is strict about resolver
type compatibility.

Practical consequences:
- top-level GraphQL fields are stable and typed in the SDL
- nested payloads are map-shaped JSON objects
- nested keys follow the runtime JSON shape, which is currently `snake_case`
- the server exposes archaeology payloads directly rather than projecting them
  into a second application-owned object model

Example query:

```graphql
query Workflow($workflowId: String!) {
  workflowProjection(workflowId: $workflowId)
}
```

Example response shape:

```json
{
  "data": {
    "workflowProjection": {
      "workflow_id": "wf-123",
      "last_event_seq": 42,
      "active_plan_version": {
        "status": "active",
        "version": 3
      }
    }
  }
}
```

This is a deliberate first pass. If GraphQL clients need a richer typed schema
later, typed wrapper objects can be added incrementally for the highest-value
surfaces.

## Runtime Wiring

`NewHandler(runtime Runtime)` constructs the GraphQL schema and HTTP handler.

The runtime expects a populated `relurpishbindings.Runtime`:
- workflow store
- plan store
- pattern store
- comment store
- any additional archaeology dependencies required by the bound services

Minimal sketch:

```go
runtime := archaeographqlserver.Runtime{
	Bindings: relurpishbindings.Runtime{
		WorkflowStore: workflowStore,
		PlanStore:     planStore,
		PatternStore:  patternStore,
		CommentStore:  commentStore,
	},
}

http.Handle("/graphql", archaeographqlserver.NewHandler(runtime))
```

## Queries

Current query groups:
- workflow queries:
  - `workflowProjection`
  - `timeline`
  - `mutationHistory`
  - `requestHistory`
  - `provenance`
  - `coherence`
  - `learningQueue`
  - `tensions`
  - `tensionSummary`
  - `activePlanVersion`
  - `planLineage`
  - `comparePlanVersions`
- exploration queries:
  - `activeExploration`
  - `explorationView`
  - `explorationByWorkflow`
- workspace queries:
  - `deferredDrafts`
  - `currentConvergence`
  - `convergenceHistory`
  - `decisionTrail`
  - `workspaceSummary`
- request queries:
  - `pendingRequests`
  - `request`

## Mutations

Current mutation groups:
- archaeology state:
  - `resolveLearningInteraction`
  - `updateTensionStatus`
  - `activatePlanVersion`
  - `archivePlanVersion`
  - `markPlanVersionStale`
  - `markExplorationStale`
  - `prepareLivingPlan`
  - `refreshExplorationSnapshot`
- deferred/convergence/decision records:
  - `createOrUpdateDeferredDraft`
  - `finalizeDeferredDraft`
  - `createConvergenceRecord`
  - `resolveConvergenceRecord`
  - `createDecisionRecord`
  - `resolveDecisionRecord`
- request lifecycle:
  - `dispatchRequest`
  - `claimRequest`
  - `renewRequestClaim`
  - `releaseRequestClaim`
  - `applyRequestFulfillment`
  - `failRequest`
  - `invalidateRequest`
  - `supersedeRequest`

The request lifecycle mutations are the main remote execution boundary when
GraphQL is used between `euclo` and `archaeo`.

## Subscriptions

Subscriptions are currently implemented as polling-backed GraphQL subscriptions.

They cover:
- workflow projection
- timeline
- request history
- learning queue
- tensions
- tension summary
- active plan version
- plan lineage
- provenance
- coherence
- deferred drafts
- current convergence
- convergence history
- decision trail

The polling interval is controlled by `Runtime.PollInterval`. If unset, the
runtime fallback is used.

This is intentionally simple. It gives GraphQL clients a realtime-compatible
surface now without introducing a separate event transport layer yet.

## Authorization

`Handler` exposes an optional `Authorize` hook:

```go
handler := archaeographqlserver.NewHandler(runtime)
handler.Authorize = func(ctx context.Context, r *http.Request) error {
	// transport-level auth/authz
	return nil
}
```

Authorization stays outside `archaeo` domain logic.

## Testing

The main transport contract coverage lives in
[http_test.go](/home/lex/Public/Relurpify/app/archaeo-graphql-server/http_test.go).

Run:

```bash
go test ./app/archaeo-graphql-server/...
```

For broader regression coverage:

```bash
go test ./app/archaeo-graphql-server/... ./archaeo/... ./named/euclo/... ./app/relurpish/... ./agents/relurpic/...
```

## Future Follow-Ups

Likely follow-up work after euclo integration starts:
- introduce typed GraphQL wrapper objects for the most important read surfaces
- add pagination on heavier history reads
- revisit subscription delivery if polling becomes too coarse
- add transport-specific auth and complexity limits beyond the current schema
  construction defaults
