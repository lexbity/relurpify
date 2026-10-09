## Contributing to Relurpify

Relurpify uses DCO sign-off on pull requests.

## Required before opening a PR

- Commit with `git commit -s` so every PR commit includes a `Signed-off-by` trailer.
- Keep merge commits limited to review/merge workflows; the CI DCO check exempts merge commits.
- Run the relevant local gates before asking for review.

## Recommended local checks

- `make lint-arch` — architecture invariant gates (domain DAG, class normalization, HITL, plus the AST-based gates `envcheck`, `shimcheck`, `symcheck`)
- `make lint-all` — structural gates (layering, invariants, dead code, ghost schemas, euclo gates)
- `make check-gates-honest` — verify the gates have not been weakened (grep patterns, AST gate presence, and wiring into `lint-arch`)
- `make check-contract-dissolution` — manifest spine removed
- `make grep-architecture-gates` — architecture grep fences
- `make check-gates-slice10` — dead code and forbidden patterns
- `make test-coverage` — per-package coverage floor (70% minimum)
- `make test-dev-agent` — dev-agent build and test baseline
- `make test-tape-fidelity` — LLM tape replay fidelity

## Gate layers

Architecture fences run in two layers. The AST-based gates (`envcheck`,
`shimcheck`, `symcheck` in `tooling/arch/cmd/`) parse the syntax tree, so they
cannot be bypassed by aliasing an import, concatenating a forbidden string, or
renaming around a pattern. The grep gates are kept as the secondary layer for
what a syntax tree cannot express — a string assembled by concatenation, for
example, is not a literal.

Each AST gate that accepts anything does so through an explicit exemption table
in its command file, naming exact paths rather than filenames. Every entry is
load-bearing: the gates' own tests fail if an exemption stops being needed, so
a hole cannot quietly grow a comment.

## Coverage requirements

All packages MUST maintain at least 70% test coverage. New code MUST include tests that bring the package to or above 70%. Packages that cannot meet 70% due to integration-test-only surfaces MUST have a justification comment in the package's `doc.go`.

## Release process

1. All gates pass on `main`.
2. Maintainer creates a tag: `git tag v0.1.0 && git push origin v0.1.0`.
3. CI runs `goreleaser release --clean`.
4. GoReleaser creates a draft GitHub Release with all artifacts (binaries, checksums, SBOMs, .deb, .rpm, AUR).
5. Maintainer reviews the draft, adds release notes, and publishes.

## Documentation changes

- Keep docs aligned with the code and checked-in config.
- If a doc references a path, command, or env var, verify it exists in the tree first.
- Prefer correcting stale references over adding new wording around them.

## Pull request hygiene

- Keep changes focused.
- Include a concise summary of what changed and why.
- Mention any known gaps, unverified behavior, or follow-up work.
