# Codesmells: `app/nexus/server`

Captured while adding phase-4 coverage around the Nexus server integration boundary.

## Observations

- `app.go` is a broad orchestration layer that wires transport, federation, runtime, admin, and scanner concerns together. The breadth is intentional, but the file is hard to reason about because a single change can affect several startup and wiring paths.
- `node_runtime.go` mixes websocket framing, node registration, capability dispatch, and federation advertisement logic. The code is structured as helpers, but the execution paths are still tightly coupled and easy to regress when transport or capability behavior changes.
- `rex_runtime.go` combines runtime projection, capability invocation, SLO signal collection, and trust-bundle publication. The responsibilities are coherent for a runtime adapter, yet the file has enough surface area that nil/empty-path handling needs explicit tests to avoid accidental breakage.
- `federation.go` is a small wrapper over the FMP service, but its nil-safe adapter shape means there are multiple fallback branches to keep consistent with the underlying service behavior.

## Tradeoff Note

- The server package is an integration boundary by design. The main risk here is maintainability and branching complexity, not that the package should be split into smaller ownership units right away.
