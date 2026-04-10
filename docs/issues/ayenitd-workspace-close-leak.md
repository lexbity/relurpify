# `Workspace.Close` can leak owned resources on service-stop failure

`ayenitd/workspace.go` returns immediately when `ServiceManager.Clear()` fails.

That means a stop error prevents the rest of `Close()` from running, so the workspace can leave owned resources open:
- backend
- workflow store
- pattern DB
- event log
- log file

This is a cleanup-order bug, not just a style issue. A failed service stop should not prevent deterministic release of the workspace's other owned handles.

Recommended follow-up:
- always attempt to close the remaining owned resources, even if service shutdown fails
- aggregate the service error with any later close errors instead of returning early

Status:
- fixed in the current worktree; `Workspace.Close` now keeps closing owned resources and returns an aggregated error
