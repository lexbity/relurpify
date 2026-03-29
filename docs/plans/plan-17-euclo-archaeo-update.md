Plan 17: Euclo Compiled Execution, Deferred Continuity, and Long-Running Runtime Maturation
Status
Proposed engineering specification and multi-phase implementation plan.

This plan is intentionally not a minimum viable pass. It assumes euclo is now mature enough that the next work should strengthen it as a coding runtime for long-running execution, constrained LLM availability, and archaeology-backed continuity.

Summary
Evolve named/euclo from a structured coding orchestrator into a stronger long-running coding runtime built around four principles:

archaeo remains the live planning and semantic memory substrate
euclo compiles actionable archaeology state into a first-class execution artifact
long-running execution must continue safely even when fresh LLM access is unavailable
context compaction must preserve semantic continuity by offloading durable meaning into archaeology/provenance and compiled execution state
This plan focuses on euclo runtime behavior, not the relurpish UX implementation itself. However, it explicitly defines the runtime state that relurpish will later need to surface.

Problem Statement
Euclo already does several things well:

task classification
mode resolution
profile selection
capability-family routing
multi-phase execution
evidence gating
resumable interaction state
archaeology-assisted plan execution
But it still behaves primarily like:

a structured coding agent with archaeology support
rather than:

a coding runtime executing against a compiled live plan with durable continuity
That gap matters because:

long-running coding tasks should not require continuous LLM access
archaeology-backed plans should become the primary execution truth for complex work
deferred ambiguity/issues should not destabilize successful execution
long-running sessions need controlled context compaction without losing provenance or execution continuity
weaker local models benefit more from compiled structure and deterministic continuation than from repeated re-reasoning
Core Design Principles
1. Euclo has two operational regimes
Euclo must explicitly support:

planning/exploration regime

LLM-assisted
archaeology-heavy
user-interactive
relurpic specialist routines active here
compiled execution regime

archaeology-plan-backed
capability/tool driven
deterministic where possible
tolerant of temporary LLM unavailability
ambiguity converted into deferred follow-up rather than immediate replanning
2. Compiled execution state is first-class
A long-running execution should not depend on reconstructing everything from:

current chat history
transient phase state
or repeated ad hoc archaeology lookups
Instead, euclo should persist a compiled execution artifact that captures the actionable subset of:

plan structure
execution intent
expectations
archaeology refs
thresholds/policies
current progress
deferred issue linkage
verification policy
3. Archaeo remains the semantic authority
Archaeo continues to own:

live planning
provenance
deferred draft / convergence / decision state
durable request and semantic memory
Euclo consumes and compiles archaeology state; it should not replace it.

4. Deferred issues are problem-scoped and user-resolved
When unresolved ambiguity, stale fulfillment, or execution-era issue appears:

euclo should create or update deferred issue state grouped by problem
execution may still succeed
deferred issues are mentioned explicitly to the user
revisiting them requires explicit user action or archaeology re-entry
5. Long-running context requires compaction
Execution-local context should be allowed to shrink aggressively over time, as long as:

compiled execution state remains intact
archaeology/provenance retains durable semantics
current step/run/deferred issue continuity remains recoverable
Target Package Ownership
named/euclo/*
Owns:

compiled execution artifact lifecycle
execution-local runtime state
LLM-availability-aware execution policy
deterministic continuation over live plans
context compaction policy and execution-local state reload
deferred issue creation triggers during execution
Does not own:

live planning semantics
convergence semantics
provenance authority
remote transport concerns
archaeo/*
Owns:

living plans
provenance
deferred drafts
convergence records
decision records
request validity and application
archaeology continuity across euclo compaction
app/relurpish/*
Owns:

rendering status, indicators, controls, and user flows for the new euclo runtime state
not part of this plan, but this plan must produce the runtime state it will need
Important New Runtime Concepts
Compiled Execution Artifact
A durable euclo-owned artifact representing the actionable compiled state for a long-running run.

Suggested fields:

workflow_id
run_id
workspace_id
compiled_at
source_plan_id
source_plan_version
source_exploration_id
source_snapshot_id
mode
profile
step_order
active_step_id
completed_step_ids
deferred_issue_ids
plan_expectations
verification_policy
execution_thresholds
continue/defer/block policy
archaeology_refs
provenance refs
plan refs
relevant deferred/convergence refs
capability_policy_snapshot
enough metadata to know what execution envelope was assumed
llm_required_for_next_transition
status
Deferred Execution Issue
A euclo execution-facing record that links to archaeology state but is owned as part of the execution continuity surface.

Grouped by problem, not plan step.

Suggested fields:

workflow_id
run_id
problem_key
title
summary
origin
mutation checkpoint
verification contradiction
stale result
ambiguity
archaeology request issue
related_step_ids
related_request_ids
related_plan_refs
deferred_draft_refs
decision_refs
comment_refs
severity
status
actionability
continue
follow_up_recommended
user_review_recommended
LLM Availability State
A runtime-visible state describing whether fresh model reasoning is available for the current run.

Suggested states:

available
temporarily_unavailable
blocked_by_session_ownership
disabled_for_execution
required_before_continue
This should be explicit in runtime state, not inferred indirectly.

Implementation Phases
Phase 1: Define And Persist Compiled Execution Artifact
Why
Euclo currently prepares execution using archaeology state and runtime state, but it does not persist one explicit execution artifact that can serve as:

failsafe
historical referral point
resume anchor
compaction anchor
Without this, long-running execution continuity is too dependent on reconstructed transient state.

Implement
Add euclo execution artifact types in named/euclo/euclotypes or a new named/euclo/executionstate package.
Persist compiled execution artifacts into the workflow artifact store using the existing euclo artifact machinery.
Compile the artifact during the transition from planning/preparation into execution.
Include:
active living plan version
ordered step set
active step
mode/profile snapshot
verification expectations
deferral thresholds
archaeology refs
current execution status
Add load/rebuild helpers:
load latest compiled execution artifact
validate it against current workflow/plan/run
resume from it when possible
Suggested package/files
named/euclo/euclotypes/types.go
new named/euclo/runtime/compiled_execution.go
new named/euclo/runtime/compiled_execution_test.go
named/euclo/agent.go
optional helper in agents/internal/workflowutil
Tests
Unit:

compile artifact from a prepared living plan
serialize/persist/load round-trip
resume reads compiled artifact correctly
stale or mismatched plan version invalidates execution artifact reuse
Integration:

extend named/euclo/euclotest/agent_test.go
extend named/euclo/euclotest/persistence_test.go
verify compiled execution artifact exists after entering execution
Acceptance:

every workflow-backed execution run that reaches execution has one current compiled execution artifact
resume can reconstruct execution continuity from artifact + archaeology state
Phase 2: Make Long-Running Execution Artifact-Driven
Why
Currently, euclo still feels phase/mode-driven first and archaeology-assisted second. For long-running work, execution should instead be driven by:

compiled execution artifact
archaeology plan state
step progress
deferred issues
That reduces dependency on repeated fresh reasoning.

Implement
Refactor prepareExecution(...) and execution-session handoff so that once execution begins:
the compiled execution artifact becomes the primary local execution state
step progression updates the artifact
plan completion/failure/defer transitions update the artifact
Separate:
planning-time phase machinery
execution-time artifact-driven progression
Keep mode/profile information, but execution should consult them as policy, not as the primary source of what to do next.
Suggested package/files
named/euclo/agent.go
named/euclo/runtime/routing.go
named/euclo/runtime/verification.go
new named/euclo/runtime/execution_progress.go
Tests
Unit:

advancing step updates compiled execution artifact
short-circuit/status path reads execution artifact correctly
execution can proceed from artifact without recomputing planning state
Integration:

extend named/euclo/euclotest/runtime_test.go
extend named/euclo/euclotest/integration_test.go
confirm execution restart from persisted artifact preserves active step and completed steps
Acceptance:

execution progress can be reconstructed from the artifact without replaying the full planning pipeline
Phase 3: Add LLM Availability Awareness To Euclo Runtime
Why
You explicitly called out the resource constraint: a long-running run may not have fresh LLM access. Euclo must model that as a first-class runtime condition.

Implement
Add explicit LLM availability state to euclo runtime status.
Add runtime helpers to decide:
can execution continue without LLM?
must this transition wait for LLM?
should this issue be deferred instead?
Introduce a clear policy split:
tool/capability execution allowed
relurpic specialist execution allowed only if it does not require unavailable LLM access
archaeology re-entry requiring model reasoning must block or defer
Persist current LLM availability state in execution-local state and artifact metadata.
Suggested package/files
named/euclo/runtime/types.go
new named/euclo/runtime/availability.go
named/euclo/agent.go
named/euclo/euclotypes/types.go
Tests
Unit:

transitions that do not require fresh LLM continue when availability is blocked
transitions that require fresh LLM mark required_before_continue
runtime status exposes availability state correctly
Integration:

new tests in named/euclo/euclotest/runtime_test.go
simulate blocked model availability and verify:
execution continues where allowed
deferred issue created where needed
explicit status is surfaced in runtime state
Acceptance:

euclo never silently assumes LLM availability during long-running execution
Phase 4: Introduce Problem-Scoped Deferred Execution Issues
Why
Deferred drafts already exist in archaeology, but euclo needs its own execution-facing continuity surface for issues discovered while executing. Those issues should be:

grouped by problem
not tied too narrowly to one step
explicitly visible
linkable back into archaeology follow-up
Implement
Add euclo deferred execution issue records.
Trigger them from:
mutation checkpoint ambiguity
verification contradiction
stale result detection surfaced during execution
issue requiring archaeology re-entry
LLM-required-but-unavailable transition
Link them to archaeology state:
deferred drafts
decision records
convergence records
request IDs
Update execution completion semantics to allow:
success with deferred follow-up
Suggested package/files
new named/euclo/runtime/deferred_issues.go
new named/euclo/runtime/deferred_issues_test.go
named/euclo/agent.go
archaeo/bindings/euclo if helper wiring is needed
Tests
Unit:

issues group by problem key, not step ID
duplicate issue for same problem updates existing record when appropriate
issue links to archaeology refs correctly
Integration:

extend named/euclo/euclotest/recovery_test.go
extend named/euclo/euclotest/persistence_test.go
verify successful run can end with deferred issue records and explicit metadata
Acceptance:

euclo can complete a run successfully while surfacing actionable deferred follow-up
Phase 5: Explicit Archaeology Re-Entry Hooks
Why
You defined explicit re-entry triggers:

user request
mode transition
not automatic stale/ambiguity reconsideration
Euclo should therefore expose an explicit runtime path for re-entering archaeology rather than smuggling that behavior through generic execution recovery.

Implement
Add explicit re-entry operations:
request archaeology refresh
request deferred issue review
request successor/deferred plan work
request convergence review
Ensure re-entry is explicit in runtime state and artifact/event trail.
Do not automatically revisit deferred issues when LLM access returns.
Re-entry should update execution-local state to indicate:
paused for archaeology
resumed from archaeology
pending user review
Suggested package/files
new named/euclo/runtime/reentry.go
named/euclo/agent.go
named/euclo/orchestrate/interactive.go
named/euclo/interaction/* if explicit transition kinds are needed
Tests
Unit:

explicit re-entry call sets runtime state correctly
deferred issue review only occurs when requested
archaeology re-entry creates the expected execution status change
Integration:

extend named/euclo/euclotest/phase9_integration_test.go
extend named/euclo/euclotest/runtime_test.go
Acceptance:

deferred issues are never silently reconsidered just because model access returns
Phase 6: Long-Running Context Compaction And Reload
Why
Long-running runs need context lifecycle management. Prompt-visible context must shrink while semantic continuity remains intact.

Implement
Introduce explicit compaction policy for euclo long-running runs.
Separate:
execution-local working context
durable semantic references
Persist enough execution-local summary state to continue after compaction:
compiled execution artifact
current step summary
recent verification summary
current deferred issue refs
current archaeology refs
Offload durable meaning into archaeology/provenance rather than keeping it in prompt history.
Add compaction triggers based on:
token pressure
duration
step count
artifact volume
Add reload logic so a compacted run can resume with:
compiled execution artifact
current step context
archaeology-backed refs
minimal recent execution context
Suggested package/files
new named/euclo/runtime/compaction.go
new named/euclo/runtime/compaction_test.go
named/euclo/euclotypes/types.go
named/euclo/agent.go
potentially framework/contextmgr integration points where appropriate
Tests
Unit:

compaction preserves required execution continuity fields
compacted state can be rehydrated
prompt-visible state shrinks while artifact continuity remains intact
Integration:

extend named/euclo/euclotest/persistence_test.go
extend named/euclo/euclotest/runtime_test.go
add explicit long-run compaction scenario tests
Acceptance:

long-running euclo execution remains resumable and semantically consistent after compaction
Phase 7: Tighten Euclo-Archaeo Integration Surface
Why
Euclo should use archaeology as live planner more directly. Some current behavior still looks like archaeology-assisted execution rather than plan-centric runtime execution.

Implement
Review archaeo/bindings/euclo and ensure it cleanly exposes the operations euclo now needs:
compiled plan readiness
archaeology re-entry helpers
deferred issue linkage
current plan/deferred/convergence reads needed during execution
Reduce ad hoc direct service composition inside named/euclo/agent.go where a stable binding operation would make the execution path clearer.
Keep euclo using archaeo directly, not GraphQL.
Suggested package/files
archaeo/bindings/euclo/*
named/euclo/agent.go
Tests
Unit:

binding methods return stable execution-facing views
binding-layer integration with requests/deferred/convergence works as expected
Integration:

extend named/euclo/euclotest/integration_test.go
extend named/euclo/euclotest/phase9_integration_test.go
Acceptance:

euclo’s archaeology interactions are clearer and more execution-oriented, without moving planning semantics into euclo
Phase 8: Rework Relurpic Specialist Integration Around Euclo + Archaeo
Why
Relurpic capabilities should behave as specialist execution routines, not as an alternate architecture bypassing archaeology or euclo runtime continuity.

Implement
Distinguish relurpic use cases:
planning/exploration specialist routines
execution-time specialist routines
Ensure euclo can invoke relurpic specialist routines against:
compiled execution state
archaeology refs
current capability/security envelope
Where necessary, reduce direct raw-store/provider wiring in favor of:
archaeology-facing provider contracts
euclo runtime invocation helpers
Do not route core integration through GraphQL.
Suggested package/files
agents/relurpic/*
archaeo/providers/*
named/euclo/capabilities/*
Tests
Unit:

relurpic execution routines can run with compiled execution inputs
planning-time specialist routines consume archaeology-backed inputs correctly
Integration:

extend agents/relurpic/*_test.go
extend named/euclo/euclotest/capability_families_test.go
extend named/euclo/euclotest/coding_capability_test.go
Acceptance:

relurpic capabilities feel like euclo specialists, not a parallel orchestration system
Phase 9: Runtime Status, Operator Surface, And Teaching Hooks
Why
You called out out-of-the-box experience and teaching users how to use euclo. This is mainly a relurpish concern, but euclo must expose the state that makes teaching and operator control possible.

Implement
Add explicit runtime status surfaces for:
active compiled execution
LLM availability state
deferred issue summary
archaeology re-entry pending status
success-with-deferred-follow-up completion state
Add concise machine-readable summaries intended for relurpish:
what mode/profile is active
why execution is paused or continuing
what the user can do next
whether archaeology re-entry is recommended
Do not implement the TUI itself in this plan, but expose the runtime state it needs.
Suggested package/files
named/euclo/runtime/observability.go
new named/euclo/runtime/status.go
possibly named/euclo/interaction/types.go
Tests
Unit:

status summary reflects execution artifact, deferred issues, and availability
success-with-deferred-follow-up is distinct from blocked state
Integration:

extend named/euclo/euclotest/observability_test.go
extend named/euclo/euclotest/runtime_test.go
Acceptance:

relurpish can later teach and guide the user from euclo runtime state without inventing missing semantics
Test Plan
Run after each phase:

go test ./named/euclo/... ./archaeo/... ./agents/relurpic/... ./app/relurpish/...
Required package-level coverage additions:

named/euclo/runtime/*_test.go
named/euclo/euclotest/*
agents/relurpic/*_test.go
binding-level tests in archaeo/bindings/euclo
Keep end-to-end testsuite/agenttests additions out of this plan; they belong to a separate validation plan as requested.

Recommended Test Focus By Phase
Phase 1-2
compiled execution artifact creation
artifact-driven resume
step progression and persistence
Phase 3
LLM availability state transitions
execution continue/defer/block behavior under unavailable model access
Phase 4-5
deferred issue grouping and archaeology linkage
explicit archaeology re-entry only
Phase 6
context compaction + reload continuity
minimal execution-local rehydration
Phase 7-8
cleaner euclo-archaeo binding usage
relurpic specialist integration against compiled execution state
Phase 9
runtime status surfaces for relurpish/operator use
Acceptance Criteria
By the end of this plan:

euclo can persist and resume from a first-class compiled execution artifact
long-running plan execution is primarily artifact/plan-driven, not prompt-history-driven
euclo explicitly models LLM availability and can continue deterministically when allowed
deferred issues are problem-scoped, visible, and compatible with successful completion
archaeology re-entry occurs only on explicit user-driven triggers
context compaction preserves continuity through compiled execution state plus archaeology-backed semantic memory
relurpic capabilities operate as specialist routines within the euclo + archaeo model
euclo exposes enough runtime status for relurpish to teach and guide users effectively
Non-Goals
This plan does not include:

GraphQL server changes
relurpish UX implementation details
end-to-end testsuite additions
replacing framework policy/capability/security ownership
removing euclo mode/profile machinery entirely
Assumptions And Defaults
archaeo remains the live planner and semantic memory system
euclo remains the primary coding runtime
explicit user request or mode transition is required to revisit deferred issues
successful completion with deferred follow-up is a valid outcome
capability/tool permissions remain owned by framework and are treated as part of the compiled execution envelope, not recomputed ad hoc by euclo
context compaction should preserve semantics through archaeology/provenance and compiled execution state, not by retaining full prompt history


