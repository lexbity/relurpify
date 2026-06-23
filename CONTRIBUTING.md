## Contributing to Relurpify

Relurpify uses DCO sign-off on pull requests.

## Required before opening a PR

- Commit with `git commit -s` so every PR commit includes a `Signed-off-by` trailer.
- Keep merge commits limited to review/merge workflows; the CI DCO check exempts merge commits.
- Run the relevant local gates before asking for review.

## Recommended local checks

- `make lint-arch`
- `make lint-all`
- `make check-contract-dissolution`
- `make grep-architecture-gates`
- `make check-gates-slice10`
- `make test-dev-agent`
- `make test-tape-fidelity`

## Documentation changes

- Keep docs aligned with the code and checked-in config.
- If a doc references a path, command, or env var, verify it exists in the tree first.
- Prefer correcting stale references over adding new wording around them.

## Pull request hygiene

- Keep changes focused.
- Include a concise summary of what changed and why.
- Mention any known gaps, unverified behavior, or follow-up work.
