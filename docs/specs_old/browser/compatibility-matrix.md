# Browser Compatibility Matrix

## Status

Current implementation status as of Phase 6 hardening.

## Backends

| Backend | Local Driver Path | Real Localhost Integration | Wait Model | Notes |
|---|---|---:|---|---|
| CDP | Chromium remote debugging websocket | Yes | Polling + document readiness | Primary Chromium-native path |
| WebDriver Classic | ChromeDriver HTTP | Yes | Polling | Accessibility tree is synthetic |
| WebDriver BiDi | ChromeDriver WebSocket URL capability | Yes | Event-driven load + polling for selector/text/url | Uses `webSocketUrl: true` session capability |

## Current Contract Coverage

| Operation | CDP | Classic | BiDi | Notes |
|---|---:|---:|---:|---|
| `Navigate` | Yes | Yes | Yes | |
| `Click` | Yes | Yes | Yes | |
| `Type` | Yes | Yes | Yes | |
| `GetText` | Yes | Yes | Yes | |
| `GetHTML` | Yes | Yes | Yes | |
| `ExecuteScript` | Yes | Yes | Yes | |
| `Screenshot` | Yes | Yes | Yes | |
| `WaitFor(load)` | Yes | Yes | Yes | BiDi is event-driven |
| `WaitFor(selector)` | Yes | Yes | Yes | |
| `CurrentURL` | Yes | Yes | Yes | |
| `GetAccessibilityTree` | Yes | Synthetic | Synthetic | Classic and BiDi currently provide structured fallback JSON rather than native browser AX data |

## CI Expectations

The browser stack is release-gated by:

- `go test ./tools/browser/... ./app/relurpish/runtime`
- `./scripts/browser-ci.sh`
- optional repeated stress runs with `RELURPIFY_BROWSER_STRESS=1`

## Known Gaps

- No WPT subset runner has been wired yet for Classic WebDriver or BiDi.
- Browser-specific manifest ergonomics are still minimal; domain permissions still come from normal agent manifests and HITL flows.
- Classic WebDriver and BiDi accessibility extraction are fallbacks, not native accessibility-tree implementations.
