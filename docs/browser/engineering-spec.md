# Relurpify Browser Automation Engineering Specification

## Status

Draft

## Purpose

This document defines the proposed browser automation subsystem for Relurpify, including:

- the backend abstraction for CDP, WebDriver Classic, and WebDriver BiDi
- the Relurpify session and tool model
- permission and HITL integration
- the phased implementation and validation plan
- the required unit, integration, and conformance test suites

This feature is intended to support high-value end-user workflows such as:

- web research
- localhost web app testing
- browser automation
- structured page extraction

## Goals

- Provide a single Relurpify-facing browser API regardless of transport.
- Support Chromium/CDP first without baking CDP assumptions into the public interface.
- Support WebDriver Classic and WebDriver BiDi with the same high-level tool surface.
- Enforce manifest network permissions and existing HITL approval flows on navigation.
- Keep extracted browser content token-budget-aware.
- Make unsupported features explicit and testable per backend.

## Non-Goals

- Full parity with every vendor-specific browser feature in v1.
- Recording/replaying arbitrary user sessions in v1.
- Full network interception support in v1.
- Cross-browser conformance beyond the explicitly declared support matrix.

## Proposed Runtime Model

### Core Abstractions

```go
type BrowserBackend interface {
    Navigate(ctx context.Context, url string) error
    Click(ctx context.Context, selector string) error
    Type(ctx context.Context, selector, text string) error
    GetText(ctx context.Context, selector string) (string, error)
    GetAccessibilityTree(ctx context.Context) (string, error)
    GetHTML(ctx context.Context) (string, error)
    ExecuteScript(ctx context.Context, script string) (any, error)
    Screenshot(ctx context.Context) ([]byte, error)
    WaitFor(ctx context.Context, condition WaitCondition, timeout time.Duration) error
    CurrentURL(ctx context.Context) (string, error)
    Close() error
}
```

The transport-specific backend must remain focused on browser automation mechanics only.

The Relurpify-owned `BrowserSession` wrapper is responsible for:

- permission checks before navigation
- HITL escalation for new domains
- output truncation and extraction
- audit and telemetry metadata
- session-scoped lifecycle management
- mapping backend-specific failures to Relurpify errors

### Session Management

Introduce a `BrowserToolProvider` or equivalent runtime provider that:

- owns one or more named browser sessions
- creates sessions during a run on demand
- stores session handles in the runtime object registry
- cleans them up at task end and runtime shutdown

### Tool Surface

Use a single model-facing `browser` tool with action dispatch in v1.

Example actions:

- `open`
- `navigate`
- `click`
- `type`
- `get_text`
- `get_html`
- `get_accessibility_tree`
- `execute_js`
- `screenshot`
- `wait`
- `close`
- `extract`

This keeps model reasoning simpler and avoids a large tool list.

### Skills

Planned skills:

- `web-research`
- `web-testing`
- `web-automation`
- `web-scraping`

Each skill should declare:

- allowed domains or host patterns
- tool execution policy
- whether HITL is required for undeclared domains
- extraction defaults suitable for the use case

## Backend Support Matrix

### CDP Backend

Primary target:

- Chromium-family browsers

Expected v1 coverage:

- navigation
- DOM extraction
- text extraction
- JS execution
- screenshot
- basic waiting
- accessibility tree

Likely implementation domains:

- `Page`
- `Runtime`
- `DOM`
- `Accessibility`
- input dispatch APIs

### WebDriver Classic Backend

Primary target:

- standards-based remote control via HTTP

Expected v1 coverage:

- session creation
- navigation
- current URL
- element finding
- click
- send keys
- get text
- page source
- execute script
- screenshot
- timeouts

### WebDriver BiDi Backend

Primary target:

- event-capable standards-based automation

Expected v1 coverage:

- session subscribe/unsubscribe
- navigation
- node lookup
- script evaluation
- actions
- screenshot
- event-driven wait support

BiDi should be treated as its own backend, not as an implementation detail of Classic WebDriver.

## Error Model

All backends must normalize into Relurpify-level errors for:

- no such element
- stale element reference
- element not interactable
- timeout
- navigation blocked by permission policy
- script evaluation failure
- backend disconnected
- unsupported operation

Errors should carry:

- backend name
- operation
- normalized code
- raw protocol details when available

## Permission and HITL Integration

### Navigation

Before `Navigate`, `BrowserSession` must:

1. parse the destination URL
2. derive host, scheme, and effective port
3. call the existing `PermissionManager.CheckNetwork`
4. trigger HITL through the normal permission path if required

### Backend Transport Permissions

Transport setup may require separate permissions from target-page navigation.

Examples:

- CDP over localhost websocket
- local WebDriver server over localhost HTTP
- remote Selenium/Grid endpoint

These permissions must be declared explicitly by the agent or skill manifest.

### Extraction Budget

Raw page content must not be inserted into context without budgeting.

At minimum:

- `get_html` must support truncation
- `get_accessibility_tree` must support truncation
- `extract` should return structured JSON
- large outputs should include metadata showing truncation

## Required Runtime Changes

The following codebase changes are required before browser backends can be added cleanly:

1. Allow runtime/skill providers to register long-lived services and tools.
2. Extend scoped object cleanup so stored resources implementing `Close() error` are closed.
3. Expand runtime shutdown to close registered providers and active browser sessions.
4. Permit network-only permission manifests for browser-only skills and agents.
5. Add explicit backend capability reporting so unsupported operations fail predictably.

## Test Strategy

Validation is split into four layers:

- shared contract tests
- backend adapter unit tests
- real-browser integration tests
- standards conformance tests

### 1. Shared Contract Tests

These tests run against every backend implementation through the same suite.

They validate:

- `Navigate` updates URL and respects context cancellation
- `Click`, `Type`, `GetText`, `GetHTML`, `ExecuteScript`, `Screenshot`, and `WaitFor` have consistent semantics
- `Close` is idempotent
- unsupported features fail with normalized `unsupported_operation`
- multiple sessions remain isolated
- permission-gated navigation is enforced by `BrowserSession`
- extraction output is token-budgeted and deterministically truncated

Recommended package layout:

- `tools/browser/contract_test.go`
- `tools/browser/session_test.go`

### 2. Backend Adapter Unit Tests

These tests mock protocol transport and validate request/response mapping.

#### CDP Adapter Unit Tests

Required coverage:

- command serialization for navigation, evaluate, HTML fetch, AX tree, and screenshot
- target/session attachment handling
- event correlation for load/wait conditions
- protocol error mapping
- disconnect handling
- timeout and context cancellation propagation

Recommended package layout:

- `tools/browser/cdp/backend_test.go`
- `tools/browser/cdp/transport_test.go`

#### WebDriver Classic Adapter Unit Tests

Required coverage:

- session create/delete
- URL navigation and current URL retrieval
- element location and element ID handling
- click and send keys mapping
- text retrieval and page source retrieval
- execute script mapping
- screenshot mapping
- timeout configuration
- HTTP error body mapping into normalized Relurpify errors

Recommended package layout:

- `tools/browser/webdriver/backend_test.go`
- `tools/browser/webdriver/http_client_test.go`

#### WebDriver BiDi Adapter Unit Tests

Required coverage:

- websocket message serialization
- request ID correlation
- subscribe/unsubscribe handling
- event receipt and fanout
- navigate, locate nodes, evaluate, perform actions, screenshot
- event-driven wait conditions
- disconnect handling
- protocol error mapping

Recommended package layout:

- `tools/browser/bidi/backend_test.go`
- `tools/browser/bidi/transport_test.go`

### 3. Real Browser Integration Tests

These tests run against deterministic localhost fixtures.

Required test pages:

- static page
- delayed render page
- SPA route change page
- redirect page
- form controls page
- iframe page
- shadow DOM page
- accessibility fixture page
- script error page

Required scenarios:

- initial navigation
- redirect handling
- text extraction from visible elements
- click on links/buttons/form controls
- typing into input, textarea, and contenteditable
- waiting for appearance, disappearance, navigation completion, and text change
- script evaluation returning primitives and objects
- screenshot returns valid PNG bytes
- HTML extraction returns stable and documented scope
- accessibility tree extraction returns expected roles/names
- stale element and detached frame behavior is normalized

Recommended package layout:

- `tools/browser/integration/fixtures/...`
- `tools/browser/integration/contract_integration_test.go`

### 4. Standards / Conformance Tests

#### WebDriver Classic

Run an approved subset of Web Platform Tests for:

- session
- navigation
- elements
- interaction
- execute script
- screenshot
- timeouts

#### WebDriver BiDi

Run an approved subset of WPT BiDi tests for:

- session
- browsingContext
- script
- input
- screenshot
- event subscription

#### CDP

CDP has no W3C conformance suite.

Required substitute:

- version compatibility matrix across at least two Chromium versions
- integration tests against a real browser
- transport unit tests for every mapped CDP domain used by Relurpify

## Phased Delivery Plan

### Phase 0: Architecture and Runtime Prep

Deliverables:

- browser package skeleton
- runtime provider hook design
- closable scoped object cleanup
- network-only permission validation fix
- backend capability model

Tests required:

- unit tests for scoped object cleanup
- unit tests for provider registration and runtime shutdown
- unit tests for network-only manifest acceptance

Exit criteria:

- runtime can own and clean up long-lived browser providers
- browser-only skill manifests validate successfully

### Phase 1: Shared Contract Suite and Session Layer

Deliverables:

- `BrowserBackend` interface
- `BrowserSession`
- normalized error model
- shared contract tests with fake backend

Tests required:

- contract tests against fake backend
- session permission enforcement tests
- session extraction budget tests

Exit criteria:

- every future backend can be validated against a common suite

### Phase 2: CDP Backend MVP

Deliverables:

- CDP transport
- Chromium-backed implementation for navigation, click, type, text, HTML, JS, screenshot, AX tree, wait, close
- localhost integration harness

Tests required:

- CDP mocked-transport unit tests
- Chromium integration suite on localhost fixtures
- cancellation and disconnect tests

Exit criteria:

- CDP backend passes shared contract suite
- CDP backend passes localhost integration suite

### Phase 3: Browser Tool Provider and Skill Wiring

Deliverables:

- model-facing `browser` tool
- provider-managed named sessions
- session handle storage in context registry
- `web-testing` skill for localhost
- `web-research` skill for declared domains

Tests required:

- tool-provider lifecycle tests
- session isolation tests
- permission and HITL navigation tests
- skill manifest and effective policy tests

Exit criteria:

- agents can create, use, and close browser sessions within one run
- policy enforcement works through the normal manifest and HITL stack

### Phase 4: WebDriver Classic Backend

Deliverables:

- HTTP transport
- Classic WebDriver backend
- support matrix documenting v1 behavior

Tests required:

- mocked HTTP adapter tests
- localhost integration tests against supported Classic drivers
- WPT subset for Classic WebDriver

Exit criteria:

- backend passes shared contract suite
- backend passes selected WPT subset

### Phase 5: WebDriver BiDi Backend

Deliverables:

- websocket transport
- BiDi backend
- event-driven waits and subscriptions

Tests required:

- mocked websocket adapter tests
- localhost integration tests against supported BiDi browsers
- WPT subset for BiDi

Exit criteria:

- backend passes shared contract suite
- backend passes selected BiDi WPT subset

### Phase 6: Hardening and CI Expansion

Deliverables:

- multi-browser CI jobs
- compatibility matrix
- flaky test quarantine policy
- release gating for browser support

Tests required:

- repeated stress runs for flaky wait/navigation cases
- parallel session tests
- resource leak tests
- browser process cleanup tests

Exit criteria:

- browser support is release-gated by automated validation

## CI Recommendations

Use separate jobs:

- `browser-unit`
- `browser-contract`
- `browser-integration-cdp`
- `browser-integration-webdriver`
- `browser-integration-bidi`
- `browser-conformance-webdriver`
- `browser-conformance-bidi`

Suggested policy:

- unit and contract tests run on every PR
- CDP localhost integration runs on every PR if browser dependencies are present
- WebDriver and BiDi integration/conformance may run on main branch and release branches first, then move to PR gating once stabilized

## Initial Support Recommendation

Recommended order:

1. runtime preparation
2. shared contract suite
3. CDP backend
4. provider and skill integration
5. WebDriver Classic backend
6. WebDriver BiDi backend
7. full CI hardening

This order minimizes architecture churn and gives the project a usable browser stack early while preserving a path to standards-based backends.

## References

- W3C WebDriver: https://www.w3.org/TR/webdriver1/
- W3C WebDriver BiDi: https://www.w3.org/TR/webdriver-bidi/
- Web Platform Tests wdspec: https://web-platform-tests.org/writing-tests/wdspec.html
- Web Platform Tests WebDriver tree: https://wpt.live/webdriver/tests/
- Chrome DevTools Protocol: https://chromedevtools.github.io/devtools-protocol/
