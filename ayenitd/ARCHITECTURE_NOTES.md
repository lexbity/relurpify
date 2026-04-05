# Architectural Issues in ayenitd

## 1. Service Lifecycle & Restart
The current `Workspace.Restart` method calls `Close()` which stops all services **and discards the service registry** (`ServiceManager.StopAll` clears the internal map). After `Close`, `StartAll` has no services to start, making `Restart` a no‑op.

**Files needing change:**
- `ayenitd/services.go` – `StopAll` should not clear the registry, or a separate `Reset` method should be provided.
- `ayenitd/workspace.go` – `Restart` should stop services without discarding definitions, or re‑register services before starting.

## 2. Service Definition vs Runtime State
`ServiceManager` currently conflates service definitions with running instances. Better separation would allow restarting without re‑registration.

**Files needing change:**
- `ayenitd/services.go` – Add a map of definitions and a map of running instances.

## 3. Workspace.Close Resource Ownership
`Close` also closes stores (patternDB, workflowStore, etc.) which may be needed after a restart. A true restart should keep stores open.

**Files needing change:**
- `ayenitd/workspace.go` – Split close into `CloseServices` and `CloseAll`.
- `ayenitd/open.go` – Possibly adjust initialization to allow re‑use of stores.

## 4. Scheduler Job Persistence
`LoadJobsFromMemory` currently logs but does not restore executable actions (due to Phase 2 contract). This limits usefulness.

**Files needing change:**
- `ayenitd/scheduler.go` – May need to store job metadata that can be mapped to capabilities later.

## Summary of Files Likely to Need Changes

### Core Logic Changes
- `ayenitd/services.go`
- `ayenitd/workspace.go`
- `ayenitd/scheduler.go`

### Unit Test Additions (new files)
- `ayenitd/services_test.go`
- `ayenitd/capability_bundle_test.go`
- `ayenitd/bootstrap_extract_test.go`
- `ayenitd/open_unit_test.go`

### Integration Test Additions
- `ayenitd/probe_integration_test.go`
- `ayenitd/workspace_restart_test.go`
