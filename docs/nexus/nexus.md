# nexus

`nexus` is the gateway server in `app/nexus`. It coordinates remote nodes,
routes capabilities, and exposes the administrative surface consumed by
`nexusish`.

> **Status:** Nexus and the Federated Mesh Protocol have been implemented but
> have not yet been fully end-to-end tested. Treat the current implementation as
> functional but not production-validated.

## Responsibilities

- node pairing and identity management
- capability routing between connected nodes
- event aggregation and observability
- admin API surface

## Start

```bash
nexus --config relurpify_cfg/nexus.yaml
```
