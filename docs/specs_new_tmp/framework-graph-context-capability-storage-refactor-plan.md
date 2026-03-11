# Framework Graph, Context, Capability, and Storage Refactor Plan

This document is a working refactor plan for the Relurpify framework runtime. It focuses on four connected concerns that currently overlap but do not form a coherent execution model:

- graph execution and resumability
- context management and summarization
- capability requirements and node placement
- durable workflow and checkpoint storage

The goal is to move these concerns from loosely coupled utilities into an explicit runtime contract that the graph can validate, execute, checkpoint, summarize, and resume safely.

This document is intentionally scoped to runtime, context, capability, and durable workflow-storage consolidation. Native retrieval-engine, ingestion-pipeline, and serving-tier design should live in a separate follow-on architecture document so this refactor does not absorb a second major subsystem.

## Goal

Refactor the framework so that:

- capabilities are the primary execution abstraction, with tools treated as one specific capability kind
- graph nodes declare their execution requirements, not just their runtime behavior
- checkpointing is resumable without replaying already-completed side effects
- context compression and summarization are explicit workflow stages, not mostly implicit budget reactions
- durable workflow state, checkpoint artifacts, and summary artifacts share one storage model
- capability trust, risk, placement, and recoverability are visible at graph planning and graph validation time

## Capability Model Clarification

Capabilities are the general runtime abstraction. Tools are a specific case of capability, not a parallel abstraction.

In Relurpify terms:

- a `skill` is typically a bundle of capabilities plus policy/instructions about when and how to use them
- `relurpicCapabilities` are closer to the runtime concept that the research describes as reusable procedural units or executable skills
- procedural memory should therefore target reusable capability-backed routines, not user-facing skill manifests directly

Implications:

- graph contracts should depend on capability requirements, not tool-only requirements
- tool nodes are capability-consuming nodes with a narrower execution path
- provider calls, remote node actions, prompts, resources, sessions, and subscriptions should fit the same contract vocabulary
- workflow planning and storage should record capability usage first, with tool usage as one subtype of capability execution
- procedural-memory retrieval should return executable routines or capability compositions, not only prose descriptions
- procedural-memory promotion and execution must remain capability-policy constrained even when a routine is already stored and reusable

## Current Problems

### 1. Graph nodes have behavior but not execution contracts

Today a graph node exposes only identity, type, and `Execute`. That keeps the runtime simple, but it prevents:

- preflight validation of required capabilities
- scheduling or placement decisions before execution
- durable recording of node requirements for audit and replay
- graph-level policy checks before a tool or provider call is attempted

As a result, capability metadata exists in the registry and policy layers, but not in the workflow structure itself.

### 2. Checkpointing resumes from the wrong semantic boundary

The current graph creates checkpoints after a node has executed, but stores the current node ID as the resume cursor. On restore, the runtime resumes by running that same node again. This is unsafe for:

- tool nodes with side effects
- remote provider calls
- delegated execution
- human approval nodes with one-time semantics

Checkpointing must resume from the next transition boundary, not by replaying the completed node.

### 3. Context management and summarization are not first-class graph operations

The framework has useful context tools today:

- `ContextManager`
- `ContextPolicy`
- `SharedContext`
- compression strategies
- file demotion and progressive loading

But the main workflow treats summarization and checkpointing as secondary concerns:

- budget pressure triggers compression reactively
- per-item `Compress()` often truncates instead of producing durable summaries
- graph checkpoints are callback-driven and storage-oriented rather than workflow-oriented
- no explicit summary artifact is produced at known graph boundaries

This makes context state harder to inspect, persist, and reuse across runs.

The current context components are intentionally separated, but their naming and contract boundaries should be clarified in the refactor:

- `Context` remains the base execution context and should stay minimal
- `SharedContext` is an extension layer over `Context` for richer shared working material
- `ContextManager` is the orchestration layer responsible for managing context movement and budgeted coordination
- `ContextPolicy` centralizes strategy selection, progressive loading, and context compression policy

The main consolidation need here is clarity of role and vocabulary, not forced merger into a single type.

### 4. Storage is split across overlapping persistence models

The repository already has multiple storage shapes:

- graph checkpoints in `framework/memory/checkpoint_store.go`
- workflow snapshots in `framework/memory/workflow_store.go` that should be folded into the checkpoint store lane
- richer workflow records in `framework/memory/workflow_state_store.go`

These are useful independently, but they do not currently define one authoritative runtime persistence model. The result is duplication in:

- cursor state
- summary state
- artifact state
- resume semantics
- auditability

In particular, `WorkflowStore` and `CheckpointStore` overlap too much as snapshot-style persistence lanes. The target design should treat `CheckpointStore` as the definitive name and abstraction for resumable workflow snapshot storage, with old `WorkflowStore` behavior merged into that lane rather than preserved as a separate long-term concept.

### 5. Memory is still treated too much like one bucket

The design direction should be stronger than "add a memory node" or "persist more context".

The runtime needs at least three distinct memory classes with different persistence rules:

- working memory for short-lived graph coordination and scratch state
- declarative memory for durable facts, decisions, constraints, and preferences
- procedural memory for reusable executable routines backed by capabilities

Without that split, transient execution noise pollutes long-term storage, retrieval quality degrades, and the runtime falls back toward prompt-bloating instead of actual reuse.

### 6. Retrieval and ingestion are not yet specified as first-class companion subsystems

The runtime refactor is necessary, but by itself it is not enough to produce a good memory system.

The framework also still needs a separate ingestion and retrieval architecture for:

- structure-aware parsing and chunking
- stable document and chunk identifiers
- append-only document versioning
- batched embedding generation
- separate document store, metadata store, and vector index roles
- logical delete with offline compaction
- metadata-first filtering before expensive retrieval stages
- hybrid sparse and dense retrieval
- reranking and bounded context packing
- retrieval and packing observability

Without those lanes, retrieval remains an implementation detail and the system risks falling back into the common failure mode of naive dense top-k retrieval directly into prompt context.

That design work should be tracked in a separate document so the runtime/storage refactor can define interfaces and storage roles without taking on full retrieval-engine implementation scope.

## Target Architecture

The refactor should introduce one explicit runtime model:

1. A graph is made of executable nodes plus node contracts.
2. Each node contract declares capability, recoverability, placement, checkpoint, and context semantics.
3. The graph runtime executes node transitions and checkpoints transition boundaries.
4. The graph actively manages separate memory classes with separate retrieval and persistence paths.
5. Summarization, retrieval, and checkpointing are represented by explicit system nodes or explicit runtime transition hooks with persisted outputs.
6. Durable storage records:
   - workflow identity
   - run identity
   - node transition history
   - checkpoint artifacts
   - summary artifacts
   - declarative memory records
   - procedural memory records
   - provider/session snapshots when needed

Here, "durable storage records" means the authoritative persisted runtime record of what the workflow is, what happened during execution, and what memory or artifacts were intentionally retained after the run.

This is distinct from:

- graph state, which is small working memory for the currently executing graph
- prompt context, which is the bounded model-visible material used at inference time
- large raw payloads, which should usually live in referenced artifacts rather than inline workflow records

In practical terms:

- workflow identity records define the long-lived workflow object
- run identity records define one concrete execution attempt of that workflow
- node transition history records define how execution moved from node to node
- checkpoint and summary artifacts preserve resumable state and durable compressed context
- declarative and procedural memory records preserve durable knowledge selected for reuse
- provider/session snapshots preserve external execution state only when resumability requires it

This runtime model should be compatible with a later native memory architecture, but it should not fully specify ingestion, indexing, ranking, and serving internals here.

## Memory and Context Model

The refactor should separate memory concerns into distinct layers with explicit roles.

Terminology note:

- "context compression" should be treated as the overarching term for the summarization and demotion mechanisms that reduce active context pressure
- specific summaries, compressed artifacts, and rolling summaries should be modeled as outputs of context compression rather than treated as unrelated concepts

### 1. Graph State for Working Memory

Graph state should hold small, structured working memory only.

Examples:

- current task metadata
- current step ID
- active decisions
- selected capability/provider IDs
- compact routing flags
- references to artifacts, checkpoints, and summaries

Graph state should not be the place where full transcripts, large tool payloads, or broad thread history live.

### 2. Declarative Long-Term Memory

Declarative memory should store durable facts and constraints, not execution chatter.

Examples:

- user or workspace preferences
- stable facts and decisions
- constraints and policies
- prior verified findings
- compact project knowledge objects

Declarative memory should live in an external store, and for Relurpify the default durable backing should be SQLite-backed storage with structured query interfaces.

### 3. Procedural Memory

Procedural memory should store executable knowledge: reusable routines, capability compositions, and verified execution strategies.

Examples:

- reusable capability sequences
- verified recovery routines
- task-specific helper flows promoted into general routines
- reusable relurpic capability compositions

Procedural memory is the main system-level analogue of the "skill library" described in the research, but in Relurpify it should be modeled below user-facing skill manifests. The durable unit should be an executable routine or routine descriptor, not a full authored skill manifest.

Procedural memory must remain capability-policy constrained:

- storing a routine does not grant new authority
- executing a stored routine must still satisfy capability selectors, trust limits, and risk limits at runtime
- routine retrieval should prefer descriptors that can be validated against current policy before activation
- routine promotion should be conservative and should not persist one-off workaround flows as reusable procedures

### 4. Memory Layers

Memory classes and memory layers are related but different. The runtime should model both.

This section separates three different concerns that were previously collapsed together:

- caches, which are acceleration layers
- storage tiers, which are durability and access layers
- derived summaries, which are compact memory artifacts

#### Caches

- exact cache for repeated grounded queries
- semantic cache for near-duplicate grounded queries

Caches are:

- optional accelerators
- eviction-driven
- not the source of truth for durable memory

#### Storage Tiers

- hot recent store for active thread and recent workflow state
- warm durable store for routinely searchable retained memory
- cold archive for infrequently accessed historical memory

Storage tiers are:

- persistence layers with different latency and access expectations
- where durable records and artifacts live over time

#### Derived Summaries

- rolling conversation summaries
- entity/state summaries

Derived summaries are:

- compact memory products derived from larger histories or artifacts
- inputs to retrieval and hydration
- not cache tiers by themselves

These memory layers should influence:

- latency expectations
- retrieval order
- eviction and compaction behavior
- persistence defaults
- what is eligible for prompt injection versus reference-only storage

These memory layers should also align with projection semantics. The framework should converge on one hot/warm/cold vocabulary rather than maintaining separate taxonomies for:

- memory storage layers
- workflow projection tiers
- coordination handoff tiers

### 5. Checkpoints for Step and Thread Persistence

Checkpoints should persist the execution cursor and enough runtime state to resume a step or thread safely.

Checkpoint contents should prefer:

- next-node cursor
- small structured state
- references to summary artifacts
- references to provider or session snapshots when needed

Checkpoint contents should avoid:

- embedding the entire conversational transcript by default
- duplicating large raw artifacts already persisted elsewhere

### 6. Retrieval Nodes

Memory should be pulled into active execution only when needed.

The runtime should support explicit retrieval nodes that:

- query declarative memory
- query procedural memory
- load only relevant summaries, routines, or artifacts
- materialize compact structured results into graph state or working context
- avoid eager loading of global history into every node

These nodes should depend on a retrieval interface contract, not on one concrete retrieval backend implementation.

### 7. Summarizer and Compressor Nodes

The runtime should include explicit summarizer/compressor nodes that compress older thread context into rolling summaries so active state remains small.

These nodes should:

- summarize old interaction windows
- emit structured summary artifacts
- preserve provenance and source ranges
- replace bulky thread context with references plus compact extracted facts

### 8. Structured Artifacts, Not Prose Handoffs

Artifacts should be small structured objects whenever possible, not long prose handoffs.

Examples:

- step result object
- decision record
- issue record
- verification record
- file-change summary object
- provider snapshot object

Freeform text is still useful for human-readable summaries, but it should usually be a view over structured artifacts rather than the authoritative payload.

## Bad Patterns to Avoid

The refactor should explicitly avoid these common workflow-runtime failures:

- putting full chat history into graph state
- passing full history through every node by default
- allowing every sub-agent to see everything regardless of task scope
- storing most outputs as prose instead of structured objects
- using text handoffs where artifact references would be more stable and queryable
- treating checkpoints as transcript dumps rather than resumable execution boundaries
- treating successful executions as automatically permanent skills
- persisting broad low-value memory by default instead of applying narrow runtime persistence rules

## Reference Runtime Decomposition

A memory-aware graph runtime should expose clear control points instead of hiding them inside one agent loop.

Suggested graph decomposition:

- `Planner`
- `StateUpdater`
- `DeclarativeRetriever`
- `ProceduralRetriever`
- `Executor`
- `Critic`
- `PersistenceWriter`

That makes the graph not just an execution graph, but the operating system for memory.

## Core Design Changes

## Phase 1: Introduce Node Contracts

Target:

- extend graph nodes so the runtime can understand execution requirements before running them

Deliverables:

- additive node contract types in `framework/graph` or `framework/core`
- graph validation support for contract-aware checks
- backward-compatible adapter path for existing nodes

Implementation steps:

1. Add a `NodeContract` type with fields for:
   - `RequiredCapabilities []core.CapabilitySelector` or a stronger selector type
   - `PreferredPlacement` with node/provider hints
   - `MaxRiskClass core.RiskClass`
   - `RequiredTrustClass core.TrustClass`
   - `Recoverability core.RecoverabilityMode`
   - `CheckpointPolicy`
   - `ContextPolicy`
   - `SideEffectClass`
   - `Idempotency`
2. Add a new optional interface in `framework/graph`, for example:
   - `type ContractNode interface { Node; Contract() NodeContract }`
3. Keep the existing `Node` interface unchanged for compatibility.
4. Add helper functions so the graph can fetch a node contract with sane defaults when a node does not implement `ContractNode`.
5. Extend graph validation to inspect node contracts before execution.
6. Add initial contract implementations for:
   - `ToolNode` as the tool-specific capability execution case
   - `LLMNode`
   - `HumanNode`
   - any planner or ReAct-specific system nodes that have known semantics

Notes:

- this phase should be additive only
- existing graph builders must continue to work

Acceptance criteria:

- existing nodes still execute without modification
- new nodes can declare contracts
- validation can fail early on clearly invalid contract combinations

## Phase 2: Separate Node Completion from Resume Cursor

Target:

- make checkpoint/resume semantics safe for side-effecting nodes

Deliverables:

- transition-boundary checkpoint records
- explicit next-node resume cursor
- safer replay semantics for tool and human nodes

Implementation steps:

1. Introduce a durable concept of a node transition record:
   - `FromNodeID`
   - `CompletedNodeID`
   - `NextNodeID`
   - `TransitionReason`
   - `CompletedAt`
2. Refactor graph execution so checkpoint creation happens after next-node resolution, not before.
3. Change the checkpoint payload to store:
   - `CompletedNodeID`
   - `NextNodeID`
   - `LastResultSummary` or equivalent lightweight metadata
   - visit counts and execution path
4. Resume from `NextNodeID`, not `CompletedNodeID`.
5. Add explicit semantics for terminal checkpoints:
   - if `NextNodeID == ""`, the checkpoint is completed and should not re-run work
6. Add tests covering:
   - side-effecting tool node checkpoint after completion
   - human approval node checkpoint after approval
   - graph resume after conditional branching
   - graph resume after parallel branch completion

Notes:

- this phase fixes the most immediate correctness bug in graph resumability
- migration code may need to reject or mark old-format checkpoints as non-resumable

Acceptance criteria:

- resumed runs do not replay completed tool nodes
- completed workflows do not restart from the final node

## Phase 3: Establish Explicit Memory Classes and State Boundaries

Target:

- separate working, declarative, and procedural memory in the runtime model

Deliverables:

- memory-class vocabulary in `framework/core` or `framework/memory`
- state-boundary rules for graph nodes
- artifact/reference-first state usage rules

Implementation steps:

1. Add explicit memory-class types and terminology:
   - working memory
   - declarative memory
   - procedural memory
2. Add state-boundary rules to `NodeContract.ContextPolicy` so nodes declare:
   - which state keys they can read
   - which state keys they can write
   - whether they can access thread history
   - whether they can perform retrieval
3. Define which data may live directly in graph state:
   - task metadata
   - step metadata
   - compact routing flags
   - artifact references
   - memory references
4. Define which data must not live directly in graph state by default:
   - full thread transcripts
   - large raw tool payloads
   - broad retrieval dumps
   - full sub-agent histories
5. Refactor existing graph nodes and agent loops to use artifact references instead of copying large values into state.
6. Add validation or lint-like helpers that flag oversize or non-structured state writes in tests and debug builds.

Notes:

- this phase creates the memory model vocabulary that the later retrieval and persistence phases depend on
- the key design rule is "small structured state, large durable artifacts"

Acceptance criteria:

- graph state usage is explicitly bounded and documented
- memory-class separation exists in code-level types and contracts

## Phase 4: Add Retrieval, Checkpoint, and Summary Nodes

Target:

- make state transitions and memory movement explicit in the graph itself

Deliverables:

- framework-provided checkpoint node
- framework-provided summarizer node
- declarative retrieval node
- procedural retrieval node
- optional hydrate/restore node
- graph builder helpers for inserting them

Implementation steps:

1. Add new system-node implementations in `framework/graph`:
   - `CheckpointNode`
   - `SummarizeContextNode`
   - `RetrieveDeclarativeMemoryNode`
   - `RetrieveProceduralMemoryNode`
   - `HydrateContextNode`
2. `CheckpointNode` should:
   - persist a checkpoint artifact
   - write checkpoint metadata into context state
   - emit telemetry as a workflow event, not only as a graph callback
3. `SummarizeContextNode` should:
   - summarize selected history and/or selected context items
   - produce a durable summary artifact
   - optionally demote or replace raw context entries with references to that summary artifact
4. `RetrieveDeclarativeMemoryNode` should:
   - query the declarative store
   - fetch only task-relevant structured memory records
   - write compact retrieved objects or references into active state
5. `RetrieveProceduralMemoryNode` should:
   - query the procedural store or routine index
   - return top-k reusable routines or capability compositions
   - attach only compact descriptors to state and leave code/routine bodies in artifacts or storage
6. `HydrateContextNode` should:
   - restore selected summary artifacts or provider state into active execution context
   - support resuming long-running workflows without loading everything into prompt context
7. Add graph builder helpers such as:
   - `BuildPlanExecuteSummarizeVerifyGraph`
   - `WrapWithCheckpointing`
   - `WrapWithPeriodicSummaries`
   - `WrapWithDeclarativeRetrieval`
   - `WrapWithProceduralRetrieval`

Notes:

- explicit nodes improve inspectability and debugging
- callback-based checkpointing can remain as a low-level fallback, but should no longer be the main orchestration API

Acceptance criteria:

- at least one production agent path uses explicit checkpoint/retrieval/summarize nodes
- retrieval results are bounded, typed, and stored as structured outputs

## Phase 5: Add Declarative and Procedural Memory Stores

Target:

- define durable storage lanes for declarative memory and procedural memory instead of treating both as generic artifacts
- establish the first consolidated durable memory substrate that later retrieval interfaces can sit on

Deliverables:

- declarative memory record schema
- procedural memory record schema
- SQLite-backed persistence path for both memory classes
- retrieval interfaces over both stores

Implementation steps:

1. Define a declarative memory schema for:
   - facts
   - decisions
   - constraints
   - preferences
   - project knowledge objects
2. Define a procedural memory schema for:
   - routine descriptors
   - executable bodies or references
   - capability dependencies
   - verification metadata
   - reuse and version metadata
3. Add SQLite-backed stores or tables for both memory classes.
4. Add retrieval interfaces that support:
   - bounded top-k fetches
   - type filtering
   - task-scope filtering
   - artifact reference return paths
5. Ensure procedural memory retrieval is keyed by compact descriptions and metadata, while the durable value remains an executable routine or routine reference.
6. Add compatibility layers so existing artifact and vector-store helpers can be reused where appropriate without making them the only storage model.
7. Enforce procedural-memory policy constraints on retrieval and execution:
   - retrieved routines must be revalidated against current capability policy
   - stored routines do not bypass trust, risk, or exposure constraints
   - promotion metadata should record which policies were satisfied at write time

Notes:

- this phase should use SQLite as the default durable substrate in Relurpify
- procedural memory should remain capability-centered, not manifest-centered
- procedural memory must remain capability-policy constrained at both retrieval time and execution time

Acceptance criteria:

- declarative and procedural memory are stored and retrieved separately
- transient execution state is not required to reconstruct either memory class

## Phase 6: Add Runtime Persistence Policies

Target:

- make persistence narrow, structured, and runtime-managed instead of broad and implicit

Deliverables:

- persistence policy types
- persistence writer nodes
- auditable persistence records

Implementation steps:

1. Define runtime persistence rules for declarative memory such as:
   - persist explicit facts, decisions, constraints, and compact summaries
   - do not persist raw interaction windows by default
   - collapse repeated near-identical records
2. Define runtime persistence rules for procedural memory such as:
   - persist verified reusable routines
   - avoid persisting one-off stateful workaround flows as durable routines
   - prefer updating or versioning an existing routine over writing duplicates
3. Add a `PersistenceWriter` node that persists memory records and artifacts through one structured path.
4. Record persistence reasons and origin metadata in workflow records for audit and tuning.
5. Ensure successful execution does not imply "store everything"; only stable structured outputs and reusable routines should be written durably.
6. Add background cleanup or compaction rules for duplicate or stale memory records where needed.

Notes:

- this phase should stay mostly runtime-managed and not require end-user tuning
- persistence rules should be conservative by default

Acceptance criteria:

- declarative persistence is narrow and structured by default
- procedural persistence prefers verified reusable routines
- the system can explain why a memory or routine was persisted

## Follow-on Retrieval Architecture

The native ingestion, retrieval, reranking, packing, cache-tier, and observability design should be specified in a separate document:

- `docs/specs_new_tmp/memory-retrieval-ingestion-architecture.md`

This refactor plan should only define the runtime-facing contracts that the retrieval architecture must satisfy:

- retrieval nodes and retrieval interfaces
- artifact and state boundaries
- unified persistence hooks for retrieval events when needed
- policy constraints on declarative and procedural memory access

Local search, progressive loading, workflow projections, and retrieval nodes should converge on these runtime-facing contracts rather than continuing as parallel long-term abstractions.

## Phase 7: Capability-Aware Preflight, Placement, and Storage Consolidation

Target:

- connect contracts, memory classes, and durable runtime storage into one authoritative control plane

Deliverables:

- graph preflight validator
- node placement evaluator
- consolidated workflow runtime store
- migration path from current checkpoint and workflow stores

Implementation steps:

1. Add a graph preflight pass that inspects every node contract before execution.
2. Validate:
   - required capabilities exist in the registry
   - tool requirements are modeled as capability requirements, not as a separate validation path
   - trust and risk constraints can be satisfied
   - placement requirements are resolvable
   - recoverability expectations match provider or session capabilities
3. Add placement resolution hooks that can:
   - choose a local tool
   - choose a provider capability
   - choose the best node based on platform, online status, and trust
   - choose relevant procedural routines before execution
4. Define a top-level workflow runtime store interface that combines:
   - workflow records
   - run records
   - node transition records
   - checkpoint artifacts
   - summary artifacts
   - declarative memory records
   - procedural memory records
   - retrieval records or memory access events
   - provider and session snapshots
5. Decide which current storage layer becomes authoritative:
   - likely `workflow_state_store` for normalized durable records
   - keep `workflow_store` only as a temporary compatibility layer while its snapshot features are merged into `CheckpointStore`
6. Treat `CheckpointStore` as the definitive name and abstraction for resumable snapshot storage, absorbing the old `WorkflowStore` snapshot role.
7. Move graph checkpoint persistence away from a standalone ad hoc JSON store as the primary path.
8. Add adapters so current code can still save, load, and list legacy checkpoints during migration.
9. Update telemetry and audit logging so storage writes correspond to durable workflow events.

Acceptance criteria:

- a graph with impossible requirements fails before execution
- workflow, checkpoint, summary, and memory records are queryable through one runtime store
- placement and preflight decisions are inspectable and reproducible

## Phase 8: Agent Integration, Evaluation, and Verification

Target:

- move existing agents onto the new runtime model and verify that memory retrieval and persistence improve behavior without causing state bloat

Deliverables:

- updated ReAct integration
- updated planner integration
- compatibility wrappers for old graph builders
- evaluation and regression test matrix

Implementation steps:

1. Update ReAct to:
   - use explicit checkpoint, retrieval, critique, and summarize nodes
   - record capability-aware node contracts for act and observe steps
   - persist summary artifacts for tool observation history
   - persist reusable execution routines through the structured procedural-memory path
2. Update planner or architect paths to:
   - checkpoint between plan, execute, verify, and summarize boundaries
   - persist plan artifacts and step summaries through the unified runtime store
3. Add compatibility helpers so legacy graph builders continue to work while new agents adopt contract-aware, memory-aware nodes.
4. Add migration toggles or feature flags for:
   - old checkpointing path
   - new checkpoint node path
   - declarative retrieval
   - procedural retrieval
   - structured persistence path
5. Add graph tests for:
   - checkpoint after next-node resolution
   - resume from conditional nodes
   - resume from terminal completion
   - parallel branch merge with checkpoint boundaries
6. Add memory tests for:
   - declarative retrieval bounds
   - procedural retrieval bounds
   - artifact-backed compression
   - summary rehydration
   - persistence rule behavior
7. Add evaluation metrics for:
   - retrieval accuracy or usefulness
   - persisted-memory precision and noise rate
   - procedural reuse rate
   - state-size growth over long runs
8. Update documentation in `docs/framework.md` and agent docs after the implementation stabilizes.

Acceptance criteria:

- at least two active agent flows use the new runtime path
- legacy flows remain functional during migration
- the runtime can recover a partially completed workflow safely
- summarization, retrieval, and persistence are observable and testable as first-class behavior

## Recommended Implementation Order

Recommended order:

1. Phase 2 first, to fix checkpoint correctness
2. Phase 1 next, to add node contracts without breaking callers
3. Phase 3, to establish memory classes and state boundaries
4. Phase 4, to introduce explicit checkpoint, retrieval, and summary nodes
5. Phase 5, to add declarative and procedural stores
6. Phase 6, to add runtime persistence rules before broad persistence
7. Phase 7, to connect preflight, placement, and unified storage
8. Phase 8 throughout, with new tests added incrementally per phase

## Suggested File Ownership

Primary file areas expected to change:

- `framework/graph/graph.go`
- `framework/graph/graph_checkpoint.go`
- `framework/graph/patterns.go`
- new graph node files under `framework/graph/`
- `framework/contextmgr/context_manager.go`
- `framework/contextmgr/context_policy.go`
- `framework/core/context_item.go`
- `framework/core/shared_context.go`
- `framework/core/capability_types.go`
- `framework/capability/node_support.go`
- `framework/memory/checkpoint_store.go`
- `framework/memory/workflow_store.go`
- `framework/memory/workflow_state_store.go`
- active agents under `agents/pattern/`

## Risks

Key risks:

- over-designing the node contract model before the migration path is proven
- making explicit summary nodes mandatory too early
- breaking compatibility with existing graph builders and checkpoint files
- coupling graph contracts too tightly to one capability registry implementation
- storing too much raw context in durable artifacts without clear retention rules

Mitigations:

- keep node contracts additive at first
- preserve compatibility adapters
- migrate one agent path at a time
- treat explicit checkpoint/summarize nodes as preferred, not immediately mandatory
- define summary artifact retention and redaction rules before broad rollout

## Consolidation Strategy for Existing Stores

The current repository has multiple overlapping persistence mechanisms. The consolidation strategy should narrow each store to one clear role and remove duplicate authority.

### 1. `WorkflowStateStore` becomes the authoritative durable runtime store

`WorkflowStateStore` should be the system of record for:

- workflow identity
- run identity
- step and step-run records
- artifacts and summaries
- declarative memory records
- procedural memory records
- workflow events
- provider and session snapshots

This is the durable runtime record that other helper stores and caches should defer to.

### 2. `CheckpointStore` becomes the definitive resumable snapshot store

`CheckpointStore` should be the definitive name and abstraction for resumable snapshot persistence.

Its responsibility should cover:

- next-node cursor checkpoints
- small structured resumable state
- checkpoint metadata
- references to summary and provider/session snapshot artifacts
- compatibility support for older snapshot-style workflow persistence

This means the old `WorkflowStore` snapshot role should be merged into `CheckpointStore`, not retained as a parallel long-term concept.

### 3. `WorkflowStore` becomes a migration layer only

The existing `WorkflowStore` should not remain a first-class storage model after consolidation.

Its role should be limited to:

- reading legacy workflow snapshots
- adapting legacy data into checkpoint and workflow-state records
- supporting temporary migration and rollback paths

After migration stabilizes, `WorkflowStore` should be removed or reduced to a thin compatibility adapter with no new feature surface.

### 4. `HybridMemory` becomes transient cache/scratch, not durable authority

`HybridMemory` should no longer be treated as the durable source of project or global memory.

After consolidation, it should be limited to:

- session-scoped scratch memory
- transient acceleration cache
- optional local convenience layer in front of authoritative durable records

Durable declarative and procedural memory should move into structured records backed by the authoritative runtime store rather than generic JSON key/value memory.

### 5. Artifacts remain a separate persistence concern

Large raw payloads should continue to live as artifacts or artifact refs rather than being expanded into inline workflow or checkpoint records.

This keeps:

- checkpoints resumable
- workflow records queryable
- summaries compact
- raw payload storage replaceable without changing runtime semantics

### 6. Resulting authority model

After consolidation, the intended authority model is:

- `WorkflowStateStore` for durable structured runtime records
- `CheckpointStore` for resumable snapshot state
- artifact storage for large raw payloads and summary bodies
- transient caches for speed only

Not:

- multiple parallel durable stores with overlapping ownership of cursor, summary, and memory state

## Additional Consolidation Decisions

The following framework-level consolidation decisions should guide implementation alongside the storage changes above.

### 1. Context roles should stay separated, but be named and documented more clearly

The intended separation is:

- `Context` as the base execution context
- `SharedContext` as the richer shared extension over `Context`
- `ContextManager` as the orchestration layer
- `ContextPolicy` as the strategy and compression-policy layer

This plan should preserve that separation while making the names and responsibilities explicit in code and docs.

### 2. Vocabulary should be normalized across checkpointing and persistence

The framework should use these terms consistently:

- `checkpoint` for resumable execution boundaries
- `snapshot` for captured provider/session or external runtime state
- `artifact` for large persisted payloads or summarized payload bodies
- `record` for structured durable runtime entities

This should reduce ambiguity across graph, workflow, provider, and persistence code.

### 3. Telemetry and event data should be separated

The intended split is:

- telemetry is graph-tied operational, audit, and debug output
- workflow/event data is separate durable event/record data

Telemetry should not become the authoritative workflow event history, and workflow events should not be treated as just another debug stream.

### 4. Policy surfaces should continue consolidating around capabilities

Tool policies, capability policies, exposure policies, insertion policies, session policies, and skill-policy hints should continue moving toward one capability-centered policy model with narrower subdomains rather than parallel policy vocabularies.

### 5. Execution paths should continue converging on one capability-centered runtime contract

Tool execution, capability execution, provider-backed execution, resource reads, and placement-aware execution should converge on one contract vocabulary with typed execution sub-kinds instead of separate orchestration models.

### 6. Context compression should be the overarching summary term

The framework should treat "context compression" as the umbrella concept that covers:

- rolling summaries
- compressed thread history
- demoted file/context representations
- summarized artifacts used for hydration or prompt insertion

This avoids keeping unrelated summary taxonomies at the same architectural level.

### 7. Runtime-facing search and retrieval features should converge

Even with full retrieval-engine design deferred to a separate document, the runtime should align:

- progressive loading
- workflow projection reads
- local search adapters
- retrieval nodes
- memory reads

under a shared retrieval/hydration contract instead of preserving each as a separate long-term interface family.

### 8. Persistence ownership should converge under `framework/memory`

The framework should standardize persistence ownership under `framework/memory` rather than splitting durable runtime concepts across competing package boundaries.

### 9. Recoverability should be framed more broadly as recovery

Checkpoint resume, provider/session resumability, delegation state restoration, and partial workflow continuation should converge under one broader recovery model.

`Recoverability` can remain as a typed field where useful, but the architectural concept should be "recovery" rather than several independent resume semantics.

### 10. Projection tiers and memory layers should be unified

The framework should converge on one hot/warm/cold model that can be used consistently for:

- workflow projection tiers
- memory storage layers
- coordination handoff shaping

This does not mean every subsystem must share the exact same implementation, but they should share one conceptual vocabulary.

## Current Recommendation

Start with a narrow implementation slice:

- add node contracts
- fix checkpoint cursor semantics
- add a basic `CheckpointNode`
- add a basic `SummarizeContextNode`
- persist both through the richer workflow state model
- define the minimal retrieval interface contract, without taking on the full retrieval-engine implementation

That slice is enough to validate the architecture without refactoring every context and storage path at once. If it works cleanly in ReAct first, the rest of the framework can migrate incrementally while retrieval-engine work proceeds as a separate architecture track.

## Proposed Core Schemas

This section proposes concrete shapes for the first implementation pass. These are not required to land exactly as written, but they should be close enough to anchor interfaces, storage, and tests.

## 1. NodeContract

Purpose:

- declare what a node needs and how it behaves operationally
- allow preflight validation, placement, checkpoint policy, and resumability decisions before execution

Suggested shape:

```go
type NodeContract struct {
	Kind                 NodeContractKind
	RequiredCapabilities []core.CapabilitySelector
	PreferredPlacement   *PlacementPolicy
	RequiredTrustClass   core.TrustClass
	MaxRiskClass         core.RiskClass
	Recoverability       core.RecoverabilityMode
	Idempotency          IdempotencyClass
	SideEffectClass      SideEffectClass
	CheckpointPolicy     CheckpointPolicy
	ContextPolicy        ContextBoundaryPolicy
	InputSchema          *core.Schema
	OutputSchema         *core.Schema
	Annotations          map[string]any
}
```

Supporting types:

```go
type NodeContractKind string

const (
	NodeContractKindLLM        NodeContractKind = "llm"
	NodeContractKindCapability NodeContractKind = "capability"
	NodeContractKindHuman      NodeContractKind = "human"
	NodeContractKindSystem     NodeContractKind = "system"
	NodeContractKindSummary    NodeContractKind = "summary"
	NodeContractKindRetrieval  NodeContractKind = "retrieval"
)

type IdempotencyClass string

const (
	IdempotencyUnknown      IdempotencyClass = "unknown"
	IdempotencyIdempotent   IdempotencyClass = "idempotent"
	IdempotencyNonIdempotent IdempotencyClass = "non_idempotent"
)

type SideEffectClass string

const (
	SideEffectNone        SideEffectClass = "none"
	SideEffectStateOnly   SideEffectClass = "state_only"
	SideEffectExternal    SideEffectClass = "external"
	SideEffectHumanGate   SideEffectClass = "human_gate"
)

type PlacementPolicy struct {
	PreferLocal            bool
	PreferNodeID           string
	PreferProviderID       string
	PreferPlatform         core.NodePlatform
	RequireOnline          bool
	RequireSessionAffinity bool
}

type CheckpointPolicy struct {
	BeforeExecution bool
	AfterExecution  bool
	AfterTransition bool
	Required        bool
}

type ContextBoundaryPolicy struct {
	ReadStateKeys        []string
	WriteStateKeys       []string
	ReadArtifacts        []string
	WriteArtifactKinds   []string
	AllowHistoryAccess   bool
	AllowMemoryRetrieval bool
}
```

Design notes:

- `RequiredCapabilities` is the primary dependency list, even for tool nodes
- tool nodes should typically map to `NodeContractKindCapability`
- `ContextBoundaryPolicy` is important for preventing the “every node sees everything” failure mode
- `Idempotency` and `SideEffectClass` should be used by checkpoint planning and resume rules

## 2. CheckpointArtifact

Purpose:

- persist resumable execution state at safe graph boundaries
- separate execution cursor state from bulky context payloads

Suggested shape:

```go
type CheckpointArtifact struct {
	CheckpointID        string
	WorkflowID          string
	RunID               string
	TaskID              string
	ThreadID            string
	Status              CheckpointStatus
	CompletedNodeID     string
	NextNodeID          string
	TransitionKind      string
	GraphVersionHash    string
	StateRef            string
	SummaryArtifactIDs  []string
	ProviderSnapshotIDs []string
	VisitCounts         map[string]int
	ExecutionPath       []string
	Metadata            map[string]any
	CreatedAt           time.Time
}
```

Supporting types:

```go
type CheckpointStatus string

const (
	CheckpointStatusReady      CheckpointStatus = "ready"
	CheckpointStatusCompleted  CheckpointStatus = "completed"
	CheckpointStatusSuperseded CheckpointStatus = "superseded"
)
```

Design notes:

- `NextNodeID` is the authoritative resume cursor
- `StateRef` should point to a compact structured state payload, not a transcript dump
- `SummaryArtifactIDs` attach rolling thread summaries that let resume avoid reloading broad history
- `ProviderSnapshotIDs` support resumable provider-backed workflows without embedding large session objects inline

## 3. SummaryArtifact

Purpose:

- hold durable compressed thread, step, or file summaries as structured artifacts
- replace prose handoffs with queryable objects plus optional human-readable text

Suggested shape:

```go
type SummaryArtifact struct {
	ArtifactID         string
	WorkflowID         string
	RunID              string
	ThreadID           string
	Kind               SummaryArtifactKind
	SourceKind         string
	SourceIDs          []string
	WindowStart        *time.Time
	WindowEnd          *time.Time
	SummaryText        string
	KeyFacts           []SummaryFact
	Decisions          []DecisionRef
	Issues             []IssueRef
	TokenEstimate      int
	CompressionMethod  string
	SummarizerName     string
	SummarizerVersion  string
	Provenance         SummaryProvenance
	Metadata           map[string]any
	CreatedAt          time.Time
}
```

Supporting types:

```go
type SummaryArtifactKind string

const (
	SummaryArtifactKindThreadRolling SummaryArtifactKind = "thread_rolling"
	SummaryArtifactKindStepResult    SummaryArtifactKind = "step_result"
	SummaryArtifactKindFileSummary   SummaryArtifactKind = "file_summary"
	SummaryArtifactKindMemoryDigest  SummaryArtifactKind = "memory_digest"
)

type SummaryFact struct {
	Kind      string
	Key       string
	Value     string
	Confidence float64
}

type DecisionRef struct {
	DecisionID string
	Title      string
	Status     string
}

type IssueRef struct {
	IssueID   string
	Title     string
	Status    string
	Severity  string
}

type SummaryProvenance struct {
	CapabilityIDs []string
	NodeIDs       []string
	ProviderIDs   []string
	CheckpointIDs []string
}
```

Design notes:

- `SummaryText` is useful, but `KeyFacts`, `Decisions`, and `Issues` should carry the durable semantics
- these artifacts should be usable both by retrieval nodes and by UI/debug surfaces
- thread-rolling summaries should be append-friendly and replaceable over time

## 4. MemoryRetrievalNodeResult

Purpose:

- make retrieval nodes return compact, structured memory instead of dumping raw records into state

Suggested shape:

```go
type MemoryRetrievalNodeResult struct {
	Query              MemoryQuery
	Results            []RetrievedMemoryRecord
	ArtifactRefs       []string
	InsertedStateKeys  []string
	Truncated          bool
	RetrievalLatencyMS int
}
```

Supporting types:

```go
type MemoryQuery struct {
	Scope          []string
	Kinds          []string
	TaskTypes      []string
	EntityKeys     []string
	TextQuery      string
	MaxResults     int
	IncludeArtifacts bool
}

type RetrievedMemoryRecord struct {
	RecordID      string
	Kind          string
	Title         string
	StructuredData map[string]any
	Summary       string
	Score         float64
	ArtifactRef   string
}
```

Design notes:

- retrieval results should be bounded and optionally ranked
- `StructuredData` should be preferred over large freeform text
- `InsertedStateKeys` makes it clear what the retrieval node actually made visible to downstream nodes

## Suggested Storage Mapping

Suggested first-pass mapping of these schemas onto the existing memory layer:

- `NodeContract`
  - stored mainly as graph/runtime metadata
  - optionally copied into workflow event or stage records for audit
- `CheckpointArtifact`
  - stored as a workflow artifact plus durable cursor metadata
- `SummaryArtifact`
  - stored as a workflow artifact or long-term memory record, depending on scope
- `MemoryRetrievalNodeResult`
  - stored as a compact workflow event payload plus optional retrieval artifact

## Suggested First Increment

The smallest useful implementation slice for these schemas is:

1. Add `NodeContract` and `ContractNode`
2. Change graph checkpoints to persist `NextNodeID`
3. Introduce `CheckpointArtifact` in the workflow storage layer
4. Introduce `SummaryArtifact` for rolling thread summaries
5. Add a minimal retrieval node that returns `MemoryRetrievalNodeResult`

That is enough to prove the architecture without fully replacing every existing context and memory helper.
