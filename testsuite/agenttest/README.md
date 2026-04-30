# Agenttest Framework

The `agenttest` package provides a comprehensive YAML-driven testing framework for validating Relurpify agent behavior. It implements the **OSB (Outcome · Security · Benchmark) Model** for structured test assertions.

## Overview

This framework allows defining test suites in YAML format that specify:
- **Test cases**: Prompts, context, and expected outcomes
- **Outcome assertions**: Hard pass/fail criteria for goal achievement
- **Security assertions**: Sandbox and manifest contract validation
- **Benchmark observations**: Behavioral telemetry (soft assertions that don't fail tests)

## OSB Model

The OSB Model separates assertions into three tiers:

| Tier | Block | Fails Test? | Purpose |
|------|-------|-------------|---------|
| **Outcome** | `outcome:` | Yes | Did the agent achieve the stated goal? |
| **Security** | `security:` | Yes | Did the agent respect its sandbox contract? |
| **Benchmark** | `benchmark:` | No | How did the agent behave? (recorded for comparison) |

### YAML Schema

```yaml
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: example-suite
  quarantined: false  # Required field
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agent.manifest.yaml
  cases:
    - name: example_case
      task_type: code_modification
      prompt: "Review this code for issues"
      expect:
        # Outcome block - hard assertions
        outcome:
          must_succeed: true
          files_changed:
            - testsuite/fixtures/output.txt
          output_contains:
            - "security issue found"

        # Security block - sandbox enforcement
        security:
          no_writes_outside_scope: true
          tools_must_not_call:
            - exec

        # Benchmark block - behavioral telemetry (never fails)
        benchmark:
          tools_expected:
            - file_read
            - file_write
          extensions:
            euclo:
              mode: review
              primary_relurpic_capability: code-review-v1
```

## Key Components

### Test Runner (`runner.go`, `runner_case.go`)

The test runner executes test cases against live agents:
- Loads YAML test suites
- Executes cases with configurable retries
- Captures execution output, file changes, and tool transcripts
- Produces structured `CaseReport` with OSB observations

### Expectation Evaluation (`runner_expectations.go`)

Implements the OSB model evaluation:
- `evaluateOutcomeExpectations()`: Hard assertions for goal achievement
- `evaluateSecurityExpectations()`: Security boundary validation
- `evaluateBenchmarkExpectations()`: Behavioral observations (never fails)

### Suite Management (`suite.go`)

Defines the YAML schema types:
- `Suite`: Top-level test suite container
- `CaseSpec`: Individual test case definition
- `ExpectSpec`: Contains OutcomeSpec, SecuritySpec, BenchmarkSpec
- `Extensions`: Optional subject-specific payloads under `outcome`, `benchmark`, and `context`

### Performance Tracking (`performance.go`)

Tracks execution metrics:
- Token usage (prompt, completion, total)
- Tool latency statistics
- Phase execution timing
- Baseline comparison for regression detection

### Coverage Analysis (`capability_coverage.go`)

Maps test cases to capabilities:
- Tracks which capabilities have test coverage
- Identifies uncovered capabilities
- Validates capability metadata

## Running Tests

### Run All Tests
```bash
go test ./testsuite/agenttest/...
```

### Run Specific Test Patterns
```bash
# Cleanup validation tests (Phase 8)
go test ./testsuite/agenttest/... -run "TestNoLegacy|TestOSB|TestReportSchema"

# Load and validation tests
go test ./testsuite/agenttest/... -run "TestLoadAllCommittedSuites"

# Capability coverage tests
go test ./testsuite/agenttest/... -run "TestCapability"
```

### Using the CLI

The `dev-agent-cli` provides commands for working with test suites:

```bash
# Run a specific test suite
dev-agent-cli agenttest run --suite testsuite/agenttests/coding.testsuite.yaml

# Run with specific model
dev-agent-cli agenttest run --suite coding.testsuite.yaml --model qwen2.5-coder:14b

# Run specific case
dev-agent-cli agenttest run --suite coding.testsuite.yaml --case simple_file_edit

# Run by capability
dev-agent-cli agenttest run --capability code-edit-v1
```

## Directory Structure

```
testsuite/agenttest/
├── runner*.go           # Test execution engine
├── suite.go             # YAML schema and validation
├── expectations.go      # OSB model evaluation functions
├── performance.go       # Metrics and baselines
├── benchmark.go         # Benchmark observation aggregation
├── consistency.go      # Cross-run consistency checks
├── determinism.go       # Determinism scoring
├── dependencies.go      # Tool dependency validation
├── tool_*.go            # Tool transcript handling
├── workspace.go         # Workspace management
└── *_test.go            # Unit and integration tests
```

## Migration Notes

Committed suites now use the OSB model with explicit subject extension blocks:

| Legacy Field | OSB Replacement |
|--------------|-----------------|
| `must_succeed` | `outcome.must_succeed` |
| `tool_calls_must_include` | `benchmark.tools_expected` |
| `tool_calls_must_exclude` | `security.tools_must_not_call` or `benchmark.tools_not_expected` |
| `context.extensions.euclo.*` | subject-specific task context |
| `outcome.extensions.euclo.*` | subject-specific hard assertions |
| `benchmark.extensions.euclo.*` | subject-specific soft telemetry |
| `llm_calls` | `benchmark.llm_calls_expected` |
| `max_*_tokens` | `benchmark.token_budget.*` |

Legacy translation layers have been removed from shared code.

## Exit Criteria

Cutover cleanup is complete when:
- All committed YAML files parse without legacy fields
- `go test ./testsuite/agenttest/...` passes
- `cleanup_test.go` validation tests pass
- No references to legacy evaluator functions remain
