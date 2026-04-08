# Codesmells: `named/rex/nexus`

## Observations

- `runtime_endpoint.go` mixes import rehydration, workflow persistence, attempt scheduling, and projection caching in a single adapter surface. The behavior is coherent, but the file is doing a lot of orchestration work for one type.
- `lineage_bridge.go` has a similar shape: it translates framework events, mutates lineage bindings, persists workflow artifacts, and updates ownership state. The branching is testable, but the code is hard to scan because it spans both policy translation and durable storage operations.
- The package also exposes a number of nil/empty-path adapters and helper shims so the runtime boundary can fail safely. That is pragmatic, but it creates more small branches than the package would have if the responsibilities were split more aggressively.
- `snapshot_store.go` is a read-path aggregator that decodes artifacts, events, and workflow data in one pass. It is practical, but the helper chain makes it easy for retrieval semantics to diverge over time.

## Tradeoff Note

- The broad adapter/bridge shape appears intentional because rex is the integration boundary between durable workflow state, FMP, and Nexus-managed execution. The primary risk here is maintainability and path duplication, not that the package has the wrong ownership model.
