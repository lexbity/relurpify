# Rex Agent

## Synopsis

Rex is a Nexus-managed named runtime for long-running, event-driven workflow
execution. It receives canonical events, classifies intent and risk, routes
work to the appropriate orchestration family, and produces a durable proof
trail for every execution.

Where most agents in Relurpify are single-invocation executors, Rex is designed
to stay alive across many tasks. It manages its own work queue, periodically
scans for recoverable workflows, and exposes health projections to the Nexus
gateway. The agent never executes work directly — it classifies, routes, and
delegates to one of the existing agent runtimes (ReAct, Planner, Architect,
Pipeline, and several experimental families), then gates completion on
route-aware verification policy.

## How It Works

Rex processes work through a deterministic pipeline of internal stages:

1. **Envelope normalization** — the inbound task and state context are merged
   into a single `Envelope` that captures the instruction, workspace, mode
   hints, edit permissions, workflow/run identity, and capability snapshot.
2. **Classification** — the envelope is analyzed for intent (`analysis`,
   `planning`, `review`, `mutation`), risk level (`low`, `medium`, `high`),
   and behavioural flags (`ReadOnly`, `DeterministicPreferred`,
   `RecoveryHeavy`, `LongRunningManaged`).
3. **Routing** — classification maps to a primary orchestration family with
   mode, profile, persistence/proof/retrieval requirements, and fallback
   chains:
   - `pipeline` — deterministic, structured tasks
   - `planner` — read-only planning
   - `architect` — mutation-capable or recovery-heavy work
   - `react` — general open-ended tasks
4. **Identity resolution** — a deterministic workflow and run ID are computed
   from a SHA-1 of the task ID, source, and instruction. Existing workflow IDs
   from the envelope take precedence.
5. **Workflow retrieval** — when the route requires retrieval, Rex queries the
   workflow state store for prior completion artifacts and knowledge base
   entries to expand the execution context.
6. **Delegate execution** — the chosen agent family receives the (possibly
   enriched) task and executes it.
7. **Completion gating** — route-aware verification policy determines whether
   the execution is allowed to complete. Mutations require passing
   verification evidence; read-only and planning routes do not.
8. **Proof persistence** — proof surface, action log, completion decision,
   verification policy, verification evidence, and success gate results are
   persisted as workflow artifacts. Start and finish events are appended to
   the workflow event log.

## Event Model

Rex defines four canonical event types, each with trust-class ingress rules:

| Event Type | Trust Requirement | Purpose |
|---|---|---|
| `rex.task.requested.v1` | Any | New task intake |
| `rex.workflow.resume.v1` | Trusted or Internal | Resume a paused workflow |
| `rex.workflow.signal.v1` | Trusted or Internal | Signal an active workflow |
| `rex.callback.received.v1` | Trusted or Internal | Deliver an expected callback |

Events carry an `IdempotencyKey` for deduplication, a `Partition` for routing,
and a `TrustClass` (`internal`, `trusted`, `untrusted`) that controls ingress
filtering. The `DefaultNormalizer` validates all events before they enter the
pipeline.

Adapters exist for:
- **Map payloads** — `MapAdapter` normalizes generic `map[string]any` into
  canonical events.
- **Framework events** — `FromFrameworkEvent` bridges the internal framework
  event bus into Rex canonical form with `TrustInternal`.

Events can be converted to `Envelope` (for classification) or `*core.Task`
(for direct execution), preserving all ingress metadata.

## Workflow Gateway

The `DefaultGateway` sits in front of Rex and makes start/signal/reject
decisions for incoming events:

- **Start** — if no workflow exists for the resolved identity, the gateway
  allows a new start. If a workflow already exists, the start is demoted to
  a signal against the existing workflow.
- **Signal** — validated against the workflow store. Stale signals (targeting
  completed or failed workflows/runs) are rejected. Callback signals must
  match an `expected_callback` key.
- **Reject** — untrusted signals are always rejected. Unknown event types
  with no trust classification are rejected.

Identity is deterministic: SHA-1 of `type::actor_id::partition::idempotency_key`,
or an explicit `workflow_id` from the event payload.

## Delegate Registry

Rex maintains a registry of orchestration family delegates. Each delegate
wraps an existing agent runtime:

| Family | Agent | Notes |
|---|---|---|
| `react` | ReActAgent | Checkpoint path configured |
| `planner` | PlannerAgent | Read-only planning |
| `architect` | ArchitectAgent | Checkpoint + workflow state paths |
| `pipeline` | PipelineAgent | Workflow state path configured |
| `blackboard` | BlackboardAgent | Experimental |
| `goalcon` | GoalConAgent | Experimental |
| `htn` | HTNAgent | Experimental |
| `rewoo` | ReWOOAgent | Experimental |
| `chainer` | ChainerAgent | Experimental |

Resolution follows the execution plan's fallback chain. If the primary family
is unavailable, Rex tries each fallback in order before returning an error.

## Runtime Manager

The runtime manager coordinates long-running work:

- **Work queue** — buffered channel with configurable capacity (default 32).
  Items that exceed capacity mark health as degraded.
- **Recovery scanning** — every 30 seconds (configurable), the manager scans
  the workflow state store for workflows in `running` or `needs_replan` status
  and records them as recovery candidates.
- **Health tracking** — three states: `healthy`, `recovering`, `degraded`.
  Health transitions on execution start/finish, recovery scans, and queue
  overflow.
- **Execution tracking** — `BeginExecution` increments active count and
  returns a finish callback that decrements it and records errors.

The manager can run as a background loop (`Start`/`Stop`) or be driven
synchronously through `BeginExecution` during `Agent.Execute`.

## Control Plane

Rex includes admission control and operational audit infrastructure:

- **LoadController** — capacity-based admission. Critical workloads bypass
  capacity limits.
- **FairnessController** — per-tenant quota enforcement. Tracks usage per
  tenant ID and rejects requests exceeding the configured limit.
- **AuditLog** — thread-safe append-only log of all admission and operator
  authorization decisions.
- **Operator actions** — operator role validation with privileged role
  checking. All authorization attempts are recorded in the audit log.
- **SLO signals** — aggregates workflow health metrics (total, running,
  completed, failed, recovery-sensitive, degraded workflow IDs).
- **DR metadata** — disaster recovery state per workflow (failover readiness,
  recovery state, runtime version, last checkpoint).

## Reconciliation

When execution produces ambiguous results, Rex's reconciliation layer handles
resolution:

- **InMemoryReconciler** — records ambiguities with a deterministic ID
  (`workflow:run:reason`). Ambiguous records start in `operator_review`
  status with retry suppressed. Resolution transitions to `verified`,
  `repaired`, `operator_review`, or `terminal`.
- **ProtectedWriter** — fencing tokens prevent stale writes. Each `Reserve`
  increments a monotonic counter per resource. `Validate` rejects writes
  with tokens older than the expected token.
- **InMemoryOutbox** — idempotent intent queue keyed by resource. Supports
  append and list operations for deferred side effects.

## Proof and Verification

Every Rex execution produces a `ProofSurface` summarizing:

- Route family, mode, and profile
- Verification status and source (pipeline or react)
- Whether verification evidence was produced
- Recovery attempt count
- Artifact kinds persisted
- Whether workflow retrieval was used
- Completion allowed/blocked

Verification policy is route-aware:

| Mode | Requires Verification | Requires Executed Check |
|---|---|---|
| `planning`, `open` | No | No |
| `structured`, `mutation` | Yes (if not read-only) | Yes (if not read-only) |

The success gate evaluates: evidence present, status in accepted list (`pass`),
and at least one passing check record when required. Manual verification is
not allowed for structured or mutation modes.

## Nexus Integration

Rex exposes itself to the Nexus gateway through:

- **RuntimeProjection** — health, active work count, queue depth, recovery
  count, last error, last proof surface, current workflow/run ID.
- **ManagedAdapter** — implements `ManagedRuntime` with `Execute`,
  `RuntimeProjection`, `Registration`, `Invoke`, and `AdminSnapshot`.
- **Registration** — declares `nexus-managed` runtime type, managed flag,
  capabilities, and projection tiers (`hot`, `warm`).
- **AdminSnapshot** — runtime projection plus workflow reference URIs and
  hot/warm state projections from the workflow state store.

## Configuration

| Setting | Default | Description |
|---|---|---|
| `RuntimeMode` | `nexus-managed` | Hosting mode (`nexus-managed` or `embedded`) |
| `QueueCapacity` | `32` | Work queue buffer size |
| `RecoveryScanPeriod` | `30s` | Interval between recovery scans |
| `IdlePollPeriod` | `200ms` | Idle polling interval |
| `RequireProof` | `true` | Whether proof is required for completion |

## Capabilities

Rex declares five capabilities: `Plan`, `Execute`, `Code`, `Explain`, and
`HumanInLoop`.

## Retrieval Policy

Context expansion follows a family-specific retrieval policy:

| Family | Strategy | Workflow Limit | Max Tokens |
|---|---|---|---|
| `planner` | `local_then_workflow` | 6 | 800 |
| `architect`, `pipeline` | `local_then_targeted_workflow` | 5 | 700 |
| `react` (default) | `local_first` | 4 | 500 |

All families start with local paths extracted from task metadata and context.
Widening to workflow retrieval happens when the policy requires it or when no
local paths are available. Workflow retrieval queries the retrieval service
scoped to the workflow ID, with fallback to knowledge base listing.

## Persisted Artifacts

Each execution persists up to seven artifact types to the workflow state store:

- `rex.proof_surface` — execution summary
- `rex.action_log` — route, classification, retrieval, verification steps
- `rex.completion` — completion gate decision
- `rex.verification_policy` — route-aware verification policy
- `rex.verification` — normalized verification evidence
- `rex.success_gate` — success gate evaluation result
- `rex.context_expansion` — retrieval expansion details (when applicable)

## Integration Status

Rex is feature-complete at the engine level. All internal modules have
comprehensive test coverage. The remaining integration work is wiring Rex
into the Nexus gateway, CLI, and TUI layers.

## Source

`named/rex/`
