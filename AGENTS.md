no shims 
no compat
no aliases
no stubs 

# Configuration
no os.Getenv outside userconfig/cfgload
no direct file loading outside userconfig/cfgload

# Package homes (taxonomy split, Slice 1+; relocated Slice 4)
# EffectClass, CapabilityScope → governance/classification (vocab relocated from capability)
# RiskClass → governance/risk (risk is a governance judgment, not a self-declared fact)
# governance/risk.Classify(effects, scope) is the sole risk producer; applies scope floor
# Old governance/taxonomy is DELETED — do not import it

# Authorization vocabulary (Slice 2+)
# AccessRequest, Decision, Principal, Action, Resource → governance/ports/authorization.go
# Enforcer interface → governance/authorization (Check(ctx, AccessRequest) Decision)
# Principal context key: execution/agentlifecycle sets it; everywhere else reads it
# ContextWithPrincipal → governance/ports, PrincipalFromContext → governance/ports
# Per-caller adapters build AccessRequest from domain types (no shared adapters package)

# Orchestration (Slice 3+)
# ExecuteAgent → execution/agentlifecycle/execute.go
# AgentRegistration.Execute was moved from governance/authorization to execution/agentlifecycle
# governance MUST NOT import execution (P14 retired)

# Enforcement status (Slice 7+)
# governance-no-orch: ENFORCE mode (governance→execution forbidden)
# no-bucket: ENFORCE mode (type-only pkgs imported by 3+ domains flagged; domain vocab exempt)
# exception-count: CI fails if exceptions.yaml gains net-new entries (baseline: 1)


