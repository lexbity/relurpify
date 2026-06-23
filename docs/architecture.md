# Architecture

This document describes the current code-level structure of Relurpify. It is
based on the package layout, `Makefile` gates, and the architecture checks in
`tooling/arch/`.

## Domain map

The repository is organized around top-level Go domains:

- `app` for user-facing binaries and the TUI runtime shell
- `ayenitd` for the workspace service and related long-running helpers
- `capability` for descriptors, handlers, registries, and sandboxing
- `cognitionzoo` for agent paradigms and their supporting execution models
- `context` for working memory, persistence, knowledge, and context streams
- `execution` for agent execution, compilation, sessions, and lifecycle flow
- `governance` for identity, authorization, policy, and permissions
- `jobs` for deferred work and durable job storage
- `model` for runtime vocabulary types
- `named` for named agents, including Euclo
- `platform` for adapters and host integrations
- `testsuite` for conformance and agent-test fixtures
- `tooling` for architecture gates and repo checks
- `userconfig` for the configuration load boundary

`testsuite` and `tooling` are treated as unrestricted by the domain DAG. The
other domains participate in direction checks.

## Domain DAG

The allowed import directions are enforced by `tooling/arch/domaindag.go` and
exposed through `make domain-check`, `make domain-check-enforce`, and
`make lint-arch`.

Current shape:

- higher-level entrypoints may import deeper runtime domains
- `app` and `named` may import the widest set of runtime domains
- `cognitionzoo` and `ayenitd` may import runtime and support domains, but not
  the reverse
- `execution`, `context`, `capability`, `governance`, `model`, `jobs`,
  `platform`, and `userconfig` are ordered so that lower layers do not import
  back into higher layers
- `context/ports` is a leaf and is enforced to stay stdlib-only

The DAG is not just a comment convention. It is checked by code in
`tooling/arch/` against the real import graph.

## Gate surface

The main architecture gates are:

- `domain-check` for direction violations in warn mode
- `domain-check-enforce` for direction violations in enforce mode
- `no-bucket` for packages imported by too many domains
- `governance-no-orch` for governance packages importing `execution`
- `check-contract-dissolution` for removed manifest-spine symbols
- `grep-architecture-gates` for legacy tool-calling symbols, env access, and
  compatibility language
- `check-gates-slice10` for selected dead-code and provider assertions

`no-bucket` treats a package as a bucket when it is imported by multiple
domains and exports only types. Pure vocabulary packages owned by a single
domain are exempt when they live under `<domain>/` or `<domain>/classification`.

`governance-no-orch` is stricter: governance packages may depend on execution
ports, but not on execution implementation packages.

`context/ports` is intentionally a leaf package. The check is explicit because
it is easy to accidentally reintroduce cycles through a new field type.

## Request and control flow

The runtime path for the main TUI is:

1. `app/relurpish/main.go` snapshots the process environment via
   `userconfig/config`.
2. `userconfig/config` loads workspace config, provider/profile manifests,
   security policies, tool manifests, and env-only secrets.
3. `app/relurpish/runtime` normalizes paths, resolves the model/provider
   catalog, and probes local readiness.
4. `app/relurpish/tui` and `app/relurpish/euclotui` render the interactive UI.
5. `execution` and `named/euclo` coordinate agent execution, compilation, and
   control flow.
6. `capability` and `platform` provide the host-facing adapters, sandboxing,
   and external integrations.

The key boundary is that config loading happens once at startup. Runtime code
consumes resolved state after that boundary instead of reading env directly.

## Sandbox and capability boundary

The sandbox boundary is owned by `capability` and the runtime code that wires
it. The design intent is:

- model-driven work should reach the host only through capability invocations
- host execution is mediated by sandbox and policy checks
- `context/ports` and similar leaf packages stay free of internal imports

The exact sandbox implementation differs by backend, but the architectural
rule is stable: execution flows through capability and sandbox boundaries
before touching host resources.

## Persistence and state

The current codebase uses a mixed persistence model:

- `jobs/store/badger.go` stores jobs in Badger
- `context/knowledge/graphdb/` uses Badger-backed graph persistence
- `context/persistence/artifactstore/` stores large artifacts on disk under
  `.relurpify_state/`

This is not a single-store architecture. Docs should say which subsystem owns
which state instead of implying a universal backing engine.

## Unknown contract vocabulary

The `Makefile` and architecture checks refer to slice IDs and spec terms such
as `AC-*`, `NFR-*`, `FR-*`, `GP-*`, and `§2.1`. Those identifiers are visible
in the repo, but their full prose definitions are not shipped here.

For documentation purposes:

- treat the vocabulary as an external contract
- cite the ID when describing a gate or comment
- do not invent the missing definitions

## Notes on `cognitionzoo`

`cognitionzoo` is the main paradigm family for the agent runtime. The top-level
package summary is intentionally short, because the useful detail lives in the
subpackages such as `react`, `rewoo`, `htn`, `plan`, `pipeline`, and
`blackboard`.
