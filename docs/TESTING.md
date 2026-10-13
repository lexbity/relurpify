# Testing

> **Audience**: contributors writing or running tests.
> **Purpose**: the three test layers, how to run each, and the supporting fixture systems (tapes, goldens, conformance).
> **Prerequisites**: [Contributing](../CONTRIBUTING.md) for gates and the coverage floor; a model backend for live layers.

## The three layers

| Layer | What it exercises | Needs a model? | Entry point |
|---|---|---|---|
| Unit | One package, colocated `*_test.go` | no | `make test-unit` |
| Framework integration | Cross-seam behavior over real runtime state | no | `make test-integ`, `make test-scenario` |
| Agent end-to-end (agenttest) | The real agent against a real or replayed model | live: yes; replay: no | `dev-agent agenttest run` |

`make test-unit` also runs the config lint and is offline-friendly with a
60-second budget.
Integration tests live in `testsuite/` and cover the authorization,
capability, knowledge, manifest, network-boundary, and telemetry seams plus a
security/redteam directory; shared builders construct workspaces, documents,
policies, and envelopes.

## Agent tests (agenttest)

Agent suites are YAML files in `testsuite/agenttests/` (named
`euclo.<name>.testsuite.yaml`).
Each case runs the real agent in a derived temp workspace with setup fixtures
and a model matrix, then asserts.

### The OSB assertion model

| Tier | Hardness | Asserts |
|---|---|---|
| Outcome | hard | What the run achieved |
| Security | hard | The sandbox contract, including expected violations |
| Benchmark | soft | Telemetry hints (calls, tokens, phases) |

Outcome and Security failures fail the case; Benchmark results are advisory.

### Running

```bash
go build -o dev-agent ./tooling/dev-agent-cli
./dev-agent agenttest run
```

Subcommands: `run`, `promote`, `report`, `rerecord`, `baseline`.

Execution profiles select where model traffic comes from:

| Profile | Behavior |
|---|---|
| `live` (default) | Real model calls against your configured backend |
| `record` | Live calls recorded into tapes |
| `replay` | Replays tapes; no network |
| `developer-live` | Live with developer conveniences |
| `ci-live` | Live, CI-sized |
| `ci-replay` | Replay, CI-sized; what the tape-fidelity gate uses |

Retries are infra-failure-only; optional model or server resets can be keyed
on error regexes.
The full YAML schema (suite header, cases, setup, expect, overrides) is
documented in [testsuite/README.md](../testsuite/README.md).

Supporting pieces: **subjects** (`testsuite/subjects/`) are per-agent adapters
that let the generic runner assert agent-specific behavior such as phases and
interaction frames.

## Tapes

Tapes are fingerprinted record/replay fixtures for LLM traffic.

- Replay matches requests by SHA-256 fingerprint; a mismatch is an actionable
  re-record error, never a silent pass.
- **Promotion** (`agenttest promote`) turns a passing live run into a golden
  fixture with lineage records.
- Freshness: a tape older than 30 days is stale; coverage reporting proposes
  re-record commands (`agenttest rerecord`).
- The committed smoke tape lineage lives at
  `testsuite/agenttests/tapes/`; replay coverage is enforced by
  `make test-tape-fidelity`.

## Conformance

`testsuite/conformance/` pins behavior that must not drift between tool
calling modes:

- Parity between native and fallback tool calling (traversal, grep, edit).
- Visibility and no-leak guarantees for tool results.
- Grant semantics and output ceiling spill.

CI runs it (`make test-conformance`); it needs no live model.

## Goldens

Two golden systems exist:

- DSL goldens: plan/graph/trace triples for thoughtrecipes, byte-identical by
  policy; re-baseline only deliberately, with explicit intent, and keep the
  golden mutation test passing (`make test-euclo-golden`).
- TUI panel goldens: rendered panel snapshots.

A golden diff is a design question first and a mechanical update second.

## Coverage

The floor is 70% per package; the gate builds and enforces it.

```bash
make test-coverage
```

Rules and the integration-surface exemption are in
[CONTRIBUTING](../CONTRIBUTING.md#coverage).

## Choosing the right layer

- Pure logic change: unit tests in the package.
- A cross-seam guarantee (policy, visibility, boundary): framework integration
  test or conformance suite.
- End-user-visible agent behavior: agenttest case; record a tape from a
  passing live run and promote it.
- Recipe DSL change: update the byte-identical goldens deliberately and run
  conformance.

## Test infrastructure map

| Path | Contents |
|---|---|
| `testsuite/agenttest/` | The agenttest harness (runner, OSB verification, tapes) |
| `testsuite/agenttests/` | The suites and tapes |
| `testsuite/agenttest_fixtures/` | Fixture projects used by cases |
| `testsuite/conformance/` | Conformance suites |
| `testsuite/subjects/` | Per-agent test adapters |
| `testsuite/testhelper/`, `testsuite/testsupport/` | Shared builders and helpers |
| `capability/configcheck/` | Manifest capability-footprint checks |
| `tooling/coverage/` | The coverage gate |
