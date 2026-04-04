Security model — gaps found
                                                                               
  ayenitd.Open() correctly wires the core security stack:
                                                                               
  - fauthorization.RegisterAgent() with HITLTimeout, AuditLimit, Sandbox,      
  BaseFS ✓                                                                     
  - registration.Permissions → BootstrapAgentRuntime ✓                         
  - Compiled policy engine → registration.Policy + registry.SetPolicyEngine() ✓
  - Permission event logger → telemetry sink ✓                                 
   
  One gap: open.go sets a log-only SetEventLogger (line 112–119) using the     
  MultiplexTelemetry. Then runtime.go overrides it a second time with the full
  nexusdb.SQLiteEventLog. The second call wins, which is correct — but the     
  first wiring in open.go is dead code for the runtime.go caller. This isn't
  broken, but it's misleading: ayenitd.Open() pretends to wire the event logger
   but runtime.go immediately replaces it. The comment in open.go at line 111
  should note this is a best-effort default that callers may override.

  ---
  3. Testing method — how ayenitd affects the testsuite
                                                                               
  The testsuite (testsuite/agenttest/runner_agent.go) bypasses ayenitd.Open() 
  entirely. It calls appruntime.BootstrapAgentRuntime directly at line 168 and 
  constructs its own HITL broker with auto-approve. This is intentional — agent
   tests need controlled HITL and deterministic permission behavior.           
                  
  What this means structurally:                                                
   
  ayenitd.Open()              ← tested by workspace_test.go (integration)      
      └── BootstrapAgentRuntime
                                                                               
  agenttest runner_agent.go   ← calls BootstrapAgentRuntime directly
      └── BootstrapAgentRuntime   (same path, different security setup)        
                                                                               
  Gaps in test coverage:
                                                                               
  1. runtime.New() has no test at all. It's the main production entry point and
   it adds event log wiring + relurpic capability registration that neither
  ayenitd.Open() nor the agenttest runner exercises.                           
  2. Security model is not asserted in integration tests. TestOpenWorkspace
  checks that ws.Environment.Registry != nil etc., but doesn't verify that     
  ws.Environment.PermissionManager == ws.Registration.Permissions, or that the
  policy engine is installed in the registry. These assertions would catch any 
  future regression in the security wiring.
  3. The SkipASTIndex behavior change (now short-circuits before BuildIndex)
  isn't tested. The existing TestBuildAgentPropagatesSkipASTIndexToBootstrap   
  only verifies propagation, not the path-filter HITL avoidance that was
  actually the bug.                                                            
                  
  The most valuable additions would be (in priority order):                    
  - A unit test asserting env.PermissionManager == registration.Permissions
  after Open() — verifies the security object identity isn't accidentally      
  broken by future refactors                                             
  - An assertion that SkipASTIndex: true results in BuildIndex not being called
   (prevents the HITL-blocking regression from recurring)                      
  - An integration smoke test for runtime.New() if you want the production     
  composition path covered

  ### Pending

| Phase | Description | Status |
|---|---|---|
| 5b | Memory job action dispatch (Phase 2 of scheduler) | Pending — loaded jobs are inert |
| 6 | framework/agentenv cleanup | Pending — `agentenv_interfaces.go` still re-exports from `framework/agentenv`; `framework/agentenv` package not yet deleted |

Phase 6 requires:
1. Delete `framework/agentenv/environment.go`
2. Move `VerificationPlanner`, `CompatibilitySurfaceExtractor` interfaces to `ayenitd/verification.go` (currently aliases in `agentenv_interfaces.go`)
3. Update all import paths across the codebase
4. Delete `agents/environment.go`

---

## Known Gaps

See `docs/issues/ayenitd-gaps.md` for a full list. Summary:

1. **Permission event logger is dead code in open.go** — `open.go:111-119` wires a log-only event logger, but `app/relurpish/runtime/runtime.go` immediately overwrites it with the SQLite event log. The open.go wiring is a best-effort default that callers replace. Needs a comment, not a fix.

2. **Security assertions missing from TestOpenWorkspace** — `workspace_test.go` checks that fields are non-nil, but does not assert `env.PermissionManager == registration.Permissions` or that the policy engine is installed in `registry`. These would catch future security wiring regressions.

3. **SkipASTIndex behavior not tested** — `TestBuildAgentPropagatesSkipASTIndexToBootstrap` verifies propagation but not that `BuildIndex` is actually skipped.

4. **`agenttest` runner bypasses ayenitd.Open()** — calls `BootstrapAgentRuntime` directly with auto-approve HITL. This is intentional; agent tests need deterministic permission behavior. `runtime.New()` (the main production composition path) has no test coverage.

---