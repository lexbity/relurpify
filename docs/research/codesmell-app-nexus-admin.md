# Codesmells: `app/nexus/admin`

Captured while adding phase-4 coverage around the Nexus admin MCP boundary and helper layers.

## Observations

- `mcp_helpers.go` concentrates scope normalization, argument coercion, request shaping, JSON decoding, and error translation in one file. The code is small enough to follow locally, but the responsibility mix makes it easy to miss a branch or to introduce a security regression in a seemingly harmless helper.
- `mcp_handlers.go` is a long handler catalog with a repeated version gate and service call pattern. The execution path duplication is deliberate, but it creates a maintenance burden because every tool follows the same structure and the per-tool differences are easy to overlook.
- The admin MCP surface also mixes authorization rules with transport-specific resource paths. That is pragmatic for the integration boundary, but it increases the test surface because policy bugs and parsing bugs can hide behind the same entry points.
- The scope alias handling is intentionally centralized so gateway/admin/nexus naming stays consistent, but that also means a small normalization mistake can overexpose tools. The phase-4 work here exposed that risk directly, which is a sign the area benefits from very explicit tests.

## Tradeoff Note

- The broad shape of this package appears intentional because it is the Nexus admin integration boundary rather than a narrow domain service. The main risk is maintainability and authorization correctness, not package ownership.
