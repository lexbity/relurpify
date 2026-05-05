# Framework Integration Test Suite

This suite is the canonical home for cross-seam framework integration tests. It validates real runtime behavior across framework packages using deterministic, repository-native fixtures.

## Operating model

- **Canonical suite home**: `testsuite/framework`
- **Execution model**: deterministic `go test ./...`
- **Behavior model**: real framework state only, no compatibility shims or mock-driven substitutes for production seams
- **Test ownership**: each test owns its workspace, manifest root, permission manager, capability registry, audit sink, telemetry sink, and cleanup

## Harness

Use `NewTestEnvironment(t)` as the entry point for framework-suite tests.

It constructs and owns:

- `WorkspacePath`
- `ManifestRoot`
- `PermissionManager`
- `Registry`
- `TelemetrySink`
- `AuditSink`

Cleanup is registered with `t.Cleanup()` and is idempotent if invoked explicitly.

## File inventory

Current suite files:

- `assertions.go`
- `authorization_audit_test.go`
- `authorization_seam_test.go`
- `capability_seam_test.go`
- `context_envelope_trigger_test.go`
- `context_streaming_integration_test.go`
- `cross_seam_scenarios_test.go`
- `determinism_test.go`
- `diagnostics_test.go`
- `e2e_integration_test.go`
- `fixture_builders.go`
- `fixtures_test.go`
- `framework_bench_test.go`
- `framework_integration_test.go`
- `graph_search_integration_test.go`
- `ingestion_pipeline_test.go`
- `knowledge_seam_test.go`
- `malformed_content_test.go`
- `manifest_seam_test.go`
- `network_boundary_seam_test.go`
- `telemetry_seam_test.go`
- `testenv_test.go`
- `workspace_scanning_test.go`

## Adding a new seam test

1. **Use the harness**
   ```go
   env := NewTestEnvironment(t)
   ```

2. **Use shared assertions** from `assertions.go` when checking audit and telemetry behavior.

3. **Prefer deterministic fixtures** from `fixture_builders.go` for workspace, manifest, policy, envelope, and audit record setup.

4. **Keep assertions at the seam boundary**
   - assert runtime effect, not only data shape
   - assert allow/deny outcomes explicitly
   - assert deterministic ordering when collections are involved

## Running tests

Run the suite:

```bash
go test ./testsuite/framework
```

Run with verbose output:

```bash
go test -v ./testsuite/framework
```

Run benchmarks:

```bash
go test -bench=. -benchmem ./testsuite/framework
```

## Exit criteria

- Cross-seam framework tests live in `testsuite/framework`
- Harness construction and cleanup are deterministic and repeatable
- README guidance matches the implemented file set and suite boundaries
- Root-level seam tests are clearly identified for migration
