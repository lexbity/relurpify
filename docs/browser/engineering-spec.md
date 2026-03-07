# Browser Security Engineering Specification

## Status

Draft

## Purpose

This document defines the engineering design for Relurpify's browser security model and the v1 browser tooling architecture.

It replaces the narrower transport-focused framing in `docs/browser/v1_driver_spec.md` with a framework-first model centered on:

- manifest-defined capabilities
- runtime permission enforcement
- HITL escalation
- gVisor-based browser process management
- context-budgeted page extraction
- auditable browser activity

## Scope

This specification covers:

- the model-facing browser tool surface
- browser session lifecycle and cleanup
- security enforcement points for CDP, WebDriver Classic, and WebDriver BiDi
- extraction and page-orientation behavior
- download handling
- error taxonomy
- required framework and runtime changes
- required test layers

This specification does not require full protocol parity across all backends in v1.

## Design Principles

- Browser automation must obey the same manifest and runtime contract as shell, file, and network tools.
- Browser execution must remain inside the current gVisor execution model; browser support must extend that model, not bypass it with host-launched side processes.
- Security enforcement must happen below the model-facing tool wrapper for protocol actions that can be triggered indirectly by the page.
- The browser subsystem must use the existing permission manager, HITL broker, audit logger, telemetry pipeline, and context budget system rather than inventing parallel policy paths.
- Browser context inserted into the model must be structured and token-bounded by default.
- Arbitrary script execution in the page context is a privileged action and must be separated from ordinary DOM interaction.

## Current State Summary

The repository already provides:

- a transport-agnostic browser backend interface in [`tools/browser/backend.go`](/home/lex/Public/Relurpify/tools/browser/backend.go)
- a `Session` wrapper with navigation permission checks and extraction budgeting in [`tools/browser/session.go`](/home/lex/Public/Relurpify/tools/browser/session.go)
- normalized browser errors in [`tools/browser/errors.go`](/home/lex/Public/Relurpify/tools/browser/errors.go)
- concrete `cdp`, `webdriver`, and `bidi` backends
- a runtime `PermissionManager` with network, HITL, and audit integration in [`framework/runtime/permissions.go`](/home/lex/Public/Relurpify/framework/runtime/permissions.go)
- context budgeting and context item primitives in [`framework/core/context_budget.go`](/home/lex/Public/Relurpify/framework/core/context_budget.go) and [`framework/core/context_item.go`](/home/lex/Public/Relurpify/framework/core/context_item.go)

The repository does not yet provide:

- a model-facing `browser` tool implementation
- protocol-level enforcement for redirect-driven or page-initiated network activity
- gVisor-managed browser process launch
- a concrete browser observation context item
- download management
- browser-specific manifest schema beyond generic tool policy and network scopes

## Threat Model

The browser subsystem must defend against:

- agent-initiated navigation to undeclared domains
- page-initiated redirects and subresource loads to undeclared domains
- arbitrary page-context JavaScript used for exfiltration or privilege escalation
- browser subprocesses launched outside the current Relurpify gVisor model
- persistent profile reuse across sessions
- prompt injection via hostile page content
- silent downloads to undeclared paths
- leaked browser processes and stale sessions after failure or HITL pauses

## Architecture

### Layers

The browser subsystem is composed of five layers:

1. Manifest and skill policy
2. Tool registry and model-facing browser tool
3. Browser session and supervisor
4. Protocol backend
5. gVisor/runtime enforcement

### Responsibilities

#### Manifest and skill policy

Defines:

- whether the browser tool is exposed
- which browser actions are allowed, denied, or require HITL
- which hosts are reachable
- whether downloads are allowed
- where downloaded files may be written
- whether credential entry requires approval

#### Browser tool

Exposes a single `browser` tool in v1 using action dispatch.

The tool is responsible for:

- action validation
- acquiring or creating the relevant session
- invoking the session
- packaging normalized results for the agent

The tool is not responsible for making final security decisions about network or protocol activity that can occur below the action layer.

#### Browser session

The session is the policy-aware wrapper over a backend. It is responsible for:

- action-level permission checks
- structured extraction defaults
- budget-aware truncation
- page state snapshot creation
- retry policy for stale element errors
- reconnect and relaunch behavior after disconnects
- tab tracking

#### Browser supervisor

The supervisor owns:

- backend construction
- browser process launch
- temporary profile creation
- download directory setup
- cleanup on cancellation and shutdown
- reconnect and relaunch after long HITL pauses

#### Protocol backend

The backend is transport-specific and is responsible for:

- issuing protocol commands
- receiving protocol events
- mapping transport errors to normalized browser errors
- exposing protocol hook points for navigation, network, downloads, tabs, and evaluation

The backend must not directly bypass framework launch, gVisor enforcement, or policy infrastructure.

## Model-Facing Tool Surface

v1 exposes one tool:

- `browser`

Required actions:

- `open`
- `navigate`
- `click`
- `type`
- `wait`
- `extract`
- `get_text`
- `get_accessibility_tree`
- `get_html`
- `current_url`
- `screenshot`
- `execute_js`
- `close`

Optional v1.1 actions:

- `list_tabs`
- `switch_tab`
- `wait_for_download`
- `download_status`

The single-tool surface is preferred for smaller models and keeps skill manifests simple. Discrete tools may be added later as an alternate registration surface backed by the same implementation.

## Manifest and Capability Model

### Existing permissions to reuse

The browser system must continue using:

- `spec.permissions.network`
- `spec.permissions.filesystem`
- `spec.agent.tool_execution_policy`
- global tag policy and HITL policy through the existing registry

The browser subsystem must also respect the existing manifest requirement that agent runtime execution is `gvisor`. Browser support does not introduce an alternate runtime.

### New browser-specific agent spec

Add a browser section under `spec.agent`:

```yaml
agent:
  browser:
    enabled: true
    default_backend: cdp
    allowed_backends: [cdp, bidi]
    actions:
      navigate: allow
      click: allow
      type: allow
      extract: allow
      get_html: ask
      execute_js: ask
      download: ask
      new_tab: allow
      fill_credentials: ask
    extraction:
      default_mode: accessibility_plus_structured
      max_html_tokens: 4000
      max_snapshot_tokens: 1200
    downloads:
      enabled: true
      directory: ${workspace}/.relurpify/downloads
    credentials:
      require_hitl: true
```

### Semantics

- `permissions.network` remains the host allowlist for browser egress.
- `agent.browser.actions` controls which browser actions the agent may perform.
- `execute_js` governs arbitrary agent-supplied page-context script execution only.
- backend-internal scripting required to implement standard actions is not controlled by `execute_js`.
- `downloads.directory` must also be covered by filesystem write permission.
- `fill_credentials` always routes through HITL when `credentials.require_hitl` is true.

## Enforcement Model

### Overview

Security enforcement must happen at three levels:

1. Tool-action gating
2. Protocol-command and event gating
3. Runtime/browser network enforcement

All three are required.

### 1. Tool-action gating

Before a browser action runs, the browser tool or session must:

- verify the tool is exposed by the registry
- apply per-tool execution policy
- apply browser-action policy
- request HITL approval when action policy is `ask`

### 2. Protocol-command and event gating

Backends must expose hook points for the following events and commands.

#### Navigation interception

Every explicit navigation command must be checked before dispatch:

- CDP: `Page.navigate`
- WebDriver Classic: `POST /session/{id}/url`
- BiDi: `browsingContext.navigate`

In addition, page-driven navigation must be detected after dispatch via protocol events and current URL changes. When a redirect or new top-level navigation lands on an undeclared host, the session must:

- block further agent actions
- mark the session as policy-blocked
- emit audit and telemetry events
- surface a normalized `navigation_blocked` error

CDP should additionally use network interception and target events to stop unauthorized requests before navigation fully commits when possible.

#### JavaScript execution gating

Arbitrary agent-supplied page-context script execution must be gated:

- CDP: `Runtime.evaluate`, `Runtime.callFunctionOn`
- WebDriver Classic: `/execute/sync`, `/execute/async`
- BiDi: `script.evaluate`, `script.callFunction`

This gating applies only to scripts originating from agent input. Backend-internal DOM helper scripts used for `click`, `type`, `wait`, `get_text`, and similar operations are classified separately as trusted backend actions.

#### Network event tap

Outbound requests initiated by the page must be observable for:

- telemetry
- audit logging
- allowlist enforcement
- download detection

CDP is the primary enforcement backend for this requirement because it can intercept and block subresource requests.

BiDi should capture request events when available.

Classic WebDriver cannot be considered fully enforcing for page-initiated subresource requests and is therefore compatibility-only for strict security mode.

#### New tab and popup handling

The backend must surface:

- popup creation
- target creation
- browsing context creation

The session must:

- track active tab
- default to following the newly opened tab when policy allows
- gate `new_tab` behavior through browser action policy

#### Download initiation

The backend must surface:

- download begin
- progress
- completion
- final file path

Downloads must be blocked unless both conditions hold:

- browser download action is allowed by browser action policy
- target path is allowed by filesystem write permissions

### 3. gVisor and browser network enforcement

Manifest allowlists must be enforced beyond the explicit `navigate` action, while preserving the current gVisor execution path used elsewhere in the framework.

Required behavior:

- browser launch must occur through the existing gVisor-backed execution path, using the same runtime wiring as other sandboxed execution
- browser profile and download directories must live within the gVisor workspace boundary
- browser egress must be restricted to declared hosts
- gVisor policy updates derived from the permission manager must be applied before browser launch and updated as approvals expand network scope
- if fine-grained gVisor host enforcement is unavailable, CDP request interception must be treated as the minimum secure implementation, but this is a temporary gap rather than a replacement for gVisor containment

The current `SandboxRuntime` stores network rules but does not yet enforce per-host egress. This gap must be closed while keeping browser execution on gVisor; browser support must not fall back to unconstrained host-side execution as a workaround.

### gVisor execution requirements

The browser subsystem must satisfy all of the following:

- Chromium, ChromeDriver, or equivalent helper processes must be launched through the existing `CommandRunner` and `SandboxRuntime` path
- the browser must run inside the same gVisor-backed container model used for other agent execution
- temporary profile directories and download paths must be mounted inside the workspace-visible sandbox boundary
- browser helper binaries must be covered by manifest executable permissions
- localhost transport endpoints exposed by the browser or driver must exist inside the same sandboxed execution topology, not as out-of-band host services

If a backend cannot satisfy these constraints, it is not eligible for secure-mode execution.

## Backend Requirements

### Backend support tiers

#### Tier 1: CDP

CDP is the primary backend for secure browser automation in v1 because it can provide:

- accessibility tree extraction
- request interception
- target and popup events
- download events
- richer telemetry

#### Tier 2: BiDi

BiDi is a strategic backend for standards-based automation and event-driven flows. It should support the same public tool surface where practical, but some security hooks may be weaker than CDP in v1.

#### Tier 3: Classic WebDriver

Classic WebDriver remains supported for compatibility and broad deployment, but not for strict security mode where subresource-level network enforcement is required.

### Backend interface changes

The current `Backend` interface is too narrow for the required security hooks. Introduce a richer internal backend contract or capability interfaces for:

- command interception
- event subscription
- popup and tab enumeration
- download events
- health and reconnect checks
- backend capability reporting

Example shape:

```go
type Capabilities struct {
    AccessibilityTree bool
    NetworkIntercept  bool
    DownloadEvents    bool
    PopupTracking     bool
    ArbitraryEval     bool
}
```

Backends must declare capabilities explicitly so unsupported operations fail predictably.

## Session Lifecycle

### Session creation

Each session must:

- use a fresh temporary profile
- use a temporary download directory
- be associated with an agent ID and task/run ID
- register cleanup against task cancellation
- keep its ephemeral filesystem state inside the gVisor-visible workspace boundary

### Reconnect and relaunch

If a session is idle during HITL and the underlying browser disconnects:

- the next tool call must perform a health check
- if reconnection is possible, reconnect
- otherwise relaunch the browser with a fresh profile
- preserve logical session metadata where possible
- return a normalized `session_expired` or `backend_disconnected` error only if recovery fails

### Cleanup

Cleanup must occur on:

- normal close
- task cancellation
- agent failure
- runtime shutdown

Cleanup includes:

- closing transport sessions
- killing browser and driver processes
- removing temporary profiles
- removing temporary download directories if policy allows
- ensuring no gVisor-launched browser helper process survives task teardown

## Extraction and Context Strategy

### Default extraction mode

The default extraction mode after navigation and major page transitions is:

- accessibility tree
- structured extraction summary

Structured extraction should include:

- page title
- current URL
- visible headings
- visible links with text and href
- forms and inputs
- buttons
- code blocks
- short body text preview

### Full HTML

Full HTML is:

- disabled from automatic insertion into context
- only available via explicit `get_html`
- subject to a stricter token cap
- usually `ask` by default in read-oriented skills

### Prompt injection hygiene

Extraction intended for model context must:

- exclude raw script and style content
- prefer visible text and semantic structure
- avoid hidden or obviously decorative content where possible

System prompts for browser-enabled agents should explicitly warn that page content may be adversarial and must not override agent instructions or permissions.

## Page Orientation

Page orientation is a first-class concept.

After every navigation and significant interaction that changes location or page state, the session must create a browser observation snapshot with:

- URL
- title
- active tab ID
- interactive element counts
- extraction mode used
- a short content preview

This snapshot must be represented as a concrete context item type under the existing observation category and inserted through the normal context management path.

Example:

```text
[Browser]
URL: https://docs.rs/serde/latest/serde/
Title: serde - Rust
Tab: 1
Interactive: 12 links, 0 forms, 0 inputs
Preview: "Serde is a framework for serializing and deserializing..."
```

## Error Model

The browser subsystem must return structured error categories so the model does not parse raw protocol messages.

Required normalized categories:

- `navigation_blocked`
- `permission_denied`
- `no_such_element`
- `stale_element_reference`
- `element_not_interactable`
- `timeout`
- `backend_disconnected`
- `session_expired`
- `script_evaluation_failed`
- `unsupported_operation`
- `download_blocked`
- `download_failed`
- `credential_entry_requires_approval`

Tool results should include:

- `error_category`
- `backend`
- `operation`
- `retryable`
- `raw_message`

### Retry policy

For stale element failures:

- re-query once
- retry once
- if the retry fails, return a stale-element error

## Downloads

Downloads are part of the v1 design.

Required behavior:

- configure browser downloads to a declared directory
- ensure the directory is inside the gVisor workspace boundary
- emit telemetry and audit events for each download
- return the downloaded file path in the tool result
- integrate with filesystem permission checks

Recommended actions:

- `wait_for_download`
- `download_status`

## Auditing and Telemetry

### Audit

Every browser-sensitive event must be auditable:

- attempted navigation
- blocked navigation
- approved HITL request
- outbound request
- arbitrary JS execution
- download begin and completion
- credential entry request

Audit records should use the existing `AuditLogger`.

### Telemetry

Emit structured telemetry for:

- browser session opened and closed
- backend selected
- page snapshot added
- extraction truncated
- redirect blocked
- download completed

## Required Framework Changes

1. Add `AgentBrowserSpec` to `framework/core/agent_spec.go`.
2. Add manifest validation for browser-specific action policy.
3. Add a concrete `BrowserObservationContextItem`.
4. Add a model-facing `browser` tool implementation and registry wiring.
5. Add a `BrowserSupervisor` that launches browsers through the current gVisor-backed runtime path.
6. Extend gVisor/runtime policy enforcement so approved network rules are actually enforceable beyond `--network none`.
7. Add session cleanup registration tied to task context cancellation.
8. Add browser capability reporting and strict-mode backend selection.
9. Add download directory management and file-permission integration.
10. Ensure browser helper binaries and local transport endpoints fit within existing executable, network, and filesystem permission semantics rather than bypassing them.

## Required Code Changes in Browser Packages

1. Refactor current backends so process launch does not call `exec.CommandContext` directly and instead goes through the gVisor-backed execution path.
2. Distinguish trusted backend DOM helper scripts from arbitrary model-supplied scripts.
3. Add event subscriptions for requests, redirects, popups, and downloads.
4. Add active-tab tracking in session state.
5. Add reconnect and relaunch behavior.
6. Add page snapshot generation after navigation and significant state transitions.

## Testing Strategy

### Unit tests

Use mock backends and transport stubs to validate:

- action gating
- navigation allowlist enforcement
- arbitrary JS gating
- stale element retry behavior
- page snapshot generation
- budget truncation
- download path validation

### Integration tests

Use `httptest` fixtures to validate:

- navigation on localhost
- same-origin and cross-origin redirects
- popup flows
- file downloads
- structured extraction behavior
- gVisor-contained browser launch and cleanup behavior

### Agent-level tests

Extend agent fixtures to support browser scenarios with deterministic page content and DOM states.

### Replay and regression

Support deterministic browser-session replay for:

- navigation sequence
- extracted page states
- accessibility snapshots
- budget pruning behavior

## Implementation Phases

### Phase 1

- add browser agent spec
- add `browser` tool
- add page snapshots
- implement CDP-first secure path
- gate arbitrary JS
- gate downloads

### Phase 2

- add supervisor and gVisor-based launch
- add reconnect and cleanup hardening
- add popup and tab tracking
- add stricter audit and telemetry coverage

### Phase 3

- improve BiDi parity
- downgrade Classic WebDriver to compatibility mode where strict enforcement is not possible
- add deterministic replay fixtures

## Acceptance Criteria

The browser subsystem is ready for v1 when:

- agents can use a single `browser` tool through the normal tool registry
- browser launch is managed through the current gVisor runtime infrastructure
- undeclared hosts are blocked for explicit navigation and page-initiated requests on the secure backend path
- arbitrary page-context JS is independently gated
- page snapshots are inserted into context automatically
- downloads are permission-checked and returned as usable file paths
- browser sessions clean up reliably on success, failure, and cancellation
- tests cover unit, integration, and agent-level browser workflows
