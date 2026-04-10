# Platform Browser Service Inventory

This document is the phase 1 output for `docs/plans/platform-browser-service-rework.md`.
It maps the browser surface as it exists today so the ownership move into `ayenitd`
does not preserve hidden coupling by accident.

## Scope

The inventory covers the current browser-related code paths in:

- `platform/browser`
- `platform/browser/cdp`
- `platform/browser/bidi`
- `platform/browser/webdriver`
- `app/relurpish/runtime/browser_provider.go`
- `app/relurpish/runtime/browser_sandbox.go`
- `framework/core/agent_spec.go`
- `framework/capability/capability_registry_policy.go`
- `framework/authorization`
- `framework/sandbox`

## Surface Summary

### `platform/browser`

Exported API:

- `Backend`
- `BrowserBackend`
- `Capabilities`
- `CapabilityReporter`
- `StructuredPageData`
- `PageState`
- `Extraction`
- `WaitCondition`
- `WaitConditionType`
- `Error`
- `ErrorCode`

Session wrapper methods:

- `Navigate`
- `Click`
- `Type`
- `GetText`
- `GetAccessibilityTree`
- `GetHTML`
- `ExecuteScript`
- `Screenshot`
- `WaitFor`
- `CurrentURL`
- `CapturePageState`
- `ExtractStructured`
- `ExtractText`
- `ExtractHTML`
- `ExtractAccessibilityTree`
- `Capabilities`
- `Close`

Policy behavior:

- Navigation is filtered through `authorization.PermissionManager.CheckNetwork`.
- Extraction is budgeted through `core.ContextBudget`.
- Backend errors are normalized into `browser.Error` with stable codes.

Wait conditions currently defined:

- `load`
- `network_idle`
- `selector`
- `selector_missing`
- `text`
- `url_contains`

Implemented backend support currently covers:

- `load`
- `selector`
- `selector_missing`
- `text`
- `url_contains`

`network_idle` is defined in the shared type but is not implemented by the current backends.

Normalized error codes:

- `unknown_operation`
- `unsupported_operation`
- `no_such_element`
- `stale_element_reference`
- `element_not_interactable`
- `timeout`
- `navigation_blocked`
- `script_evaluation_failed`
- `backend_disconnected`
- `invalid_url`

## Backend Inventory

### `platform/browser/cdp`

Launch path:

- `New(ctx, Config)` launches Chromium when `WebSocketURL` is empty.
- `launchChromium` resolves a browser executable from `ExecutablePath` or
  `chromium`, `chromium-browser`, `google-chrome`, `google-chrome-stable`.
- A temporary profile directory is created with `os.MkdirTemp("", "relurpify-cdp-*")`.

Transport:

- Chrome DevTools Protocol over websocket.
- The backend resolves the websocket debugger target from `http://127.0.0.1:<port>/json/list`.

Policy enforcement:

- `cfg.Policy.AllowCommand` validates the browser command before launch.

Capabilities reported:

- `AccessibilityTree: true`
- `NetworkIntercept: true`
- `DownloadEvents: false`
- `PopupTracking: false`
- `ArbitraryEval: true`

Wait support:

- `load`
- `selector`
- `selector_missing`
- `text`
- `url_contains`

Error mapping:

- `mapCDPError` normalizes backend, transport, timeout, and selector errors.
- Invalid or missing page targets are surfaced as launch-time failures.

Filesystem paths:

- Temporary browser profile directory from `os.MkdirTemp`.
- Chromium process state is owned by the backend and removed on close.

### `platform/browser/bidi`

Launch path:

- `New(ctx, Config)` launches ChromeDriver when `RemoteURL` is empty.
- The driver is started through `launchChromeDriver`.
- Session startup creates a Chrome profile directory using either
  `fmt.Sprintf("/tmp/relurpify-bidi-%d", ...)` or `os.MkdirTemp("", "relurpify-bidi-*")`.

Transport:

- WebDriver HTTP session creation plus BiDi websocket transport.
- The backend requests `webSocketUrl` from the new session response.

Policy enforcement:

- `cfg.Policy.AllowCommand` validates the driver command before launch.

Capabilities reported:

- `AccessibilityTree: false`
- `NetworkIntercept: false`
- `DownloadEvents: false`
- `PopupTracking: false`
- `ArbitraryEval: true`

Wait support:

- `load`
- `selector`
- `selector_missing`
- `text`
- `url_contains`

Error mapping:

- `mapBiDiError` normalizes protocol and transport errors.
- Invalid sessions and closed transports map to `backend_disconnected`.

Filesystem paths:

- Temporary browser profile directory from `os.MkdirTemp` or `/tmp/relurpify-bidi-*`.
- ChromeDriver process state is owned by the backend and removed on close.

### `platform/browser/webdriver`

Launch path:

- `New(ctx, Config)` launches ChromeDriver when `RemoteURL` is empty.
- The driver is started through `launchChromeDriver`.
- Session startup creates a Chrome profile directory using either
  `fmt.Sprintf("/tmp/relurpify-webdriver-%d", ...)` or `os.MkdirTemp("", "relurpify-webdriver-*")`.

Transport:

- WebDriver HTTP session traffic.

Policy enforcement:

- `cfg.Policy.AllowCommand` validates the driver command before launch.

Capabilities reported:

- `AccessibilityTree: false`
- `NetworkIntercept: false`
- `DownloadEvents: false`
- `PopupTracking: false`
- `ArbitraryEval: true`

Wait support:

- `load`
- `selector`
- `selector_missing`
- `text`
- `url_contains`

Error mapping:

- `mapWebDriverError` normalizes standard WebDriver protocol errors.
- Invalid sessions and closed transports map to `backend_disconnected`.

Filesystem paths:

- Temporary browser profile directory from `os.MkdirTemp` or `/tmp/relurpify-webdriver-*`.
- ChromeDriver process state is owned by the backend and removed on close.

## Runtime Ownership Inventory

### `app/relurpish/runtime/browser_provider.go`

Current responsibilities:

- Registers the `browser` provider when `AgentBrowserSpec.Enabled` is true.
- Adds capability exposure policy for `browser`.
- Registers the invocable `tool:browser` capability.
- Resolves backend selection from action args and agent spec.
- Applies action policy from `AgentBrowserSpec.Actions`.
- Requests HITL approval through `PermissionManager.RequireApproval`.
- Tracks browser session handles in a provider-local map.
- Performs session recovery after `backend_disconnected`.
- Records page snapshots, telemetry, and provider/session snapshots.

Current action surface:

- `open`
- `navigate`
- `click`
- `type`
- `get_text`
- `extract`
- `get_html`
- `get_accessibility_tree`
- `execute_js`
- `screenshot`
- `wait`
- `current_url`
- `close`

Current action-state helpers:

- `browser.default_session`
- `browser.last_page_state`
- `browser.page_states`

Current policy boundary:

- Capability exposure is handled by the registry.
- Browser action policy is handled in the tool handler.
- Network checks are handled inside `platform/browser.Session`.
- Command authorization for backend launch is routed through `framework/authorization`.

### `app/relurpish/runtime/browser_sandbox.go`

Current responsibilities:

- Builds sandbox launch and cleanup commands for browser containers.
- Validates `docker` or equivalent runtime commands through command policy.
- Launches either Chromium or ChromeDriver in a containerized runtime.
- Removes the container on cleanup.

Current filesystem/path usage:

- Container launch uses `--user-data-dir=/tmp/relurpify-browser` for the CDP path.
- The browser container itself is controlled through runtime command execution only.

### `framework/core/agent_spec.go`

Current browser spec surface:

- `Enabled`
- `DefaultBackend`
- `AllowedBackends`
- `Actions`
- `Extraction`
- `Downloads`
- `Credentials`

Current validation drift:

- The validator still accepts browser action names that are not implemented by
  the current runtime, including tab management and download-related actions.
- The extraction and credentials sub-sections are validated but are not consumed
  by the current runtime path.

### `framework/capability/capability_registry_policy.go`

Current browser-related registry hook:

- `effectiveExposurePolicies` adds a callable browser exposure policy when the
  agent browser spec is enabled.

This means browser visibility is already partly registry-driven, even though
the provider implementation still lives in `app/relurpish/runtime`.

### `framework/authorization`

Current browser-related checks:

- `PermissionManager.CheckNetwork` is used by browser navigation.
- `NewCommandAuthorizationPolicy` is used for browser launch and cleanup commands.
- `RequireApproval` is used for browser actions marked `ask`.

### `framework/sandbox`

Current browser-related checks:

- `CommandPolicy` is the enforcement interface used for browser launches.
- `CommandRequest` carries browser launch and cleanup commands into policy.

File-scope enforcement is not yet attached to browser-owned directories in the
current runtime path.

## Gaps Observed During Inventory

- The current `platform/browser` package comment is outdated and only describes
  WebDriver even though CDP and BiDi backends exist.
- Browser ownership still sits in the runtime layer instead of a workspace
  service.
- `AgentBrowserSpec` exposes unsupported actions that the runtime does not
  execute.
- File-scope policy is not yet integrated with browser profile or download
  directories.
- `WaitForNetworkIdle` exists in the shared type but has no backend support.

## Phase 1 Exit State

The current browser surface is now explicitly mapped by package, backend,
policy hook, and runtime ownership path. The next phases can move ownership out
of `app/relurpish/runtime` with the current behavior documented instead of
rediscovered.
