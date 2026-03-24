# Euclo

## Synopsis

Euclo is the primary coding agent in Relurpify. When a manifest sets
`spec.agent.implementation` to `coding` or `euclo`, the runtime instantiates
Euclo. It is a named agent (`named/euclo/`) that transforms user instructions
into structured, multi-phase coding workflows before delegating actual execution
to the ReAct agent paradigm.

Euclo exists because small local models benefit from structured decomposition.
Rather than dropping a raw instruction into a reasoning loop and hoping the model
figures it out, Euclo classifies the task, selects an appropriate execution
profile, and gates progress between phases with evidence requirements. The model
still does the reasoning — Euclo constrains and sequences it.

---

## How It Works

A task flows through Euclo in this order:

```
Instruction arrives
        │
        ▼
Session Scoping ── prevent recursive Euclo invocations
        │
        ▼
Task Intake ── normalize instruction + context into TaskEnvelope
        │
        ▼
Classification ── signal-based scoring: keywords, error text, context hints
        │
        ▼
Mode Resolution ── choose mode: code, debug, planning, review, tdd
        │
        ▼
Profile Selection ── choose execution profile: which phases, capabilities, gates
        │
        ▼
Phase Machine ── execute mode-specific phases with evidence gates
        │
        ▼
Verification ── collect evidence (tests, compilation), evaluate success gate
        │
        ▼
Result ── artifact normalization, persistence, final report
```

### Classification

Euclo classifies every incoming task using signal-based scoring:

- **Keyword signals** — words like "test", "fix", "refactor", "debug", "plan"
  contribute to intent families
- **Error text signals** — stack traces, panics, and runtime errors push toward
  debug mode
- **Task structure** — test-related patterns push toward tdd mode
- **Context hints** — explicit `mode` or `mode_hint` values in `task.Context`
  take priority

Classification produces a `TaskClassification` with intent families, evidence
requirements, risk level, and a confidence score.

### Modes

Each mode defines a different phase structure:

| Mode | Purpose | Phases |
|------|---------|--------|
| **code** | Standard edit/implement task | 5 phases |
| **debug** | Debugging and reproduction | 6 phases |
| **planning** | Strategic planning | 6 phases |
| **review** | Code review | Defined, phases TBD |
| **tdd** | Test-driven development | Defined, phases TBD |

Mode resolution follows a priority order: explicit hint in task context →
constraint-based selection → resumed mode → classifier recommendation.

### Execution Profiles

An execution profile defines which phases to run, which capabilities are
eligible, and what evidence gates must pass between phases. Examples:

- **edit_verify_repair** (default) — explore → plan → edit → verify
- **reproduce_localize_patch** (debug-focused) — reproduce → localize → patch → verify
- **plan_stage_execute** — planning-centric with staged execution

Profile selection considers mode, edit permissions, evidence requirements, and
risk level.

### Phase Machine

Each mode has a `PhaseMachine` — a state machine where each phase is handled by
a `PhaseHandler`. Phases emit `InteractionFrame` objects to communicate with the
UX layer:

- **Proposals** — suggested actions for user confirmation
- **Questions** — requests for user input
- **Drafts** — intermediate results for review
- **Results** — completed phase output
- **Transitions** — phase boundary notifications

Between phases, evidence gates validate that required artifacts exist and meet
criteria before the next phase can begin. If a gate fails, the phase machine can
retry, ask for input, or abort.

### Verification and Success Gate

After execution, Euclo applies a verification policy:

- Collects verification evidence (test results, compilation output, execution logs)
- Evaluates the success gate against collected evidence
- Decides whether changes are approved for mutation

The verification policy can be `required`, `optional`, or `none` depending on
the profile and risk level.

---

## Artifact Model

Euclo records structured artifacts at every stage for auditability and
resumability:

- **Intake** — normalized task envelope
- **Classification** — intent, confidence, evidence requirements
- **Mode/Profile** — resolution decisions and rationale
- **Retrieval** — context gathered from codebase
- **Routing** — capability dispatch decisions
- **Edits** — file modifications
- **Verification** — test and compilation evidence
- **Reports** — final summary with action log and proof surface

All artifacts are typed with `ArtifactKind` and persisted to a
`WorkflowArtifactStore` (SQLite) when configured. This enables multi-session
resumption — Euclo can detect a resumed session from a previous invocation and
continue from where it left off.

---

## Relationship to Other Layers

Euclo is a **named agent** in `named/euclo/`. It composes rather than
reimplements:

- **Delegates to** `agents/react.ReActAgent` for actual LLM reasoning and tool
  execution
- **Uses** `framework/capability.Registry` for capability admission and policy
- **Uses** `framework/graph` for the underlying execution graph
- **Uses** `framework/memory` for artifact persistence

The key difference from using ReAct directly is that Euclo adds classification,
mode selection, phased execution, evidence gating, and structured artifact
tracking on top. A raw ReAct agent receives an instruction and reasons freely;
Euclo constrains the reasoning into a structured workflow that small models can
follow reliably.

---

## Interaction Model

Euclo is UX-agnostic. It communicates with the application layer through
`InteractionFrame` objects:

- The TUI (`app/relurpish`) renders frames as user-facing prompts and collects
  responses
- Batch or headless execution can use a `NoopEmitter` that auto-approves
- An `AgencyResolver` maps user phrases to capability triggers or phase jumps

This means Euclo's orchestration logic works identically whether invoked from the
TUI, the dev-agent CLI, or programmatically.

---

## Selection

Euclo is selected via manifest:

```yaml
spec:
  agent:
    implementation: coding   # or: euclo
```

Both `coding` and `euclo` resolve to the same runtime. The factory
(`named/factory/factory.go`) instantiates Euclo and wires it with the
`AgentEnvironment`.

---

## Package Structure

```
named/euclo/
├── agent.go                 # Root Agent type and Execute entry point
├── session_scoping.go       # Recursion prevention
├── interaction_registry.go  # Mode machine registration
├── euclotypes/              # Core types: modes, profiles, artifacts, registries
├── interaction/             # Phase machine, frame emitter, agency resolver
│   └── modes/               # Mode implementations (code, debug, planning, review, tdd)
├── orchestrate/             # Profile controller, recovery, interactive bridge
├── gate/                    # Evidence gate definitions and evaluation
├── runtime/                 # Classification, routing, verification
├── capabilities/            # Coding capability registry and implementations
├── internal/                # Internal helpers
└── euclotest/               # Test utilities
```
