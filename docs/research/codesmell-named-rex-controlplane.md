# Codesmells: `named/rex/controlplane`

## Observations

- `controlplane.go` combines fairness admission, load shedding, operator authorization, SLO collection, and DR metadata shaping in a single package. Each surface is testable, but the package is broader than a narrowly scoped control-plane shim.
- `CollectSLOSignals` mixes workflow-state counts with run-mode inspection, which is useful for current reporting but easy to drift if the workflow model evolves.

## Tradeoff Note

- This breadth looks intentional because rex control-plane logic needs to coordinate admission, audit, and reporting from one place. The risk is more about maintainability and duplicated policy reasoning than about the package being mis-specified for its role.
