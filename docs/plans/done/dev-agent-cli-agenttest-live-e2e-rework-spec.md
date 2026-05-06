# Dev-Agent CLI / Agenttest Live E2E Rework Specification

**Status:** Active
**Date:** 2026-05-06
**Scope:** Rework the live end-to-end test path so `testsuite/agenttest` prepares a full test workspace, `dev-agent-cli` composes and controls the live runtime services for that workspace, and Euclo executes against the real agent/service stack with structured telemetry, service lifecycle control, sandbox enforcement, backend selection, and run-scoped artifacts.
**Policy:** No minimum viable product, no stubs, no shims, no compat layers, no aliases, no transitional wrappers that preserve the old architecture.

---

## 1. Executive Summary

The current live testing stack is split across three incomplete responsibilities:

- `testsuite/agenttest` creates a derived workspace and then bootstraps agent execution directly.
- `app/dev-agent-cli` opens a workspace and runs an agent, but does not consistently force the real named-agent composition path for service setup and restart.
- `named/euclo/services` provides the named-agent registration surface, but the current live path does not reliably exercise it as part of the runtime composition lifecycle.

That split is acceptable for smoke tests, but it is not sufficient for a live CI path that is supposed to validate the actual runtime shape:

- workspace preparation and fixture injection
- security model enforcement through the framework sandbox
- framework service composition
- named-agent registration
- service start/restart/reset semantics
- live LLM-backed execution through a selectable provider backend
- post-run verification against the prepared workspace
- run-scoped logs and telemetry for setup and execution

This specification defines a replacement live-test pipeline:

1. `testsuite/agenttest` prepares a test workspace and emits a prepared-run descriptor.
2. `dev-agent-cli` reads that descriptor, composes the workspace runtime, registers framework and named-agent services, and starts or restarts services.
3. Euclo executes through the CLI against the live LLM and the initialized workspace/service stack.
4. `testsuite/agenttest` verifies the resulting files, outputs, tool traces, and verification artifacts.
5. Logs and telemetry are written to run-scoped setup and execution directories.

This is a hard architectural rework. The old direct-bootstrap path must be removed, not preserved behind an adapter.
The execution path must remain sandboxed by default and backend-agnostic by design.

---

## 2. Goals and Non-Goals

### 2.1 Goals

- Make `testsuite/agenttest` responsible for workspace preparation, fixture overlay, scenario seeding, and post-run verification.
- Make `dev-agent-cli` responsible for workspace composition, named-agent registration, service lifecycle, service restart/reset, and live execution.
- Ensure Euclo service registration happens through the real registration path using `framework/services` and `named/euclo/services`.
- Keep the sandbox enabled by default for live runs and treat sandbox blocks as expected security-enforcement outcomes, not harness failures.
- Support a prepared-workspace handoff between `testsuite/agenttest` and `dev-agent-cli` without hidden global state.
- Add explicit run-scoped logs and telemetry for setup, service initialization, execution, retry/reset handling, and verification.
- Preserve the live model interaction model while upgrading the runtime composition path underneath it.
- Make backend selection a first-class test dimension so live runs can target Ollama, LM Studio, and other supported `platform/llm` backends without changing the test semantics.
- Remove the direct agent bootstrap path from `testsuite/agenttest`.
- Remove any architecture that requires `testsuite/agenttest` to know how to instantiate or execute Euclo directly.

### 2.2 Non-Goals

- No stubbed service initialization.
- No mock service manager in place of the real runtime.
- No compatibility mode that preserves the old `agenttest` direct boot path.
- No alias packages or pass-through wrappers that obscure ownership.
- No partial migration that keeps both old and new live paths active.
- No minimum viable plan that postpones service composition to a later iteration.
- No test mode that disables sandbox enforcement as the default live path.
- No provider-specific logic baked into the live path when the provider can be resolved from the descriptor or CLI selection.

---

## 3. Current-State Assessment

### 3.1 What the code does today

The current code shape is not aligned with the intended live execution model:

- `testsuite/agenttest/runner_case.go` materializes a derived workspace and then calls `buildAgent(...)` directly.
- `testsuite/agenttest/runner_agent.go` bootstraps an agent runtime, sets up sandboxing, permission management, telemetry, and the workspace registry directly.
- `app/dev-agent-cli/start.go` opens a workspace with `agentenv.Open(...)` and an empty `agentenv.AgentRegistrationFuncs{}` value, then initializes and executes the selected agent.
- `app/dev-agent-cli/workspace.go` exposes workspace inspection and service listing helpers that also open the workspace with empty agent registration functions.
- `named/euclo/agent.go` performs Euclo-specific registration during `Initialize()`, but that path is not guaranteed to be the same composition root used by the live test setup.
- sandbox enforcement is present in the framework runtime, but the live test path does not yet assert that sandbox remains active by default and isolated from verification failures.

### 3.2 Why this is insufficient

The current shape does not validate the real runtime composition path because:

- `testsuite/agenttest` can succeed without proving the composition root is correct.
- `dev-agent-cli` can run Euclo without proving that the prepared workspace lifecycle is being honored.
- Service startup is not anchored to a run descriptor that ties together workspace, manifest, configuration, logs, telemetry, and test artifacts.
- Service restart/reset behavior is not part of the test contract, so stale live state can survive across test iterations.
- Telemetry and logs are not consistently run-scoped for setup and execution phases, which makes debugging CI failures unnecessarily opaque.
- Backend/provider selection is not yet formalized as a matrix input, so the existing live path is effectively coupled to the backend configuration the CLI happens to open.

### 3.3 Desired ownership boundaries

The target ownership split is:

- `testsuite/agenttest`
  - prepares and verifies
  - owns test fixtures, workspace derivation, and assertions
  - does not compose the live service stack

- `dev-agent-cli`
  - composes and runs
  - owns live workspace service initialization, reset, restart, and agent execution
  - consumes a prepared workspace contract from `testsuite/agenttest`
  - resolves the backend/provider indicated by the prepared-run descriptor or explicit run selection
  - preserves sandbox activation as the default live execution mode

- `framework/agentenv` and `ayenitd`
  - remain the runtime composition layer that builds workspace services
  - expose the composition root used by the CLI
  - apply sandbox and permission policy regardless of backend/provider

- `named/euclo/services`
  - remain the Euclo service registration layer
  - are invoked through the composition path, not by ad hoc direct wiring

---

## 4. Target Architecture

### 4.1 End-to-end flow

The intended live test flow is:

1. `testsuite/agenttest` creates a derived workspace from the scenario and fixture set.
2. `testsuite/agenttest` writes a prepared-run descriptor containing workspace, manifest, config, logs, telemetry, and verification metadata.
3. `dev-agent-cli` loads that descriptor, resolves the selected provider backend, and composes the runtime services for the prepared workspace.
4. `dev-agent-cli` starts or restarts the services in the prepared workspace using the real named-agent registration flow.
5. `dev-agent-cli` executes Euclo against the selected live LLM backend and the initialized workspace with sandbox enforcement active.
6. `testsuite/agenttest` performs outcome and verification checks after execution.
7. Artifacts are written back into run-scoped directories under `relurpify_cfg/testsetup/...` and `relurpify_cfg/test_runs/...`.

The test driver must support targeting distinct backend families in the same suite definition or as an explicit selection at run time. Supported backends include Ollama and LM Studio, and the architecture must not assume that one provider's process model or reset semantics apply to all others.

### 4.2 Prepared-run contract

The central contract between `testsuite/agenttest` and `dev-agent-cli` is a prepared-run descriptor. It must be a persisted artifact, not an in-memory convention.

The descriptor should encode:

- `run_id`
- `suite_path`
- `suite_name`
- `case_name`
- `agent_name`
- `workspace_root`
- `derived_workspace_root`
- `manifest_path`
- `config_path`
- `agents_dir`
- `logs_dir`
- `telemetry_dir`
- `setup_dir`
- `execution_dir`
- `service_reset_strategy`
- `backend_provider`
- `backend_family`
- `backend_endpoint`
- `backend_binary`
- `backend_service`
- `backend_reset_strategy`
- `model_name`
- `skip_ast_index`
- `strict_mode`
- `max_iterations`
- `max_retries`
- `sandbox_backend`
- `setup_overlays`
- `seeded_state`
- `verification_steps`
- `verification_script`
- `expected_artifacts`

The descriptor is the authoritative handoff between preparation and execution. It is not a compatibility object and must not imply the old direct-bootstrap runtime shape.

### 4.3 Service composition requirements

The live service initialization path must use the actual workspace composition path so that:

- framework services are created through the framework service builders
- Euclo services are registered through `named/euclo/services`
- the service manager belongs to the same runtime that will be used for execution
- service restart/reset is performed against the same workspace and service registry that execution uses
- the composition path remains the single source of truth for service lifecycle ownership

The CLI must not recreate the runtime by ad hoc direct package wiring if the composition root already exists.
The CLI must also preserve the framework sandbox as the default mode of execution, and backend-specific setup must not bypass the manifest-declared security model.

### 4.4 Logging and telemetry requirements

The live test path must emit verbose logs and telemetry into run-scoped directories. The required artifact roots are:

- `relurpify_cfg/testsetup/<run_id>/`
  - workspace preparation trace
  - service initialization trace
  - service restart/reset trace
  - setup-phase telemetry
  - setup-phase errors and warnings

- `relurpify_cfg/test_runs/<agent>/<run_id>/`
  - execution logs
  - telemetry JSONL
  - tool transcripts
  - assertions
  - run report
  - verification outputs

The plan expects these paths to be configurable in code, but not optional in live CI mode.
Backend setup and startup logs must include the selected provider family, endpoint, binary/service target, and any reset strategy so failures can be attributed to backend availability versus sandbox enforcement versus agent behavior.

### 4.5 Verification model

Verification remains the responsibility of `testsuite/agenttest`, but it must operate against artifacts produced by `dev-agent-cli`.

Verification must be able to inspect:

- changed files
- output text or regex checks
- tool transcript validation
- workspace diff validation
- command verification results such as build or unit-test checks
- benchmark and telemetry comparison
- backend selection and reset provenance
- sandbox-denied operations that are expected by policy

The verifier must not re-run setup or compose services. It validates the completed run only.

### 4.6 Failure classification model

The live test harness must distinguish between at least four failure classes:

- `sandbox_expected`
  - The agent attempted an operation that the framework security model intentionally blocked.
  - This is not a harness failure.
  - It is only a test failure if the case did not declare the denial as expected behavior.

- `sandbox_violation`
  - The agent or runtime crossed a security boundary that the case or manifest forbids.
  - This is a harness-visible security failure.
  - It must be reported separately from backend or model failures.

- `backend_failure`
  - The selected LLM backend is unavailable, unhealthy, misconfigured, or does not satisfy the requested model selection.
  - Examples include provider connectivity failures, model lookup failures, or provider reset failures.
  - These failures are infrastructure or environment failures, not agent behavior failures.

- `agent_failure`
  - The agent or its graph execution failed while the backend and sandbox were available.
  - This includes invalid graph execution, wrong route selection, unexpected output, missing file changes, or incorrect verification results.

The run report, telemetry artifacts, and CLI exit behavior must preserve this distinction. The goal is to avoid conflating policy-enforced sandbox blocks with backend outages or with actual agent logic regressions.

### 4.7 Backend selection model

The live path must support two backend selection shapes:

- `single-backend`
  - One backend family/provider is selected for the run.
  - The selection may come from the descriptor, suite defaults, or an explicit CLI override.
  - This is the default shape for most live runs.

- `backend-matrix`
  - A single case or suite may execute against multiple backend families/providers.
  - The matrix is explicit in the descriptor or CLI invocation.
  - Each matrix entry must produce its own backend provenance, telemetry, and outcome record.
  - Matrix entries must not share mutable runtime state beyond the prepared workspace fixture set.

Matrix runs must support the repository's supported provider backends, including at minimum Ollama and LM Studio. The design must stay provider-agnostic so backend-specific process semantics do not leak into agent or verification logic.

---

## 5. Special API Surface

The rework requires explicit API surfaces owned by the correct layer.

### 5.1 `testsuite/agenttest` API

`testsuite/agenttest` must expose a preparation surface that returns a concrete live-run descriptor and a verification contract.

The preparation API must:

- create a derived workspace from a test suite case
- render the workspace-specific `relurpify_cfg`
- seed setup files, memory, workflow state, and other scenario inputs
- create run-scoped setup artifacts
- emit a serialized run descriptor for the CLI
- record the verification contract for the case
- record the requested backend family/provider selection and any alternative backends to run the case against
- record whether the case is a single-backend execution or a backend-matrix execution

The API must answer:

- which workspace the CLI should attach to
- which manifest and config files should be used
- which log and telemetry paths should be written
- which service reset behavior is required
- which verification steps should run after execution
- which backend family/provider should be used
- whether the case should run across a backend matrix or a single explicit backend selection
- which backend provenance should be captured per matrix entry

### 5.2 `dev-agent-cli` API

`dev-agent-cli` must expose a run surface that consumes a prepared workspace descriptor and manages services for that workspace.

The CLI run surface must:

- load a prepared workspace descriptor
- resolve the real workspace root and runtime configuration
- resolve the live backend/provider backend from the descriptor or an explicit CLI override
- initialize services through the composition path
- start services before execution
- restart services after backend reset or service failure when required by the scenario
- run Euclo using the selected model and backend against the prepared workspace
- write detailed logs and telemetry to the run-scoped artifact roots
- return a structured execution report for `testsuite/agenttest`
The CLI run surface must keep sandbox enforcement on by default and may only disable it in explicitly out-of-band diagnostic modes that are not used by live CI.
The CLI must preserve per-backend run provenance so matrix runs produce independently attributable artifacts and reports for each backend entry.

### 5.3 Composition-root requirements

The composition root must be able to:

- build the workspace runtime for a prepared workspace
- wire framework services
- wire Euclo registrations
- construct the service manager
- support start, stop, clear, and restart semantics for the run
- expose enough state for the CLI to decide whether restart/reset is necessary

The CLI should not need to duplicate the composition logic.

---

## 6. Cleanup and Migration Policy

### 6.1 Hard rules

- Remove old direct-bootstrap live paths instead of leaving them in place.
- Remove temporary aliases and shims instead of routing around them.
- Remove tests that only validate the old architecture once the new architecture has equivalent coverage.
- Do not preserve old command semantics if they obscure the new workflow.
- Do not leave both the old and new runtime paths active in CI.

### 6.2 Migration sequencing

Migration must keep the repository buildable at each phase, but the final result must not retain dual live paths.

Important distinction:

- intermediate implementation code may temporarily exist while the new path is being wired
- final architectural compatibility layers must be deleted before sign-off

### 6.3 What must be removed or rewritten

The following categories of code are expected to be reworked or deleted:

- direct `agentenv.Open(..., agentenv.AgentRegistrationFuncs{})` live-path usage in CLI commands
- direct `buildAgent(...)` and `bootstrapAgentRuntime(...)` use from `testsuite/agenttest` live execution
- any test that asserts the old direct bootstrap behavior is the correct architecture
- any log or telemetry path assumption that is not run-scoped and descriptor-driven
- any `agenttest` behavior that mixes preparation and runtime composition responsibilities

---

## 7. Implementation and Rework Plan

The plan is divided into eight phases. Each phase includes:

- implementation goal
- cleanup goal
- dependencies
- files to implement
- files to cleanup or rework
- unit tests to remove
- unit tests to write
- exit criteria

The phases are ordered so that the prepared-run contract exists before the CLI depends on it, and the CLI service composition exists before `testsuite/agenttest` stops invoking runtime bootstrap directly.

---

## Phase 1 - Define the prepared-run contract and artifact layout

### Implementation goal

Create the authoritative live-test handoff object that `testsuite/agenttest` will produce and `dev-agent-cli` will consume. This must include the on-disk layout for setup artifacts, execution artifacts, and verification artifacts.

### Cleanup goal

Remove any assumptions that live execution can be started from `testsuite/agenttest` without a structured descriptor.

### Dependencies

- `testsuite/agenttest` suite model and workspace helpers
- `framework/manifest`
- `framework/core`
- `platform/llm`

### Files to implement

- `testsuite/agenttest/run_descriptor.go`
- `testsuite/agenttest/run_descriptor_test.go`
- `testsuite/agenttest/run_artifacts.go`
- `testsuite/agenttest/run_artifacts_test.go`
- `testsuite/agenttest/workspace.go` rework
- `testsuite/agenttest/descriptor_schema_test.go`

### Files to cleanup or rework

- `testsuite/agenttest/runner_case.go`
- `testsuite/agenttest/runner.go`
- `testsuite/agenttest/runner_agent.go`
- `testsuite/agenttest/README.md`
- `docs/livetesting/testsuite.md`

### Unit tests to remove

- Live tests that assume `testsuite/agenttest` directly creates and runs the Euclo agent runtime.
- Any test that only validates the old artifact directories without validating the descriptor.

### Unit tests to write

- descriptor serialization and deserialization tests
- artifact-root path derivation tests
- workspace-root and manifest-path resolution tests
- setup-output path tests for `relurpify_cfg/testsetup/<run_id>/`
- run-report path tests for `relurpify_cfg/test_runs/<agent>/<run_id>/`
- descriptor schema tests for required fields and path normalization
- backend-selection descriptor tests for single-backend and matrix runs

### Exit criteria

- A prepared run descriptor exists as a first-class object.
- The descriptor is written to disk as part of workspace preparation.
- The directory layout for setup and execution artifacts is defined and tested.
- No live execution path depends on implicit workspace state.
- The descriptor can represent both the backend family in use and any backend matrix requested for the case.

---

## Phase 2 - Introduce the CLI service composition entrypoint

### Implementation goal

Add a `dev-agent-cli` entrypoint that can attach to a prepared workspace descriptor and run the real workspace composition path, including framework services and Euclo service registration.

### Cleanup goal

Remove the assumption that `dev-agent-cli` may open a workspace with empty registration funcs and still qualify as the live E2E path.

### Dependencies

- Phase 1
- `framework/agentenv`
- `framework/services`
- `named/euclo/services`
- `named/euclo/registration.go`
- `platform/llm`

### Files to implement

- `app/dev-agent-cli/agenttest_run.go`
- `app/dev-agent-cli/agenttest_workspace.go`
- `app/dev-agent-cli/agenttest_service.go`
- `app/dev-agent-cli/agenttest_execution.go`
- `app/dev-agent-cli/agenttest_report.go`
- `app/dev-agent-cli/agenttest_run_test.go`
- `app/dev-agent-cli/agenttest_descriptor.go`

### Files to cleanup or rework

- `app/dev-agent-cli/start.go`
- `app/dev-agent-cli/workspace.go`
- `app/dev-agent-cli/service.go`
- `app/dev-agent-cli/root.go`
- `app/dev-agent-cli/agents.go`

### Unit tests to remove

- Tests that only validate `agentenv.Open(...)` is called with empty registration funcs.
- Tests that only check service listing without proving real initialization from the prepared workspace.

### Unit tests to write

- CLI command tests for prepared-run loading
- service init/restart tests against a prepared workspace descriptor
- command tests proving the workspace path comes from the descriptor, not from cwd defaults
- failure-path tests for invalid descriptor paths
- tests that prove the CLI surfaces the resolved workspace root and config paths
- tests that prove the CLI surfaces the resolved backend family and endpoint
- tests that prove backend selection defaults to the descriptor when no override is supplied

### Exit criteria

- The CLI can attach to a prepared workspace descriptor.
- The CLI initializes the workspace through the real composition path.
- The CLI can start and restart services for that workspace.
- The CLI no longer qualifies as a live path if it bypasses the prepared workspace contract.
- The CLI can run the same prepared workspace against multiple supported backend families without changing the test semantics.

---

## Phase 3 - Wire Euclo registration through the real composition path

### Implementation goal

Make sure the live CLI path reaches Euclo through `named/euclo/services` and the framework composition stack, not through ad hoc direct wiring.

### Cleanup goal

Remove the direct bootstrap assumptions that let Euclo initialize without being composed as a named workspace service.

### Dependencies

- Phase 2
- `framework/agentenv`
- `framework/manifest`
- `named/euclo/agent.go`
- `named/euclo/services/*`
- `platform/llm`

### Files to implement

- `named/euclo/registration.go` rework if needed for the chosen registration surface
- `named/euclo/services/register.go`
- `named/euclo/services/capabilities.go`
- `named/euclo/services/prompts.go`
- `named/euclo/services/recipes.go`
- `framework/agentenv/workspace.go` rework to accept and use the registration surface where appropriate
- `framework/services/prompt_registry.go` if it needs to expose any additional composition hooks

### Files to cleanup or rework

- `named/euclo/agent.go`
- `named/euclo/doc.go`
- `named/euclo/services/doc.go`
- `app/dev-agent-cli/start.go`

### Unit tests to remove

- Tests that only prove `euclo.Initialize()` can register services in isolation without exercising the live workspace composition path.
- Tests that validate the old direct registration flow as the main integration story.

### Unit tests to write

- composition-path tests that prove Euclo service registration occurs during workspace setup
- tests that verify capabilities, prompt providers, and recipes are loaded through the chosen registration path
- tests that confirm the prepared workspace can be started with Euclo registration active
- tests that confirm the Euclo agent sees the same workspace environment that the CLI composed
- tests that confirm sandbox behavior remains active in the composed live environment

### Exit criteria

- Euclo services are registered as part of the live workspace composition path.
- The live CLI path no longer needs to manually emulate service registration.
- The composition path is the only supported way to initialize Euclo services for live testing.
- The same composition path can be used with multiple llm providers selected by descriptor.

---

## Phase 4 - Add run-scoped verbose logging and telemetry for setup and execution

### Implementation goal

Create deterministic run-scoped artifact routing for logs and telemetry across setup, service lifecycle, and execution.

### Cleanup goal

Remove vague logging that writes only to default locations or to process-global stderr/stdout when a run-scoped path is required.

### Dependencies

- Phases 1 to 3
- `framework/telemetry`
- `framework/manifest`
- `framework/agentenv`
- `platform/llm`

### Files to implement

- `app/dev-agent-cli/agenttest_logging.go`
- `app/dev-agent-cli/agenttest_telemetry.go`
- `app/dev-agent-cli/agenttest_report.go`
- `testsuite/agenttest/telemetry.go` rework
- `testsuite/agenttest/runner.go` rework
- `testsuite/agenttest/run_artifacts.go` rework

### Files to cleanup or rework

- `app/dev-agent-cli/start.go`
- `app/dev-agent-cli/workspace.go`
- `testsuite/agenttest/runner_case.go`
- `testsuite/agenttest/runner_execution.go`
- `testsuite/agenttest/README.md`
- `docs/livetesting/dev-agent-cli.md`

### Unit tests to remove

- Tests that only assert default log paths without a run descriptor.
- Tests that do not distinguish setup logs from execution logs.

### Unit tests to write

- log path creation tests for `relurpify_cfg/testsetup/<run_id>/`
- telemetry path creation tests for `relurpify_cfg/test_runs/<agent>/<run_id>/`
- tests that verify setup logs capture workspace initialization steps
- tests that verify execution telemetry is written to the run directory
- tests that assert service start/restart events are visible in the setup log stream
- tests that assert backend family/endpoint/binary/service target is emitted in setup telemetry

### Exit criteria

- Setup, service lifecycle, and execution each emit artifacts to predictable run-scoped directories.
- The prepared-run descriptor includes log and telemetry locations.
- The live test flow can be debugged from the run artifacts alone.
- The logs and telemetry clearly distinguish backend failure, sandbox enforcement, and agent behavior failure.

---

## Phase 5 - Rework testsuite/agenttest to prepare and verify, not compose and run

### Implementation goal

Move `testsuite/agenttest` fully into the preparation and verification role. It should create the test workspace, emit the descriptor, invoke the CLI run surface, and then verify outcomes.

### Cleanup goal

Remove the direct agent bootstrap path from `testsuite/agenttest`.

### Dependencies

- Phases 1 to 4
- `testsuite/agenttest/workspace.go`
- `testsuite/agenttest/runner_case.go`
- `testsuite/agenttest/runner_execution.go`
- `testsuite/agenttest/runner_verify.go`

### Files to implement

- `testsuite/agenttest/prepare_run.go`
- `testsuite/agenttest/prepare_run_test.go`
- `testsuite/agenttest/verification_driver.go`
- `testsuite/agenttest/verification_driver_test.go`
- `testsuite/agenttest/run_descriptor.go` rework
- `testsuite/agenttest/runner.go` rework

### Files to cleanup or rework

- `testsuite/agenttest/runner_case.go`
- `testsuite/agenttest/runner_agent.go`
- `testsuite/agenttest/runner_execution.go`
- `testsuite/agenttest/runner.go`
- `testsuite/agenttest/runner_utils.go`
- `testsuite/agenttest/runner_preflight.go`

### Unit tests to remove

- Tests that directly assert `buildAgent(...)` is the canonical live path.
- Tests that make `testsuite/agenttest` responsible for running Euclo directly.

### Unit tests to write

- tests for `PrepareRun(...)`
- tests for serializing the run descriptor
- tests for invoking CLI execution from a prepared run
- tests for post-run verification using run artifacts only
- tests that prove `testsuite/agenttest` can prepare a workspace without composing the agent runtime itself

### Exit criteria

- `testsuite/agenttest` no longer owns the live runtime composition path.
- It can prepare a workspace, hand it off, and verify the result.
- Live run orchestration can be executed entirely through the descriptor-driven path.

---

## Phase 6 - Add service reset and restart orchestration semantics

### Implementation goal

Make service reset and restart a first-class part of the live test system so that stale live state can be cleared between cases or when the backend signals a reset condition.

### Cleanup goal

Remove any backend reset logic that is only tied to the old direct bootstrap path.

### Dependencies

- Phases 2 to 5
- `framework/agentenv/service.go`
- `app/dev-agent-cli/service.go`
- `testsuite/agenttest/runner_reset.go`

### Files to implement

- `app/dev-agent-cli/agenttest_service_reset.go`
- `app/dev-agent-cli/agenttest_service_restart.go`
- `app/dev-agent-cli/agenttest_service_test.go`
- `testsuite/agenttest/service_reset_contract.go`
- `testsuite/agenttest/service_reset_contract_test.go`
- `testsuite/agenttest/run_descriptor.go` additions for reset policy

### Files to cleanup or rework

- `app/dev-agent-cli/service.go`
- `testsuite/agenttest/runner_reset.go`
- `testsuite/agenttest/runner_preflight.go`

### Unit tests to remove

- Tests that only verify a reset helper calls a backend interface without integrating with the workspace/service lifecycle.
- Tests that assume backend reset is independent of service lifecycle.

### Unit tests to write

- service restart tests against the prepared workspace
- reset-on-failure tests
- reset-between-cases tests
- service manager lifecycle tests for start, stop, clear, and restart in the live path
- descriptor-driven tests for service reset strategy selection

### Exit criteria

- Service reset and restart are driven from the live test descriptor and service manager.
- CI can request resets between cases without falling back to direct runtime bootstrapping.
- The test harness can recover from stale live state through the same composition path used for execution.

---

## Phase 7 - Rework live E2E verification and remove obsolete tests

### Implementation goal

Make verification explicitly consume the live execution artifacts and avoid any dependence on the old direct bootstrap shape.

### Cleanup goal

Delete or rewrite tests that lock in old architecture assumptions, especially tests that only prove direct agent construction works.

### Dependencies

- Phases 1 to 6
- `testsuite/agenttest/runner_expectations.go`
- `testsuite/agenttest/tool_transcript.go`
- `testsuite/agenttest/path_safety.go`
- `named/euclo/testsuite/*`

### Files to implement

- `testsuite/agenttest/verification_suite.go`
- `testsuite/agenttest/verification_suite_test.go`
- `testsuite/agenttest/live_case_driver_test.go`
- `named/euclo/testsuite/live_workspace_handshake_test.go`
- `testsuite/agenttest/verification_contract.go`

### Files to cleanup or rework

- `testsuite/agenttest/runner_verify.go`
- `testsuite/agenttest/runner_verify_test.go`
- `testsuite/agenttest/runner_workflow_test.go`
- `testsuite/agenttest/runner_agent_bugfix_test.go`
- `testsuite/agenttest/runner_preflight_test.go`
- `testsuite/agenttest/runner_reset_test.go`
- `named/euclo/agent_initialize_test.go`
- `named/euclo/agent_test.go`
- `testsuite/agenttest/runner_llm_test.go` portions that assume a single backend family if they no longer reflect the matrix-driven model

### Unit tests to remove

- Tests that only validate `named/euclo` initialization in isolation and are now superseded by live workspace handshake coverage.
- Tests that only exercise the old agent bootstrap path inside `testsuite/agenttest`.
- Tests that assert the old direct workspace-open flow is the canonical live test.

### Unit tests to write

- live handshake tests between `testsuite/agenttest` and `dev-agent-cli`
- verification tests for code tasks, including build or unit-test checks
- verification tests for workspace mutation and artifact capture
- transcript and telemetry validation tests
- tests that verify the result report includes verification outcomes and artifact locations
- backend-matrix verification tests that can execute a case across more than one supported provider backend
- sandbox-enforcement verification tests that classify expected denials as policy outcomes rather than harness failures

### Exit criteria

- The suite verifies live execution against the prepared workspace and the CLI-generated artifacts.
- Obsolete tests tied to the old architecture are removed or rewritten.
- Verification reads from the execution result, not from hidden internal state.

---

## Phase 8 - CI wiring, documentation cleanup, and final architecture lock

### Implementation goal

Finalize CI entrypoints, developer documentation, and repo guidance so the new live test flow is the only documented path.

### Cleanup goal

Remove stale docs, references, examples, and commands that describe the old direct-bootstrap live path.

### Dependencies

- Phases 1 to 7
- `docs/livetesting/testsuite.md`
- `docs/livetesting/dev-agent-cli.md`
- `docs/framework/testing.md`
- any CI config that invokes live testing
- any backend matrix configuration used by CI for live runs

### Files to implement

- `docs/plans/dev-agent-cli-agenttest-live-e2e-rework-spec.md` completion and revision as needed
- `docs/livetesting/testsuite.md` rework
- `docs/livetesting/dev-agent-cli.md` rework
- `docs/framework/testing.md` update if it documents live test ownership
- any CI workflow or taskfile entry that launches live tests
- `app/dev-agent-cli/doc.go` if command semantics changed

### Files to cleanup or rework

- `docs/README.md` if it still points to obsolete live-test instructions
- stale references inside `testsuite/agenttest/README.md`
- stale references inside `named/euclo/services/doc.go`
- stale references inside `app/dev-agent-cli/root.go`

### Unit tests to remove

- Tests that duplicate doc-driven assumptions and no longer reflect the architecture.
- Tests that enforce old command semantics for live execution.

### Unit tests to write

- CLI documentation/flag contract tests if command semantics changed
- smoke tests that the live run path can be discovered and invoked from CI
- file-path and artifact-path regression tests for the final directory layout
- tests that validate the final descriptor schema as consumed by the CLI
- tests that validate the backend matrix runner semantics in CI

### Exit criteria

- Documentation matches the implemented architecture.
- CI uses the new descriptor-driven live path.
- The old architecture is no longer the documented or tested live route.

---

## 8. Required Exit Conditions for the Whole Rework

The rework is complete only when all of the following are true:

- `testsuite/agenttest` prepares the workspace and verifies the result, but does not own live runtime composition.
- `dev-agent-cli` consumes a prepared workspace descriptor and initializes services through the real composition path.
- Euclo service registration is exercised through `framework/services` and `named/euclo/services`.
- Run-scoped setup logs, execution logs, and telemetry are written to the designated artifact roots.
- Service restart/reset is available from the live path without falling back to old direct-bootstrap behavior.
- The live test flow is deterministic enough to debug from the run artifacts alone.
- Sandbox enforcement remains active by default in all live runs, and expected denials are surfaced as security assertions rather than infrastructure failures.
- The same live flow can be exercised across supported provider backends such as Ollama and LM Studio.
- Obsolete tests and docs tied to the old architecture are removed or rewritten.
- There are no remaining shims, aliases, or compatibility layers that preserve the old live path.

---

## 9. Implementation Notes

- Treat the prepared-run descriptor as the contract boundary between the two subsystems.
- Keep the workspace preparation and runtime composition responsibilities separate even if they share filesystem paths.
- Prefer explicit artifact files over implicit in-memory wiring.
- Make the service lifecycle observable. If a service starts, resets, or restarts, that event should be written to the run-scoped logs.
- Keep verification strictly downstream of execution. Verification should inspect artifacts and workspace state, not re-run the runtime setup.
- If a test exists only to prove the old architecture still works, remove it or rewrite it to prove the new one.
