# Labs — *Cooking Agentic Thoughts with Relurpify*

This folder holds every artifact the book builds, exactly as it reads at the
end of the chapter that introduced it. It is the book's cumulative code
listing; Appendix D is generated from it.

## Layout

- `ch01-stringkit/` — the stringkit demo repo as seeded in Chapter 1
  (buggy `slugify`, one intentionally failing test, captured transcript).
- Later chapters add one folder each (`ch04-explain-test/`, …) containing the
  DSL and config artifacts at that chapter's end state.
- `verify.sh` — the tiered verifier. See below.

## Prerequisites

- The relurpify binaries built from tag `BETA-RC1-FALL2026`
  (`go build -o relurpish ./app/relurpish`,
  `go build -o relurplint ./app/relurplint`). Source builds report
  `relurpish dev (commit none, built unknown)`.
- Python 3 with pytest for the stringkit chapters.
- **Ollama serving `gemma4:e4b`** — the book's model backend (the
  materialization default). The offline scripted backend appears only where
  the book says so (deterministic test tiers).
- Docker (and gVisor `runsc` on Linux for the default sandbox backend) for
  the tiers that boot the runtime.

## The verifier

```bash
./verify.sh ch01          # runs the Chapter 1 check
```

Tiers, and what each one honestly proves:

1. **lint** (Chapters 4+) — materializes a scratch workspace with
   `relurpish doctor --workspace <scratch>` (plain doctor materializes a
   fresh workspace without prompting; `--fix` re-materializes and asks
   before overwriting), overlays the chapter's `relurpify_cfg/` files, and
   runs `relurplint --check recipes|prompts|all --workspace <scratch>`.
   Proves grammar, schema, and load-time rules.
2. **boot** (Chapters 4+) — additionally registers the overlaid artifacts in
   a booted offline runtime. Proves what lint cannot: tool-vocabulary and
   prompt-import resolution happen at boot.
3. **behavior** (Chapter 9+) — runs agenttest suites offline and compares
   against recorded expectations.

The Chapter 1 check is simpler than all three tiers: it runs stringkit's
tests in a scratch copy and asserts the seeded failure — one test red, one
green, exactly as captured in `ch01-stringkit/expected-failure.txt`.

## Honesty rules

- Every transcript in the labs was produced by a real run on a real machine
  (Linux, Docker + gVisor present, Python 3.14, pytest 9.1, Go 1.27).
- Where a transcript was path-normalized (for example a scratch directory
  rendered as `~/code/stringkit`), the chapter or this README says so.
- Binaries built from the tree that became `BETA-RC1-FALL2026`; source
  builds report `dev` in `--version`.
