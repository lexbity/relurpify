# Labs — *Agent Construction with Relurpify*

This folder holds every artifact the book builds, exactly as it reads at the
end of the chapter that introduced it. It is the book's cumulative code
listing; Appendix D is generated from it.

## Layout

- `stringkit/` — the demo repo, **reset for book 2** (D11): `slugify` strips
  punctuation, both tests pass, and no seeded failure is carried over from
  book 1. The critic you build in this book reads and critiques code against
  this library; chapters that need a broken state seed their own.
- `testdata/overlay/` — from Chapter 2 on, one folder per chapter
  (`ch02-fake-model/`, …, `ch13-final/`) holding the files the reader has
  added or changed by that chapter's end, at true repo-root-relative paths.
  The folder is named `testdata/` on purpose: the Go toolchain (and the
  repository's own AST gates) ignore `testdata` directories, so the lab
  source of truth never pollutes gate scans; `verify.sh` copies the overlay
  into place. Chapter 1 has no overlay: its baseline is the unmodified
  repository.
- `transcripts/` — captured output from real runs: `ch01-baseline.md` holds
  the build, vet, `make lint-arch`, doctor, and turn transcripts quoted in
  Chapter 1; `ch02-critic-package.md` and `ch03-gate-failures.md` hold the
  Chapters 2–3 captures.
- `verify.sh` — the tiered verifier. See below.

## Where the book's Go code lives (D13)

Every file the book writes is **self-contained in one sub-package**,
`cognitionzoo/critic/` — the critic agent and everything that supports it.
The overlay mirrors that path; nothing else in the repository is modified
except the one registration import in Chapter 9, which is the act of
declaring a paradigm. Two sequencing rules the repository's own consumer
gate forces, and the book obeys from the first line:

- Until the package has a production importer (the Chapter 6 runner
  command), all of its files are `_test.go` files. The consumer gate counts
  a test-only package as consumed by its own tests, and the AST gates
  deliberately police production code only (`gatescan` walks with
  `IncludeTests: false`).
- Production files arrive when they have a consumer: `ensemble.go` and the
  `cmd/critic-runner` command in Chapter 6, the contract and agent in
  Chapters 7–8 (imported via the runner), the runtime wiring in Chapter 9.

## The governance rule (D14)

The book's code never cheats the framework's security/policy/sandbox model
— not in production code, not in tests:

- **Models may be scripted.** `FakeModel` is a test double for *data*: the
  framework itself ships offline and tape model backends, and a model's
  output is not an action.
- **Capabilities may never be faked.** Tests that invoke a capability use
  the real `capability/registry.CapabilityRegistry`
  (`NewRegistry()` + `RegisterLegacyTool`, the framework's own unit-test
  pattern) behind a forwarding adapter. What a tool returns may be
  scripted; how it is admitted may not.
- The critic's paradigm code (Chapter 8) requires the permission checker
  from `paradigm.Deps` and fails closed without it, executes commands only
  through the sandboxed command runner under its policy, and no node
  touches the filesystem, network, or subprocess outside the governed
  path.

## Prerequisites

- A relurpify checkout at the edition pin `BETA-RC1-FALL2026`
  (`git clone https://github.com/lexbity/relurpify && cd relurpify &&
  git checkout BETA-RC1-FALL2026`). Source builds report
  `relurpish dev (commit none, built unknown)`.
- Go (go directive 1.26.0, toolchain go1.27.2), git, Python 3 + pytest for
  stringkit, jq for the telemetry receipts.
- **Ollama serving `gemma4:e4b`** — the book's model backend (you already
  run it from book 1). Docker, plus gVisor `runsc` on Linux, for turns that
  execute commands.

## The verifier — three tiers

```bash
./verify.sh build  ch01     # tier 1: compile + unit truth
./verify.sh gates  ch01     # tier 2: the framework's law
./verify.sh behavior ch09   # tier 3: end-to-end behavior (from Chapter 9)
```

What each tier honestly proves:

1. **build** — applies the chapter overlay (if any) to a scratch clone at
   the pin, then `go build ./... && go vet ./...` plus `go test` over the
   chapter's packages. Proves compile and unit truth. Chapter 1 has no
   overlay, so tier 1 checks your checkout as it stands.
2. **gates** — additionally `make lint-arch` (and, as chapters introduce
   them: `make lint-all`, `make domain-check`, `make test-euclo-golden`,
   `make test-conformance`, `make test-coverage`). Proves the framework's
   law holds for the contribution so far.
3. **behavior** — (Chapter 9+) the relurpish offline pinned run of the
   critic recipe against stringkit and
   `dev-agent agenttest run --profile ci-replay`. Proves end-to-end
   behavior; needs the sandbox toolchain.

Live-model tiers (Chapter 12) are documented but not runnable in
CI-style verification — their transcripts are captured during authoring.

## Honesty rules

- Every transcript in `transcripts/` was produced by a real run on a real
  machine: Linux 6.18, Go 1.27.3, Ollama 0.40.8 serving `gemma4:e4b`,
  Docker 29.9.2 with `runsc` release-20260406.0, Python 3.14.8, pytest
  9.1.1. The telemetry `service.version` receipt reads
  `v0.0.0-20261020195847-1bc57c620296+dirty` — the tree that became the
  edition pin.
- Where a transcript was path-normalized (for example a scratch directory
  rendered as `~/code/stringkit`) or elided (always marked), the chapter or
  this README says so.
- Binaries were built from that tree; source builds report `dev` in
  `--version`.
