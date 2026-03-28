# Euclo Archaeology and Living Plan Server

## Status

Proposed engineering specification.

This document defines a server-oriented architecture for the parts of euclo
that are not inherently tied to a single UX surface:

- archaeology
- intent grounding
- pattern and anchor exploration
- living-plan formation
- execution lifecycle state
- verification and convergence
- learning and guidance interactions

The intent is not to move all of euclo behind a server. The intent is to
separate euclo's durable reasoning/runtime domain from relurpish's default TUI
presentation layer so that:

- relurpish can remain the default and most tightly integrated UX
- euclo itself stays UX-agnostic
- alternate UXs can be built without re-implementing euclo's domain model
- domain state can survive across sessions and agent process boundaries

note:
This document is deliberately not over-summarized. The domain is relationship-
heavy and lifecycle-heavy; compressing it too aggressively would hide the key
engineering constraints.

## Goal

LLMs have internalized patterns that exist in literature but aren't widely taught, emerging patterns from recent codebases, and logical combinations of named patterns
   that haven't been formally written up anywhere. No individual developer has that breadth — not because they aren't senior, but because the synthesis only becomes
  visible at scale. This is the oracle property that actually matters: not "tell me the answer" but "show me the possibility space I can't see from where I'm standing."

  through Pattern surfacing, it asks the user to
  engage with structure, learn , and form their own conclusions. Which 
  is epistemically honest because the user can see what the claim is 
  grounded in, question it, push back on it.

  Senior developers benefit from this just as much as novices, but differently. A senior developer's mental model of a codebase they own diverges from reality over time
   — patterns they introduced get violated, new patterns emerge in areas they don't own, early decisions still appear in the pattern map even after the intent behind
  them changed. Pattern surfacing is a way to audit their own mental model against what's actually there.

### Why this should exist

The current system has made substantial progress on backend primitives:

- durable graph storage
- durable living-plan storage
- pattern/gap/comment stores
- guidance and deferral infrastructure
- euclo execution gating
- relurpic archaeology-related capabilities

However, euclo still behaves primarily like an execution-oriented agent with
additional reasoning features bolted into its execution path. The architecture
is still too close to:

- one active UX
- one in-process orchestration loop
- one transient LLM session as the main coherence boundary

That is not enough for the deeper system we want.

The target system is an intent-grounded reasoning runtime, not merely a coding
agent with chat and tools. The main system property we want is:

 The human and euclo should be able to iteratively construct, inspect, refine,
 and execute a durable model of codebase intent and implementation structure,
 with that model surviving across sessions, clients, and bounded LLM contexts.

This requires separating several concerns that are currently too entangled:

- euclo domain logic vs relurpish UI
- durable workflow state vs ephemeral LLM session state
- pattern surfacing vs answer generation
- learning interactions vs operational guidance interactions
- plan formation vs plan execution
- current code state vs versioned semantic snapshots
  

### Why a server boundary is appropriate

The proposed 'archaeo' server is local-only for now. It is not being introduced to expose
euclo over a network for remote multi-user SaaS operation. It is being
introduced because the domain itself benefits from a stable process boundary and
a formal query/mutation/subscription surface.

The server should make it possible to:

- run archaeology independently from a single TUI session
- preserve living-plan lifecycle state outside the LLM context
- let multiple local UXs inspect the same domain state
- support richer UXs than a terminal UI can reasonably provide
- centralize eventing, resumability, provenance, and versioning
- make domain contracts explicit instead of implicit in `agent.go`

### Why GraphQL is preferred here

For this domain, GraphQL is a better fit than plain REST because the domain is:

- relationship-heavy
- graph-shaped
- exploration-oriented
- provenance-heavy
- partially durable and partially live

Examples of first-class queries we will want:

- a living plan with steps, dependencies, invalidation rules, attempts,
  convergence status, and linked patterns
- a pattern cluster with instances, anchors, comments, tensions, and blast
  radius previews
- an archaeology session with candidate patterns, unresolved questions,
  confirmed intent, and phase progress
- a convergence target with unresolved tensions and supporting evidence

Trying to flatten these into purely REST-shaped resources will either:

- over-fragment the model
- produce inefficient client orchestration
- or push complexity into ad hoc read-model endpoints anyway

That said, GraphQL should still be used in a command-oriented style:

- queries for exploration and read models
- mutations for domain transitions
- subscriptions for long-running jobs and workflow events

This is not a database façade.

## Non-Goals

The initial server is not intended to:

- replace relurpish as euclo's primary UX
- own every euclo capability
- become a generic transport wrapper around all agent execution
- expose arbitrary internal framework stores directly
- handle broad remote-user auth and tenancy concerns
- replace local explicit user actions such as editor launch or session export

The initial focus is the durable reasoning/runtime domain surrounding
archaeology, intent grounding, planning, verification, and their related
interactions.

## Architectural Position

The desired layering is:

1. `framework/*`
2. `named/euclo/*`
3. `archaeo/*`
4. `app/relurpish/*` and any future UX clients

### Framework responsibilities

Framework remains the reusable substrate. It should continue to own:

- `graphdb`
- `plan`
- `patterns`
- `retrieval`
- `guidance`
- persistence helpers
- capability execution and policy/sandbox infrastructure
- event/memory primitives

Framework should not become euclo-specific simply because euclo uses it most
heavily.

### Euclo responsibilities

Euclo should own:

- the phase machine
- archaeology orchestration
- intent grounding lifecycle
- living-plan lifecycle
- execution lifecycle
- convergence lifecycle
- learning/guidance orchestration semantics
- domain event production

Euclo is the domain runtime.

### Relurpic responsibilities

Relurpic changes the dependency picture enough that it should be named
explicitly here.

Today, `agents/relurpic/*` is not merely "another agent." It already contains a
substantial amount of archaeology-domain behavior expressed as capabilities:

- pattern detection
- gap detection
- prospective pattern matching
- comment-to-anchor promotion
- convergence verification
- planner/reviewer/verifier style structured reasoning helpers

This means the initial `archaeo` implementation should treat relurpic as a
transitional archaeology-domain dependency, not just an incidental downstream
consumer of framework primitives.

That has two implications:

- `archaeo` should orchestrate or adapt relurpic-backed archaeology services
  where appropriate instead of immediately re-implementing all of that logic
- longer term, the durable/domain-oriented parts of relurpic should likely be
  extracted behind `archaeo` services, leaving relurpic capabilities as thin
  orchestration or UX-facing wrappers

The architectural goal is not to permanently entangle `archaeo` with
capability-registration code. The goal is to acknowledge the current codebase
honestly and create a clean migration seam.

### Server responsibilities

The euclo server should own:

- transport-level schema
- domain query surfaces
- domain command/mutation surfaces
- subscriptions to euclo events/jobs/interactions
- process-local orchestration handles for long-running tasks

The server is a facade over euclo domain services, not a reinvention of them.

### Relurpish responsibilities

Relurpish remains the default TUI UX and can be deeply integrated, but it
should treat the server/domain boundary as authoritative for server-owned state.

Relurpish should continue to own:

- pane layout
- selection state
- local input workflows
- command palette and keybindings
- local-only user-side affordances

Relurpish should not be the system of record for archaeology or living-plan
state.

## Archaeo Package Layout

`archaeo` should be split into subpackages. A single package with mixed domain,
transport, and orchestration concerns will recreate the same entanglement that
currently exists in `agent.go`.

Suggested structure:

- `archaeo/domain`
- `archaeo/phases`
- `archaeo/archaeology`
- `archaeo/learning`
- `archaeo/plans`
- `archaeo/execution`
- `archaeo/verification`
- `archaeo/events`
- `archaeo/projections`
- `archaeo/graphql`
- `archaeo/jobs`
- `archaeo/adapters/framework`
- `archaeo/adapters/relurpic`

### Package responsibilities

`domain`

- core artifact types
- identifiers and references
- durable status enums
- revision/freshness metadata

`phases`

- phase machine
- transition validation
- phase-entry/exit policies
- resumability rules

`archaeology`

- archaeology runs
- exploration-session lifecycle
- candidate finding orchestration
- recomputation triggers

`learning`

- learning interaction model
- confirm/reject/refine/defer flows
- semantic grounding decisions
- comment/provenance-linked refinement

`plans`

- living-plan draft formation
- versioning
- activation
- invalidation-aware plan evolution

`execution`

- execution against an activated plan version
- phase-aware coordination with euclo runtime
- guidance/escalation hooks related to execution

`verification`

- verification jobs
- convergence evaluation
- tension/coherence checks
- verification result persistence

`events`

- domain event definitions
- event append/publish interfaces
- subscription fan-out contracts

`projections`

- read models for GraphQL
- exploration summaries
- learning queues
- active plan/convergence snapshots
- timeline projections

`graphql`

- schema
- resolvers
- mutations
- subscriptions
- transport-specific input/output shaping

`jobs`

- long-running archaeology and verification job runners
- cancellation
- progress reporting
- retry/recompute coordination

`adapters/framework`

- integration with framework stores and services
- graph/retrieval/plan/pattern/guidance/memory adapters

`adapters/relurpic`

- adapters over current relurpic archaeology-domain capabilities and helpers
- migration seam so `archaeo` can depend on relurpic behavior without hard
  coupling its domain model to capability-registration code

This layout is intentionally domain-first. It should be possible to test the
phase machine, archaeology logic, and plan lifecycle without loading GraphQL or
relurpish.

## Core Design Principles

### 1. Durable domain state must live outside the LLM context

LLM context is transient, bounded, compacted, and not inspectable enough to be
the authoritative source of:

- current archaeology findings
- current intent assumptions
- current living plan
- pending learning/guidance requests
- versioned semantic relationships

LLM sessions consume and update domain state. They do not own it.

### 2. Pattern surfacing is not summarization

The primary architectural objective is not "better summaries." It is a system
that can:

- surface candidate structure
- let the human confirm/reject/refine it
- preserve that engagement as durable intent evidence

This means the core domain artifact is not just a paragraph of text. It is a
set of structured findings and decisions grounded in code state.

### 3. Learning interactions are distinct from operational guidance

Permission, guidance, and learning are not one abstraction.

- Permission interaction answers: "may this action execute?"
- Guidance interaction answers: "what should execution do now?"
- Learning interaction answers: "is this inferred structure/intent actually
  correct?"

These produce different artifacts, have different lifecycles, and should not be
collapsed into a single generic enum without preserving those differences.

### 4. Versioned semantic state matters

Archaeology and planning are not only about current code state. They are also
about changes over time:

- what pattern existed before
- what pattern drifted
- what intent was confirmed at a specific revision
- what plan version was grounded on which code state

This requires explicit snapshot/version references.

### 5. Recomputation is expected

We should assume that:

- code changes outside euclo can occur
- the graph may lag reality briefly
- some workflows resume after drift
- not every change source will be captured in-process

The system should model freshness and recomputation explicitly rather than
pretending it can avoid recomputation entirely.

## Phase Dependencies

The implementation should be discussed in terms of phase-specific dependency
sets, not only in terms of a generic framework inventory. Different phases use
different slices of the current codebase, and some phases depend on relurpic as
it exists today.

### Archaeology scan and exploration bootstrap

Primary dependencies:

- `framework/ast`
- `framework/graphdb`
- `framework/retrieval`
- `framework/contextmgr`
- `framework/memory`
- `framework/capability`
- `agents/relurpic`

Why these are needed:

- `ast` resolves file/package/symbol scope and provides indexed structural
  context
- `graphdb` provides neighborhood traversal, impact analysis, and structural
  relationships
- `retrieval` provides anchors, drift information, and semantic evidence
- `contextmgr` helps prepare bounded excerpts and prompts for archaeology jobs
- `memory` should persist archaeology runs, findings, and projections
- `capability` matters because relurpic archaeology behavior is currently
  exposed through capabilities
- `relurpic` already implements grounded pattern detection and related
  archaeology flows

Current dependency chain:

- `archaeo archaeology services -> relurpic archaeology adapters`
- `relurpic -> ast + graphdb + retrieval + capability`
- `archaeo -> memory/events/projections for durable state`

### Pattern proposal, intent grounding, and learning

Primary dependencies:

- `framework/guidance`
- `framework/patterns`
- `framework/retrieval`
- `framework/memory`
- `agents/relurpic`

Why these are needed:

- `guidance` provides broker, events, and deferral mechanics that can be reused
  for transport/infrastructure
- `patterns` stores proposals, confirmations, comments, and gaps
- `retrieval` stores promoted anchors and anchor drift state
- `memory` should persist learning interactions and their workflow history
- `relurpic` already provides comment promotion, prospective matching, and
  pattern-oriented archaeology flows that feed learning

Important note:

This phase should still define its own `LearningInteraction` artifact rather
than collapsing into the current guidance model. Guidance infrastructure is
reusable; the semantic artifact model is distinct.

### Tension and contradiction analysis

Primary dependencies:

- `framework/retrieval`
- `framework/graphdb`
- `framework/plan`
- `framework/guidance`
- `framework/memory`
- `agents/relurpic`

Why these are needed:

- relurpic gap detection already records anchor drift, writes graph contract
  edges, and invalidates plan steps
- `retrieval` anchors and drifts are part of tension detection inputs
- `graphdb` expresses structural and semantic contract relationships
- `plan` is already affected by detected contradictions and invalidations
- `guidance` supports escalation and deferral when contradictions are severe
- `memory` should persist tension findings and contradiction events as durable
  workflow artifacts

This phase is important because it shows archaeology and plan lifecycle are not
strictly sequential. Archaeology findings can mutate plan validity.

### Living-plan draft formation

Primary dependencies:

- `framework/plan`
- `framework/graphdb`
- `framework/retrieval`
- `framework/patterns`
- `framework/memory`
- `agents/relurpic`

Why these are needed:

- `plan` is the canonical plan structure and lifecycle substrate
- `graphdb` and `retrieval` provide grounding scope and evidence
- `patterns` provides confirmed or candidate structural/behavioral findings
- `memory` should persist draft versions, evolution, and phase transitions
- `relurpic` already has planner/reviewer/verifier-style structured reasoning
  helpers that may be adapted during early implementation

This is the phase where `ExplorationSession -> LivingPlanDraft ->
VersionedLivingPlan` should become explicit.

### Execution

Primary dependencies:

- `framework/capability`
- `framework/plan`
- `framework/guidance`
- `framework/graphdb`
- `framework/contextmgr`
- `framework/memory`
- `named/euclo`

Why these are needed:

- `capability` remains the main execution-policy and sandboxed tool surface
- `plan` provides evidence gates, step lifecycle, and invalidation state
- `guidance` handles operational intervention and recovery requests
- `graphdb` supports blast-radius and invalidation-aware execution behavior
- `contextmgr` helps bound execution context for working agent sessions
- `memory` should persist attempts, events, and execution projections
- `named/euclo` remains the core execution/runtime owner

Compared with archaeology, relurpic is less central here unless execution
actively invokes archaeology-domain services mid-flight.

### Verification and convergence

Primary dependencies:

- `framework/plan`
- `framework/patterns`
- `framework/retrieval`
- `framework/graphdb`
- `framework/memory`
- `agents/relurpic`

Why these are needed:

- convergence depends on plan targets and verification rules
- confirmed patterns and tensions are part of the convergence signal
- anchors and semantic evidence matter for checking whether intent still holds
- graph relationships provide structural and semantic context
- `memory` should persist verification and convergence runs
- relurpic already owns the current convergence verifier implementation

### Surfacing, query, and external UX integration

Primary dependencies:

- `framework/memory`
- `framework/guidance`
- `archaeo/events`
- `archaeo/projections`
- `archaeo/graphql`

Why these are needed:

- clients should read projections rather than raw store tables
- subscriptions should be driven by durable event production
- guidance/learning queues need live and durable views
- GraphQL becomes the query/mutation/subscription contract for server-owned
  state

This phase is where relurpish and any future UX should start consuming
server-owned read models rather than owning archaeology or plan state
themselves.

## Domain Model

The system needs a stronger domain model than "graph + plan + guidance + some
pattern tools." The artifacts need to be explicit.

### Exploration Session

This is the durable workspace for archaeology and intent grounding before and
during plan formation.

Decision:

- there is one active exploration session per project workspace
- archaeology runs create dated snapshots under that exploration session
- snapshot creation is triggered by the agent/runtime when archaeology or
  recomputation is performed
- workflows may mutate artifacts derived from exploration, but the exploration
  session itself remains workspace-scoped rather than workflow-scoped

This is not the living plan. It is the pre-plan and mid-plan reasoning
workspace.

Suggested shape:

```go
type ExplorationStatus string

const (
    ExplorationStatusActive     ExplorationStatus = "active"
    ExplorationStatusStale      ExplorationStatus = "stale"
    ExplorationStatusBlocked    ExplorationStatus = "blocked"
    ExplorationStatusArchived   ExplorationStatus = "archived"
)

type ExplorationScope struct {
    WorkspaceRoot string
    CorpusScope   string
    FilePaths     []string
    SymbolIDs     []string
    PackagePaths  []string
}

type ExplorationSession struct {
    ID                   string
    WorkspaceID          string
    Status               ExplorationStatus
    Scope                ExplorationScope
    ActiveSnapshotID     string
    LatestSnapshotID     string
    BasedOnRevision      string
    ComputedAt           time.Time
    RecomputeRequired    bool
    StaleReason          string
    CandidatePatternIDs  []string
    CandidateAnchorRefs  []string
    TensionIDs           []string
    OpenLearningIDs      []string
    ConfirmedIntentRefs  []string
    DeferredFindingRefs  []string
    CommentRefs          []string
    CreatedAt            time.Time
    UpdatedAt            time.Time
}

type ExplorationSnapshot struct {
    ID                  string
    ExplorationID       string
    SnapshotKey         string
    BasedOnRevision     string
    CreatedAt           time.Time
    TriggeredByAgent    string
    CandidatePatternIDs []string
    CandidateAnchorRefs []string
    TensionIDs          []string
    OpenLearningIDs     []string
    Summary             string
}
```

`SnapshotKey` should be a unique ID plus date/time component so that archaeology
snapshots are stable, human-inspectable, and easy to sort.

### Living Plan

The existing living plan is the right direction, but it should become clearly
versioned and explicitly separated from the exploration session that informed
it.

Execution should always operate against a specific plan version.

Suggested shape:

```go
type LivingPlanVersionStatus string

const (
    LivingPlanVersionDraft      LivingPlanVersionStatus = "draft"
    LivingPlanVersionActive     LivingPlanVersionStatus = "active"
    LivingPlanVersionSuperseded LivingPlanVersionStatus = "superseded"
    LivingPlanVersionArchived   LivingPlanVersionStatus = "archived"
)

type VersionedLivingPlan struct {
    ID                     string
    WorkflowID             string
    Version                int
    ParentVersion          *int
    DerivedFromExploration string
    BasedOnRevision        string
    SemanticSnapshotRef    string
    Status                 LivingPlanVersionStatus
    RecomputeRequired      bool
    StaleReason            string
    ComputedAt             time.Time
    ActivatedAt            *time.Time
    SupersededAt           *time.Time
    Plan                   frameworkplan.LivingPlan
    CommentRefs            []string
    TensionRefs            []string
    PatternRefs            []string
    AnchorRefs             []string
    CreatedAt              time.Time
    UpdatedAt              time.Time
}
```

### Pattern Registry

The current `PatternStore` is necessary but not sufficient for the "living
pattern registry" idea in the research note.

The registry must support:

- proposed patterns
- confirmed patterns
- rejected patterns
- refined/superseded patterns
- links to anchors
- links to comments
- links to tensions
- links to plan steps
- code revision context

The main difference from the current store is not only status. It is lifecycle
and relationship richness.

Decision:

- comments remain in `patterns` for now
- the broader provenance model can still evolve later, but the first
  implementation should not split comments out into a new storage subsystem

### Learning Interaction

This should be its own artifact and should remain euclo-owned for now rather
than being pushed into framework.

Possible resolution outcomes:

- confirm
- reject
- refine
- defer

This differs from guidance because the output is semantic grounding.

Suggested shape:

```go
type LearningInteractionKind string

const (
    LearningPatternProposal  LearningInteractionKind = "pattern_proposal"
    LearningAnchorProposal   LearningInteractionKind = "anchor_proposal"
    LearningTensionReview    LearningInteractionKind = "tension_review"
    LearningIntentRefinement LearningInteractionKind = "intent_refinement"
)

type LearningSubjectType string

const (
    LearningSubjectPattern     LearningSubjectType = "pattern"
    LearningSubjectAnchor      LearningSubjectType = "anchor"
    LearningSubjectTension     LearningSubjectType = "tension"
    LearningSubjectExploration LearningSubjectType = "exploration"
)

type LearningChoice struct {
    ID          string
    Label       string
    Description string
}

type LearningResolution struct {
    ChoiceID        string
    RefinedPayload  map[string]any
    CommentRef      string
    ResolvedBy      string
    ResolvedAt      time.Time
}

type LearningInteraction struct {
    ID                string
    WorkflowID        string
    ExplorationID     string
    SnapshotID        string
    Kind              LearningInteractionKind
    SubjectType       LearningSubjectType
    SubjectID         string
    Title             string
    Description       string
    Evidence          []EvidenceRef
    Choices           []LearningChoice
    DefaultChoice     string
    TimeoutBehavior   LearningTimeoutBehavior
    Status            InteractionStatus
    Resolution        *LearningResolution
    BasedOnRevision   string
    CreatedAt         time.Time
    UpdatedAt         time.Time
}
```

### Guidance Interaction

Guidance remains operational:

- confidence too low
- blast radius expanded
- contradiction too severe
- recovery exhausted

The current guidance package is a strong foundation and likely remains the
shared infrastructure, but euclo should represent guidance requests as domain
artifacts in its own workflow timeline as well.

### Tension

The research note repeatedly points toward tensions as a first-class concept.
This is not yet adequately modeled.

A tension artifact should express:

- which patterns/anchors/symbol scopes are in tension
- whether the tension is inferred, confirmed, accepted, or unresolved
- what blast radius it implies
- which resolution patterns or migration paths are candidates

Suggested shape:

```go
type TensionStatus string

const (
    TensionInferred   TensionStatus = "inferred"
    TensionConfirmed  TensionStatus = "confirmed"
    TensionAccepted   TensionStatus = "accepted"
    TensionUnresolved TensionStatus = "unresolved"
    TensionResolved   TensionStatus = "resolved"
)

type Tension struct {
    ID                  string
    ExplorationID       string
    SnapshotID          string
    PatternIDs          []string
    AnchorRefs          []string
    SymbolScope         []string
    Kind                TensionKind
    Description         string
    Severity            string
    Status              TensionStatus
    BlastRadiusNodeIDs  []string
    SuggestedResolves   []PatternProposalRef
    RelatedPlanStepIDs  []string
    CommentRefs         []string
    BasedOnRevision     string
    CreatedAt           time.Time
    UpdatedAt           time.Time
}
```

### Provenance and Comments

Comments should be first-class provenance-bearing artifacts. They should not
only attach to pattern records opportunistically.

Comments should be attachable to:

- patterns
- anchors
- tensions
- plan steps
- convergence decisions
- exploration sessions

They should carry:

- author kind
- trust class
- intent type
- code revision reference if applicable
- subject reference

This makes user confirmation/refinement explainable over time.

Suggested shape:

```go
type CommentSubjectType string

const (
    CommentSubjectPattern     CommentSubjectType = "pattern"
    CommentSubjectAnchor      CommentSubjectType = "anchor"
    CommentSubjectTension     CommentSubjectType = "tension"
    CommentSubjectPlanStep    CommentSubjectType = "plan_step"
    CommentSubjectConvergence CommentSubjectType = "convergence"
    CommentSubjectExploration CommentSubjectType = "exploration"
)

type CommentRecord struct {
    ID              string
    SubjectType     CommentSubjectType
    SubjectID       string
    IntentType      string
    AuthorKind      string
    TrustClass      string
    Body            string
    BasedOnRevision string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

## Phase Machine

This is the largest architectural gap in euclo today.

The current implementation has partial logic for:

- archaeology-related behavior
- confidence gating
- blast-radius preflight
- verification
- convergence

But these are not first-class durable phases. They are control-flow segments in
an execution-oriented agent.

### Proposed top-level phases

```go
type EucloPhase string

const (
    PhaseArchaeology       EucloPhase = "archaeology"
    PhaseIntentElicitation EucloPhase = "intent_elicitation"
    PhasePlanFormation     EucloPhase = "plan_formation"
    PhaseExecution         EucloPhase = "execution"
    PhaseVerification      EucloPhase = "verification"
    PhaseSurfacing         EucloPhase = "surfacing"
    PhaseBlocked           EucloPhase = "blocked"
    PhaseDeferred          EucloPhase = "deferred"
    PhaseCompleted         EucloPhase = "completed"
)
```

### Phase record

```go
type WorkflowPhaseState struct {
    WorkflowID          string
    CurrentPhase        EucloPhase
    EnteredAt           time.Time
    LastTransitionAt    time.Time
    ActiveExplorationID string
    ActivePlanID        string
    ActivePlanVersion   *int
    BlockedReason       string
    PendingGuidance     []string
    PendingLearning     []string
    RecomputeRequired   bool
    BasedOnRevision     string
}
```

### Phase semantics

#### Archaeology

Produces:

- candidate patterns
- candidate anchors
- candidate tensions
- blast-radius-relevant symbol relationships

This phase can be:

- explicitly requested
- re-entered after drift
- locally re-run for a narrowed scope during execution

Entry conditions:

- workflow exists
- workspace scope is known
- either no active exploration exists or recomputation is required

Primary activities:

- resolve workspace/project exploration session
- create a new exploration snapshot for the current archaeology pass
- run pattern detection, gap/tension discovery, and anchor-oriented analysis
- persist candidate artifacts into the active exploration session
- emit archaeology progress and completion events

Exit conditions:

- at least one exploration snapshot exists for the active session
- candidate findings and/or open learning questions have been materialized
- phase transitions either to `intent_elicitation`, `plan_formation`, or
  `surfacing`

#### Intent Elicitation

Consumes archaeology findings and learning interactions to establish:

- which patterns are intentional
- which inferred structures are rejected
- which anchors should be promoted to durable policy/commitment truth

This is where learning interactions primarily live.

Entry conditions:

- an active exploration session exists
- there are unresolved learning interactions, candidate patterns, or candidate
  tensions that require human confirmation/refinement

Primary activities:

- create learning interactions from archaeology outputs
- resolve confirm/reject/refine/defer outcomes
- update patterns, comments, anchors, and tensions accordingly
- record resulting provenance and semantic decisions

Exit conditions:

- no blocking learning interactions remain for the current plan-formation path
- the exploration session has enough confirmed intent to support planning
- phase transitions to `plan_formation`, `deferred`, or `surfacing`

#### Plan Formation

Produces a draft versioned living plan from:

- confirmed or accepted patterns
- confirmed anchors
- tensions
- current scope and task intent

Entry conditions:

- an active exploration session exists
- sufficient confirmed intent and scope grounding are available

Primary activities:

- synthesize a draft plan version from exploration findings
- attach evidence, comments, anchors, tensions, and pattern links
- persist the plan as a new versioned draft
- optionally activate that version when it becomes execution-authoritative

Exit conditions:

- a draft or active plan version exists for the workflow
- phase transitions to `execution`, `surfacing`, or back to
  `intent_elicitation` if plan grounding is insufficient

#### Execution

Executes plan steps against a specific plan version.

Entry conditions:

- there is an active plan version
- required evidence gates and scope checks have passed

Primary activities:

- hand execution off to euclo runtime and capability orchestration
- persist attempt state, guidance requests, and invalidation results
- monitor for drift, blockage, or recomputation triggers

Exit conditions:

- step execution advances, blocks, fails, or completes
- phase transitions to `verification`, `blocked`, `deferred`, or back to
  `archaeology` / `intent_elicitation` when recomputation is required

#### Verification

Runs:

- step-boundary gap detection
- derivation/provenance updates
- convergence verification
- semantic checkpoint artifact generation

Entry conditions:

- execution has produced a meaningful boundary worth checking, or an explicit
  verification request was made

Primary activities:

- run verification/convergence checks
- materialize tensions or contradictions discovered post-execution
- update plan validity and convergence state
- emit verification and convergence events

Exit conditions:

- verification result is persisted
- phase transitions to `surfacing`, `execution`, `intent_elicitation`, or
  `completed`

#### Surfacing

Presents accumulated findings or stable pause points to the client.

This is a phase, not just a UI flourish, because it indicates a meaningful
workflow boundary.

Entry conditions:

- there is a stable state worth exposing to one or more clients

Primary activities:

- project the current exploration, plan, learning, and verification state into
  read models
- expose a coherent pause/resume point
- allow client-specific UX to inspect without becoming the source of truth

Exit conditions:

- the workflow either pauses, resumes into another phase, or completes

#### Blocked

This phase represents an execution or reasoning stop that requires an external
decision before forward progress is allowed.

Typical reasons:

- unresolved operational guidance
- missing required evidence
- failed preconditions for plan execution
- unresolved contradictions that exceed automatic escalation thresholds

#### Deferred

This phase represents intentionally postponed meaning or workflow work.

Typical reasons:

- low-severity contradictions deferred as engineering observations
- unresolved learning interactions that are not blocking
- archaeology findings that should be preserved but not acted on immediately

#### Completed

This phase means the current workflow reached a stable terminal state for the
selected goal and active plan version.

Completion does not imply that exploration or semantic state will never be
recomputed later.

### Important property

Transitions are not strictly linear. For example:

- verification may transition back to intent elicitation
- execution may transition to blocked or deferred
- archaeology may be re-entered locally for blast-radius re-evaluation

The phase machine must support resumability and non-linear transitions.

## Learning HITL Modality

This is not merely "guidance with a different prompt."

### Core purpose

Learning HITL exists to refine euclo's semantic model of the codebase through
human engagement with surfaced structure.

Examples:

- "This pattern appears intentional; confirm?"
- "These two variants seem to represent the same boundary contract; merge or
  distinguish?"
- "This tension appears in three places; is it intentional debt or drift?"

### Expected outputs

Learning resolution should be able to create or update:

- anchor records
- pattern status
- pattern supersession chains
- pattern-to-anchor relationships
- tension status
- comments/explanations
- plan confidence inputs

### Why it must remain distinct

Guidance answers execution questions.
Learning answers meaning questions.

If these are collapsed together:

- the resulting artifacts become ambiguous
- provenance quality degrades
- plan grounding becomes less trustworthy

## Living Plan Lifecycle

This requires stronger lifecycle modeling than we currently have.

### Lifecycle stages

1. `Exploration`
2. `DraftPlan`
3. `ActivePlan`
4. `SupersededPlan`
5. `ArchivedPlan`

### Why plans must be versioned

The same workflow may produce:

- an initial draft plan
- a revised plan after archaeology confirmation
- a re-planned version after execution drift
- a post-verification refined plan

If we mutate one plan in place, we lose:

- provenance
- meaningful comparison
- confidence lineage
- the ability to revert to the last verified plan version

### Freshness and invalidation

Plan versions should carry:

- `based_on_revision`
- `computed_at`
- `stale_reason`
- `recompute_required`

Execution should not silently assume an old plan version is still valid after
significant drift.

## Versioning, Snapshots, and Code Revision Linkage

The research direction strongly implies that semantic state needs explicit
version linkage.

### Snapshot candidates

The system should be able to snapshot or reference:

- graph state
- active anchor state
- pattern registry state
- exploration session state
- living plan version

At minimum, each artifact should reference:

- code revision or workspace state hash
- creation timestamp
- parent/superseded artifact if applicable

This does not require immediate full snapshotting of every internal structure,
but the identifiers and model boundaries need to be designed now.

### Git and non-git contexts

Git is useful but should not be the only source of truth.

We should support:

- git revision references when available
- workspace content hash fallback when not

## Query and Mutation Model

Because the domain is complex, the API should be domain-shaped.

The initial GraphQL schema should be organized by boundary, not by raw store.

### Initial query boundaries

`WorkflowQuery`

- `workflow(id)`
- `workflowPhaseState(workflowId)`
- `workflowTimeline(workflowId)`

`ExplorationQuery`

- `explorationSession(id)`
- `activeExploration(workspaceId)`
- `explorationSnapshot(id)`
- `explorationSnapshots(explorationId)`
- `tensions(explorationId)`

`PatternQuery`

- `pattern(id)`
- `patternRegistry(scope)`
- `patternComments(patternId)`

`PlanQuery`

- `livingPlan(workflowId, version)`
- `activeLivingPlan(workflowId)`
- `planVersions(workflowId)`
- `planDiff(workflowId, fromVersion, toVersion)`

`InteractionQuery`

- `guidanceQueue(workflowId)`
- `learningQueue(workflowId)`
- `deferredObservations(workflowId)`

`VerificationQuery`

- `convergenceState(workflowId)`
- `verificationRuns(workflowId)`

These should resolve against projections/read models where possible, not by
joining raw stores in resolvers.

Ownership rule:

- exploration is workspace-scoped
- workflows may reference the active exploration for their workspace
- plan, guidance, verification, and execution-handoff state remain primarily
  workflow-scoped

### Initial mutation boundaries

`ExplorationMutation`

- `startArchaeology`
- `refreshArchaeology`
- `archiveExplorationSnapshot`

`LearningMutation`

- `confirmPattern`
- `rejectPattern`
- `refinePattern`
- `confirmAnchorCandidate`
- `resolveLearningInteraction`
- `deferObservation`

`PlanMutation`

- `createDraftPlan`
- `activatePlanVersion`
- `invalidatePlanVersion`
- `resumeWorkflow`

`ExecutionMutation`

- `handOffPlanExecution`

`VerificationMutation`

- `runVerification`

`GuidanceMutation`

- `resolveGuidance`

These should map to valid domain transitions, not generic row updates.

Decision:

- the server boundary stops at plan execution handoff
- `archaeo` is responsible for archaeology, intent grounding, plan formation,
  convergence-relevant verification, learning interactions, and phase state
- execution of the accepted plan is handed off to euclo runtime/capabilities
  outside the initial server scope

### Initial subscriptions

- `workflowPhaseChanged(workflowId)`
- `archaeologyProgress(workspaceId)`
- `explorationSnapshotCreated(explorationId)`
- `patternProposed(explorationId)`
- `learningInteractionRequested(explorationId)`
- `guidanceRequested(workflowId)`
- `planVersionCreated(workflowId)`
- `planVersionActivated(workflowId)`
- `verificationUpdated(workflowId)`
- `convergenceUpdated(workflowId)`
- `deferredObservationAdded(workflowId)`

Subscriptions are particularly important for relurpish and richer clients.

### Proposed GraphQL schema types

This is not intended to be a final SDL frozen forever, but it is intended to be
substantially more concrete than a boundary sketch. The goal is to define the
shape of the initial domain contract so package boundaries, projections, and
mutation semantics can be implemented against something specific.

#### Scalars and common enums

```graphql
scalar DateTime
scalar JSON

enum EucloPhase {
  ARCHAEOLOGY
  INTENT_ELICITATION
  PLAN_FORMATION
  EXECUTION
  VERIFICATION
  SURFACING
  BLOCKED
  DEFERRED
  COMPLETED
}

enum ExplorationStatus {
  ACTIVE
  STALE
  BLOCKED
  ARCHIVED
}

enum LivingPlanVersionStatus {
  DRAFT
  ACTIVE
  SUPERSEDED
  ARCHIVED
}

enum LearningInteractionKind {
  PATTERN_PROPOSAL
  ANCHOR_PROPOSAL
  TENSION_REVIEW
  INTENT_REFINEMENT
}

enum LearningInteractionStatus {
  PENDING
  RESOLVED
  EXPIRED
  DEFERRED
}

enum LearningResolutionKind {
  CONFIRM
  REJECT
  REFINE
  DEFER
}

enum TensionStatus {
  INFERRED
  CONFIRMED
  ACCEPTED
  UNRESOLVED
  RESOLVED
}

enum GuidanceState {
  PENDING
  RESOLVED
  DEFERRED
  TIMED_OUT
}
```

#### Common supporting types

```graphql
type RevisionRef {
  value: String!
  isGitRevision: Boolean!
  isWorkspaceHash: Boolean!
}

type Comment {
  id: ID!
  subjectType: String!
  subjectId: String!
  intentType: String!
  authorKind: String!
  trustClass: String!
  body: String!
  revision: RevisionRef
  createdAt: DateTime!
  updatedAt: DateTime!
}

type CommentRef {
  id: ID!
  subjectType: String!
  subjectId: String!
  intentType: String!
  authorKind: String!
  trustClass: String!
}

type EvidenceRef {
  kind: String!
  id: String!
  label: String
  excerpt: String
  revision: RevisionRef
}

type SymbolRef {
  id: ID!
  name: String!
  filePath: String
  packagePath: String
}

type FileRef {
  path: String!
}

type PageInfo {
  hasNextPage: Boolean!
  endCursor: String
}
```

#### Workflow and phase types

```graphql
type Workflow {
  id: ID!
  workspaceId: ID!
  phaseState: WorkflowPhaseState
  activeExploration: ExplorationSession
  activeLivingPlan: LivingPlanVersion
  convergenceState: ConvergenceState
}

type WorkflowPhaseState {
  workflowId: ID!
  currentPhase: EucloPhase!
  enteredAt: DateTime!
  lastTransitionAt: DateTime!
  activeExplorationId: ID
  activePlanId: ID
  activePlanVersion: Int
  blockedReason: String
  pendingGuidanceIds: [ID!]!
  pendingLearningIds: [ID!]!
  recomputeRequired: Boolean!
  basedOnRevision: RevisionRef
}

type WorkflowTimelineEvent {
  id: ID!
  type: String!
  occurredAt: DateTime!
  revision: RevisionRef
  payload: JSON!
}

type WorkflowTimelineConnection {
  nodes: [WorkflowTimelineEvent!]!
  pageInfo: PageInfo!
}
```

#### Exploration and archaeology types

```graphql
type ExplorationScope {
  workspaceRoot: String!
  corpusScope: String
  filePaths: [String!]!
  symbolIds: [ID!]!
  packagePaths: [String!]!
}

type ExplorationSession {
  id: ID!
  workspaceId: ID!
  status: ExplorationStatus!
  scope: ExplorationScope!
  activeSnapshot: ExplorationSnapshot
  latestSnapshot: ExplorationSnapshot
  basedOnRevision: RevisionRef
  computedAt: DateTime!
  recomputeRequired: Boolean!
  staleReason: String
  candidatePatterns: [PatternRecord!]!
  candidateAnchors: [AnchorCandidate!]!
  tensions: [Tension!]!
  openLearning: [LearningInteraction!]!
  confirmedIntentRefs: [String!]!
  deferredFindings: [DeferredObservation!]!
  comments: [CommentRef!]!
  createdAt: DateTime!
  updatedAt: DateTime!
}

type ExplorationSnapshot {
  id: ID!
  explorationId: ID!
  snapshotKey: String!
  basedOnRevision: RevisionRef
  createdAt: DateTime!
  triggeredByAgent: String
  candidatePatterns: [PatternRecord!]!
  candidateAnchors: [AnchorCandidate!]!
  tensions: [Tension!]!
  openLearning: [LearningInteraction!]!
  summary: String
}

type ArchaeologyProgress {
  workspaceId: ID!
  workflowId: ID
  explorationId: ID!
  snapshotId: ID
  stage: String!
  message: String
  progressPercent: Float
  updatedAt: DateTime!
}
```

#### Pattern, anchor, comment, and tension types

```graphql
type PatternInstance {
  symbolId: ID
  filePath: String!
  startLine: Int
  endLine: Int
  excerpt: String
}

type PatternRecord {
  id: ID!
  kind: String!
  status: String!
  title: String!
  description: String!
  confidence: Float!
  basedOnRevision: RevisionRef
  instances: [PatternInstance!]!
  anchorRefs: [String!]!
  commentRefs: [CommentRef!]!
  supersededBy: ID
  supersedes: ID
}

type AnchorCandidate {
  id: ID!
  term: String!
  definition: String
  trustClass: String
  sourceRef: String
}

type Tension {
  id: ID!
  explorationId: ID!
  snapshotId: ID
  patternIds: [ID!]!
  anchorRefs: [String!]!
  symbolScope: [String!]!
  kind: String!
  description: String!
  severity: String!
  status: TensionStatus!
  blastRadiusNodeIds: [ID!]!
  suggestedResolves: [PatternRecord!]!
  relatedPlanStepIds: [String!]!
  commentRefs: [CommentRef!]!
  basedOnRevision: RevisionRef
  createdAt: DateTime!
  updatedAt: DateTime!
}

type DeferredObservation {
  id: ID!
  workflowId: ID!
  title: String!
  description: String!
  sourceType: String!
  sourceId: String
  createdAt: DateTime!
}
```

#### Learning and guidance types

```graphql
type LearningChoice {
  id: ID!
  label: String!
  description: String
}

type LearningResolution {
  kind: LearningResolutionKind!
  choiceId: String
  refinedPayload: JSON
  commentRef: CommentRef
  resolvedBy: String
  resolvedAt: DateTime!
}

type LearningInteraction {
  id: ID!
  workflowId: ID!
  explorationId: ID!
  snapshotId: ID
  kind: LearningInteractionKind!
  subjectType: String!
  subjectId: String!
  title: String!
  description: String
  evidence: [EvidenceRef!]!
  choices: [LearningChoice!]!
  defaultChoice: String
  timeoutBehavior: String!
  status: LearningInteractionStatus!
  resolution: LearningResolution
  basedOnRevision: RevisionRef
  createdAt: DateTime!
  updatedAt: DateTime!
}

type GuidanceRequest {
  id: ID!
  workflowId: ID!
  kind: String!
  title: String!
  description: String
  state: GuidanceState!
  createdAt: DateTime!
  resolvedAt: DateTime
}
```

#### Plan and verification types

```graphql
type PlanStep {
  id: ID!
  title: String!
  description: String
  status: String!
  confidence: Float!
  requiredSymbols: [String!]!
  requiredAnchors: [String!]!
  dependsOn: [String!]!
  invalidatedAt: DateTime
}

type LivingPlanVersion {
  id: ID!
  workflowId: ID!
  version: Int!
  parentVersion: Int
  derivedFromExploration: ID
  basedOnRevision: RevisionRef
  semanticSnapshotRef: String
  status: LivingPlanVersionStatus!
  recomputeRequired: Boolean!
  staleReason: String
  computedAt: DateTime!
  activatedAt: DateTime
  supersededAt: DateTime
  steps: [PlanStep!]!
  comments: [CommentRef!]!
  tensions: [Tension!]!
  patterns: [PatternRecord!]!
  anchors: [String!]!
  createdAt: DateTime!
  updatedAt: DateTime!
}

type PlanDiff {
  workflowId: ID!
  fromVersion: Int!
  toVersion: Int!
  summary: String
  addedStepIds: [String!]!
  removedStepIds: [String!]!
  changedStepIds: [String!]!
}

type VerificationRun {
  id: ID!
  workflowId: ID!
  planId: ID
  planVersion: Int
  status: String!
  startedAt: DateTime!
  completedAt: DateTime
  failureReasons: [String!]!
}

type ConvergenceState {
  workflowId: ID!
  targetSatisfied: Boolean!
  unresolvedTensions: [Tension!]!
  missingPatternIds: [ID!]!
  lastVerifiedAt: DateTime
  lastRun: VerificationRun
}
```

#### Query root sketch

```graphql
type Query {
  workflow(id: ID!): Workflow
  workflowPhaseState(workflowId: ID!): WorkflowPhaseState
  workflowTimeline(workflowId: ID!, after: String, first: Int = 50): WorkflowTimelineConnection!

  explorationSession(id: ID!): ExplorationSession
  activeExploration(workspaceId: ID!): ExplorationSession
  explorationSnapshot(id: ID!): ExplorationSnapshot
  explorationSnapshots(explorationId: ID!): [ExplorationSnapshot!]!
  tensions(explorationId: ID!): [Tension!]!

  pattern(id: ID!): PatternRecord
  patternRegistry(scope: String): [PatternRecord!]!
  patternComments(patternId: ID!): [Comment!]!

  livingPlan(workflowId: ID!, version: Int!): LivingPlanVersion
  activeLivingPlan(workflowId: ID!): LivingPlanVersion
  planVersions(workflowId: ID!): [LivingPlanVersion!]!
  planDiff(workflowId: ID!, fromVersion: Int!, toVersion: Int!): PlanDiff

  guidanceQueue(workflowId: ID!): [GuidanceRequest!]!
  learningQueue(workflowId: ID!): [LearningInteraction!]!
  deferredObservations(workflowId: ID!): [DeferredObservation!]!

  convergenceState(workflowId: ID!): ConvergenceState
  verificationRuns(workflowId: ID!): [VerificationRun!]!
}
```

#### Input types

```graphql
input ArchaeologyScopeInput {
  workspaceId: ID!
  workflowId: ID
  corpusScope: String
  filePaths: [String!]
  symbolIds: [ID!]
  packagePaths: [String!]
}

input CommentInput {
  intentType: String!
  authorKind: String!
  body: String!
}

input ResolveLearningInteractionInput {
  interactionId: ID!
  expectedStatus: LearningInteractionStatus = PENDING
  resolutionKind: LearningResolutionKind!
  choiceId: String
  refinedPayload: JSON
  comment: CommentInput
}

input ConfirmPatternInput {
  patternId: ID!
  comment: CommentInput
}

input RejectPatternInput {
  patternId: ID!
  reason: String
  comment: CommentInput
}

input RefinePatternInput {
  patternId: ID!
  title: String
  description: String
  mergeWithPatternId: ID
  comment: CommentInput
}

input ConfirmAnchorCandidateInput {
  explorationId: ID!
  candidateId: ID!
  trustClass: String
  comment: CommentInput
}

input CreateDraftPlanInput {
  workflowId: ID!
  explorationId: ID!
  expectedExplorationSnapshotId: ID
  note: String
}

input ActivatePlanVersionInput {
  workflowId: ID!
  version: Int!
  expectedCurrentVersion: Int
}

input InvalidatePlanVersionInput {
  workflowId: ID!
  version: Int!
  reason: String!
}

input ResumeWorkflowInput {
  workflowId: ID!
  expectedPhase: EucloPhase
}

input HandOffPlanExecutionInput {
  workflowId: ID!
  planVersion: Int!
  expectedPhase: EucloPhase = EXECUTION
}

input RunVerificationInput {
  workflowId: ID!
  planVersion: Int
}

input ResolveGuidanceInput {
  guidanceId: ID!
  choiceId: String!
  note: String
}

input DeferObservationInput {
  workflowId: ID!
  sourceType: String!
  sourceId: String
  title: String!
  description: String!
}
```

#### Mutation payloads

GraphQL mutations should return structured payloads rather than only a boolean
or bare node. These payloads should carry enough state for clients to update
their local view model without immediately issuing multiple follow-up queries.

```graphql
type MutationError {
  code: String!
  message: String!
  field: String
  retryable: Boolean!
}

type StartArchaeologyPayload {
  workflow: Workflow
  exploration: ExplorationSession
  snapshot: ExplorationSnapshot
  phaseState: WorkflowPhaseState
  jobId: ID
  errors: [MutationError!]!
}

type RefreshArchaeologyPayload {
  exploration: ExplorationSession
  snapshot: ExplorationSnapshot
  phaseState: WorkflowPhaseState
  jobId: ID
  errors: [MutationError!]!
}

type ArchiveExplorationSnapshotPayload {
  exploration: ExplorationSession
  archivedSnapshotId: ID
  errors: [MutationError!]!
}

type ConfirmPatternPayload {
  pattern: PatternRecord
  exploration: ExplorationSession
  learningInteraction: LearningInteraction
  errors: [MutationError!]!
}

type RejectPatternPayload {
  pattern: PatternRecord
  exploration: ExplorationSession
  learningInteraction: LearningInteraction
  errors: [MutationError!]!
}

type RefinePatternPayload {
  pattern: PatternRecord
  supersededPattern: PatternRecord
  exploration: ExplorationSession
  learningInteraction: LearningInteraction
  errors: [MutationError!]!
}

type ConfirmAnchorCandidatePayload {
  exploration: ExplorationSession
  anchorRef: String
  errors: [MutationError!]!
}

type ResolveLearningInteractionPayload {
  interaction: LearningInteraction
  exploration: ExplorationSession
  updatedPattern: PatternRecord
  updatedTension: Tension
  errors: [MutationError!]!
}

type DeferObservationPayload {
  observation: DeferredObservation
  exploration: ExplorationSession
  errors: [MutationError!]!
}

type CreateDraftPlanPayload {
  livingPlan: LivingPlanVersion
  phaseState: WorkflowPhaseState
  errors: [MutationError!]!
}

type ActivatePlanVersionPayload {
  livingPlan: LivingPlanVersion
  phaseState: WorkflowPhaseState
  errors: [MutationError!]!
}

type InvalidatePlanVersionPayload {
  livingPlan: LivingPlanVersion
  phaseState: WorkflowPhaseState
  errors: [MutationError!]!
}

type ResumeWorkflowPayload {
  workflow: Workflow
  phaseState: WorkflowPhaseState
  errors: [MutationError!]!
}

type HandOffPlanExecutionPayload {
  workflow: Workflow
  phaseState: WorkflowPhaseState
  handoffAccepted: Boolean!
  handoffRef: String
  errors: [MutationError!]!
}

type RunVerificationPayload {
  verificationRun: VerificationRun
  convergenceState: ConvergenceState
  phaseState: WorkflowPhaseState
  jobId: ID
  errors: [MutationError!]!
}

type ResolveGuidancePayload {
  guidance: GuidanceRequest
  phaseState: WorkflowPhaseState
  errors: [MutationError!]!
}

type Mutation {
  startArchaeology(scope: ArchaeologyScopeInput!): StartArchaeologyPayload!
  refreshArchaeology(explorationId: ID!): RefreshArchaeologyPayload!
  archiveExplorationSnapshot(snapshotId: ID!): ArchiveExplorationSnapshotPayload!

  confirmPattern(input: ConfirmPatternInput!): ConfirmPatternPayload!
  rejectPattern(input: RejectPatternInput!): RejectPatternPayload!
  refinePattern(input: RefinePatternInput!): RefinePatternPayload!
  confirmAnchorCandidate(input: ConfirmAnchorCandidateInput!): ConfirmAnchorCandidatePayload!
  resolveLearningInteraction(input: ResolveLearningInteractionInput!): ResolveLearningInteractionPayload!
  deferObservation(input: DeferObservationInput!): DeferObservationPayload!

  createDraftPlan(input: CreateDraftPlanInput!): CreateDraftPlanPayload!
  activatePlanVersion(input: ActivatePlanVersionInput!): ActivatePlanVersionPayload!
  invalidatePlanVersion(input: InvalidatePlanVersionInput!): InvalidatePlanVersionPayload!
  resumeWorkflow(input: ResumeWorkflowInput!): ResumeWorkflowPayload!

  handOffPlanExecution(input: HandOffPlanExecutionInput!): HandOffPlanExecutionPayload!
  runVerification(input: RunVerificationInput!): RunVerificationPayload!
  resolveGuidance(input: ResolveGuidanceInput!): ResolveGuidancePayload!
}
```

#### Subscription payloads

Subscription payloads should carry enough information for clients to update
readable state directly rather than only exposing a string event type.

```graphql
type WorkflowPhaseChangedEvent {
  workflowId: ID!
  phaseState: WorkflowPhaseState!
  occurredAt: DateTime!
}

type ArchaeologyProgressEvent {
  workflowId: ID!
  progress: ArchaeologyProgress!
  occurredAt: DateTime!
}

type ExplorationSnapshotCreatedEvent {
  explorationId: ID!
  snapshot: ExplorationSnapshot!
  occurredAt: DateTime!
}

type PatternProposedEvent {
  workspaceId: ID!
  explorationId: ID!
  pattern: PatternRecord!
  occurredAt: DateTime!
}

type LearningInteractionRequestedEvent {
  workspaceId: ID!
  explorationId: ID!
  interaction: LearningInteraction!
  occurredAt: DateTime!
}

type GuidanceRequestedEvent {
  workflowId: ID!
  guidance: GuidanceRequest!
  occurredAt: DateTime!
}

type PlanVersionCreatedEvent {
  workflowId: ID!
  livingPlan: LivingPlanVersion!
  occurredAt: DateTime!
}

type PlanVersionActivatedEvent {
  workflowId: ID!
  livingPlan: LivingPlanVersion!
  phaseState: WorkflowPhaseState!
  occurredAt: DateTime!
}

type VerificationUpdatedEvent {
  workflowId: ID!
  verificationRun: VerificationRun!
  occurredAt: DateTime!
}

type ConvergenceUpdatedEvent {
  workflowId: ID!
  convergenceState: ConvergenceState!
  occurredAt: DateTime!
}

type DeferredObservationAddedEvent {
  workflowId: ID!
  observation: DeferredObservation!
  occurredAt: DateTime!
}

type Subscription {
  workflowPhaseChanged(workflowId: ID!): WorkflowPhaseChangedEvent!
  archaeologyProgress(workspaceId: ID!): ArchaeologyProgressEvent!
  explorationSnapshotCreated(explorationId: ID!): ExplorationSnapshotCreatedEvent!
  patternProposed(explorationId: ID!): PatternProposedEvent!
  learningInteractionRequested(explorationId: ID!): LearningInteractionRequestedEvent!
  guidanceRequested(workflowId: ID!): GuidanceRequestedEvent!
  planVersionCreated(workflowId: ID!): PlanVersionCreatedEvent!
  planVersionActivated(workflowId: ID!): PlanVersionActivatedEvent!
  verificationUpdated(workflowId: ID!): VerificationUpdatedEvent!
  convergenceUpdated(workflowId: ID!): ConvergenceUpdatedEvent!
  deferredObservationAdded(workflowId: ID!): DeferredObservationAddedEvent!
}
```

#### Mutation semantics notes

The schema above implies a few important rules:

- mutating operations should perform optimistic concurrency checks through
  `expectedCurrentVersion`, `expectedStatus`, or `expectedPhase` style fields
- errors should be structured and machine-consumable
- payloads should return both the mutated artifact and any immediately relevant
  enclosing state such as `phaseState` or `exploration`
- asynchronous operations should return a `jobId` when full results are not
  available synchronously

This should help keep the server domain-oriented without forcing the client to
rebuild state from scratch after every mutation.

## Event Model

Durable domain events should exist independently of UI subscriptions.

Decision:

- the first implementation keeps the archaeology-domain event model in euclo
  rather than trying to generalize it into framework immediately
- framework event infrastructure can still be reused as transport/logging
  substrate, particularly `framework/event`
- euclo should own the archaeology-specific event vocabulary and projection
  semantics for now

### Why

Events are needed for:

- replay/debugging
- alternate UX synchronization
- auditability
- provenance
- resumability
- downstream read-model projection

### Event record shape

Suggested shape:

```go
type ArchaeoEvent struct {
    ID            string
    PartitionKey  string
    WorkflowID    string
    ExplorationID string
    SnapshotID    string
    PlanID        string
    PlanVersion   *int
    Type          string
    OccurredAt    time.Time
    RevisionRef   string
    Payload       json.RawMessage
    Metadata      map[string]string
}
```

Initial partitioning should follow artifact ownership:

- workspace-partitioned for archaeology and exploration events
- workflow-partitioned for plan, guidance, verification, execution-handoff, and
  workflow phase events

This keeps exploration state aligned with its workspace scope while preserving
workflow-tailability for execution-oriented state.

### Event categories

`Phase events`

- `WorkflowPhaseTransitioned`
- `WorkflowBlocked`
- `WorkflowDeferred`
- `WorkflowCompleted`

`Exploration events`

- `ArchaeologyStarted`
- `ArchaeologyProgressed`
- `ArchaeologyCompleted`
- `ExplorationSnapshotCreated`
- `ExplorationMarkedStale`

`Pattern and learning events`

- `PatternProposed`
- `PatternConfirmed`
- `PatternRejected`
- `PatternRefined`
- `LearningInteractionRequested`
- `LearningInteractionResolved`
- `CommentAttached`

`Anchor and tension events`

- `AnchorDeclared`
- `AnchorDriftDetected`
- `TensionDetected`
- `TensionResolved`
- `ContradictionDeferred`

`Plan lifecycle events`

- `LivingPlanDrafted`
- `LivingPlanActivated`
- `LivingPlanSuperseded`
- `PlanVersionInvalidated`

`Verification events`

- `VerificationStarted`
- `VerificationCompleted`
- `ConvergenceFailed`
- `ConvergenceVerified`

`Execution handoff events`

- `PlanExecutionHandOffRequested`
- `PlanExecutionHandOffAccepted`

The last category exists because the server boundary stops at handoff, but the
handoff itself is still a domain event worth projecting.

### Projection model

Initial projections should include:

- workflow phase state projection
- exploration summary projection
- learning queue projection
- guidance queue projection
- active living plan projection
- plan version history projection
- convergence status projection
- workflow timeline projection

Subscriptions should be served from projection updates driven by the event
stream rather than ad hoc resolver-side polling.

## Concurrency and Multi-Client Concerns

Even local-only systems still need concurrency discipline.

### Concerns

- multiple UX clients attached to one local server
- stale client views
- conflicting learning or plan mutations
- repeated archaeology requests on the same workflow
- replan/execute races

### Recommended approach

- optimistic concurrency via version fields
- workflow-level operation leases or lightweight locks
- explicit mutation preconditions
- idempotent mutation semantics where practical

Examples:

- `activatePlanVersion` must assert the currently active version
- `resolveLearningInteraction` must fail if already resolved
- `executePlanStep` must assert plan version and current phase

## Long-Running Jobs and LLM Session Boundaries

Archaeology, exploration, planning, and verification may all be long-running.

### Server-side expectation

The server should manage jobs as durable domain operations with:

- job IDs
- progress
- cancellation
- partial result publication
- resumability markers

### Important constraint

LLM usage still remains bounded by the relurpify framework's runtime model.
We should not assume the server can arbitrarily preserve internal LLM context
across all operations.

Therefore:

- durable domain artifacts must live outside the LLM
- jobs may spin up or reuse agent execution sessions
- recomputation may occur after session loss or boundary transitions

This is another reason to treat the server as a domain coordinator rather than
as "the place where one immortal LLM lives."

## Framework vs Euclo Boundary

This is worth making explicit to avoid architectural drift.

### Framework should own

- generic durable stores and engines
- graph primitives
- plan primitives
- pattern/comment/anchor persistence primitives
- guidance transport/broker primitives
- capability policy/sandbox execution primitives
- reusable event transport foundations

### Euclo should own

- phase semantics
- archaeology workflow semantics
- intent-grounding workflow semantics
- learning interaction semantics
- plan lifecycle semantics
- convergence semantics
- euclo-specific event taxonomy and orchestration

### Server should own

- schema and resolvers
- command dispatch
- query projection access
- subscriptions
- lifecycle of server-owned local sessions/jobs

## Relationship to Relurpish

Relurpish should remain euclo's default TUI.

That means:

- it may be the most complete or most ergonomically optimized client
- it may expose richer UX shortcuts than alternate clients
- some relurpish-only UX features may remain client-local

But relurpish should not be the only place where euclo's domain model exists.

### Server-owned state

- archaeology sessions
- pattern proposals and confirmations
- learning interactions
- active living plan versions
- convergence state
- workflow phase state

### Client-owned state

- pane focus and layout
- local navigation history
- current visual filters
- ephemeral selection state
- local view-mode preferences
