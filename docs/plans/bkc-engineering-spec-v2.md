# Bi-directional Knowledge Compiler (BKC) — Engineering Specification v2

## Status

Engineering specification. Phase 1 implementation in progress.

---

## 1. Problem Statement

The current system treats context as a linear, bounded tape. Knowledge accumulates
within a session through archaeology findings, pattern confirmations, intent
grounding, and plan formation, but when the context window fills, continuity is
still weaker than the archaeology domain model implies it should be.

The current system already persists meaningful continuity artifacts:

- versioned living plans
- semantic provenance references
- learning interactions
- tensions
- compiled execution state
- provider/session restore state

That existing continuity is valuable, but it is still not a compiled semantic
knowledge graph. A restored session receives plan lineage, semantic references,
and execution continuity, but it does not yet receive a principled,
budget-aware semantic slice assembled from an unbounded durable graph.

The deeper problem remains algorithmic: how do you stream knowledge to a
bounded-context system in an unlimited way, without hard resets and without
reconstructing reasoning from scratch each time?

The answer is still a graph-based knowledge store with a budget-aware
compilation layer. The graph is unbounded. Any individual context window is
bounded. The missing piece is a principled mechanism to compile the right
bounded slice from the unbounded graph on demand, and to grow that graph
continuously without requiring full recomputation.

---

## 2. Core Concept

**Knowledge is a graph. Context is a compiled slice of that graph.**

A `KnowledgeChunk` is the atomic unit: a bounded, addressable, versioned,
provenance-preserving packet of inferred or confirmed knowledge. Chunks carry
multiple typed views and are never forced into a single ontology. A chunk about
a module's design intent may simultaneously be interpretable as a pattern
instance, a design decision, a constraint on future changes, and a plan
precondition. None of those views is authoritative; all are derived
projections.

The **forward pass** (compilation) takes findings from archaeology exploration,
pattern confirmation, living plan formation, user interaction, and background
workspace scans, then compiles them into chunks with typed relationship edges.

The **backward pass** (streaming) takes a mode-specific seed and a context
budget, traverses the chunk graph in dependency order, and emits an ordered
chunk sequence for injection into the context system. The session starts warm.

The graph grows continuously. Chunks become stale when code drifts; staleness
propagates through the edge graph; stale chunks surface as tensions for the
user to resolve. No hard reset is required.

---

## 3. Ownership Model

This version adopts the archaeology/euclo seam explicitly.

- `archaeo` owns durable semantic state, provenance, anchoring, invalidation,
  chunk storage, and streaming interfaces
- `named/euclo` owns execution behavior and Euclo-specific relurpic capability
  assemblies that invoke BKC services
- `ayenitd` deploys and manages infrastructure services such as workspace
  bootstrap and git watchers, but does not own BKC semantics

This keeps the dependency direction aligned with the existing architecture:

```text
framework/*  -> foundational primitives
archaeo/*    -> archaeology domain runtime and durable semantics
named/euclo  -> coding/execution runtime using archaeo
app/*        -> UX layers
agents/*     -> worker/capability implementations
```

---

## 4. Package Structure

BKC has a clean ownership split across archaeology-owned semantic runtime and
Euclo-owned relurpic capability behavior:

```text
archaeo/bkc/                       # archaeo-owned: durable semantic graph runtime
    chunk.go                       # KnowledgeChunk, KnowledgeEdge, all core types
    store.go                       # durable chunk storage
    graph.go                       # traversal, subgraph extraction, topological ordering
    staleness.go                   # freshness transitions and propagation
    view.go                        # lazy typed view rendering
    compiler.go                    # compiler interfaces and shared contracts
    stream.go                      # backward-pass streaming interfaces
    events.go                      # BKC event types/adapters over archaeology events

named/euclo/relurpicabilities/bkc/ # euclo-owned capability behavior
    relurpic.go                    # euclo:bkc.* capability registrations
    compile.go                     # capability entrypoint for forward pass
    stream.go                      # capability entrypoint for backward pass
    checkpoint.go                  # plan anchoring behavior
    invalidate.go                  # euclo-owned execution-side invalidation orchestration
```

`archaeo/bkc/` is allowed to depend on existing archaeology features and
constructs. It should reuse archaeology persistence and provenance systems
where practical instead of introducing a wholly separate storage stack.

---

## 5. Storage Model

`KnowledgeChunk` is a new semantic model, but it is not necessarily a wholly
separate persistence stack in Phase 1.

The implementation should:

- reuse existing archaeology durability patterns first
- remain compatible with workflow-backed persistence and provenance
- optionally use `framework/graphdb` as the relationship/index substrate where
  that materially improves graph traversal and invalidation

The rollout is **augment-first**, not replace-first:

- existing `SemanticInputBundle` and compiled execution continuity remain valid
- chunk-backed continuity is added as a richer semantic substrate
- backward streaming initially augments the current restore path
- later phases may migrate more semantic continuity to chunk-first seeding

---

## 6. Seed Sources and Rollout

Initial seed resolution order remains:

- planning / archaeology: anchored plan versions and archaeology session state
- chat / debug: selected files, scopes, and active tensions

Planning and archaeology are treated as adjacent parts of the same broader
archaeology/planning seam, but the rollout order remains acceptable as written
in the original specification.

---

## 7. Relurpic Capability Role

Relurpic remains a framework runtime family primitive. Euclo owns the
coding-specific capability assemblies that invoke BKC behavior.

BKC relurpic capabilities therefore live on the Euclo side of the seam, while
the semantic chunk system itself remains archaeology-owned.

---

## 8. Services

BKC services are either archaeology-owned or Euclo-owned, but are deployed and
managed by `ayenitd` in the same way as other bootstrapped services.

For the initial implementation, ayenitd-managed BKC services should remain
strictly infrastructure-oriented:

- workspace bootstrap scan
- git watcher / invalidation trigger

Compile and stream execution remain request-driven through archaeology and
Euclo runtime paths.

---

## 9. Phase Plan

The original phase structure is retained, with ownership adjusted to the
architecture above.

### Phase 1: `archaeo/bkc` foundation

**Goal**: Establish the durable chunk graph store and traversal foundation.
No Euclo dependency. No ayenitd dependency. No LLM dependency.

**Deliverables**:

- `archaeo/bkc/chunk.go` or equivalent chunk foundation package
- durable chunk storage and edge storage
- graph traversal helpers
- staleness propagation
- lazy view rendering registry

**Dependencies**:

- `framework/graphdb`
- existing archaeology durability/provenance patterns

**Notes**:

- Phase 1 may begin as a narrower package surface if that better fits the
  current codebase
- implementation should prefer compatibility with current archaeology and
  restore systems over forcing the original package shape mechanically

### Phase 2+

Subsequent phases remain conceptually the same as the original specification,
but with these ownership corrections:

- compiler/streaming contracts live in archaeology-owned BKC runtime
- Euclo relurpic capabilities invoke archaeology BKC services
- ayenitd manages only infrastructure services
- chunk-backed continuity augments existing semantic restore paths before any
  replacement is attempted

---

## 10. Implementation Guidance

The following constraints govern implementation:

- preserve the original technical intent and terminology where possible
- reuse existing archaeology systems rather than duplicating them
- prefer an additive migration over a parallel rewrite
- keep Euclo responsible for execution and relurpic capability behavior
- keep archaeology responsible for semantic durability and provenance
