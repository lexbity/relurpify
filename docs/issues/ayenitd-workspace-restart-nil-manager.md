# `Workspace.Restart` panics when `ServiceManager` is nil

`ayenitd/workspace.go` calls `w.ServiceManager.StartAll(ctx)` unconditionally after `stopServices()`.

`stopServices()` tolerates a nil manager, but `Restart()` does not. If a workspace is partially constructed or a caller clears the manager reference, `Restart()` will panic instead of returning an error or a no-op.

Recommended follow-up:
- guard the `StartAll` call with a nil check
- decide whether nil-manager restart should be a no-op or a returned error
- add a regression test for the nil-manager path

Status:
- fixed in the current worktree; `Workspace.Restart` now returns `service manager unavailable` instead of panicking when the manager is nil
