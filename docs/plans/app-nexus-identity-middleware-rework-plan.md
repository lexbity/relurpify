# App Nexus Identity Middleware Rework Plan

## Goals

Rework the Nexus identity/auth boundary so that network-facing identity resolution lives in `framework/middleware`, while `app/nexus` becomes composition, persistence wiring, and app-specific policy registration.

This plan is intended to resolve the original code-quality findings:

1. Bearer-token resolution must not depend on scanning a capped token list.
2. Subject-kind resolution must be explicit, not inferred from `subject_id`.
3. Authorization failures must remain distinguishable from backend/service errors.

The broader architectural goal is to make Nexus depend on middleware contracts for identity and gateway enforcement, instead of re-implementing auth resolution in the entrypoint.

---

## Why

### Current boundary drift

`app/nexus/main.go` currently performs bearer-token resolution, token hashing, static-token mapping, and dynamic principal construction. That logic is too close to app bootstrapping and too far from the network/auth boundary.

The gateway and session middleware already model tenant-aware enforcement:

- `framework/middleware/gateway` owns connection principal resolution and feed scoping.
- `framework/middleware/session` owns tenant-bound session authorization.
- `framework/middleware/fmp` owns tenant-aware handoff and resume authorization.

The remaining gap is a reusable identity middleware layer that Nexus can plug into without reproducing lookup and inference rules.

### Why this is not a rewrite

The middleware stack already has the right shape. This plan does not replace it.

Instead, it:

- Extracts the identity resolution logic into a dedicated middleware package.
- Tightens the contract between token lookup, subject lookup, and principal creation.
- Preserves app-specific Nexus wiring, config, and persistence.

---

## Scope

### In scope

- New `framework/middleware/identity` package.
- Direct token lookup by hash or equivalent indexed key.
- Explicit subject resolution by `(tenant_id, subject_kind, subject_id)`.
- Resolver-backed principal construction for gateway and any future network transport.
- Migration of Nexus entrypoint code away from local auth inference.
- Test coverage for both the new package and the migrated call sites.

### Out of scope

- Rewriting the gateway server.
- Rewriting session or FMP policy engines.
- Changing Nexus admin semantics beyond the auth boundary required to use the new identity package.
- Introducing a new network protocol.

---

## Design Principles

1. Middleware owns network-facing identity resolution.
2. Nexus owns application wiring and persistence.
3. Subject identity must be explicit when a token is bound to a principal.
4. Tenant checks must be performed at the layer that consumes tenant-bound resources, not inferred late in the flow.
5. Authorization errors and backend errors must remain separate categories.
6. A token lookup path must be indexed or direct; it must not depend on `ListTokens()` for authentication.

---

## Proposed Package Shape

### `framework/middleware/identity`

This package should contain the reusable network-facing identity boundary.

Suggested responsibilities:

- Resolve bearer tokens into authenticated principals.
- Normalize tenant IDs consistently.
- Resolve subjects explicitly from the identity store.
- Construct gateway-compatible principals and actor metadata.
- Provide narrow interfaces that can be implemented by Nexus stores or future transports.

Suggested internal contracts:

```go
type TokenLookupStore interface {
    GetTokenByHash(ctx context.Context, tokenHash string) (*core.AdminTokenRecord, error)
}

type SubjectLookupStore interface {
    GetTenant(ctx context.Context, tenantID string) (*core.TenantRecord, error)
    GetSubject(ctx context.Context, tenantID string, kind core.SubjectKind, subjectID string) (*core.SubjectRecord, error)
}

type PrincipalResolver interface {
    ResolvePrincipal(ctx context.Context, token string) (fwgateway.ConnectionPrincipal, error)
}
```

Suggested behavior:

- Static token entries are pre-hashed and resolved in O(1) map lookups.
- Persistent tokens are looked up by token hash directly, not by list scan.
- If a stored token does not include `SubjectKind`, the resolver should fail closed unless the caller explicitly provides a subject-kind mapping rule.
- If a token refers to a tenant or subject that is disabled, resolution fails with a typed error.

### Error model

Define typed errors for:

- Invalid or missing bearer token.
- Unknown token.
- Disabled tenant.
- Disabled subject.
- Ambiguous subject resolution.
- Backend lookup failure.

The caller should be able to distinguish policy denial from infrastructure failure without parsing strings.

---

## Phase 1: Contract Definition and Inventory

### Objective

Define the identity middleware boundary and inventory every Nexus call site that currently duplicates identity or token logic.

### Work

- Add `framework/middleware/identity` package scaffolding.
- Add package docs that explain the boundary and explicitly state that the package is network-facing identity, not LLM-agent auth.
- Inventory all Nexus code paths that currently:
  - hash bearer tokens locally,
  - scan token lists,
  - infer subject kind from role or subject ID,
  - resolve tenant ownership from ad hoc store access,
  - flatten backend errors into denials.
- Identify which existing store methods are sufficient and which must be extended.

### Deliverables

- Package doc and interfaces.
- A migration map for:
  - `app/nexus/main.go`
  - `app/nexus/server/app.go`
  - `app/nexus/admin/service_sessions_identity.go`
  - relevant `framework/middleware/gateway` hooks

### Tests

- Package-level compile test for new interfaces.
- Regression inventory written as test TODOs or tracked comments only if needed for staged migration.

### Exit criteria

- The package boundary is documented.
- All auth-related Nexus call sites are enumerated.
- The migration path is explicit before behavior changes begin.

---

## Phase 2: Token Lookup and Principal Construction

### Objective

Eliminate token-list scanning and create a direct, indexed token lookup path.

### Work

- Extend the admin token store contract with a direct lookup method:

```go
GetTokenByHash(ctx context.Context, tokenHash string) (*core.AdminTokenRecord, error)
```

- Implement the direct lookup in the SQLite token store.
- Add an identity middleware resolver that:
  - hashes the presented bearer token once,
  - queries the token store by hash,
  - rejects revoked or expired tokens,
  - resolves the associated subject explicitly,
  - constructs a `fwgateway.ConnectionPrincipal`.
- Preserve static-token support, but keep it in the identity middleware layer.
- Remove the need for `ListTokens()` in auth resolution.

### Tests

- Valid token resolves successfully through direct lookup.
- Unknown token returns a typed unknown-token error.
- Revoked token is rejected.
- Expired token is rejected.
- Large token store does not change resolution correctness.
- No code path in the resolver uses `ListTokens()` for authentication.

### Exit criteria

- Authentication no longer depends on capped list pagination.
- The original token-scan correctness risk is gone.
- Principal creation is centralized in middleware.

---

## Phase 3: Explicit Subject Resolution

### Objective

Remove subject-kind inference from the token resolver and make subject binding explicit.

### Work

- Add a strict subject-resolution helper in `framework/middleware/identity`.
- Require one of the following:
  - token record includes `SubjectKind`, or
  - static configuration explicitly provides `SubjectKind`, or
  - the resolver returns an ambiguous-subject error.
- Avoid `ListSubjects()`-based fallback matching on `subject_id` alone.
- Keep tenant lookup and subject disabled checks in the identity layer.
- If a token references a missing subject, fail closed with a typed error.

### Tests

- Token with explicit subject kind resolves to the correct subject.
- Token with missing subject kind and a unique explicit mapping resolves.
- Token with missing subject kind and multiple matching subjects fails as ambiguous.
- Token for a disabled subject fails.
- Token for a disabled tenant fails.

### Exit criteria

- Subject resolution is no longer order-dependent.
- The original ambiguous-kind smell is removed.
- The identity layer enforces a strict contract instead of guessing.

---

## Phase 4: Gateway Integration

### Objective

Replace Nexus-local principal resolution with the new middleware resolver in the gateway path.

### Work

- Update `framework/middleware/gateway.Server` wiring so it accepts the new resolver implementation directly.
- Replace Nexus-local gateway principal code with a thin adapter that builds the identity resolver from config and stores.
- Keep static-token auth and persistent-token auth behavior consistent with current semantics, but move the logic out of `app/nexus/main.go`.
- Ensure gateway feed scopes continue to function unchanged.

### Tests

- Gateway accepts a valid token through the new resolver.
- Gateway rejects unknown tokens.
- Gateway rejects disabled tenant and disabled subject cases.
- Gateway feed scope behavior remains unchanged.
- Gateway connection principal shape remains stable.

### Exit criteria

- `app/nexus/main.go` no longer owns bearer-token resolution details.
- Gateway auth behavior is preserved while the implementation moves behind middleware.

---

## Phase 5: Nexus Wiring Cleanup

### Objective

Strip auth inference out of Nexus entrypoint and server composition code.

### Work

- Replace `gatewayPrincipalResolver(...)` in `app/nexus/main.go` with identity middleware construction.
- Remove local token-list scanning and duplicated principal assembly from Nexus.
- Keep `app/nexus` responsible for:
  - config loading,
  - store construction,
  - middleware wiring,
  - app-specific policy registration.
- Keep `app/nexus/server/app.go` focused on composition and route assembly.

### Tests

- Entry-point tests still pass with the new resolver.
- Static-token auth still works.
- Issued-token auth still works.
- No regression in node connection or admin MCP startup.

### Exit criteria

- `app/nexus` is thinner and more obviously an application composition layer.
- The auth boundary is no longer split between Nexus and middleware.

---

## Phase 6: Authorization Error Semantics

### Objective

Preserve the distinction between backend failure and authorization denial.

### Work

- Update the session authorization hooks in `app/nexus/server/app.go` so backend errors are returned as errors, not silently converted to denial.
- Align gateway/session/FMP authorization hooks so:
  - denial means policy refusal,
  - error means backend failure or lookup failure.
- Add typed or wrapped errors where needed so callers can log and classify failures correctly.

### Tests

- Policy denial still denies access.
- Store failure now returns an error.
- Missing boundary still returns a denial where appropriate.
- Error handling in gateway replay and session authorization remains stable.

### Exit criteria

- Infrastructure failures are no longer hidden as policy denials.
- Observability and debugging improve without changing the policy outcome model.

---

## Phase 7: Store and Model Cleanup

### Objective

Make the supporting persistence layer match the new identity boundary.

### Work

- Add any missing store method required by the new resolver.
- Audit `SQLiteAdminTokenStore` and `SQLiteIdentityStore` for interface symmetry.
- Keep the schema stable unless a direct lookup index or field normalization requires a migration.
- Add or update indexes if the direct lookup path needs them.

### Tests

- Direct token lookup works under load and with multiple records.
- Schema migrations remain idempotent.
- Existing admin token and identity tests continue to pass.

### Exit criteria

- The persistence layer supports the new resolver without fallback hacks.

---

## Phase 8: Decommission Old Paths

### Objective

Remove the duplicated Nexus-local identity logic after the new path is stable.

### Work

- Delete or reduce the Nexus-local resolver helpers that are now obsolete.
- Remove any compatibility shims that duplicate the new middleware behavior.
- Keep only the minimal adapters required to construct the middleware resolver from app config and stores.

### Tests

- Search for the old code paths returns only the new middleware boundary or compatibility wrappers.
- No test depends on the removed local auth inference.

### Exit criteria

- Identity logic has one canonical implementation path.
- Nexus no longer carries dead or redundant auth code.

---

## Phase Order

```
Phase 1 -> Phase 2 -> Phase 3 -> Phase 4 -> Phase 5 -> Phase 6 -> Phase 7 -> Phase 8
```

Phases 2 and 3 are tightly coupled and should be treated as one migration stream, even if the direct token lookup lands first.

Phase 6 can begin once the gateway wiring is stable enough to exercise error paths in tests.

---

## Risks

- Token-store interface expansion may affect admin services that also issue or inspect tokens.
- Subject ambiguity may surface previously hidden data quality issues in test fixtures or existing stores.
- Error-semantics cleanup may expose backend failures that were previously masked, which is desirable but can look like a behavior regression during rollout.
- The new middleware package must stay narrow; if it starts absorbing app-specific policy, the same boundary drift will reappear.

---

## Success Criteria

- Bearer-token resolution is direct and indexed.
- Subject-kind resolution is explicit.
- Authorization failures and backend errors remain distinguishable.
- `app/nexus` is reduced to wiring and composition at the identity boundary.
- The new middleware package becomes the canonical place for network-facing identity resolution.
- Nexus node architecture is cleaner without changing the tenant-aware behavior already enforced by the middleware stack.

