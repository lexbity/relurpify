# Lab — ch07-substrate

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 7 ("The
Knowledge Substrate"). **Pin:** tag `BETA-RC1-FALL2026`; captured at
commit `1bc57c62029641dcc919c04a8076593794246409` while the tag is
pending.

**What this lab is:** index a seeded workspace and inspect the chunk
graph — via the substrate's own tests, which build real stores, index
real files, and emit real events. No model backend, no sandbox, no
manual workspace setup: every fixture is seeded and cleaned by the
tests themselves.

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | `go test ./context/knowledge/... -count=1` | verdicts yes; timings no | 7 packages `ok`: knowledge, ast, graphdb, ingestion, memory, retrieval, search |
| 2 | `go test ./context/knowledge -run 'TestWorkspaceBootstrap' -count=1 -v` | yes | `--- PASS: TestWorkspaceBootstrapIndexFailureDegradesNotAborts` and `--- PASS: TestWorkspaceBootstrapSuccessIndexesAndEmitsComplete` |
| 3 | `sed -n '168,196p' context/knowledge/chunk.go` | yes | the `KnowledgeChunk` struct: identity, three axes, provenance, tombstoning |
| 4 | `sed -n '121,136p' context/knowledge/chunk.go` | yes | the sixteen `EdgeKind` constants |
| 5 | `sed -n '1,2p' context/knowledge/graphdb/doc.go` and `sed -n '488p' context/knowledge/store.go` | yes | "in-memory adjacency maps"; "NOTE: full scan" |

## Honesty notes

- The bootstrap success test seeds a one-file workspace, so
  `IndexedFiles = 1` is the fixture's number, not a general measurement;
  the *event shape* (kind, payload fields) is the portable claim.
- The full-scan note at `store.go:488` is a source comment on one
  method — the *complete* list of known full scans (by-workspace chunk
  lookup, lineage search, compilation-record load) lives in
  `docs/ARCHITECTURE.md:303-304`, and this lab verifies one exemplar,
  not the whole list.
- Timings in the transcript (4.5s for the knowledge package) are
  machine-specific — Badger opens real stores under `-count=1`.

## Verification

`verify.sh ch07` (from the labs root) replays card items 1–5 and diffs
the deterministic outputs (verdict lines, struct and constant
declarations, doc lines).
