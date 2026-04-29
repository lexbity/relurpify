# Agents Paradigm Migration Plan

## Part I - Engineering Specification

### 1. Problem Statement

The `agents/` tree is not a single agent implementation. It is a set of interoperable execution paradigms that all need to share the same framework backends while keeping their own orchestration style, context shape, and handoff policy.

This means the migration cannot be a one-size-fits-all rewrite. Each package should be treated as a separate generic implementation of an agent execution paradigm, while still interoperating through the shared framework boundary:

- `framework/contextdata.Envelope`
- `framework/agentgraph`
- `framework/contextstream`
- shared memory, persistence, compiler, and capability backends

The migration also needs to eliminate legacy context assumptions, especially any package that still treats `core.Context` or implicit state maps as the primary runtime object.

### 2. Architectural Principles

#### 2.1 Paradigm isolation

Each subpackage in `agents/` owns its own execution style, node set, prompt context model, and state conventions. Packages may share helpers, but they should not collapse into a single runtime abstraction.

#### 2.2 Shared backends

Agent packages may differ in how they manage context, but when they hand off work they use the same backends in the framework for:

- capability invocation
- retrieval
- compiler-triggered context streaming
- persistence
- working memory

#### 2.3 Handoff via envelope

Default handoff is a cloned envelope. A package may choose a filtered snapshot only if its paradigm requires stronger isolation or reduced payload transfer.

#### 2.4 Direct graph support

All agent packages should support `agentgraph` nodes directly and can define their own special nodes where required.

#### 2.5 Streaming is agent-controlled

Context streaming is not automatic. Each paradigm decides when to invoke it and whether the stream request blocks or runs in the background.

### 3. Package-by-Package Responsibilities

#### 3.1 `agents/react`

ReAct should remain the most envelope- and prompt-heavy paradigm. It needs streamed context access for prompt assembly, but it should still keep its own internal phase, message, and observation management.

#### 3.2 `agents/pipeline`

Pipeline is a linear, stage-driven paradigm and should be migrated to the new plan executor and envelope model cleanly. It likely benefits from sync streaming before stage prompt generation.

#### 3.3 `agents/blackboard`

Blackboard is stateful and shared-memory-heavy. It should preserve the blackboard object as a domain object, but its interaction with context should move toward cleaner envelope handoff and graph-native nodes.

#### 3.4 `agents/goalcon`

GoalCon should keep its backward-chaining and execution split, but its step execution and plan execution should become contextdata- and agentgraph-native.

#### 3.5 `agents/htn`

HTN already has a strong runtime model. It should keep its structured execution state, but align all handoffs and graph integration with the new envelope contract.

#### 3.6 `agents/rewoo`

ReWOO should preserve its step/checkpoint/replan structure, but move fully onto the new envelope and contextstream model without old checkpoint ownership.

#### 3.7 `agents/planner`

Planner needs to consume streamed context in prompt shaping and should use the new plan executor path and handoff contract.

#### 3.8 `agents/chainer`

Chainer should remain a light orchestration layer and should not depend on legacy core-context state models.

#### 3.9 `agents/reflection`

Reflection should keep its review/delegate structure but align all node execution and handoff paths with envelope semantics.

#### 3.10 `agents/relurpic`

Relurpic is a coordination layer. Its capability handlers should be adapted to the new envelope contract, cloned handoff, and stream-aware context model.

#### 3.11 `agents/llm`

LLM node behavior should be preserved as a direct graph node, with contextstream hooks available when a plan or paradigm requests them.

### 4. Context Survival Rules on Handoff

The preservation policy for handoff should come from the domain recipe of the source paradigm, not from a global one-size-fits-all rule.

The default preservation set should cover:

- task metadata
- working memory relevant to the current task
- streamed refs
- retrieval refs
- checkpoint refs
- required paradigm-local keys

The exact set should be derived from the semantics described in `named/euclo/thoughtrecipes` and similar domain-specific recipes.

### 5. Agentgraph Integration Strategy

Every agent package should be able to do one of the following:

- build and execute a graph directly
- define its own agent-specific nodes and still use `agentgraph`
- hand off into another agent through cloned or filtered envelopes

The migration should not invent a second workflow runtime.

### 6. Common Migration Risks

- packages that still use `core.Context` as their primary runtime object
- prompt builders that assume streamed context is unavailable
- checkpoint logic still embedded in agent packages instead of using compiler/persistence ownership
- packages that mix domain state and framework state in the same key space without a clear boundary

---

## Part II - Technical Multi-Phase Implementation Plan

### Phase 1 - Inventory and classify all agent paradigms

**Goal:** Establish the package-by-package migration map and classify the context model used by each agent family.

**Dependencies:** None.

#### Files and features to inspect and clean up

- `agents/doc.go`
- `agents/builder.go`
- `agents/environment.go`
- `agents/scope.go`
- every `agents/*/doc.go`
- every `agents/*/new.go`

#### Files and tests to create

- `docs/plans/agents-paradigm-migration-plan.md` itself
- a package inventory checklist if needed

#### Exit criteria

- every agent package is classified as graph-native, envelope-native, or legacy-handoff-boundary
- the migration order is explicit

### Phase 2 - Remove core-context dependencies from linear orchestration packages

**Goal:** Clean up packages that still rely on `core.Context` and align them to `contextdata.Envelope`.

**Dependencies:** Phase 1.

#### Primary packages

- `agents/pipeline`
- `agents/chainer`
- `agents/planner`
- `agents/goalcon`

#### Files and features to clean up

- `agents/pipeline/runner.go`
- `agents/pipeline/tools.go`
- `agents/pipeline/stage.go`
- `agents/chainer/runner.go`
- `agents/chainer/chainer_agent.go`
- `agents/planner/planner.go`
- `agents/goalcon/goalcon_agent.go`
- `agents/goalcon/execution/execution_adapter.go`
- `agents/goalcon/execution/step_executor.go`

#### Files and tests to update

- `agents/pipeline/*_test.go`
- `agents/chainer/*_test.go`
- `agents/planner/*_test.go`
- `agents/goalcon/*_test.go`

#### Exit criteria

- no production code in these packages depends on `core.Context` as the primary runtime state
- these packages all execute against `contextdata.Envelope`

### Phase 3 - Rework context assembly in prompt-heavy packages

**Goal:** Make prompt assembly explicitly stream-aware and envelope-native.

**Dependencies:** Phase 2 and the common contextstream refactor.

#### Primary packages

- `agents/react`
- `agents/planner`
- `agents/llm`
- `agents/relurpic`

#### Files and features to implement

- `agents/react/prompt_context.go`
  - stream-aware prompt assembly
  - streamed refs consumption
  - background vs synchronous request hints
- `agents/react/react_messages.go`
  - context-sensitive message loading and compaction
- `agents/planner/planner.go`
  - prompt generation that can request context streaming before model calls
- `agents/relurpic/*`
  - envelope-aware capability handlers and handoff boundaries

#### Files and tests to clean up

- `agents/react/react_state.go`
- `agents/react/react_messages.go`
- `agents/relurpic/agent_capability_handler.go`
- `agents/relurpic/relurpic_capabilities_*.go`

#### Exit criteria

- prompt construction can use streamed context refs where appropriate
- packages no longer assume streamed context is unavailable

### Phase 4 - Restore and normalize graph-native agent nodes

**Goal:** Ensure each paradigm keeps or gains graph-native nodes rather than hiding execution inside package-private loops.

**Dependencies:** Phases 2 and 3.

#### Primary packages

- `agents/react`
- `agents/blackboard`
- `agents/goalcon`
- `agents/rewoo`
- `agents/htn`

#### Files and features to implement

- `agents/react/react_graph.go`
- `agents/blackboard/blackboard_graph.go`
- `agents/goalcon/execution/execution_adapter.go`
- `agents/rewoo/graph_materializer.go`
- `agents/htn/htn_agent.go`
- package-specific node types as needed

#### Files and tests to clean up

- any ad hoc runner loops that duplicate graph execution
- any node code that mutates legacy context state directly

#### Exit criteria

- all major paradigms can execute through `agentgraph`
- special nodes remain package-local where they add value

### Phase 5 - Introduce streaming-trigger integration in agent workflows

**Goal:** Make the new streaming-trigger node usable from agent execution flows.

**Dependencies:** Common refactor Phases 2 and 3.

#### Primary packages

- `agents/react`
- `agents/planner`
- `agents/pipeline`
- `agents/htn`
- `agents/goalcon`
- `agents/blackboard`
- `agents/rewoo`

#### Files and features to implement

- package-level logic that inserts or invokes the streaming trigger node before LLM calls
- package-specific policies for sync versus async streaming
- package-specific handling of trimmed context results

#### Files and tests to clean up

- old prompt assembly TODOs that assume streamed context is missing
- any hidden refresh logic that bypasses the new node

#### Exit criteria

- at least one package can request context streaming before an LLM call
- both sync and background streaming are represented in agent workflows

### Phase 6 - Normalize checkpoint and persistence boundaries in agents

**Goal:** Remove checkpoint ownership from agent code and align agents with the common persistence/compiler boundary.

**Dependencies:** Common refactor checkpoint cleanup plus Phase 2.

#### Primary packages

- `agents/rewoo`
- `agents/htn`
- `agents/pipeline`
- `agents/react`
- `agents/blackboard`

#### Files and features to clean up

- `agents/rewoo/checkpoint_store.go`
- `agents/rewoo/checkpoint_node.go`
- `agents/htn/htn_agent_persistence.go`
- `agents/htn/persistence/checkpoint.go`
- checkpoint-specific code in `agents/react/react_graph.go`
- checkpoint-specific code in `agents/pipeline/pipeline_agent.go`

#### Files and tests to update

- package tests that assume local checkpoint materialization

#### Exit criteria

- agent packages only request or consume checkpoint state
- persistence ownership is no longer duplicated in agent code

### Phase 7 - Formalize paradigm-specific handoff and filtering rules

**Goal:** Encode the default cloned-envelope handoff and the paradigm-specific filtered snapshot behavior.

**Dependencies:** Phase 2 and the contextdata handoff contract.

#### Primary packages

- `agents/react`
- `agents/blackboard`
- `agents/htn`
- `agents/pipeline`
- `agents/goalcon`
- `agents/rewoo`
- `agents/relurpic`

#### Files and features to implement

- handoff helpers that clone the envelope by default
- package-local filters for snapshot handoff where needed
- preservation of task, working memory, streamed refs, retrieval refs, and agent-local keys

#### Files and tests to clean up

- old ad hoc context-copying helpers
- direct `map[string]any` handoff code where a filtered envelope should be used

#### Exit criteria

- every paradigm has a documented handoff policy
- cloned envelope is the default transfer mechanism

### Phase 8 - Package-specific convergence passes

**Goal:** Apply the migration to the unique edge cases of each paradigm.

**Dependencies:** Phases 2 through 7.

#### Package-specific work

- `agents/react`
  - reconcile prompt compaction, observations, and streaming updates
- `agents/blackboard`
  - keep blackboard state domain-local while using envelope handoff cleanly
- `agents/goalcon`
  - complete the separation between planning and execution surfaces
- `agents/htn`
  - align runtime state snapshots and dispatch metadata with envelope semantics
- `agents/pipeline`
  - move plan execution out and wire the new plan package
- `agents/rewoo`
  - normalize replan/checkpoint logic around the shared framework boundary
- `agents/planner`
  - finalize stream-aware prompt assembly and result persistence
- `agents/chainer`
  - keep the chain runtime minimal and envelope-native
- `agents/reflection`
  - align review and delegate nodes with envelope handoff
- `agents/relurpic`
  - ensure orchestration capability handlers are envelope-driven

#### Exit criteria

- each package has no remaining local legacy state assumptions that conflict with the shared framework model

### Phase 9 - Global cleanup and stale reference removal

**Goal:** Remove any remaining stale references to removed legacy patterns and confirm the package graph is internally consistent.

**Dependencies:** Phases 1 through 8.

#### Files and tests to clean up

- all `agents/*` production files that still reference deleted graph runtime files
- all tests that still assume old context or checkpoint ownership
- docs that still describe `core.Context` as the shared agent runtime object

#### Files to create

- boundary verification scripts if needed for agents

#### Exit criteria

- stale references are gone
- repo docs describe the envelope-first, stream-aware architecture

### Phase 10 - Full regression pass

**Goal:** Prove the migrated agents still interoperate through framework backends.

**Dependencies:** Phases 1 through 9.

#### Validation tasks

- targeted `go test` for affected agent packages
- targeted `go test` for `framework/contextdata`, `framework/contextstream`, and `framework/agentgraph`
- repository-wide search for removed symbols and context-shim patterns

#### Exit criteria

- the repo builds for affected paths
- the agent packages still interoperate through shared framework backends
- the new context-stream trigger node is available and usable from agent workflows

## Appendix - Package Change Index

| Package | Primary Migration Theme |
| --- | --- |
| `agents/react` | stream-aware prompt assembly and envelope-native execution |
| `agents/pipeline` | move plan execution and remove core-context dependencies |
| `agents/blackboard` | clean envelope handoff while preserving domain-local blackboard state |
| `agents/goalcon` | backward-chaining and step execution on the new shared runtime |
| `agents/htn` | runtime state alignment and checkpoint boundary cleanup |
| `agents/rewoo` | replan/checkpoint normalization |
| `agents/planner` | prompt assembly + streaming-aware execution |
| `agents/chainer` | minimal envelope-native chain execution |
| `agents/reflection` | review/delegate node alignment |
| `agents/relurpic` | coordination handlers on envelope + graph boundaries |

