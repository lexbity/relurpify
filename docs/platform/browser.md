# Platform Browser

## Scope

`platform/browser` is the backend/session layer for browser automation. It is
transport-agnostic and intentionally does not own workspace lifecycle,
capability registration, or process supervision.

Workspace ownership lives in `ayenitd/service/browser`; this package provides
the reusable browser session wrapper and backend interfaces that the service
drives.

---

## Package Layout

The package is organized into:

- `backend.go` for the transport-agnostic interface and shared value types
- `session.go` for policy-aware browser session behavior
- `errors.go` for normalized browser error codes
- `cdp/` for Chrome DevTools Protocol transport
- `bidi/` for WebDriver BiDi transport
- `webdriver/` for classic WebDriver transport

The subpackages implement protocol-specific mechanics. The root package defines
the shared contract that higher layers consume.

---

## Core Types

### Backend

`Backend` is the minimum transport-agnostic contract. Backends implement
navigation, element interaction, extraction, screenshot capture, and page
waiting.

### SessionConfig

`SessionConfig` wires the browser session wrapper to:

- a backend implementation
- a backend name
- the permission manager
- agent/session identity
- an optional context budget

### Session

`Session` wraps a backend with:

- URL permission checks before navigation
- structured extraction helpers
- token budgeting for model-visible output
- page-state capture
- normalized retry/recovery behavior

---

## Session Behavior

The session wrapper is where browser actions become agent-safe operations.

Important responsibilities:

- guard navigation with the permission manager
- surface extraction results in a consistent structure
- keep the current page snapshot in a model-friendly form
- normalize backend failures into stable browser error codes
- expose backend capabilities to higher layers

The wrapper does not choose the backend transport. It assumes the caller has
already selected and initialized the backend.

---

## Backend Model

Supported transports:

- CDP for Chromium-family automation
- WebDriver Classic for standards-based remote control
- WebDriver BiDi for event-capable standards-based automation

The transport packages are responsible for:

- protocol command dispatch
- transport-specific event handling
- error normalization into `platform/browser` error codes
- capability reporting
- resource cleanup

The root package intentionally stays neutral about the launch mechanism.

---

## Error Model

The package normalizes transport failures into a small set of browser errors:

- no such element
- stale element reference
- element not interactable
- timeout
- navigation blocked by permission policy
- script evaluation failure
- backend disconnected
- unsupported operation

This keeps higher layers from depending on protocol-specific exception text.

Error values carry backend and operation metadata so the workspace service can
report state consistently.

---

## Security Boundary

The root package enforces policy, but it does not own policy decisions.

Current enforcement layers:

- browser service authorization decides whether an action may proceed
- `PermissionManager.CheckNetwork` gates navigation egress
- sandbox command policy gates launch and cleanup commands
- browser backends only execute work after those checks pass

`execute_js` is treated as a privileged action by the workspace service, not by
the session wrapper itself.

---

## Capabilities

`Capabilities` reports transport features such as arbitrary JavaScript
evaluation support. Backends should report the real feature set, because the
workspace service uses the capability surface to decide whether an action should
be exposed or approximated.

When a backend does not implement a capability directly, the session wrapper
must not pretend that it does.

---

## Page State and Extraction

The session wrapper exposes page-state helpers that higher layers use to keep
observations compact and serializable.

Typical outputs include:

- URL
- title
- counts of links, forms, inputs, and buttons
- preview text
- structured extraction payloads

Large extraction results are truncated and annotated so model context stays
bounded.

---

## Relation to the Browser Service

The browser service builds on this package to provide the actual workspace
capability:

- the service chooses and launches the backend
- the session wrapper executes actions safely
- the service tracks recovery and session lifecycle
- the service persists metadata and snapshots

Think of this package as the browser engine API, not the workspace owner.

---

## Source Map

Relevant implementation files:

- [`/home/lex/Public/Relurpify/platform/browser/backend.go`](/home/lex/Public/Relurpify/platform/browser/backend.go)
- [`/home/lex/Public/Relurpify/platform/browser/session.go`](/home/lex/Public/Relurpify/platform/browser/session.go)
- [`/home/lex/Public/Relurpify/platform/browser/errors.go`](/home/lex/Public/Relurpify/platform/browser/errors.go)
- [`/home/lex/Public/Relurpify/platform/browser/doc.go`](/home/lex/Public/Relurpify/platform/browser/doc.go)
- [`/home/lex/Public/Relurpify/platform/browser/cdp`](/home/lex/Public/Relurpify/platform/browser/cdp)
- [`/home/lex/Public/Relurpify/platform/browser/bidi`](/home/lex/Public/Relurpify/platform/browser/bidi)
- [`/home/lex/Public/Relurpify/platform/browser/webdriver`](/home/lex/Public/Relurpify/platform/browser/webdriver)
