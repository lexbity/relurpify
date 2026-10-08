## Contributing to Relurpify

Relurpify uses DCO sign-off on pull requests.

## Required before opening a PR

- Commit with `git commit -s` so every PR commit includes a `Signed-off-by` trailer.
- Keep merge commits limited to review/merge workflows; the CI DCO check exempts merge commits.
- Run the relevant local gates before asking for review.

## Recommended local checks

- `make lint-arch` — architecture invariant gates (domain DAG, class normalization, HITL, env access, shim language)
- `make lint-all` — structural gates (layering, invariants, dead code, ghost schemas, euclo gates)
- `make check-gates-honest` — verify gate patterns have not been weakened
- `make check-contract-dissolution` — manifest spine removed
- `make grep-architecture-gates` — architecture grep fences
- `make check-gates-slice10` — dead code and forbidden patterns
- `make test-coverage` — per-package coverage floor (70% minimum)
- `make test-dev-agent` — dev-agent build and test baseline
- `make test-tape-fidelity` — LLM tape replay fidelity

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
