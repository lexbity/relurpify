# Euclo Architecture

## Purpose

Euclo should be the canonical coding runtime built on the Relurpify framework.

That statement has a precise meaning:

- Euclo is not a replacement for framework primitives.
- Euclo is not a second policy or permission model.
- Euclo is not a monolithic loop that hides the rest of the system.
- Euclo is the coding-runtime layer that composes existing framework and agent
  surfaces into a coherent execution system for software engineering tasks.

The current codebase already contains strong primitives for capability
admission, authorization, retrieval, context management, graph execution,
subagent invocation, workflow persistence, typed pipelines, and multiple
reasoning paradigms. Euclo should leverage those features directly rather than
compressing them into a generic "coding assistant" abstraction inherited from
other systems.

This document is intentionally not framed as an MVP document. The goal is to
define a serious engineering runtime that uses the actual strengths of
Relurpify, including features that do not fit the default industry idea of a
coding agent.

## Design Fundamentals

The research note in `docs/research/euclo.txt` establishes the right baseline.
Those fundamentals should be carried into the implementation design directly,
not treated as product messaging.

### Environmental Grounding

Euclo must treat the environment as the source of truth.

That means the runtime should privilege:

- workspace files
- tool capability results
- build and test outputs
- diagnostics
- traces
- indexes
- symbol graphs
- admitted execution capabilities
- persisted workflow state

This is stronger than "use tools when possible." It means Euclo should not let
latent model recall substitute for repository facts, and it should not mark
tasks complete without environment-backed evidence.

### Narrow-to-Wide Reasoning

Euclo should support a directional reasoning model:

1. start from a local file, symbol, error, failing test, or user instruction
2. gather the smallest relevant evidence set
3. widen scope only when global constraints or neighboring systems require it
4. preserve explicit reasoning artifacts at each scope change

This should be reflected in execution-profile design, retrieval policy, and artifact
shape. It should not be left as a prompt preference.

### Constraint-Satisfaction Over Pure Generation

The runtime must assume that many coding tasks are not open-ended generation
tasks. They are constraint-satisfaction tasks involving:

- API compatibility
- repo conventions
- type system constraints
- build constraints
- test expectations
- policy restrictions
- file write boundaries
- verification requirements

Euclo should therefore carry explicit constraint state between phases rather
than allowing each subagent call to rediscover constraints from scratch.

### Evidence-First Execution

Euclo must prefer proof artifacts over unsupported confidence claims.

A coding runtime is stronger when it can produce:

- a failing reproduction
- a localized suspect area
- a patch artifact
- verification evidence
- a root-cause summary
- review findings
- benchmark deltas
- rollback or compatibility notes

This is not only a UX principle. It should govern execution-profile transitions.

### Observability Over Private Reasoning

Euclo should expose an action log, artifacts, explicit stage outputs, and proof
surfaces rather than relying on private chain-of-thought style output. The
system already has persistence, graph runtime state, and typed stage results.
Euclo should use those surfaces to make execution legible.

## Architectural Position

Euclo should sit above the generic framework and agent layers:

- `/framework` defines primitives and enforcement.
- `/agents` defines generic reasoning paradigms.
- `/named/euclo` defines canonical coding behavior.

This split is important because Relurpify already contains sophisticated
execution machinery. Euclo should not duplicate that machinery. It should
decide how coding tasks are interpreted and how those execution surfaces are
combined.

Euclo therefore owns:

- coding task classification
- mode resolution
- execution profile selection
- artifact expectations
- paradigm routing
- success gating
- coding-specific reporting

Euclo does not own:

- capability admission
- authorization
- runtime safety
- manifest parsing
- generic graph execution
- generic agent implementations

## Euclo Runtime Responsibilities

Euclo should be implemented as a named runtime at `/named/euclo`.

The runtime should expose a single stable coding-facing entrypoint and should
be responsible for the following phases.

### 1. Task Intake

Euclo accepts:

- direct UX task requests
- API requests
- delegated subagent invocations
- relurpic-orchestrated coding requests

The intake layer should normalize these into a common task envelope carrying:

- task identifier
- instruction
- task type
- mode hints
- user constraints
- explicit verification constraints
- workspace and branch metadata when available
- capability and policy snapshot references

### 2. Task Classification

Euclo classifies the request into one or more coding intents.

At minimum the classifier should determine:

- whether the task is implementation, debugging, review, planning, tracing, or mixed
- whether edits are permitted in the current environment
- whether the task requires an evidence-first execution profile before mutation
- whether the task is local, cross-cutting, branchy, or high-risk
- whether the task requires deterministic stage structure or open-ended search

Classification should not depend only on prompt text. It should also consume:

- admitted capabilities
- available verification families
- AST/LSP availability
- write authority
- known execution state and framework workflow state
- previous artifacts

### 3. Mode Resolution

Mode is a coding-runtime behavior profile. It is not an agent type.

Recommended modes:

- `code`
- `debug`
- `tdd`
- `review`
- `planning`

Mode resolution should determine:

- default execution profile family
- preferred paradigm ordering
- evidence threshold before edits
- context strategy
- verification strictness
- artifact expectations
- whether review or reflection is mandatory

### 4. Execution Profile Selection

An execution profile is the Euclo-selected bounded coding strategy for the
task. It is not a user-facing operating style, and it should not be confused
with the framework's existing use of `workflow` for persisted runtime
execution state.

Execution profile selection should be driven by:

- mode
- task shape
- capability availability
- current artifact state
- recoverability requirements
- whether edits are currently permitted

The execution-profile selector should produce:

- primary execution profile identifier
- fallback execution profile identifiers
- selected paradigm for each phase
- required artifact list
- completion contract
- failure escalation policy

### 5. Execution

Execution should be delegated to generic agent paradigms and framework-native
capability surfaces. Euclo should not implement its own hidden reasoning loop.

### 6. Artifact Consolidation

Every phase should emit durable artifacts into shared execution state. Final
output should be assembled from artifacts, not from a free-form transcript.

### 7. Success Gating

Euclo should determine completion by artifact contracts, capability-backed
evidence, and execution-profile rules, not by a model deciding it feels
finished.

## Terminology

The architecture should reserve `workflow` for the Relurpify framework meaning:

- persisted runtime execution state
- workflow identifiers
- workflow checkpoints
- workflow retrieval
- workflow state stores

Euclo should use `execution profile` for its own bounded coding strategy
selection.

The intended term split is:

- `mode`: user-intent and behavior posture selected or inferred by Euclo
- `execution profile`: Euclo-selected coding strategy for the task
- `relurpic capability`: specialist execution surface inside the execution profile
- `paradigm`: backing implementation style for a relurpic capability
- `workflow`: framework runtime execution record and persistence concept

## Modes

Modes should be implemented as structured runtime intent profiles, not as
prompt strings and not as direct aliases for paradigms.

A mode is the primary signal of user intent inside Euclo.

Mode answers:

- what kind of engineering help the user wants
- what behavioral posture the system should take
- what evidence threshold should apply before action
- what kinds of outputs are primary
- how aggressive or conservative mutation and verification should be

Mode does not answer:

- which generic agent should run
- which paradigm is mandatory
- the full concrete execution path

Those downstream decisions belong to execution-profile selection and relurpic
capability selection.

### Mode Semantics

Every mode should define:

- mode identifier
- user-intent meaning
- edit posture
- evidence threshold before mutation
- verification strictness
- review posture
- default execution profile family
- allowed fallback execution profile families
- preferred relurpic capability families
- context strategy bias
- recovery bias

### What Modes Constrain

Modes should constrain the runtime in the following ways:

- execution-profile eligibility
- relurpic capability preference ordering
- whether edits are permitted immediately, delayed, or disallowed
- whether verification is required, optional, or must be multi-phase
- whether review is primary, secondary, or mandatory
- whether planning breadth should be narrow or expansive
- whether evidence collection is primary or supporting

### What Modes Must Not Do

Modes should not:

- map one-to-one to paradigms
- hardcode a single relurpic capability
- bypass capability policy
- replace execution-profile selection
- collapse user intent and runtime implementation into the same term

### Suggested Mode Descriptor Shape

Each mode profile should contain:

- `mode_id`
- `intent_family`
- `edit_policy`
- `evidence_policy`
- `verification_policy`
- `review_policy`
- `default_execution_profiles`
- `fallback_execution_profiles`
- `preferred_relurpic_capability_families`
- `context_strategy`
- `recovery_policy`
- `reporting_policy`

### `code`

Intent:

- balanced implementation and repair

Meaning:

- the user wants the system to make progress on code changes without requiring
  heavy preamble, but still within evidence-backed engineering constraints

Runtime posture:

- edits are allowed once local evidence is sufficient
- mutation should trigger HITL before execution
- verification is required before completion
- review is secondary unless risk indicators rise
- planning is treated primarily as context collection and clarification when the
  task shape is underspecified or broader than the current local evidence
- speculative or branch execution should not occur in `code`; if branch or
  candidate comparison becomes necessary, the runtime should transition into
  `planning`

Default execution profile families:

- `edit_verify_repair`

Allowed fallbacks:

- `reproduce_localize_patch`
- `plan_stage_execute`
- `review_suggest_implement`

Preferred relurpic capability families:

- bounded implementation
- verification
- targeted planning
- optional review

Typical use:

- bug fixes
- bounded feature work
- local refactors
- test additions tied to an implementation change

### `debug`

Intent:

- diagnosis, reproduction, localization, and repair only after evidence exists

Meaning:

- the user wants the runtime to behave like an engineering debugger rather than
  a code generator

Runtime posture:

- edits are delayed until a reproduction artifact, trace artifact, diagnostics
  artifact, or strong localization artifact exists
- evidence collection is primary
- verification must include rerun of the relevant failing path when possible
- a `root_cause` artifact is required before completion
- what counts as sufficient evidence is context-dependent and should not be
  reduced to one hardcoded artifact threshold
- if a defensible root-cause artifact cannot be produced from the current
  evidence, the runtime should fall back to trace-oriented relurpic capabilities
  to gather the missing causal evidence

Default execution profile families:

- `reproduce_localize_patch`

Allowed fallbacks:

- `trace_execute_analyze`
- `edit_verify_repair`
- `plan_stage_execute`

Preferred relurpic capability families:

- debugging
- tracing
- diagnostics
- verification
- root-cause synthesis

Typical use:

- failing tests
- regressions
- runtime bugs
- flaky behavior
- unclear failure reports

### `tdd`

Intent:

- test-first engineering with explicit red-to-green behavior

Meaning:

- the user wants implementation to be organized around executable evidence of
  expected behavior

Runtime posture:

- failing test or equivalent failing verification artifact is required before
  implementation unless policy or environment explicitly waives it
- patch quality is judged primarily by passing verification after the failing
  state has been established
- review is secondary to the red-green contract
- if the failing-test-first path becomes awkward, the runtime should stop and
  prompt the user rather than silently weakening the mode contract
- if legacy behavior is unclear, the ambiguity should be recorded and the user
  should be prompted before encoding that behavior into tests

Default execution profile families:

- `test_driven_generation`

Allowed fallbacks:

- `edit_verify_repair`
- `plan_stage_execute`

Preferred relurpic capability families:

- test-first implementation
- verification
- constrained planning

Typical use:

- feature development with strong expected behavior
- bug fixes where regression locking is valuable
- repositories with strong test discipline

### `review`

Intent:

- inspection, critique, risk discovery, and evidence-oriented assessment

Meaning:

- the user primarily wants findings, not mutation
- the user may also want runtime, trace, or benchmark evidence when that
  evidence materially improves the quality of the findings

Runtime posture:

- implementation is disabled by default unless explicitly enabled
- findings artifact is the primary output
- critique quality and evidence quality matter more than raw speed
- verification is only required when implementation follow-on occurs
- trace, diagnostics, benchmark, or runtime-observation artifacts may be gathered
  when they materially improve the review result
- runtime evidence gathered in this mode remains subordinate to the review
  objective rather than becoming a separate mode
- if the amount or type of evidence required starts turning the task into a
  root-cause investigation, the runtime should prompt before transitioning into
  `debug`

Default execution profile families:

- `review_suggest_implement`
- `trace_execute_analyze` when review quality depends on runtime evidence

Allowed fallbacks:

- `plan_stage_execute` when the task turns into broad structural review
- `edit_verify_repair` only if implementation follow-on is explicitly requested

Preferred relurpic capability families:

- review
- compatibility analysis
- trace-backed analysis
- diagnostics
- reporting
- optional bounded implementation

Typical use:

- PR review
- patch review
- architecture review
- risk assessment
- missing-test identification

### `planning`

Intent:

- preproduction planning, decomposition, and approach design before committing
  to execution

Meaning:

- the user wants the runtime to reason about shape, approach, sequencing,
  constraints, and verification before or instead of mutating code
- the user may still want the runtime to gather evidence through non-mutating
  execution surfaces during planning

Runtime posture:

- implementation is disabled by default
- mutation is disallowed by default
- the mode should operate as read-oriented and non-mutating under the current
  admitted capability and permission envelope
- non-mutating relurpic capabilities are allowed
- non-mutating tool capabilities are allowed
- bounded observation, diagnostics, search, dry-run analysis, trace inspection,
  test discovery, static analysis, and similar evidence-gathering execution are
  permitted when they do not mutate workspace or runtime state
- decomposition breadth is high
- candidate comparison is encouraged
- plan quality, constraints, and verification strategy are primary outputs
- this mode may feed later execution in `code`, `debug`, or `tdd`
- if the requested planning output is underspecified, the runtime should prompt
  the user for the plan type rather than guessing

Plan-type clarification should include cases such as:

- implementation plan
- engineering spec
- verification strategy
- execution risk analysis
- compatibility notes

Permission and transition semantics:

- planning mode does not imply write authority
- a plan artifact does not itself grant mutating execution authority
- planning mode may use non-mutating execution authority when that authority is
  admitted by policy and capability selection
- any transition from planning into mutating code execution must be an explicit
  mode transition
- any such transition must also respect the newly admitted permission and
  capability envelope for the target mode
- if the environment does not admit the required write or execution
  capabilities for the target mode, the transition must fail even if planning
  artifacts exist

Default execution profile families:

- `plan_stage_execute`

Allowed fallbacks:

- `review_suggest_implement`
- `edit_verify_repair` only when explicit execution follow-on is authorized

Preferred relurpic capability families:

- planning
- design alternatives
- staged execution design
- compatibility analysis
- review
- non-mutating diagnostics
- non-mutating verification discovery

Typical use:

- preproduction planning
- architectural decomposition
- migration design
- large refactor planning
- evaluating multiple implementation approaches

## Mode Resolution

Mode resolution should combine:

- explicit user choice
- task language
- current artifact state
- admitted capability envelope
- repository context

Recommended precedence:

1. explicit user-selected mode
2. explicit task constraints that imply a stronger posture
3. prior execution state if resuming
4. Euclo classifier inference
5. default fallback to `code`

Mode resolution should be stable but revisable. If the task begins in one mode
and evidence shows that another posture is required, Euclo may recommend a mode
transition, but should not silently reinterpret user intent without recording
that change.

Mode-specific prompting should be treated as part of correct execution rather
than as a fallback failure path.

Euclo should distinguish between prompt types and should use the framework's
HITL facilities rather than inventing a separate prompt mechanism.

Prompt types should include:

- clarification prompt
- permission prompt
- mode-transition prompt

Prompting is specifically expected when:

- `code` is about to mutate and permission approval is required
- `tdd` becomes awkward or legacy behavior is ambiguous
- `review` needs to transition into `debug`
- `planning` request type is underspecified
- branch or speculative execution would require escalation out of `code`

The intended mapping is:

- clarification prompt: underspecified planning request, awkward TDD condition,
  ambiguous legacy behavior
- permission prompt: mutating step, edit admission, execution step requiring
  policy approval
- mode-transition prompt: `review -> debug`, `planning -> code`, or other
  posture-changing transitions

## Mode Transitions

Modes should be able to transition under controlled conditions.

Examples:

- `code -> debug` when verification repeatedly fails and no localized fix exists
- `code -> planning` when the task expands into broad staged work or requires
  explicit decomposition before safe continuation
- `debug -> review` when runtime evidence is needed mainly to support findings
  or diagnosis rather than immediate repair
- `review -> code` only when explicit implementation follow-on is authorized
- `planning -> code` only when a chosen execution path is explicitly approved
  and the target mode's capability and permission envelope permits mutation

Planning-to-execution should not be treated as implicit continuation. It is a
mode transition across a permission boundary.

Mode transition should produce an explicit artifact or state record so that the
runtime's behavior shift is inspectable.

## Execution Profiles

The term `workflow` should be avoided on the Euclo side because it collides
with the framework runtime meaning. Euclo should use `execution profile`
instead.

Execution profiles should be concrete runtime objects defined under Euclo, each
with an explicit contract.

Each execution profile definition should include:

- execution profile identifier
- supported modes
- entry predicate
- required capability families
- selected paradigms per phase
- artifact schema
- success conditions
- failure transitions
- resumability rules

### `edit_verify_repair`

Purpose:

- default implementation loop for bounded code changes

Required phases:

- exploration
- issue analysis
- patch generation or execution
- verification
- optional repair loop

Required artifacts:

- `file_selection`
- `issue_analysis`
- `patch`
- `verification`

Mutation and approval semantics:

- mutation authority is determined by the framework through scope, policy,
  session, and admitted capabilities
- Euclo does not grant mutation authority on its own
- any mutating step inside this execution profile should trigger framework HITL
  before execution when required by policy
- broadening from a bounded local patch into branch or speculative execution is
  not allowed inside this profile; that requires transition into `planning`

Failure and escalation semantics:

- verification failure should first attempt bounded repair inside the profile
- if bounded repair stops being local or becomes branchy, Euclo should
  transition into `planning`
- if the problem becomes primarily diagnostic rather than implementation-led,
  Euclo should transition into `debug`

Success condition:

- patch artifact exists
- verification artifact has passing or acceptable constrained status

### `reproduce_localize_patch`

Purpose:

- debugging loop for bugs that should be reproduced before mutation

Required phases:

- reproduction
- localization
- patch
- rerun verification
- root-cause summary

Required artifacts:

- `reproduction` with failing status
- `issue_analysis` or `diagnostics`
- `patch`
- `verification` showing rerun outcome
- `root_cause`

Mutation and approval semantics:

- if strict reproduction is impossible or weak, the user should be prompted
  through framework HITL before mutation proceeds
- a `root_cause` artifact is required before mutation, not only before
  completion
- if the root cause cannot be defended from current evidence, the profile
  should fall back to trace-oriented relurpic capabilities before any mutating
  step is allowed

### `trace_execute_analyze`

Purpose:

- trace-centric or benchmark-centric diagnosis

Required phases:

- instrumentation or trace acquisition
- result analysis
- optional patch
- verification or performance comparison

Required artifacts:

- `trace` or `benchmark`
- `analysis`
- optional `patch`
- `verification` or `benchmark_comparison`

### `test_driven_generation`

Purpose:

- test-first feature or bug-fix execution profile

Required phases:

- failing test creation or identification
- implementation
- passing test verification

Required artifacts:

- `reproduction` or failing `verification`
- `patch`
- passing `verification`

Awkwardness and legacy semantics:

- legacy code and unclear existing behavior are first-class special cases
- if behavior is unclear, the ambiguity should be recorded and the user should
  be prompted before encoding that behavior into tests
- if TDD conditions become awkward, Euclo should first attempt to resolve the
  awkwardness within the profile rather than silently weakening the contract
- if the awkwardness cannot be resolved cleanly, Euclo should fall back to
  `review` and prompt the user before continuing

### `review_suggest_implement`

Purpose:

- review-first execution profile where findings are primary

Required phases:

- evidence gathering
- review pass
- optional implementation pass
- optional verification

Required artifacts:

- `review_findings`
- optional `patch`
- optional `verification`

Mutation and transition semantics:

- this profile is non-mutating by default
- it may branch into a mutating subphase only through explicit user approval
  using framework HITL
- the mutating subphase must still respect the target permission and capability
  envelope
- this profile may be used as a fallback from `code` or `planning`
- if the evidence required begins turning the task into debug-grade root-cause
  work, Euclo should prompt before transitioning into `debug`

### `plan_decompose`

Purpose:

- non-mutating staged planning and decomposition for broad tasks

Required phases:

- planning
- alternatives or clarification when needed
- summary and execution guidance

Required artifacts:

- `plan`
- optional `candidate_plan`
- optional planning-oriented review or compatibility artifacts

Permission semantics:

- this profile is non-mutating by default
- it may use non-mutating relurpic capabilities and non-mutating tool
  capabilities
- it does not grant authority to execute the resulting plan

### `stage_execute`

Purpose:

- staged multi-step execution of an already selected plan under an execution-capable mode

Required phases:

- execution admission
- step execution
- recovery handling
- summary and verification

Required artifacts:

- `plan`
- step artifacts
- verification artifacts where applicable

Mutation and approval semantics:

- this profile is separate from `plan_decompose` because planning and execution
  cross different authority boundaries
- entering this profile from `planning` requires an explicit mode transition
- mutating steps remain subject to framework HITL and policy

## Artifact System

Artifacts should be first-class runtime objects persisted in workflow state.

Euclo should define a typed artifact model instead of relying on ad hoc context
keys.

### Artifact Descriptor

Each artifact should include:

- `artifact_id`
- `workflow_id`
- `run_id`
- `type`
- `producer_kind`
- `producer_id`
- `mode`
- `workflow`
- `execution_profile`
- `status`
- `summary`
- `created_at`
- `updated_at`
- `payload`
- `evidence_refs`
- `parent_artifact_ids`
- `selection_score` when applicable

### Core Artifact Types

The initial artifact set should include:

- `task_classification`
- `mode_resolution`
- `execution_profile_selection`
- `file_selection`
- `issue_analysis`
- `diagnostics`
- `plan`
- `candidate_plan`
- `patch`
- `diff_summary`
- `reproduction`
- `trace`
- `benchmark`
- `verification`
- `review_findings`
- `root_cause`
- `final_report`

### Evidence References

`evidence_refs` should point to concrete evidence-bearing records such as:

- tool execution outputs
- command outputs
- diagnostics bundles
- trace files
- benchmark measurements
- diff identifiers
- test result records
- stage result identifiers

This matters because Euclo should be able to explain why an execution profile
advanced or completed without reconstructing history from unstructured text.

## Execution Selection Hierarchy

Euclo should not route directly from task to generic agent in the common case.
That model is too shallow for the architecture described in this document.

The execution-selection hierarchy should be:

1. Euclo resolves user intent into mode and task shape.
2. Euclo selects an execution profile.
3. The execution profile selects one or more relurpic capabilities.
4. Each relurpic capability selects its backing paradigm or paradigm mix.
5. Recovery and fallback may redirect to another capability or another paradigm.

This is the correct place to integrate the full set of execution paradigms,
including `blackboard`, `HTN`, and `chainer`.

### Level 1: Euclo Mode and Task Routing

At the top level, Euclo should decide:

- what kind of engineering intent the user expressed
- whether the task is implementation, debugging, review, planning, tracing, or mixed
- whether edits are permitted
- what evidence threshold applies before mutation
- whether the task is local, broad, branchy, deterministic, or exploratory

This level does not choose a generic agent directly. It chooses a coding
behavior profile and an execution-profile family.

### Level 2: Execution Profile Selection

Execution profiles define the bounded coding loop for the task.

Examples:

- `edit_verify_repair`
- `reproduce_localize_patch`
- `trace_execute_analyze`
- `test_driven_generation`
- `review_suggest_implement`
- `plan_stage_execute`

Execution profile selection should determine:

- required artifact contract
- entry and completion conditions
- required capability families
- allowed fallback execution profiles
- whether the execution profile is deterministic, recipe-driven, evidence-driven, or open-ended

### Level 3: Relurpic Capability Selection

Relurpic capabilities are the specialist execution surfaces inside an execution
profile.

Examples:

- `relurpic:planner.plan`
- `relurpic:execution_profile.select`
- `relurpic:debug.reproduce_localize_patch`
- `relurpic:trace.execute_analyze`
- `relurpic:code.edit_verify_repair`
- `relurpic:verify.change`
- `relurpic:review.findings`
- `relurpic:report.final_coding`

This is the main execution-selection layer below Euclo itself.

At this level the runtime decides:

- which specialist execution unit should run next
- what artifact it must produce
- what capability bundle and permission envelope it needs
- whether it should run in shared, cloned, forked, or branch-local isolation

### Level 4: Paradigm Selection

Each relurpic capability should declare its primary paradigm and any supporting
or fallback paradigms.

This is where generic agents enter the design:

- `ChainerAgent`
- `PipelineAgent`
- `HTNAgent`
- `BlackboardAgent`
- `ReActAgent`
- `ArchitectAgent`
- `PlannerAgent`
- `ReflectionAgent`

The paradigm layer is therefore an implementation detail of relurpic
capabilities, not the first routing decision Euclo makes.

### Level 5: Recovery and Fallback

Recovery should be able to redirect at multiple levels:

- paradigm fallback inside the same relurpic capability
- relurpic capability swap inside the same execution profile
- execution-profile escalation or downgrade under the same mode
- explicit mode reconsideration when user intent or evidence state changes

This matters because a failed deterministic phase should not automatically
force a totally different user-intent interpretation.

### Paradigm Selection Rules

The baseline paradigm-selection rules should be:

- use `ChainerAgent` for linear artifact transforms, normalization, report
  compilation, and strict input/output synthesis
- use `PipelineAgent` for deterministic typed stages with stable contracts and
  explicit stage boundaries
- use `HTNAgent` for structured engineering recipes where Euclo should own
  decomposition explicitly through declared methods
- use `BlackboardAgent` for evidence-driven specialist coordination where the
  next step depends on evolving artifacts
- use `ReActAgent` for open-ended local search, local editing, diagnosis, and
  recovery when structure is uncertain
- use `ArchitectAgent` for broader multi-step execution when dependency-aware
  staged execution and resumability matter
- use `PlannerAgent` for plan-only decomposition, alternatives, and candidate
  generation
- use `ReflectionAgent` for critique, review quality, and confidence-increasing
  second passes

These are not top-level Euclo routing rules. They are paradigm-selection rules
used by relurpic capabilities.

### Important Constraint

Euclo should not embed all paradigms inside a new hidden monolithic loop. It
should invoke bounded relurpic capabilities, and those capabilities should
invoke explicit paradigms and publish explicit artifacts.

## Relurpic Capability Taxonomy

Under the Euclo architecture, relurpic capabilities should be treated as
specialist execution surfaces.

They sit above raw framework primitives and below the top-level Euclo runtime.
They are not just "tools" and they are not identical to raw generic agent
types.

A relurpic capability may compose:

- a subagent
- a skill bundle
- tool capabilities
- admitted permissions under the Relurpify security model
- an artifact contract
- a bounded execution policy

This means a relurpic capability is the callable specialist layer where
workflow behavior, specialist orchestration, and bounded execution semantics
live.

The distinction should be:

- framework primitives: generic capabilities, authorization, policy, graph,
  memory, retrieval, checkpointing
- tool capabilities: concrete local execution or read/write capabilities
- subagent capabilities: callable generic reasoning runtimes such as
  `agent:react`, `agent:architect`, `agent:pipeline`, `agent:planner`,
  `agent:blackboard`, `agent:htn`, `agent:chainer`
- relurpic capabilities: specialist coding execution surfaces built from
  subagents, skills, and tools under policy
- Euclo execution profiles: top-level coding-runtime compositions selected by Euclo

### Relurpic Capability Families

The following families should be treated as relurpic capabilities rather than
as bare tools or raw generic agent invocations.

#### Planning and Decomposition

Examples:

- `planner.plan`
- `design_alternatives`
- `execution_candidate_selection`
- `plan_stage_execute`

Primary paradigm:

- `PlannerAgent`

Common mixes:

- `PlannerAgent + ReflectionAgent`
- `PlannerAgent + HTN`

Use when:

- the capability's main value is decomposition, alternatives, or candidate
  selection before execution

#### Deterministic Verification and Reporting

Examples:

- `verify_change`
- `compile_verification_report`
- `final_coding_report`
- `artifact_report_compiler`

Primary paradigm:

- `PipelineAgent`

Common mixes:

- `PipelineAgent + Chainer`
- `PipelineAgent + ReflectionAgent`

Use when:

- the capability has a fixed contract and should transform evidence into a
  typed result with minimal open-ended search

#### Debugging and Investigation

Examples:

- `trace_execute_analyze`
- `reproduce_localize_patch`
- `investigate_regression`
- `root_cause_and_repair`

Primary paradigm:

- `BlackboardAgent`

Common mixes:

- `BlackboardAgent + ReActAgent`
- `BlackboardAgent + PipelineAgent`

Use when:

- the next specialist should depend on evolving evidence
- execution order is data-driven rather than fixed up front
- multiple partial signals must be integrated before action

#### Structured Implementation Recipes

Examples:

- `test_driven_generation`
- `api_compatible_refactor`
- `cross_file_rename_with_verification`
- `migration_execute`
- `feature_slice_delivery`

Primary paradigm:

- `HTNAgent`

Common mixes:

- `HTNAgent + ArchitectAgent`
- `HTNAgent + PipelineAgent`

Use when:

- Euclo should own the decomposition recipe explicitly
- the task benefits from declared engineering methods rather than open-ended
  structure discovery

#### Review and Critique

Examples:

- `review_findings`
- `review_then_implement_if_safe`
- `risk_surface_review`
- `compatibility_review`

Primary paradigm:

- `ReflectionAgent`

Common mixes:

- `ReActAgent + ReflectionAgent`
- `BlackboardAgent + ReflectionAgent`

Use when:

- the capability's main value is critique quality, risk identification, or
  confidence increase

#### Linear Artifact Transforms

Examples:

- `summarize_diff_for_review`
- `normalize_build_failure`
- `convert_trace_to_root_cause_candidates`
- `review_findings_to_patch_plan`

Primary paradigm:

- `ChainerAgent`

Common mixes:

- `ChainerAgent + PipelineAgent`

Use when:

- the capability is a narrow linear transform over existing artifacts
- strict input/output discipline is more important than exploratory reasoning

## Blackboard, HTN, and Chainer Usage

The `blackboard`, `HTN`, and `chainer` agents should not be treated as
top-level coding personas. In Euclo they are implementation paradigms for
relurpic specialist capabilities.

### Blackboard-Backed Relurpic Capabilities

`BlackboardAgent` is appropriate when capability execution should be
artifact-driven and specialist scheduling should emerge from current evidence.

Relurpic capabilities that should likely use blackboard:

- `trace_execute_analyze`
- `reproduce_localize_patch`
- `investigate_regression`
- `root_cause_and_repair`
- `multi_signal_review`
- `candidate_patch_competition`
- `benchmark_diagnose_optimize`

Implementation characteristics:

- the capability owns a shared typed artifact board
- specialists publish partial outputs such as traces, diagnostics, localizations,
  patch candidates, verification results, and review findings
- the controller selects the next specialist based on missing or weak artifacts
- execution order is not fully predetermined

Blackboard is strongest when:

- there are multiple plausible next steps
- the task mixes observation, diagnosis, patching, and verification
- Euclo wants evidence to drive specialist choice

### HTN-Backed Relurpic Capabilities

`HTNAgent` is appropriate when the capability should follow a declared
engineering method library.

Relurpic capabilities that should likely use HTN:

- `test_driven_generation`
- `api_compatible_refactor`
- `migration_execute`
- `deprecation_rollout`
- `plan_stage_execute`
- `feature_slice_delivery`
- `cross_file_rename_with_verification`
- `review_then_implement_if_safe`

Implementation characteristics:

- the capability binds to an explicit Euclo method library
- each method decomposes a specialist task into ordered subtasks
- leaf subtasks are executed through bounded primitive executors
- the runtime controls structure rather than leaving decomposition to the model

HTN is strongest when:

- engineering doctrine should be encoded explicitly
- the task is broad but structurally predictable
- decomposition should be inspectable and stable across runs

### Chainer-Backed Relurpic Capabilities

`ChainerAgent` is appropriate when a specialist capability is a linear artifact
transform rather than an exploratory loop.

Relurpic capabilities that should likely use chainer:

- `summarize_diff_for_review`
- `normalize_build_failure`
- `extract_verification_report`
- `convert_trace_to_root_cause_candidates`
- `review_findings_to_patch_plan`
- `artifact_report_compiler`
- `final_coding_report`

Implementation characteristics:

- each chain link declares exact input keys and one output key
- the capability performs deterministic or near-deterministic synthesis
- outputs are generally used by other relurpic capabilities or by Euclo final
  reporting

Chainer is strongest when:

- the task is structurally linear
- open-ended search would add noise rather than value
- the main job is normalization, reduction, or report compilation

## Paradigm Mapping Rules

The following rules should guide relurpic capability implementation.

- Use `ReActAgent` when a relurpic capability needs open-ended search, local
  editing, diagnosis, or recovery.
- Use `PipelineAgent` when a relurpic capability has fixed typed stages and a
  stable contract.
- Use `ArchitectAgent` when a relurpic capability spans multiple dependent
  steps and resumable execution matters.
- Use `PlannerAgent` when the capability's output is a plan, alternatives, or
  decomposition rather than direct execution.
- Use `ReflectionAgent` when the capability's value is critique, review, or a
  confidence-increasing second pass.
- Use `BlackboardAgent` when specialist scheduling should be driven by
  accumulated artifacts and missing evidence.
- Use `HTNAgent` when Euclo should encode engineering recipes explicitly as
  methods.
- Use `ChainerAgent` when the capability is a strict linear transform over
  existing artifacts.

## Mixed-Paradigm Relurpic Capabilities

Many relurpic capabilities should not be implemented by a single paradigm.

The preferred approach is to define:

- an outer execution contract
- the primary paradigm
- fallback or supporting paradigms
- the artifact interface between them

Important mixed cases:

- `trace_execute_analyze`
  - outer structure: `BlackboardAgent`
  - supporting paradigms: `ReActAgent`, `PipelineAgent`
  - reason: evidence collection is data-driven, but some analysis or verification
    steps may be bounded or exploratory

- `reproduce_localize_patch`
  - outer structure: `BlackboardAgent`
  - supporting paradigms: `ReActAgent`, `ReflectionAgent`
  - reason: reproduction and localization are evidence-driven, patching may be
    exploratory, final review may need critique

- `test_driven_generation`
  - outer structure: `HTNAgent`
  - supporting paradigms: `PipelineAgent`, `ReActAgent`
  - reason: the engineering recipe is stable, but implementation or repair may
    need local exploratory work

- `plan_stage_execute`
  - outer structure: `ArchitectAgent` or `HTNAgent`
  - supporting paradigms: `ReActAgent`
  - reason: staged execution is explicit, but failed steps may require open-ended
    recovery

- `final_coding_report`
  - outer structure: `ChainerAgent`
  - supporting paradigms: `ReflectionAgent`
  - reason: report assembly is linear, but final critique can improve quality

## Canonical Relurpic Capability Registry

Euclo should define a canonical registry of relurpic capabilities for coding
work. These are not all required on day one, but the architecture should be
designed around an explicit registry shape rather than around ad hoc specialist
names.

Each relurpic capability should define:

- capability identifier
- purpose
- accepted inputs
- produced artifacts
- required predecessor artifacts
- required capability families
- primary paradigm
- supporting paradigms
- execution isolation mode
- success condition

The registry below is the recommended starting taxonomy.

### Planning and Selection Capabilities

#### `relurpic:planner.plan`

Purpose:

- produce a structured execution plan for a coding task

Inputs:

- normalized task envelope
- mode
- workspace context summary
- prior artifacts

Produces:

- `plan`
- optional `candidate_plan`

Requires:

- read-oriented exploration capabilities

Primary paradigm:

- `PlannerAgent`

Supporting paradigms:

- `ReflectionAgent`

Success condition:

- at least one valid plan artifact with ordered steps and declared verification

#### `relurpic:design.alternatives`

Purpose:

- generate multiple candidate execution strategies for broad or ambiguous tasks

Inputs:

- normalized task envelope
- prior artifacts
- architecture context

Produces:

- one or more `candidate_plan` artifacts

Requires:

- read capabilities
- planning capability family

Primary paradigm:

- `PlannerAgent`

Supporting paradigms:

- `ReflectionAgent`
- `HTNAgent`

Success condition:

- at least two materially distinct candidate plans with comparison notes

#### `relurpic:execution_profile.select`

Purpose:

- choose the best execution profile and paradigm route for the current task state

Inputs:

- task classification
- mode resolution
- capability and policy snapshot
- prior artifacts

Produces:

- `execution_profile_selection`

Requires:

- no special tool family beyond current context

Primary paradigm:

- `ChainerAgent`

Supporting paradigms:

- `PlannerAgent`

Success condition:

- one execution-profile selection artifact with primary path, fallbacks, and rationale

### Investigation and Debugging Capabilities

#### `relurpic:debug.reproduce_localize_patch`

Purpose:

- perform bug reproduction, fault localization, repair, and re-verification

Inputs:

- normalized task envelope
- prior artifacts

Produces:

- `reproduction`
- `issue_analysis`
- `patch`
- `verification`
- `root_cause`

Requires:

- read capabilities
- verification capabilities
- write capabilities if repair is enabled

Primary paradigm:

- `BlackboardAgent`

Supporting paradigms:

- `ReActAgent`
- `ReflectionAgent`

Isolation mode:

- shared artifact board, isolated specialist executions

Success condition:

- failing reproduction or equivalent evidence, patch artifact, passing or improved
  verification, and root-cause artifact

#### `relurpic:trace.execute_analyze`

Purpose:

- collect runtime evidence and convert it into an actionable engineering result

Inputs:

- normalized task envelope
- prior artifacts

Produces:

- `trace`
- `diagnostics`
- `analysis`
- optional `patch`
- `verification`

Requires:

- tracing, execution, or diagnostics capability families

Primary paradigm:

- `BlackboardAgent`

Supporting paradigms:

- `ReActAgent`
- `PipelineAgent`

Isolation mode:

- shared artifact board

Success condition:

- trace or diagnostics artifact exists and analysis artifact supports the resulting
  action or recommendation

#### `relurpic:debug.investigate_regression`

Purpose:

- investigate a regression across code, tests, and recent change surfaces

Inputs:

- task envelope
- regression symptom description
- optional prior reproduction artifact

Produces:

- `reproduction`
- `diagnostics`
- `issue_analysis`
- optional `patch`
- `verification`

Requires:

- read/search capabilities
- verification capabilities

Primary paradigm:

- `BlackboardAgent`

Supporting paradigms:

- `ReActAgent`
- `ArchitectAgent`

Success condition:

- localized regression path and either a patch or a ranked next-step diagnosis

### Structured Implementation Capabilities

#### `relurpic:code.edit_verify_repair`

Purpose:

- perform bounded implementation and verification with repair if checks fail

Inputs:

- task envelope
- prior artifacts

Produces:

- `file_selection`
- `issue_analysis`
- `patch`
- `verification`

Requires:

- write capabilities
- verification capabilities

Primary paradigm:

- `ReActAgent`

Supporting paradigms:

- `PipelineAgent`
- `ReflectionAgent`

Isolation mode:

- single execution path unless explicit branch mode is requested

Success condition:

- patch artifact and verification artifact satisfying execution-profile policy

#### `relurpic:tdd.generate`

Purpose:

- implement a change through an explicit test-first recipe

Inputs:

- task envelope
- prior artifacts

Produces:

- failing `verification` or `reproduction`
- `patch`
- passing `verification`

Requires:

- write capabilities
- test execution capabilities

Primary paradigm:

- `HTNAgent`

Supporting paradigms:

- `PipelineAgent`
- `ReActAgent`

Success condition:

- failing test evidence exists before implementation and passing verification
  exists after implementation, unless explicitly waived by policy

#### `relurpic:refactor.api_compatible`

Purpose:

- perform a constrained refactor while preserving external behavior

Inputs:

- task envelope
- compatibility constraints
- prior artifacts

Produces:

- `plan`
- `patch`
- `verification`
- optional `review_findings`

Requires:

- write capabilities
- verification capabilities
- symbol-aware capabilities when available

Primary paradigm:

- `HTNAgent`

Supporting paradigms:

- `ArchitectAgent`
- `ReflectionAgent`

Success condition:

- compatibility-preserving patch plus verification evidence

#### `relurpic:migration.execute`

Purpose:

- execute a broader structured migration with explicit decomposition

Inputs:

- task envelope
- migration constraints
- prior artifacts

Produces:

- `plan`
- step artifacts
- `patch`
- `verification`
- optional `review_findings`

Requires:

- write capabilities
- verification capabilities

Primary paradigm:

- `HTNAgent`

Supporting paradigms:

- `ArchitectAgent`

Success condition:

- method-complete execution with verification at required checkpoints

### Review and Critique Capabilities

#### `relurpic:review.findings`

Purpose:

- produce code review findings, risks, and missing-test flags

Inputs:

- task envelope
- target files or diff context
- prior artifacts

Produces:

- `review_findings`

Requires:

- read/search capabilities

Primary paradigm:

- `ReflectionAgent`

Supporting paradigms:

- `ReActAgent`

Success condition:

- findings artifact with concrete risks or an explicit no-findings result

#### `relurpic:review.implement_if_safe`

Purpose:

- perform a review-first pass and optionally implement constrained follow-up work

Inputs:

- task envelope
- prior artifacts

Produces:

- `review_findings`
- optional `patch`
- optional `verification`

Requires:

- read capabilities
- write and verification capabilities only if implementation follow-on is enabled

Primary paradigm:

- `HTNAgent`

Supporting paradigms:

- `ReflectionAgent`
- `ReActAgent`

Success condition:

- review findings always exist; implementation and verification artifacts exist
  only when follow-on execution is allowed and selected

#### `relurpic:review.compatibility`

Purpose:

- assess compatibility risk for a proposed or completed change

Inputs:

- patch artifact or target symbols/files
- prior artifacts

Produces:

- `review_findings`
- optional `compatibility_notes` embedded in payload

Requires:

- read/search capabilities
- symbol-aware capabilities when available

Primary paradigm:

- `ReflectionAgent`

Supporting paradigms:

- `ChainerAgent`

Success condition:

- findings artifact identifying compatibility risks or an explicit compatibility-clear result

### Verification and Synthesis Capabilities

#### `relurpic:verify.change`

Purpose:

- run execution-profile-defined verification and produce a typed verification artifact

Inputs:

- task envelope
- patch artifact
- verification policy

Produces:

- `verification`

Requires:

- verification capability family selected by policy and environment

Primary paradigm:

- `PipelineAgent`

Supporting paradigms:

- `ReActAgent`

Success condition:

- verification artifact with explicit evidence references

#### `relurpic:report.final_coding`

Purpose:

- compile the final coding report from artifacts

Inputs:

- artifact bundle
- execution-profile result state

Produces:

- `final_report`

Requires:

- no additional execution tools unless evidence hydration is needed

Primary paradigm:

- `ChainerAgent`

Supporting paradigms:

- `ReflectionAgent`

Success condition:

- final report artifact with summary, patch outcome, verification status, and
  unresolved issues

#### `relurpic:report.verification_summary`

Purpose:

- normalize verification evidence into a concise summary for users or downstream
  capabilities

Inputs:

- verification artifact
- supporting evidence refs

Produces:

- summary payload attached to a reporting artifact or downstream context item

Requires:

- no extra tools in the common case

Primary paradigm:

- `ChainerAgent`

Supporting paradigms:

- none by default

Success condition:

- one normalized summary derived from explicit evidence refs

### Artifact and Transform Capabilities

#### `relurpic:artifact.diff_summary`

Purpose:

- transform a patch or diff into a structured summary artifact

Inputs:

- patch artifact
- optional file metadata

Produces:

- `diff_summary`

Requires:

- diff visibility

Primary paradigm:

- `ChainerAgent`

Supporting paradigms:

- `ReflectionAgent`

Success condition:

- diff summary artifact with changed files, intent, and risk notes

#### `relurpic:artifact.trace_to_root_cause_candidates`

Purpose:

- convert raw trace evidence into ranked root-cause candidates

Inputs:

- trace artifact
- diagnostics artifact when available

Produces:

- `issue_analysis`

Requires:

- trace or diagnostics artifacts

Primary paradigm:

- `ChainerAgent`

Supporting paradigms:

- `ReflectionAgent`

Success condition:

- ranked candidate analysis with trace-backed evidence refs

## Registry Design Notes

The registry above should not be interpreted as a flat tool menu. It is a
specialist execution inventory for Euclo execution profiles.

Important design constraints:

- relurpic capabilities may call tools internally but should expose a larger
  bounded contract than a single tool call
- relurpic capabilities may invoke subagents internally but should not collapse
  into unrestricted generic agent access
- each relurpic capability should publish artifacts that Euclo can route on
- capability contracts should be stable even if their internal paradigm mix evolves

## Implementation Scope

All relurpic capabilities documented in this spec should be treated as
mandatory implementation scope for Euclo planning. They are not aspirational
extras outside the intended design.

Implementation may still be phased, but the architecture should assume the full
documented relurpic capability set rather than a reduced "initial subset"
model.

## Advanced Execution Paradigms

Relurpify can support more than conventional single-loop coding-agent behavior.
Euclo should take advantage of that.

### Evidence-Gated Execution

This should be mandatory and foundational.

Execution-profile transitions should depend on artifacts rather than on model
claims.

Implementation detail:

- each execution-profile phase defines required predecessor artifact types
- each artifact validator checks capability-backed evidence presence
- the execution-profile controller refuses advancement if required artifacts are missing
- failure transitions can trigger targeted recovery or alternate paradigms

### Speculative Branch Execution

Euclo should support bounded branch execution for ambiguous or optimization-like
tasks.

Implementation detail:

- the execution-profile selector emits multiple candidate execution branches
- each branch gets a fresh or cloned state container and a fresh agent instance
- each branch produces a branch-local artifact bundle
- a branch selector compares results using explicit ranking logic
- the selected branch contributes mergeable artifacts to the parent run

Branch selection criteria may include:

- verification status
- diff size
- confidence score
- benchmark score
- policy compliance
- review findings count

### Blackboard-Orchestrated Execution

Euclo should support an artifact-board model where bounded specialists publish
outputs to a shared workspace-level execution state.

Implementation detail:

- the board is not free-form shared chat
- the board stores typed artifacts and claims
- specialists subscribe to missing-artifact predicates
- the Euclo controller schedules the next specialist based on missing or weak evidence

This is a better fit for engineering work than a pure single-session dialogue
model because the work naturally decomposes into plan, execute, verify, review,
and diagnose roles.

## Integration With Existing Framework Features

Euclo should deliberately use framework features that already exist.

### Capability Registry and Policy

Euclo should consult the admitted capability registry and capability metadata
before execution-profile selection. It should not construct a parallel allowlist or
permission language.

Euclo should not classify capabilities on its own. Capability classification
belongs to the framework layer through capability metadata, tags, selectors,
effect information, and policy-admitted descriptors.

Euclo should therefore:

- consume framework-exposed capability metadata
- consume framework capability tags and selectors
- consume policy-admitted capability descriptors
- route modes, execution profiles, and relurpic capabilities based on admitted
  framework metadata

Euclo should not:

- invent a Euclo-only capability taxonomy
- reclassify capabilities independently of framework metadata
- attach authority semantics that bypass framework policy and selector logic

### Relurpic Capabilities

relurpic capabilities are not just “orchestration helpers”; they are the callable specialist-execution layer built from:

subagents
skills
tool capabilities
policy-bounded permissions
runtime coordination behavior
So a relurpic capability is best understood as a security-scoped specialist runtime surface, not merely a tool.

What falls under relurpic capabilities:

Specialist execution profiles such as trace_execute_analyze, reproduce_localize_patch, edit_verify_repair, review_suggest_implement, test_driven_generation
Narrow specialist executors such as planner, reviewer, verifier, debugger, tracer, patcher, localizer
Delegated bounded subagent runs with a defined goal, artifact contract, and tool/permission envelope
Skill-shaped execution bundles where the skill selects capability families, verification rules, recovery probes, and planning hints
Tool-backed specialist surfaces where the exposed unit is bigger than a single tool call, even if it uses tools internally
What does not fall under relurpic capabilities:

Raw framework primitives
Raw generic agent types like ReActAgent or PipelineAgent by themselves
Bare tool capabilities like file_read or go_test considered in isolation
Manifest/policy infrastructure itself

Relurpic capabilities should remain visible and invocable. Euclo may use them
as orchestration primitives or bounded delegation surfaces, but they remain
framework-native capabilities.

execution paradigms, I would map them like this:

planner.plan

Paradigm: PlannerAgent
Mix: planner only, optionally reflection for high-confidence planning
review_findings

Paradigm: ReflectionAgent or ReActAgent + Reflection
Mix: react for code exploration, reflection for structured findings
verify_change

Paradigm: PipelineAgent
Mix: deterministic pipeline with tool-required verification stage; optional react fallback if verification fails and diagnosis is needed
trace_execute_analyze

Paradigm: PipelineAgent + ReActAgent
Mix: pipeline for trace -> collect -> analyze contracts, react for open-ended interpretation or recovery
reproduce_localize_patch

Paradigm: PipelineAgent + ReActAgent
Mix: pipeline for reproduce -> localize, react for patching and repair loops, optional reflection for final review
edit_verify_repair

Paradigm: ReActAgent + PipelineAgent
Mix: react for local editing/search, pipeline or deterministic verifier for proof-producing verification
test_driven_generation

Paradigm: PipelineAgent
Mix: deterministic failing test -> implementation -> passing verification, with react fallback if test creation or repair becomes ambiguous
plan_stage_execute

Paradigm: ArchitectAgent
Mix: planner + step executor, with react recovery inside failed steps
specialist_debugger

Paradigm: ReActAgent
Mix: react as primary, optional planner if multiple hypotheses need ranking
specialist_reviewer

Paradigm: ReflectionAgent
Mix: react for evidence gathering, reflection for critique synthesis

### Workflow Persistence

Euclo should reuse existing workflow state and checkpoint stores. Artifact
records should be stored in a way that allows interrupted runs, branch
selection, and review of prior evidence.

### Retrieval and Context Management

Euclo should reuse the framework context manager, retrieval systems, AST index,
and search services, but it should determine when and why those are used based
on coding execution state rather than on generic defaults alone.



## Why Euclo Should Be More Than a Standard Coding Agent

Relurpify already has features that conventional coding-agent products often do
not have as first-class architecture:

- explicit capability admission and policy
- generic subagent invocation
- multiple reusable reasoning paradigms
- workflow persistence and checkpoints
- retrieval and context machinery designed for small local models
- graph runtime surfaces

A conventional coding agent usually collapses these concerns into one hidden
agent loop and one user-facing chat abstraction. Euclo should do the opposite.

Euclo should expose a richer execution model:

- bounded execution profiles instead of one universal loop
- multiple paradigms instead of one reasoning style
- artifact contracts instead of vague completion
- explicit policy alignment instead of implicit tool authority
- inspectable execution history instead of opaque prompt state

That is how Euclo can leverage the Relurpify framework rather than simply
imitating present expectations for coding agents.

## Summary

Euclo should be the coding-runtime layer that turns Relurpify's primitives into
a coherent engineering execution system.

Its defining characteristics should be:

- environment-grounded execution
- coding-specific mode and execution-profile selection
- artifact-first state and reporting
- capability-aware routing
- paradigm routing across generic agents
- explicit success gating
- isolation for advanced execution

The most important implementation move is not prompt work. It is architectural
separation: move coding semantics out of generic `/agents` surfaces and into
`/named/euclo`, then let Euclo orchestrate the framework as a real runtime.



