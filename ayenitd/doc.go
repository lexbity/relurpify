// Package ayenitd is the Relurpify service runner: a spawned, per-workspace
// process that supervises background services and executes the durable jobs
// queue (S8 adds the runner binary ayenitd/cmd/relurpify-runner).
//
// Charter (normative until S8 fills it):
//   - The runner supervises runner-internal services (spool watcher,
//     executor, status writer) and drains the file spool into the durable
//     jobs store (context/jobsstore, Badger-backed).
//   - Payload is derived-structure maintenance only (knowledge.bootstrap,
//     knowledge.refresh) — never user-visible action. Anything needing
//     governance (approvals, HITL) does not belong in a runner job.
//   - The runner process holds no secrets and no agent identity; handlers
//     do their own authorization via the normal capability path.
//   - The app spawns it per workspace (Q16); submission is spool-only, so
//     the app never opens the jobs store (Badger's single-process-per-
//     directory lock is the mutual-exclusion primitive).
//
// Between S7 and S8 this domain intentionally contains only this charter
// (the bounded interim-empty state): the bootstrap service it used to host
// relocated to context/knowledge, the browser stack and git watcher were
// deleted in S5.
package ayenitd
