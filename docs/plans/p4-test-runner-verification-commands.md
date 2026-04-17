# P4 — Test Runner Verification Commands

**Status:** Planning  
**Owner:** agent-platform  
**Scope:** `testsuite/agenttest/`, `testsuite/agenttest_fixtures/`, `testsuite/agenttests/euclo.*.yaml`

---

## Problem Statement

Euclo coding tests (euclo.code, euclo.debug, euclo.tdd) assert behavioral correctness using
`files_contain` string matching on agent-produced output. This is the wrong oracle. An agent that
writes `b + a` instead of `a + b`, or factors a calculation into a helper, produces correct code
that fails the assertion. Conversely, an agent that writes `a + b` but introduces a syntax error
passes the assertion.

The correct oracle for any coding task is the language test suite. If the pre-existing tests pass
after the agent's changes, the task is done. If they fail, it is not.

The `coding.*` test suites already use this model correctly (`cli_go`, `cli_python` etc.); they
instruct the agent to run tests and assert `tools_expected`. However, Euclo operates differently:
the agent is a separate entity from the test harness and should not be relied upon to self-report
correctness. The test runner must independently verify the final workspace state.

---

## Design Goals

1. The **test runner** independently executes verification after the agent finishes — not the agent.
2. Verification uses `platform/lang` tools (Go, Python, Rust, JS) through the existing
   `sandbox.CommandRunner` so the same security policies apply to runner-side verification as to
   agent-side tool use.
3. Verification is a **sequence of named steps** (clean → build → test) enabling realistic
   multi-step language toolchain workflows.
4. A **bash script** escape hatch handles cases too complex for the named-tool sequence (e.g.
   provisioning a temp DB before running migrations-plus-tests).
5. Fixture files containing test oracles (the `*_test.go`, `test_*.py`, etc.) live in
   `testsuite/agenttest_fixtures/` and are **never reset by `setup.files`** — they are permanent
   oracles that cannot be overwritten by the agent setup mechanism.
6. `files_contain` is retired from coding task assertions and replaced by `verify`.

---

## Engineering Specification

### 1. New YAML Types in `testsuite/agenttest/suite.go`

#### `VerifySpec`

Added to `OutcomeSpec`:

```go
// VerifySpec describes runner-side post-execution verification using
// platform/lang tools or a bash script.
type VerifySpec struct {
    // Steps executes a named platform/lang tool sequence in order.
    // Execution stops at the first failing step unless ContinueOnFailure is set.
    Steps []VerifyStepSpec `yaml:"steps,omitempty"`

    // Script is a workspace-relative path to a bash script.
    // Executed via the same CommandRunner as named tools; exit 0 = pass.
    // Used when the tool sequence is insufficient (e.g. DB setup + migration + test).
    Script string `yaml:"script,omitempty"`
}

// VerifyStepSpec describes one step in a verification sequence.
type VerifyStepSpec struct {
    // Tool is a platform/lang tool name: go_test, go_build, python_pytest,
    // rust_cargo_test, rust_cargo_check, node_npm_test, node_syntax_check, etc.
    Tool string `yaml:"tool"`

    // Args are passed directly to tool.Execute as the args map.
    Args map[string]any `yaml:"args,omitempty"`

    // ContinueOnFailure allows the sequence to continue past this step
    // even if it fails. Subsequent steps run; the sequence still fails overall.
    ContinueOnFailure bool `yaml:"continue_on_failure,omitempty"`
}
```

`OutcomeSpec` gains:

```go
type OutcomeSpec struct {
    // ... existing fields unchanged ...

    // Verify defines runner-side post-execution verification.
    // Each step runs after agent execution completes.
    // Results are hard outcome assertions (Tier: "outcome").
    Verify *VerifySpec `yaml:"verify,omitempty"`
}
```

#### Example YAML (Go)

```yaml
expect:
  outcome:
    must_succeed: true
    files_changed:
    - testsuite/agenttest_fixtures/gosuite/calculator/calculator.go
    verify:
      steps:
      - tool: go_build
        args:
          package: ./testsuite/agenttest_fixtures/gosuite/calculator
      - tool: go_test
        args:
          package: ./testsuite/agenttest_fixtures/gosuite/calculator
```

#### Example YAML (Python)

```yaml
expect:
  outcome:
    must_succeed: true
    files_changed:
    - testsuite/agenttest_fixtures/pysuite/calc/calc.py
    verify:
      steps:
      - tool: python_pytest
        args:
          path: testsuite/agenttest_fixtures/pysuite/calc
          extra_args: ["-q", "--tb=short"]
```

#### Example YAML (Rust)

```yaml
expect:
  outcome:
    must_succeed: true
    verify:
      steps:
      - tool: rust_cargo_check
        args:
          working_directory: testsuite/agenttest_fixtures/rustsuite
      - tool: rust_cargo_test
        args:
          working_directory: testsuite/agenttest_fixtures/rustsuite
```

#### Example YAML (bash script escape hatch)

```yaml
expect:
  outcome:
    must_succeed: true
    verify:
      script: testsuite/agenttest_fixtures/gosuite/integration/verify.sh
```

---

### 2. Tool Registry for Verification

`runner_case.go` (or a new `runner_verify.go`) builds a tool index from
`shell.CommandLineTools(workspace, runner)` keyed by tool name. This is the same factory called
when bootstrapping agent runtime, so every tool registered for agents is also available to the
runner-side verifier with identical `CommandRunner` and sandbox scope.

```go
// buildVerifyToolIndex indexes all CommandLineTools by name for O(1) lookup.
func buildVerifyToolIndex(workspace string, runner sandbox.CommandRunner) map[string]core.Tool {
    tools := shell.CommandLineTools(workspace, runner)
    index := make(map[string]core.Tool, len(tools))
    for _, t := range tools {
        index[t.Name()] = t
    }
    return index
}
```

The `runner` value is the exact same `fsandbox.CommandRunner` (local or gVisor) that was
constructed for the agent earlier in `runCase`. No new runner is created.

---

### 3. Verification Execution in `runner_expectations.go`

```go
// VerifyStepResult captures the outcome of one verification step.
type VerifyStepResult struct {
    StepIndex int
    ToolName  string
    Success   bool
    Stdout    string
    Stderr    string
    Summary   string
}

// runVerificationSteps executes the VerifySpec steps or script and returns
// one AssertionResult per step plus an overall combined result.
func runVerificationSteps(
    ctx context.Context,
    spec VerifySpec,
    workspace string,
    runner sandbox.CommandRunner,
) []AssertionResult {
    index := buildVerifyToolIndex(workspace, runner)
    var results []AssertionResult

    for i, step := range spec.Steps {
        tool, ok := index[step.Tool]
        if !ok {
            results = append(results, AssertionResult{
                AssertionID: fmt.Sprintf("verify.step[%d].%s", i, step.Tool),
                Tier:        "outcome",
                Passed:      false,
                Message:     fmt.Sprintf("verification tool %q not found in registry", step.Tool),
            })
            // Unknown tool = hard stop regardless of ContinueOnFailure
            break
        }

        toolResult, err := tool.Execute(ctx, core.NewContext(), normalizeVerifyArgs(step.Args))
        passed := err == nil && toolResult != nil && toolResult.Success
        msg := extractVerifyMessage(toolResult, err)

        results = append(results, AssertionResult{
            AssertionID: fmt.Sprintf("verify.step[%d].%s", i, step.Tool),
            Tier:        "outcome",
            Passed:      passed,
            Message:     msg,
        })

        if !passed && !step.ContinueOnFailure {
            break
        }
    }

    if spec.Script != "" {
        results = append(results, runVerifyScript(ctx, spec.Script, workspace, runner))
    }

    return results
}
```

`normalizeVerifyArgs` converts `map[string]any` YAML args into the `map[string]interface{}`
signature `tool.Execute` expects (they are the same underlying type; the function is a no-op cast
that makes the intent explicit).

`extractVerifyMessage` reads `toolResult.Data["summary"]`, `toolResult.Data["first_failure"]`,
`toolResult.Data["stdout"]` in that order of preference to produce a human-readable failure
message surfaced in the test report.

---

### 4. Script Execution

```go
func runVerifyScript(
    ctx context.Context,
    scriptPath, workspace string,
    runner sandbox.CommandRunner,
) AssertionResult {
    absScript := scriptPath
    if !filepath.IsAbs(scriptPath) {
        absScript = filepath.Join(workspace, scriptPath)
    }
    // Executed via CommandTool so it goes through the same sandbox runner.
    scriptTool := command.NewCommandTool(workspace, command.CommandToolConfig{
        Name:    "verify_script",
        Command: "bash",
        Category: "verify",
        Timeout: 120 * time.Second,
    })
    scriptTool.SetCommandRunner(runner)
    result, err := scriptTool.Execute(ctx, core.NewContext(), map[string]interface{}{
        "args": []interface{}{absScript},
    })
    passed := err == nil && result != nil && result.Success
    msg := ""
    if result != nil {
        if s, ok := result.Data["stdout"].(string); ok {
            msg = s
        }
        if s, ok := result.Data["stderr"].(string); ok && s != "" {
            msg += "\n" + s
        }
    }
    if err != nil {
        msg = err.Error()
    }
    return AssertionResult{
        AssertionID: fmt.Sprintf("verify.script[%s]", filepath.Base(scriptPath)),
        Tier:        "outcome",
        Passed:      passed,
        Message:     strings.TrimSpace(msg),
    }
}
```

The bash script runs inside the gVisor sandbox when `opts.Sandbox == true`, subject to the same
filesystem and executable policies declared in the agent manifest. The script must only access
paths and binaries already permitted.

---

### 5. Integration into `runner_case.go`

In `evaluateOutcomeExpectations`, after all existing checks, add:

```go
if spec.Verify != nil && (len(spec.Verify.Steps) > 0 || spec.Verify.Script != "") {
    verifyResults := runVerificationSteps(ctx, *spec.Verify, workspace, runner)
    results = append(results, verifyResults...)
    for _, vr := range verifyResults {
        if !vr.Passed {
            failures = append(failures, vr.Message)
        }
    }
}
```

`evaluateOutcomeExpectations` signature gains a `runner sandbox.CommandRunner` parameter. All
callers in `runner_case.go` pass the runner already in scope.

---

### 6. Fixture Structure

Each test case requiring verification gets its own sub-package inside the language fixture tree.
The oracle test file is **never written by `setup.files`** — it is a permanent file in the repo.

```
testsuite/agenttest_fixtures/
  gosuite/
    mathutil/               ← existing; used by coding.go suite
      mathutil.go
      mathutil_test.go      ← oracle
    calculator/             ← new; used by euclo.code tests
      calculator.go         ← placeholder; overridden by setup.files to inject broken impl
      calculator_test.go    ← oracle (TestAdd, TestSub etc.)
    hello/                  ← new
      hello.go
      hello_test.go
    debug_divide/           ← new; used by euclo.debug tests
      divide.go
      divide_test.go
  pysuite/
    calc/
      calc.py
      test_calc.py          ← oracle
    debug_parser/
      parser.py
      test_parser.py
  rustsuite/
    src/                    ← existing Cargo workspace
      lib.rs                ← broken impl reset by setup.files
    tests/
      integration_test.rs   ← oracle; never in setup.files
```

The `setup.files` entry for each case only covers the implementation file. The test file is
absent from `setup.files`, so the runner never overwrites it and the agent setup never replaces
it.

---

### 7. `files_contain` Retirement Policy

For cases that have `verify` steps, `files_contain` assertions are removed. The two assertions
test different things — one is "did the agent write this string", the other is "does the code
work" — and the latter subsumes the former as the correct oracle.

`files_contain` remains valid for non-code assertions (e.g., confirming a config key was written,
confirming a YAML structure was emitted). It is only retired for coding-task oracle assertions.

---

### 8. Available Verification Tools (from `cli_registry.go`)

| Language | Tool Name         | Description                                      |
|----------|------------------|--------------------------------------------------|
| Go       | `go_build`       | Compiles the package; fails on syntax/type errors |
| Go       | `go_test`        | Runs `go test`; returns structured pass/fail      |
| Python   | `python_compile_check` | `python -m py_compile`; syntax only         |
| Python   | `python_pytest`  | Runs `pytest`; returns structured pass/fail      |
| Python   | `python_unittest`| Runs `python -m unittest`                        |
| Rust     | `rust_cargo_check` | `cargo check`; fast type check without linking  |
| Rust     | `rust_cargo_test`| `cargo test`; returns structured pass/fail       |
| JS/Node  | `node_syntax_check` | `node --check`; syntax only                   |
| JS/Node  | `node_npm_test`  | `npm test`; delegates to package.json test script |
| Any      | _(script)_       | Bash script via `bash <path>` through CommandRunner |

---

## Implementation Plan

### Phase 1 — Core Infrastructure

**Goal:** Add `VerifySpec`/`VerifyStepSpec` types, wire `runVerificationSteps` into the
expectations evaluator, add `runVerifyScript`, and update the `evaluateOutcomeExpectations`
signature. No test suite YAML changes yet.

**Files:**
- `testsuite/agenttest/suite.go` — add `VerifySpec`, `VerifyStepSpec`, `OutcomeSpec.Verify`
- `testsuite/agenttest/runner_expectations.go` — add `runVerificationSteps`, `runVerifyScript`,
  `buildVerifyToolIndex`, `VerifyStepResult`, `extractVerifyMessage`, `normalizeVerifyArgs`;
  update `evaluateOutcomeExpectations` signature
- `testsuite/agenttest/runner_case.go` — pass `runner` to `evaluateOutcomeExpectations`
- `testsuite/agenttest/runner_verify.go` _(new file)_ — house the verification helpers to keep
  `runner_expectations.go` focused on assertion logic

**Dependencies:** None beyond existing `platform/shell`, `platform/lang`, `framework/sandbox`
packages which are already imported by the runner.

**Unit Tests:**
- `TestRunVerificationSteps_AllPass` — stubs two tools returning `Success: true`; asserts two
  `AssertionResult{Tier: "outcome", Passed: true}` entries
- `TestRunVerificationSteps_StopsOnFirstFailure` — first step fails; asserts second step result
  is absent
- `TestRunVerificationSteps_ContinueOnFailure` — first step fails with `ContinueOnFailure: true`;
  asserts both step results present, overall sequence fails
- `TestRunVerificationSteps_UnknownTool` — tool name not in index; asserts single failed result
  with informative message
- `TestRunVerifyScript_Pass` — script exits 0; asserts `Passed: true`
- `TestRunVerifyScript_Fail` — script exits 1; asserts `Passed: false`, message contains stderr
- `TestBuildVerifyToolIndex_ContainsGoTest` — verifies `go_test` is in the index (integration
  test, requires no network)
- `TestVerifySpecYAMLRoundTrip` — YAML marshal/unmarshal of a VerifySpec with steps and script;
  asserts field fidelity
- `TestOutcomeSpecWithVerifyLoadsCorrectly` — loads a suite YAML containing `verify:` in outcome;
  asserts no parse error and fields populated

**Exit Criteria:**
- `go build ./testsuite/agenttest/...` clean
- All new unit tests pass
- `TestSuiteValidate` still passes (YAML validation must accept `verify:` in `OutcomeSpec`)
- A manually crafted suite YAML with `verify.steps: [{tool: go_test, ...}]` loads without error

---

### Phase 2 — Go Fixture Expansion

**Goal:** Create oracle-bearing sub-packages in `agenttest_fixtures/gosuite/` for each planned
euclo.code and euclo.debug test case. Confirm that `go test ./testsuite/agenttest_fixtures/...`
passes against the correct (non-broken) implementation files.

**Files (new):**
- `testsuite/agenttest_fixtures/gosuite/calculator/calculator.go` — correct impl
- `testsuite/agenttest_fixtures/gosuite/calculator/calculator_test.go` — `TestAdd`, `TestSub`
- `testsuite/agenttest_fixtures/gosuite/hello/hello.go` — correct impl
- `testsuite/agenttest_fixtures/gosuite/hello/hello_test.go` — `TestHello`
- `testsuite/agenttest_fixtures/gosuite/debug_divide/divide.go` — correct impl
- `testsuite/agenttest_fixtures/gosuite/debug_divide/divide_test.go` — `TestDivide`,
  `TestDivideByZero`
- `testsuite/agenttest_fixtures/gosuite/debug_sort/sort.go` — correct impl
- `testsuite/agenttest_fixtures/gosuite/debug_sort/sort_test.go` — `TestSort`

**Dependencies:** Phase 1 complete (types must exist for YAML references to be valid at parse
time, though Phase 2 tests are pure Go code that compile independently).

**Unit Tests (fixture self-tests):**
- Each `*_test.go` file is itself the test; `go test ./testsuite/agenttest_fixtures/gosuite/...`
  must pass against the correct (unbroken) implementation before Phase 3 begins

**Exit Criteria:**
- `go test ./testsuite/agenttest_fixtures/gosuite/...` — all pass
- Intentionally breaking each `*.go` impl (simulate what `setup.files` will inject) and running
  `go test` produces a test failure — confirms the oracle is sensitive to the bug

---

### Phase 3 — Euclo Code Test Migration

**Goal:** Migrate `euclo.code.testsuite.yaml` cases from `files_contain`-based assertions to
`verify` steps. Each case gains a `verify.steps` block; `files_contain` is removed.

**Files:**
- `testsuite/agenttests/euclo.code.testsuite.yaml` — update affected cases

**Case-level changes per test:**

| Case | Broken impl injected via `setup.files` | Verify step |
|------|---------------------------------------|-------------|
| `basic_edit_task` | `hello.go` returning `"hello"` | `go_test ./agenttest_fixtures/gosuite/hello` |
| `code_evidence_first_upgrade` | `calculator.go` with `a - b` bug | `go_build` then `go_test ./agenttest_fixtures/gosuite/calculator` |
| `edit_with_verification` | existing fixture TBD | `go_test` |

Setup files for each case write only the **implementation** file (broken version). The
`*_test.go` oracle file is not in `setup.files`.

**Dependencies:** Phase 2 (fixture packages must exist).

**Unit Tests:**
- `TestSuiteFilesAreLoadable` (existing) must continue to pass with updated YAML
- `TestSuiteValidate` must pass
- Manual dry-run: load the updated YAML and confirm `verify` fields parse without error

**Exit Criteria:**
- `go test ./testsuite/agenttest/... -run TestSuiteFilesAreLoadable` passes
- At least one euclo.code case end-to-end against a live model returns a `verify.step` result in
  its `CaseReport.AssertionResults`
- No `files_contain` entries remain in cases that have `verify` steps

---

### Phase 4 — Euclo Debug Test Migration

**Goal:** Migrate failing `euclo.debug` test cases. Debug tests exercise a buggy implementation
that the agent must locate and fix; the oracle is `go_test` passing after repair.

**Files:**
- `testsuite/agenttest_fixtures/gosuite/debug_divide/` — from Phase 2
- `testsuite/agenttest_fixtures/gosuite/debug_sort/` — from Phase 2
- `testsuite/agenttests/euclo.debug.testsuite.yaml` — update affected cases

**Key difference from Phase 3:** debug test `setup.files` injects a **subtly broken** impl (e.g.
a divide function that doesn't handle zero, an off-by-one in a sort). The test file exercises the
exact broken scenario. The oracle failing with the broken impl and passing with the fixed impl is
the validation property.

**Dependencies:** Phase 2 + Phase 3 complete (establishes the pattern).

**Unit Tests:**
- Same pattern as Phase 2: `go test` against correct impl passes; `go test` against broken impl
  fails
- `TestSuiteFilesAreLoadable` passes with updated YAML

**Exit Criteria:**
- All `euclo.debug` cases reference fixture packages with oracle test files
- No `files_contain` entries in debug cases that have `verify` steps
- At least one debug case live-tested with a model returns `verify.step` results

---

### Phase 5 — Multi-language Fixture Expansion (Python, Rust)

**Goal:** Add Python and Rust fixture sub-packages mirroring the Go pattern. Extend
`euclo.code.testsuite.yaml` or create `euclo.code.python.testsuite.yaml` and
`euclo.code.rust.testsuite.yaml` with `verify` using `python_pytest` and `rust_cargo_test`.

**Files (new):**
- `testsuite/agenttest_fixtures/pysuite/calc/calc.py` — correct impl
- `testsuite/agenttest_fixtures/pysuite/calc/test_calc.py` — oracle; `pytest` compatible
- `testsuite/agenttest_fixtures/rustsuite/src/lib.rs` — correct impl (already in Cargo workspace)
- `testsuite/agenttest_fixtures/rustsuite/tests/integration_test.rs` — oracle if not present

**Dependencies:** Phase 1 (tool infrastructure), Phase 3 pattern established.

**Unit Tests:**
- Python: `pytest testsuite/agenttest_fixtures/pysuite/calc` passes with correct impl
- Rust: `cargo test --manifest-path testsuite/agenttest_fixtures/rustsuite/Cargo.toml` passes
- Broken impl injections fail their respective test runs

**Exit Criteria:**
- At least one Python verify case in a YAML suite loads and the `python_pytest` step returns
  structured results when invoked directly via `runVerificationSteps`
- At least one Rust verify case does the same for `rust_cargo_test`
- `TestBuildVerifyToolIndex_ContainsGoTest` extended to also assert `python_pytest` and
  `rust_cargo_test` are present

---

### Phase 6 — `files_contain` Audit and Cleanup

**Goal:** Audit all euclo `*.testsuite.yaml` files; remove any remaining `files_contain` entries
that are coding-task oracles and have been superseded by `verify` steps. Document the permitted
residual uses of `files_contain` (config files, non-compilable text output).

**Files:**
- All `testsuite/agenttests/euclo.*.testsuite.yaml`
- `testsuite/agenttest/suite_coverage_test.go` — add a lint test that flags `files_contain` +
  `verify` coexistence on the same case as a warning

**Dependencies:** Phases 3–5 complete.

**Unit Tests:**
- `TestNoFilesContainOnVerifiedCases` — scans all YAML suites; for any case that has both
  `outcome.verify` steps and `outcome.files_contain`, emits a test warning (soft failure —
  benchmark tier, not outcome tier, so CI is not blocked during migration)

**Exit Criteria:**
- Zero `files_contain` entries remaining on cases that have `verify.steps` or `verify.script`
- `TestNoFilesContainOnVerifiedCases` reports zero warnings
- `go test ./testsuite/agenttest/...` passes (excluding pre-existing hardcoded-path failures)

---

## Cross-Cutting Constraints

**Sandbox policy:** Verification tool invocations use the same `CommandRunner` constructed for
the agent. In sandbox mode (`opts.Sandbox == true`) this is a gVisor runner; the script and tool
steps are subject to the same filesystem scope and executable allowlist declared in the agent
manifest. Fixtures must live under the workspace root so the runner's `FileScopePolicy` permits
access.

**Timeout:** The case-level `timeout` applies to agent execution only. Verification steps run
after the timeout boundary. Each tool step gets the tool's own internal timeout (default 60s for
`CommandTool`). Verification is expected to be fast (sub-10s for unit tests); long-running
verification indicates a fixture design problem.

**Fixture reset:** The Go root `go.mod` covers `testsuite/agenttest_fixtures/gosuite/`, so no
module init is needed. Python fixtures require a `conftest.py` or `pyproject.toml` at the
fixture root to be pytest-discoverable. Rust fixtures are covered by the existing
`rustsuite/Cargo.toml`.

**Oracle immutability:** The oracle test file must never appear in a case's `setup.files`. A
lint check in `suite_coverage_test.go` (Phase 6) can enforce this by cross-referencing
`setup.files` paths against known fixture oracle file names.
