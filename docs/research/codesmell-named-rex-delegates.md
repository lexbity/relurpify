# Codesmell: `named/rex/delegates`

Observed while expanding test coverage in the delegate registry.

## Notes

- `NewRegistry` centralizes many concrete agent constructors in a single map literal.
- `Resolve` is simple, but the registry shape encourages route-to-agent coupling through family names instead of narrower capability contracts.
- The path helpers for react, architect, and pipeline are straightforward, but they are embedded in the same file as registry resolution and pass-through adapter logic.

## Risk

- Maintainability risk: the registry grows as more families are added, and the file becomes the coordination point for unrelated agent wiring.
- Testability risk: the registry is easy to instantiate, but concrete constructor wiring can obscure whether a failure came from routing logic or from an agent-specific dependency chain.

## Tradeoff

- The central registry is reasonable here because rex needs a single family-to-executor bridge, even if that means the adapter layer is somewhat broad.

