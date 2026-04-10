# `BuildBuiltinCapabilityBundle` cleanup leak

`ayenitd/capability_bundle.go` opens several owned resources before all failure paths have been exhausted:
- the AST SQLite store
- the graph database
- the code index

Some early returns after these allocations do not close the already-open handles. That makes the composition root leak resources on register/build failures and can leave partially initialized state behind during tests or startup retries.

Recommended follow-up:
- add owned cleanup on every error return after resource acquisition
- or wrap the intermediate handles in a small lifecycle helper that closes them on failure
- add regression tests for the cleanup branches once the fix is in place

Status:
- fixed in the current worktree; `BuildBuiltinCapabilityBundle` now routes post-acquisition failures through a consolidated cleanup hook and the regression tests cover the cleanup branches
