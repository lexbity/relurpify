# Contributing to Relurpify

> **Audience**: anyone who wants to change the framework.
> **Purpose**: the rules of the contribution workflow: sign-off, gates, coverage, commits, releases.
> **Prerequisites**: a working build ([Getting started](docs/GETTING_STARTED.md)); test details live in [TESTING](docs/TESTING.md).

## Ground rules

Relurpify is a solo-maintained open-source project and accepts contributor
pull requests.

- Every PR commit must carry a `Signed-off-by` trailer (DCO):
  commit with `git commit -s`.
  The CI DCO check exempts merge commits, but keep merge commits limited to
  review/merge workflows.
- The project charter forbids shims, compatibility layers, aliases, and
  stubs; gates grep for both the vocabulary and the telltale type-converter
  signature. A PR that introduces any of the four does not merge.
- Never read process environment variables outside the configuration
  boundary (`userconfig/config`); a grep gate enforces this.

## Dev setup

```bash
git clone <your-fork> && cd relurpify
go build ./...
go vet ./...
```

In sandboxed machines, repo-local Go caches help:

```bash
export GOMODCACHE=$PWD/.gomodcache
export GOCACHE=$PWD/.gocache
```

The repository is itself a Relurpify workspace: its checked-in
`relurpify_cfg/` tree is a generated materialization of the embedded
template bundle (`userconfig/templates/embedfs/`).
To change shipped configuration or prompts, edit the templates and run
`make generate-config`; a CI gate (`make check-config-tree-drift`) fails if
the checked-in tree diverges from the bundle.

## Branching

Trunk-based: `main` is the integration branch and must stay green.
Use short-lived topic branches for anything larger than a one-commit fix and
open pull requests against `main`.

## Commit messages

Match the existing history: a short, lowercase, imperative subject line
(keep it under about 72 characters when you can), an optional body separated
by a blank line, and the `Signed-off-by` trailer from `git commit -s`.
The project does not use Conventional Commits prefixes.

## Gates

Architecture fences run in two layers:

- **AST gates** (`envcheck`, `shimcheck`, `symcheck` in `tooling/arch/cmd/`)
  parse the syntax tree, so they cannot be bypassed by aliasing an import,
  concatenating a forbidden string, or renaming around a pattern.
- **Grep gates** cover what a syntax tree cannot express, such as a forbidden
  string assembled by concatenation.

Each AST gate that accepts anything does so through an explicit exemption
table in its command file, naming exact paths rather than filenames.
Every entry is load-bearing: the gates' own tests fail if an exemption stops
being needed, so a hole cannot quietly grow a comment.

### Local gate commands

| Command | What it checks |
|---|---|
| `make lint-arch` | Architecture invariants: dependency DAG, class normalization, HITL posture, plus the AST gates |
| `make lint-all` | Structural gates: layering, dead code, ghost schemas, euclo gates |
| `make check-gates-honest` | The gates themselves have not been weakened |
| `make grep-architecture-gates` | Architecture grep fences |
| `make check-gates-slice10` | Dead code and forbidden patterns |
| `make check-contract-dissolution` | The removed manifest spine stays removed |
| `make lint-class-normalization` | Class-string normalization at every boundary |
| `make lint-no-permissive-hitl` | No permissive HITL defaults creep in |
| `make lint-config` | Configuration schema lint |
| `make check-config-tree-drift` | Checked-in config matches the template bundle |
| `make generate-config` | Regenerate `relurpify_cfg/` from templates |
| `make lint-go` / `make lint-go-fix` | golangci-lint run / auto-fix |
| `make domain-check` | Domain dependency rules |

### Lint ratchet

golangci-lint (v2.15.0 — must be ≥ v2.15.0 to typecheck Go 1.27) runs as a
ratchet: CI lints only new code
(`--new-from-rev=origin/main`), and a second pass enforces the "graduated"
linter set over the whole tree (`.golangci-graduated.txt`).
Do not add new violations; fixing old ones is welcome.

## Coverage

All packages must keep at least 70% test coverage.
New code must include tests that bring its package to or above 70%.
A package that cannot reach 70% because its surface is integration-test-only
must carry a justification comment in the package's `doc.go`.

```bash
make test-coverage        # builds the coverage gate and enforces the floor
make coverage-baseline    # regenerate the baseline after deliberate changes
```

## CI

Every push and pull request runs, on GitHub Actions:

| Job | Steps |
|---|---|
| DCO | `Signed-off-by` present on every PR commit |
| Architecture gates | `make lint-arch`, `make lint-all`, `make check-gates-honest` |
| Lint | golangci-lint ratchet + graduated set |
| Test | `make test-unit`, `make test-conformance` |
| Build and vet | `go build ./...`, `go vet ./...`, `govulncheck`, config drift, contract dissolution, grep gates, `make test-dev-agent`, `make test-tape-fidelity` |
| Coverage floor | `make test-coverage` |

Live agent tests, conformance, and DSL goldens run locally because they need
a model; see [TESTING](docs/TESTING.md).

## Pull request checklist

- [ ] Commits are signed off (`git commit -s`).
- [ ] `make lint-arch` and `make lint-all` pass.
- [ ] `make test-unit` and `make test-conformance` pass.
- [ ] `make test-coverage` passes; new packages meet the 70% floor.
- [ ] Configuration changes were made in the templates and regenerated
      (`make generate-config`, `make check-config-tree-drift`).
- [ ] New envelope keys are registered and documented in the key registry.
- [ ] Goldens were re-baselined only deliberately (see [TESTING](docs/TESTING.md#goldens)).
- [ ] Docs that reference changed paths, commands, or env vars were updated,
      and references were verified against the tree.
- [ ] The PR summary states what changed and why and names any known gaps.

## Reporting bugs and requesting features

Open an issue with: what you ran (exact command), what you expected, what
happened (the real error text), and your platform.
For agent-behavior bugs, attach the relevant
`.relurpify_state/telemetry/telemetry.jsonl` lines (they contain no source
text; redaction scrubs sensitive values).

[TODO: issue-tracker link — GitHub issues on github.com/lexbity/relurpify or Codeberg; confirm the canonical tracker]

## Release process

1. All gates pass on `main`.
2. The maintainer tags the release: `git tag v0.1.0 && git push origin v0.1.0`.
3. CI runs GoReleaser (`release --clean`).
4. GoReleaser creates a **draft** GitHub Release with all artifacts:
   binaries for Linux and macOS (amd64 + arm64), `checksums.txt`, SBOMs,
   `.deb`, `.rpm`, and an AUR package.
   The internal tooling binaries (`archcheck`, `domaincheck`, `driftcheck`)
   additionally build for Windows.
5. The maintainer reviews the draft, adds release notes, and publishes.

Update [CHANGELOG](CHANGELOG.md) in the same PR that changes user-visible
behavior.
