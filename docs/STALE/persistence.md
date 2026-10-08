# Persistence

Relurpify uses a mixed persistence model. There is no single storage backend
for all runtime state.

## Verified storage paths

### Job storage

- `jobs/store/badger.go` uses Badger for durable job records.
- `jobs.Job` carries the durable job state machine: queued, running,
  completed, failed, and cancelled.
- `jobs.Event` records lifecycle transitions such as created, started,
  checkpointed, completed, failed, cancelled, and retried.

### Graph persistence

- `context/knowledge/graphdb/` uses a Badger-backed embedded graph engine.
- The engine stores nodes, edges, history, mutation results, and snapshots.
- `context/persistence/lifecycle_repository.go` builds workflow/run/delegation
  records on top of the graph engine.

### Artifact persistence

- `context/persistence/artifactstore/` stores large artifacts on disk.
- Artifacts are written under `.relurpify_state/artifacts/<session>/`.
- That store is filesystem-backed, not Badger-backed.

### Checkpoints and snapshots

- `context/persistence/checkpoint.go` writes checkpoint artifacts and stores
  a reference back into the envelope.
- `context/knowledge/graphdb/snapshot.go` writes graph snapshots as JSON.
- `context/knowledge/graphdb/engine.go` can snapshot or flush the durable
  backend on close or during background maintenance.

## What the graph engine persists

The graph engine stores:

- nodes
- edges
- node history
- edge history
- mutation results
- snapshots

It also runs a background maintenance loop that can autosave based on
threshold and interval settings.

## What is not a persistence guarantee

Do not describe the current system as a single-store architecture. The repo
uses:

- Badger for some durable indexes and job storage
- filesystem-backed artifact storage
- JSON snapshot files for some engine state

If a doc needs durability, atomicity, or consistency claims, it should name
the subsystem and cite the implementation directly.
