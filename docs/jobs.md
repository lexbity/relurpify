# Jobs

Relurpify has two related job models:

1. the durable `jobs` package
2. the in-memory `contextstream` background request job

## Durable jobs

The `jobs` package defines the persistent job boundary.

- `jobs.Job` is the durable record
- `jobs.Spec` defines the work to run
- `jobs.State` tracks the lifecycle
- `jobs.Event` records transitions and checkpoints
- `jobs.Submitter` submits work into the boundary

The durable store implementation in `jobs/store/badger.go` persists the job
records in Badger.

### Job states

- `queued`
- `running`
- `completed`
- `failed`
- `cancelled`

### Job events

- `created`
- `started`
- `checkpoint`
- `completed`
- `failed`
- `cancelled`
- `retried`

## Background context-stream jobs

`context/contextstream/trigger.go` exposes a separate job type for async
context-stream requests.

- `RequestBlocking` runs the compiler synchronously.
- `RequestBackground` creates a `contextstream.Job`, starts a goroutine, and
  completes the job when the compiler returns.
- `contextstream.Job.Wait` blocks until the job finishes or the context ends.

That job type is in-memory only. It is used to model request progress, not to
persist work across process restarts.

## Indexing and staleness workers

Background work in the knowledge subsystem is driven by indexing and
invalidation helpers rather than a generic queue:

- `context/knowledge/ast/index_manager.go` can launch a background workspace
  index pass.
- `ayenitd/bkc_bootstrap.go` runs a one-shot bootstrap indexing pass and emits
  a bootstrap-complete event.
- `context/knowledge/staleness.go` applies freshness updates and propagates
  invalidation through derived chunks.
- `context/knowledge/invalidation.go` wires chunk-staled events into the
  staleness manager.

## Euclo background jobs

Euclo has a background job node in `named/euclo/orchestrate/background.go`.
That node:

- builds a `jobs.Spec`
- submits it through a `jobs.Submitter`
- records submission/completion metadata in the envelope
- emits telemetry events for job submission and completion

## Operational note

The durable `jobs` package and the in-memory `contextstream.Job` are related
but distinct. When documenting behavior, name which one you mean.
