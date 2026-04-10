# Browser Service

## Scope

`ayenitd/service/browser` owns browser lifecycle for the workspace. It is the
service boundary for browser startup, session tracking, recovery, snapshotting,
and cleanup. The package is intentionally workspace-owned rather than
`app/relurpish/runtime` owned.

This doc describes the browser service as implemented today, not the migration
plan.

---

## Responsibilities

The browser service is responsible for:

- registering the `browser` capability into the workspace registry
- creating browser sessions lazily on `open`
- validating browser actions before backend dispatch
- enforcing network permission checks for navigation
- enforcing HITL approval for sensitive actions such as `execute_js`
- maintaining session recovery state when a backend disconnects
- writing session metadata and service snapshots under browser-scoped paths
- shutting down browser sessions on workspace close

The service does not own the model runtime, manifest loading, or capability
policy resolution. Those remain in `ayenitd`, `framework/authorization`, and
`framework/capability`.

---

## Service Wiring

The workspace boot sequence constructs the service from `ayenitd.Open()` after
registration and sandbox primitives are available.

The browser service receives:

- `WorkspaceRoot`
- `FileScope` policy rooted at the workspace
- `AgentRegistration`
- shared capability `Registry`
- `PermissionManager`
- agent runtime spec
- command policy
- default and allowed backend lists
- telemetry sink

The `ayenitd` composition root now delegates browser setup to a helper so the
browser wiring stays isolated from the rest of workspace initialization.

---

## Browser Roots

Browser-owned paths are derived from the workspace config root and are validated
against file-scope policy before use.

Service roots:

- `relurpify_cfg/browser/`
- `relurpify_cfg/browser/launch/`
- `relurpify_cfg/browser/profiles/`
- `relurpify_cfg/browser/sessions/`
- `relurpify_cfg/browser/downloads/`
- `relurpify_cfg/browser/cache/`
- `relurpify_cfg/browser/crash/`
- `relurpify_cfg/browser/metadata/`
- `relurpify_cfg/browser/logs/`

Per-session roots:

- profile directory
- download directory
- cache directory
- crash directory
- metadata JSON file
- log file

The service fails closed if any path escapes the workspace scope.

---

## Lifecycle

### Start

`Start(ctx)`:

1. checks that the registry is available
2. validates browser-owned roots
3. registers the `browser` capability if the agent spec enables browser support
4. records the service start time

### Open session

`open` creates a session handle with:

- backend name
- task ID and workflow ID
- creation time and last-seen time
- path roots
- recovery counters

The handle is stored in the workspace registry and tracked in the service’s
session map.

### Close session

`close` removes the registry entry, untracks the session, emits telemetry, and
persists updated session metadata.

### Stop

`Stop()` closes all tracked sessions and clears the session map. It is called by
workspace shutdown through `ServiceManager`.

---

## Capability Dispatch

The browser tool is an action-dispatch capability with the following supported
actions:

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

Unsupported manifest actions were removed from the accepted surface. If an
action is not implemented end to end, it should not be advertised as supported
in the manifest.

### Authorization behavior

- `open` checks whether the backend is allowed by the agent spec and service
  policy.
- `navigate` parses the URL and calls `PermissionManager.CheckNetwork`.
- `execute_js` requires a dedicated high-risk approval path.
- other actions follow the agent spec action policy through the service helper.

---

## Session Recovery

Session recovery is explicit and bounded.

When a backend disconnect is detected:

1. the session wrapper closes the failed backend
2. the service asks the factory for a replacement backend
3. recovery metadata is updated in the session record
4. session metadata is persisted again under the browser metadata root

Recovery is intentionally scoped to reconnecting the backend and restoring the
session wrapper, not to replaying arbitrary UI state.

---

## Snapshot Model

The service exposes a workspace-level snapshot that summarizes browser state.

Service snapshot fields include:

- service ID
- start time
- active session count
- default backend
- browser path roots
- backend distribution
- overall health label
- per-session snapshots

Session snapshot fields include:

- session ID
- agent ID
- task ID
- workflow ID
- backend
- transport type
- created time
- last activity time
- recovery count
- last error
- last page state
- path roots

The snapshot output is intended for observability and workspace-level recovery
inspection, not as a persistence format.

---

## Launch and Sandbox Policy

Browser startup still passes through sandbox command policy. Launching the
container or browser runtime is mediated by the same approval surface used for
other privileged command execution.

The service also creates a workspace-owned browser launch directory for backend
startup. That directory lives under the browser service root and is removed when
the backend is closed.

This removes the last hardcoded `/tmp` launch path from browser startup.

---

## Operational Notes

- The service is registered by `ayenitd.Open()` when browser support is enabled
  in the agent spec.
- `Workspace.Close()` tears the service down through `ServiceManager`.
- Integration tests use a shared manifest helper to keep browser and executable
  permissions in one place.

---

## Source Map

Relevant implementation files:

- [`/home/lex/Public/Relurpify/ayenitd/browser_service.go`](/home/lex/Public/Relurpify/ayenitd/browser_service.go)
- [`/home/lex/Public/Relurpify/ayenitd/open.go`](/home/lex/Public/Relurpify/ayenitd/open.go)
- [`/home/lex/Public/Relurpify/ayenitd/service/browser/service.go`](/home/lex/Public/Relurpify/ayenitd/service/browser/service.go)
- [`/home/lex/Public/Relurpify/ayenitd/service/browser/handler.go`](/home/lex/Public/Relurpify/ayenitd/service/browser/handler.go)
- [`/home/lex/Public/Relurpify/ayenitd/service/browser/launch.go`](/home/lex/Public/Relurpify/ayenitd/service/browser/launch.go)
- [`/home/lex/Public/Relurpify/ayenitd/service/browser/paths.go`](/home/lex/Public/Relurpify/ayenitd/service/browser/paths.go)
- [`/home/lex/Public/Relurpify/ayenitd/service/browser/session.go`](/home/lex/Public/Relurpify/ayenitd/service/browser/session.go)
- [`/home/lex/Public/Relurpify/ayenitd/service/browser/snapshot.go`](/home/lex/Public/Relurpify/ayenitd/service/browser/snapshot.go)
