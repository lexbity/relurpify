# Engineering Specification: Agenttest Assertion Redesign
## Outcome · Security · Benchmark (OSB) Model

**Status:** Planning  
**Scope:** `testsuite/agenttest/`, all 57 YAML suite files (~280 test cases)  
**Motivation:** The current assertion framework conflates goal achievement, framework security enforcement, and behavioral telemetry into a single flat pass/fail surface. This causes test instability against non-deterministic LLM outputs, produces misleading failures when routing changes but outcomes are correct, and fails to distinguish genuine security violations from stylistic routing preferences. The redesign separates these three concerns permanently.

---

## 1. Design Principles

### 1.1 Three Tiers of Assertion

| Tier | Name | Fails test? | Purpose |
|------|------|-------------|---------|
| **Outcome** | `outcome:` | Yes | Did the agent achieve the stated goal? |
| **Security** | `security:` | Yes | Did the agent respect its declared sandbox contract? |
| **Benchmark** | `benchmark:` | No | How did the agent route and behave? (recorded for model comparison) |

**Rule for placement:**
- **Outcome**: Would an end user notice if this assertion failed? Does it measure a tangible deliverable (file content, task success, finding produced)?
- **Security**: Is this a declared sandbox/manifest boundary? Would a violation represent unauthorized access, capability escape, or policy bypass?
- **Benchmark**: Is this about HOW the agent did it — which capability, recipe, profile, tool sequence, token count?

### 1.2 Security Assertions Are Manifest-Driven

Security assertions are not free-form behavioral exclusions. They are structural: the agent manifest declares what the agent is permitted to do, and security assertions verify the runtime respected that contract. The runner cross-references actual actions (from the audit log and tool transcript) against the manifest's declared `PermissionSet`.

### 1.3 Benchmark Observations Are Structured, Not Failures

Benchmark mismatches produce `BenchmarkObservation` records in the case report. These are first-class data: they accumulate across runs and models, enabling comparison of routing quality, capability selection accuracy, and performance characteristics between different LLM backends. A test with 20 benchmark observations that all mismatch still PASSES; the observations are engineering signal, not blockers.

### 1.4 Backward Compatibility During Migration

Phases 1–5 keep the old `expect:` block fully functional. The new `outcome:`, `security:`, `benchmark:` blocks are additive — when present they take precedence over the equivalent old fields. Phase 8 (cleanup) removes the old fields. This lets migration happen incrementally across phases 6–7 without breaking any currently-passing test.

---

## 2. New YAML Schema

### 2.1 Full Structure

```yaml
- name: example_case
  task_type: code_modification

  expect:
    # ───────────────────────────────────────────────────────────────────
    # OUTCOME BLOCK — hard pass/fail: goal achievement
    # ───────────────────────────────────────────────────────────────────
    outcome:
      must_succeed: true

      # File system outcomes
      no_file_changes: true             # no files may be modified
      files_changed:                    # these files must have been modified
        - testsuite/fixtures/foo/bar.go
      files_contain:                    # file content assertions
        - path: testsuite/fixtures/foo/bar.go
          contains: ["return n + 1"]
          not_contains: ["return n - 1"]

      # Output / state outcomes
      output_contains: ["fixed"]
      output_regex: ["\\bfixed\\b"]
      state_key_not_empty:
        - euclo.review_findings
      state_keys_must_exist:
        - euclo.artifacts
      memory_records_created: 1
      workflow_state_updated: true

      # Intent outcome: was the declared execution mode respected?
      euclo_mode: debug                 # mode must match (formerly euclo.mode)

    # ───────────────────────────────────────────────────────────────────
    # SECURITY BLOCK — hard pass/fail: sandbox/manifest contract
    # ───────────────────────────────────────────────────────────────────
    security:
      # Filesystem scope enforcement (cross-referenced with manifest)
      no_writes_outside_scope: true     # no file_write outside declared paths
      no_reads_outside_scope: false     # reads outside scope allowed (default false = not checked)

      # Tool-level contract enforcement
      # These are sandbox-hard exclusions: tools that the manifest or profile
      # disallows in this execution context. Violation = security failure.
      tools_must_not_call:
        - file_write                    # e.g. read-only profile must never write
        - file_delete

      # Profile-level mutation constraint
      # When true: asserts that if the resolved profile has mutation_allowed:false,
      # then no file_write/file_create/file_delete calls occurred in the transcript.
      mutation_enforced: true

      # Network boundary enforcement
      no_network_outside_manifest: true # no network calls outside declared endpoints

      # Executable boundary enforcement
      no_exec_outside_manifest: true    # no executable calls outside declared binaries

      # Explicit scope for negative tests: violations that ARE expected
      # (used when testing that the sandbox correctly blocked something)
      expected_violations: []

    # ───────────────────────────────────────────────────────────────────
    # BENCHMARK BLOCK — soft: telemetry, never fails the test
    # ───────────────────────────────────────────────────────────────────
    benchmark:
      # Tool usage observations
      tools_expected: [file_read, file_search]    # expected but absence = observation only
      tools_not_expected: [go_test]               # expected absent but presence = observation only

      # LLM usage hints (advisory budgets, not hard failures)
      llm_calls_expected: 5
      max_tool_calls_hint: 20

      # Token budget hints
      token_budget:
        max_prompt: 50000
        max_completion: 8000
        max_total: 58000

      # Euclo implementation telemetry (all formerly in euclo: block)
      euclo:
        behavior_family: stale_assumption_detection
        profile: trace_execute_analyze
        primary_relurpic_capability: euclo:debug.investigate-repair
        supporting_relurpic_capabilities:
          - euclo:debug.root-cause
          - euclo:debug.localization
        specialized_capability_ids: []
        recipe_ids:
          - debug.investigate-repair.reproduce
        artifacts_produced:
          - euclo.explore
          - euclo.analyze
        phases_executed: [analyze, trace]
        phases_skipped: []
        result_class: "localization_complete"
        assurance_class: "medium"
        recovery_status: ""
        degradation_mode: ""
        success_gate_reason: ""
        min_transitions_proposed: 0
        max_transitions_proposed: 2
        min_frames_emitted: 1
        max_frames_emitted: 10
        frame_kinds_emitted: [artifact, transition]
        frame_kinds_not_expected: [error]
        artifact_chain: []

    # ─────────────────────────────────────────────────────────────────
    # LEGACY (kept for backward compat during migration, removed in Phase 8)
    # ─────────────────────────────────────────────────────────────────
    # must_succeed: true              ← migrates to outcome.must_succeed
    # tool_calls_must_include: [...]  ← migrates to benchmark.tools_expected
    # tool_calls_must_exclude: [...]  ← splits: security.tools_must_not_call
    #                                   or benchmark.tools_not_expected
    # euclo: { ... }                  ← mode → outcome.euclo_mode, rest → benchmark.euclo
```

### 2.2 Field Classification Reference

The following table governs how every existing `expect:` field maps to the new schema. This is the authoritative reference for the migration tool (Phase 6).

| Old field | New location | Notes |
|-----------|-------------|-------|
| `must_succeed` | `outcome.must_succeed` | Unchanged semantics |
| `no_file_changes` | `outcome.no_file_changes` | Hard outcome; also triggers `security.mutation_enforced` if profile is non-mutating |
| `files_changed` | `outcome.files_changed` | Unchanged |
| `files_contain` | `outcome.files_contain` | Unchanged; add `not_contains` as new complement |
| `output_contains` | `outcome.output_contains` | Unchanged |
| `output_regex` | `outcome.output_regex` | Unchanged |
| `state_key_not_empty` | `outcome.state_key_not_empty` | Unchanged |
| `state_keys_must_exist` | `outcome.state_keys_must_exist` | Unchanged |
| `memory_records_created` | `outcome.memory_records_created` | Unchanged |
| `workflow_state_updated` | `outcome.workflow_state_updated` | Unchanged |
| `euclo.mode` | `outcome.euclo_mode` | Promoted: mode is an outcome, not telemetry |
| `tool_calls_must_exclude: [file_write, file_delete]` | `security.tools_must_not_call` | Security-hard exclusions (mutation tools in read-only context) |
| `tool_calls_must_exclude: [go_test]` | `benchmark.tools_not_expected` | Behavioral guidance only |
| `tool_calls_must_include` | `benchmark.tools_expected` | All become soft observations |
| `tool_calls_in_order` | `benchmark.tool_sequence_expected` | Becomes soft observation |
| `llm_calls` | `benchmark.llm_calls_expected` | Advisory |
| `max_tool_calls` | `benchmark.max_tool_calls_hint` | Advisory |
| `max_prompt_tokens` | `benchmark.token_budget.max_prompt` | Advisory |
| `max_completion_tokens` | `benchmark.token_budget.max_completion` | Advisory |
| `max_total_tokens` | `benchmark.token_budget.max_total` | Advisory |
| `tool_success_rate` | `benchmark.tool_success_rate` | Observation |
| `tool_call_latency_ms` | `benchmark.tool_call_latency_ms` | Observation |
| `max_total_tool_time_ms` | `benchmark.max_total_tool_time_hint_ms` | Advisory |
| `determinism_score` | `benchmark.determinism_score_hint` | Advisory |
| `llm_response_stable` | `benchmark.llm_response_stable_hint` | Advisory |
| `tool_recovery_observed` | `benchmark.tool_recovery_observed` | Observation |
| `tools_required` | `benchmark.tools_expected` | Unified with must_include |
| `tool_dependencies` | `benchmark.tool_dependencies` | Observation |
| `euclo.profile` | `benchmark.euclo.profile` | Telemetry |
| `euclo.behavior_family` | `benchmark.euclo.behavior_family` | Telemetry |
| `euclo.primary_relurpic_capability` | `benchmark.euclo.primary_relurpic_capability` | Telemetry |
| `euclo.supporting_relurpic_capabilities` | `benchmark.euclo.supporting_relurpic_capabilities` | Telemetry |
| `euclo.specialized_capability_ids` | `benchmark.euclo.specialized_capability_ids` | Telemetry |
| `euclo.recipe_ids` | `benchmark.euclo.recipe_ids` | Telemetry |
| `euclo.result_class` | `benchmark.euclo.result_class` | Telemetry |
| `euclo.assurance_class` | `benchmark.euclo.assurance_class` | Telemetry |
| `euclo.success_gate_reason` | `benchmark.euclo.success_gate_reason` | Telemetry |
| `euclo.recovery_status` | `benchmark.euclo.recovery_status` | Telemetry |
| `euclo.degradation_mode` | `benchmark.euclo.degradation_mode` | Telemetry |
| `euclo.phases_executed` | `benchmark.euclo.phases_executed` | Telemetry |
| `euclo.phases_skipped` | `benchmark.euclo.phases_skipped` | Telemetry |
| `euclo.artifacts_produced` | `benchmark.euclo.artifacts_produced` | Telemetry |
| `euclo.artifact_kind_produced` | `benchmark.euclo.artifacts_produced` | Merged |
| `euclo.artifact_chain` | `benchmark.euclo.artifact_chain` | Telemetry |
| `euclo.recovery_attempted` | `benchmark.euclo.recovery_status` | Merged |
| `euclo.recovery_strategies` | `benchmark.euclo.recovery_status` | Merged |
| `euclo.min_transitions_proposed` | `benchmark.euclo.min_transitions_proposed` | Advisory |
| `euclo.max_transitions_proposed` | `benchmark.euclo.max_transitions_proposed` | Advisory |
| `euclo.min_frames_emitted` | `benchmark.euclo.min_frames_emitted` | Advisory |
| `euclo.max_frames_emitted` | `benchmark.euclo.max_frames_emitted` | Advisory |
| `euclo.frame_kinds_emitted` | `benchmark.euclo.frame_kinds_emitted` | Observation |
| `euclo.frame_kinds_must_exclude` | `benchmark.euclo.frame_kinds_not_expected` | Observation |

**Split rule for `tool_calls_must_exclude`:** A tool belongs in `security.tools_must_not_call` if calling it in this context represents a security/sandbox boundary violation (e.g., `file_write` or `file_delete` in a read-only investigation profile, any executable not in the manifest). It belongs in `benchmark.tools_not_expected` if calling it represents a behavioral routing preference that doesn't affect security (e.g., `go_test` in a static analysis task). When in doubt, use benchmark.

---

## 3. New Go Types

### 3.1 Spec Types (`suite.go` additions)

```go
// OutcomeSpec defines hard pass/fail assertions about goal achievement.
type OutcomeSpec struct {
    MustSucceed         bool                    `yaml:"must_succeed,omitempty"`
    NoFileChanges       bool                    `yaml:"no_file_changes,omitempty"`
    FilesChanged        []string                `yaml:"files_changed,omitempty"`
    FilesContain        []FileContentExpectation `yaml:"files_contain,omitempty"`
    OutputContains      []string                `yaml:"output_contains,omitempty"`
    OutputRegex         []string                `yaml:"output_regex,omitempty"`
    StateKeyNotEmpty    []string                `yaml:"state_key_not_empty,omitempty"`
    StateKeysMustExist  []string                `yaml:"state_keys_must_exist,omitempty"`
    MemoryRecordsCreated int                    `yaml:"memory_records_created,omitempty"`
    WorkflowStateUpdated bool                   `yaml:"workflow_state_updated,omitempty"`
    EucloMode           string                  `yaml:"euclo_mode,omitempty"`
}

// SecuritySpec defines hard pass/fail assertions about sandbox contract enforcement.
// Assertions here are cross-referenced against the agent manifest's PermissionSet.
type SecuritySpec struct {
    // Filesystem scope
    NoWritesOutsideScope bool     `yaml:"no_writes_outside_scope,omitempty"`
    NoReadsOutsideScope  bool     `yaml:"no_reads_outside_scope,omitempty"`

    // Tool contract: these tools must not have been called in this context.
    // Use for mutation tools (file_write, file_delete) in read-only contexts.
    // Cross-referenced with manifest.Spec.Permissions.Executables.
    ToolsMustNotCall []string `yaml:"tools_must_not_call,omitempty"`

    // Profile mutation enforcement: if the resolved execution profile has
    // mutation_allowed:false, assert that no mutation tools were called.
    MutationEnforced bool `yaml:"mutation_enforced,omitempty"`

    // Network and executable boundaries
    NoNetworkOutsideManifest bool `yaml:"no_network_outside_manifest,omitempty"`
    NoExecOutsideManifest    bool `yaml:"no_exec_outside_manifest,omitempty"`

    // Expected violations: for negative/boundary tests that verify the
    // sandbox correctly blocked something. These do not fail the test.
    ExpectedViolations []ExpectedViolation `yaml:"expected_violations,omitempty"`
}

// ExpectedViolation describes a sandbox block that is explicitly expected.
type ExpectedViolation struct {
    Kind     string `yaml:"kind"`     // "file_write", "exec", "network"
    Resource string `yaml:"resource"` // path or binary name (glob OK)
    Reason   string `yaml:"reason"`   // human annotation
}

// BenchmarkSpec defines soft observations about agent routing and behavior.
// Mismatches produce BenchmarkObservation records but never fail the test.
type BenchmarkSpec struct {
    // Tool usage
    ToolsExpected        []string          `yaml:"tools_expected,omitempty"`
    ToolsNotExpected     []string          `yaml:"tools_not_expected,omitempty"`
    ToolSequenceExpected []string          `yaml:"tool_sequence_expected,omitempty"`
    ToolSuccessRate      map[string]string `yaml:"tool_success_rate,omitempty"`
    ToolCallLatencyMs    map[string]string `yaml:"tool_call_latency_ms,omitempty"`
    ToolDependencies     []ToolDependency  `yaml:"tool_dependencies,omitempty"`
    ToolRecoveryObserved bool              `yaml:"tool_recovery_observed,omitempty"`

    // LLM usage hints
    LLMCallsExpected        int  `yaml:"llm_calls_expected,omitempty"`
    MaxToolCallsHint        int  `yaml:"max_tool_calls_hint,omitempty"`
    MaxTotalToolTimeHintMs  int  `yaml:"max_total_tool_time_hint_ms,omitempty"`
    LLMResponseStableHint   bool `yaml:"llm_response_stable_hint,omitempty"`
    DeterminismScoreHint    string `yaml:"determinism_score_hint,omitempty"`

    // Token budget hints
    TokenBudget *TokenBudgetHint `yaml:"token_budget,omitempty"`

    // Euclo implementation telemetry
    Euclo *EucloBenchmarkSpec `yaml:"euclo,omitempty"`
}

// TokenBudgetHint captures advisory token usage expectations.
type TokenBudgetHint struct {
    MaxPrompt     int `yaml:"max_prompt,omitempty"`
    MaxCompletion int `yaml:"max_completion,omitempty"`
    MaxTotal      int `yaml:"max_total,omitempty"`
}

// EucloBenchmarkSpec captures euclo-specific routing observations.
// All fields are soft telemetry.
type EucloBenchmarkSpec struct {
    BehaviorFamily               string              `yaml:"behavior_family,omitempty"`
    Profile                      string              `yaml:"profile,omitempty"`
    PrimaryRelurpicCapability    string              `yaml:"primary_relurpic_capability,omitempty"`
    SupportingRelurpicCapabilities []string          `yaml:"supporting_relurpic_capabilities,omitempty"`
    SpecializedCapabilityIDs     []string            `yaml:"specialized_capability_ids,omitempty"`
    RecipeIDs                    []string            `yaml:"recipe_ids,omitempty"`
    ArtifactsProduced            []string            `yaml:"artifacts_produced,omitempty"`
    PhasesExecuted               []string            `yaml:"phases_executed,omitempty"`
    PhasesSkipped                []string            `yaml:"phases_skipped,omitempty"`
    ResultClass                  string              `yaml:"result_class,omitempty"`
    AssuranceClass               string              `yaml:"assurance_class,omitempty"`
    RecoveryStatus               string              `yaml:"recovery_status,omitempty"`
    DegradationMode              string              `yaml:"degradation_mode,omitempty"`
    SuccessGateReason            string              `yaml:"success_gate_reason,omitempty"`
    MinTransitionsProposed       int                 `yaml:"min_transitions_proposed,omitempty"`
    MaxTransitionsProposed       int                 `yaml:"max_transitions_proposed,omitempty"`
    MinFramesEmitted             int                 `yaml:"min_frames_emitted,omitempty"`
    MaxFramesEmitted             int                 `yaml:"max_frames_emitted,omitempty"`
    FrameKindsEmitted            []string            `yaml:"frame_kinds_emitted,omitempty"`
    FrameKindsNotExpected        []string            `yaml:"frame_kinds_not_expected,omitempty"`
    ArtifactChain                []ArtifactChainSpec `yaml:"artifact_chain,omitempty"`
}
```

### 3.2 Report Types (`runner.go` additions)

```go
// SecurityObservation records one security-relevant event observed during the run.
// Used for both violations (unexpected boundary crossings) and grants
// (expected boundary crossings that were permitted per manifest).
type SecurityObservation struct {
    Kind       string `json:"kind"`       // "file_write", "exec", "network", "read"
    Resource   string `json:"resource"`   // affected path, binary, or endpoint
    Action     string `json:"action"`     // "write", "execute", "connect", "read"
    InScope    bool   `json:"in_scope"`   // true if within manifest-declared permissions
    Blocked    bool   `json:"blocked"`    // true if the sandbox blocked it
    Expected   bool   `json:"expected"`   // true if listed in security.expected_violations
    Timestamp  string `json:"timestamp"`
    AgentID    string `json:"agent_id,omitempty"`
    PolicyRule string `json:"policy_rule,omitempty"` // which manifest rule applied
}

// BenchmarkObservation records one soft telemetry mismatch or measurement.
type BenchmarkObservation struct {
    Category string `json:"category"` // "tool_usage", "euclo_routing", "token_usage", "performance"
    Field    string `json:"field"`    // dotted field name, e.g. "euclo.behavior_family"
    Expected string `json:"expected"` // expected value (stringified)
    Actual   string `json:"actual"`   // actual value observed
    Matched  bool   `json:"matched"`  // true if expected == actual
    Note     string `json:"note,omitempty"`
}

// Updated CaseReport additions
type CaseReport struct {
    // ... existing fields unchanged ...

    // New fields
    SecurityObservations []SecurityObservation `json:"security_observations,omitempty"`
    BenchmarkObservations []BenchmarkObservation `json:"benchmark_observations,omitempty"`
    // Replaces the flat Error string for structured assertion tracking
    AssertionResults []AssertionResult `json:"assertion_results,omitempty"`
}

// AssertionResult captures one outcome or security assertion result.
type AssertionResult struct {
    AssertionID string `json:"assertion_id"` // e.g. "outcome.must_succeed"
    Tier        string `json:"tier"`         // "outcome" or "security"
    Passed      bool   `json:"passed"`
    Message     string `json:"message,omitempty"`
}
```

### 3.3 Assertion Evaluator Signatures (`runner_expectations.go`)

```go
// evaluateOutcomeExpectations evaluates hard goal-achievement assertions.
// Returns error if any outcome assertion fails.
func evaluateOutcomeExpectations(
    spec OutcomeSpec,
    workspace, output string,
    changed []string,
    snapshot *core.ContextSnapshot,
    tokenUsage TokenUsageReport,
    memoryOutcome MemoryOutcomeReport,
) ([]AssertionResult, error)

// evaluateSecurityExpectations evaluates hard sandbox boundary assertions.
// Requires the loaded manifest and the audit log from the run.
// Returns error if any security assertion fails (unexpected boundary violation).
func evaluateSecurityExpectations(
    spec SecuritySpec,
    manifest *manifest.AgentManifest,
    toolTranscript *ToolTranscriptArtifact,
    auditLog []core.AuditRecord,
    snapshot *core.ContextSnapshot,
    toolCalls map[string]int,
) ([]AssertionResult, []SecurityObservation, error)

// evaluateBenchmarkExpectations evaluates soft telemetry observations.
// Never returns an error; always returns observations.
func evaluateBenchmarkExpectations(
    spec BenchmarkSpec,
    toolCalls map[string]int,
    toolTranscript *ToolTranscriptArtifact,
    events []core.Event,
    snapshot *core.ContextSnapshot,
    tokenUsage TokenUsageReport,
    coverage *CapabilityCoverage,
) []BenchmarkObservation
```

---

## 4. Security Evaluation Architecture

### 4.1 Data Sources

The security evaluator needs three data sources that are already available in the runner:

1. **Agent manifest** (`manifest.AgentManifest`): Already loaded in `runner_case.go:47`. Contains the declared `PermissionSet` (filesystem paths, executables, network endpoints).

2. **Tool transcript** (`ToolTranscriptArtifact`): Already built in `runner_case.go` via `BuildToolTranscript(events)`. Contains every tool call with its `CallMetadata` (includes `path` for file operations, `binary` for exec, `url` for network).

3. **Audit log** (`[]core.AuditRecord`): The `PermissionManager` writes to an `InMemoryAuditLogger`. This needs to be **surfaced from the agent bootstrap chain** to the runner's assertion phase. Currently this logger exists but is not returned to the test runner. A new `AuditLogFromBootstrap(boot appruntime.AgentBootstrapResult) []core.AuditRecord` extractor function is needed.

### 4.2 Scope Violation Detection Algorithm

For `no_writes_outside_scope: true`:

```
For each ToolTranscriptEntry where tool ∈ {file_write, file_create, file_delete}:
    path = entry.CallMetadata["path"] or entry.CallMetadata["args"]["path"]
    Normalize path to absolute using workspace root
    Check if any manifest.Spec.Permissions.FileSystem[i] covers this path:
        - Action ∈ {fs:write, fs:delete}
        - Path glob matches the normalized path
    If no manifest rule covers it → SecurityObservation{InScope: false, Blocked: false}
    
For each AuditRecord where Action == "file_access" and Result == "denied":
    → SecurityObservation{InScope: false, Blocked: true}
```

For `mutation_enforced: true` (profile-level constraint):

```
Read snapshot["euclo.execution_profile_selection"]["mutation_allowed"] (bool)
If mutation_allowed == false:
    count file_write + file_create + file_delete calls from toolCalls map
    If count > 0:
        If security.tools_must_not_call does not already cover these:
            → AssertionResult{Tier: "security", Passed: false,
                             Message: "mutation_allowed:false profile but N mutation tool calls observed"}
```

### 4.3 Manifest Permission Matching

A new utility function for the runner:

```go
// ManifestCoversFileAction returns true if the manifest explicitly permits
// the given action on the given path. Path may be absolute or relative to workspace.
func ManifestCoversFileAction(
    m *manifest.AgentManifest,
    action core.FileSystemAction,
    path, workspace string,
) bool

// ManifestCoversExecutable returns true if the manifest declares the given binary.
func ManifestCoversExecutable(m *manifest.AgentManifest, binary string) bool

// ManifestCoversNetworkCall returns true if the manifest declares the given host:port.
func ManifestCoversNetworkCall(m *manifest.AgentManifest, host string, port int) bool
```

These functions canonicalize paths and evaluate manifest glob patterns. They will be located in a new file `testsuite/agenttest/manifest_scope.go`.

---

## 5. Benchmark Observation Architecture

### 5.1 Observation Categories

| Category | Fields captured |
|----------|----------------|
| `tool_usage` | tools_expected, tools_not_expected, tool_sequence, tool_success_rate, tool_recovery |
| `euclo_routing` | All EucloBenchmarkSpec fields |
| `token_usage` | token_budget fields vs. actual TokenUsageReport |
| `performance` | latency, total_tool_time, llm_calls, determinism |

### 5.2 Observation Format

Each observation is one row in `CaseReport.BenchmarkObservations`. The `Field` is a dotted path matching the YAML structure:

```
euclo.behavior_family            → expected "stale_assumption_detection", actual "tension_assessment", matched: false
euclo.recipe_ids[0]              → expected "debug.investigate-repair.reproduce", actual "debug.investigate-repair.reproduce", matched: true
tool_usage.tools_expected[1]     → expected tool "go_test" to be observed, actual: not observed, matched: false
token_usage.max_total            → expected ≤58000, actual 61234, matched: false
```

### 5.3 Report Aggregation

`SuiteReport` gains a `BenchmarkSummary` that aggregates across cases:

```go
type BenchmarkSummary struct {
    TotalObservations   int                          `json:"total_observations"`
    MatchedObservations int                          `json:"matched_observations"`
    MatchRate           float64                      `json:"match_rate"`         // matched/total
    ByCategory          map[string]CategorySummary   `json:"by_category"`
    ByField             map[string]FieldSummary      `json:"by_field"`
    WorstFields         []FieldSummary               `json:"worst_fields"`       // lowest match rate
}

type CategorySummary struct {
    Total   int     `json:"total"`
    Matched int     `json:"matched"`
    Rate    float64 `json:"rate"`
}

type FieldSummary struct {
    Field         string   `json:"field"`
    Total         int      `json:"total"`
    Matched       int      `json:"matched"`
    Rate          float64  `json:"rate"`
    ActualValues  []string `json:"actual_values,omitempty"` // distinct observed values
}
```

This enables a `dev-agent-cli agenttest benchmark` report that shows which fields diverge most from expectations across a suite run, directly supporting model comparison workflows.

---

## 6. Migration Tooling Architecture

### 6.1 `agenttest-migrate` CLI Tool

A new Go binary located at `app/agenttest-migrate/` (or as a subcommand of `dev-agent-cli`):

```
dev-agent-cli agenttest migrate [flags] <suite-file-or-dir>

Flags:
  --dry-run           Print transformed YAML without writing
  --diff              Show unified diff of before/after
  --batch             Process all *.testsuite.yaml in the given directory
  --backup            Write .bak before overwriting
  --strict-security   Classify all tool_calls_must_exclude as security (not benchmark)
  --verify            After writing, parse the output YAML and validate schema
```

### 6.2 Migration Rule Engine

The migration tool applies deterministic classification rules per the field classification table in section 2.2. For ambiguous cases (e.g., `tool_calls_must_exclude: [go_test]`), it uses a configurable rule set:

```go
var securityToolNames = map[string]bool{
    "file_write":  true,
    "file_create": true,
    "file_delete": true,
    "file_move":   true,
    // shell, exec tools added here
}

// classifyToolExclusion returns "security" or "benchmark" for a tool exclusion.
func classifyToolExclusion(toolName string, caseTaskType string, eucloBehaviorFamily string) string {
    if securityToolNames[toolName] {
        return "security"
    }
    return "benchmark"
}
```

### 6.3 Validation

After each file is written, the tool runs `suite.LoadSuite(path)` and `validateOSBSchema(suite)` to verify the output parses correctly and no old/new fields conflict.

---

## 7. Implementation Phases

---

### Phase 1 — Type System and Schema

**Goal:** Define all new Go types and YAML schema without changing any runner behavior. All existing tests continue to pass unchanged.

**Deliverables:**
- New structs in `suite.go`: `OutcomeSpec`, `SecuritySpec`, `BenchmarkSpec`, `EucloBenchmarkSpec`, `TokenBudgetHint`, `ExpectedViolation`
- New structs in `runner.go`: `SecurityObservation`, `BenchmarkObservation`, `AssertionResult`, `BenchmarkSummary`, `CategorySummary`, `FieldSummary`
- Updated `ExpectSpec` in `suite.go`: add `Outcome *OutcomeSpec`, `Security *SecuritySpec`, `Benchmark *BenchmarkSpec` fields alongside existing fields (not replacing them)
- Updated `CaseReport` in `runner.go`: add `SecurityObservations`, `BenchmarkObservations`, `AssertionResults` fields
- Updated `SuiteReport` in `runner.go`: add `BenchmarkSummary *BenchmarkSummary`
- New file `testsuite/agenttest/manifest_scope.go`: `ManifestCoversFileAction`, `ManifestCoversExecutable`, `ManifestCoversNetworkCall` utilities
- YAML round-trip: all new struct fields use `yaml:",omitempty"` so existing YAML without the new blocks parses and serializes correctly

**Unit tests to implement** (`testsuite/agenttest/schema_test.go`):
- `TestOutcomeSpecRoundTrip` — marshal/unmarshal OutcomeSpec preserves all fields
- `TestSecuritySpecRoundTrip` — marshal/unmarshal SecuritySpec preserves all fields
- `TestBenchmarkSpecRoundTrip` — marshal/unmarshal BenchmarkSpec and EucloBenchmarkSpec
- `TestExpectSpecBackwardCompat` — existing test YAML parses without error; Outcome/Security/Benchmark are nil
- `TestManifestCoversFileAction` — covers declared path, rejects undeclared path, handles glob patterns
- `TestManifestCoversExecutable` — covers declared binary, rejects undeclared
- `TestManifestCoversNetworkCall` — covers declared host:port, rejects undeclared
- `TestExpectedViolationParsing` — YAML with expected_violations parses correctly

**Exit criteria:**
- `go build ./testsuite/agenttest/...` passes
- `go test ./testsuite/agenttest/... -run TestSchema` passes
- `go test ./testsuite/agenttest/... -run TestManifest` passes
- All existing suite YAML files parse without error with the new schema
- `go test ./testsuite/agenttest/...` (full suite, unit tests only) passes

---

### Phase 2 — Outcome Evaluation

**Goal:** Implement `evaluateOutcomeExpectations` and wire it into `runCase`. When the `outcome:` block is present in a case, use the new evaluator. When absent, fall through to the existing `evaluateExpectations`. Both paths produce `AssertionResult` records.

**Deliverables:**
- New function `evaluateOutcomeExpectations` in `runner_expectations.go`
- Equivalent logic to existing assertions: `must_succeed`, `no_file_changes`, `files_changed`, `files_contain`, `output_contains`, `output_regex`, `state_key_not_empty`, `state_keys_must_exist`, `memory_records_created`, `workflow_state_updated`
- New: `euclo_mode` assertion (reads `snapshot["euclo.mode"]` or equivalent path from context)
- New: `files_contain.not_contains` complement field evaluation
- Runner wiring in `runner_case.go`: if `expect.Outcome != nil`, call `evaluateOutcomeExpectations` and populate `CaseReport.AssertionResults`; old `evaluateExpectations` still runs if `expect.Outcome == nil` (backward compat)
- Each assertion produces one `AssertionResult{AssertionID: "outcome.must_succeed", Tier: "outcome", ...}`
- Updated `runner_case.go` to write `AssertionResults` to `{artifacts_dir}/assertion_results.json`

**Unit tests to implement** (`testsuite/agenttest/outcome_test.go`):
- `TestOutcomeMustSucceed_Pass` — result.Success true, assertion passes
- `TestOutcomeMustSucceed_Fail` — result.Success false, assertion fails
- `TestOutcomeNoFileChanges_Pass` — changed == nil, assertion passes
- `TestOutcomeNoFileChanges_Fail` — changed has entries, assertion fails
- `TestOutcomeFilesChanged_AllPresent` — all expected files in changed, passes
- `TestOutcomeFilesChanged_Missing` — one file missing, fails
- `TestOutcomeFilesChanged_GlobPattern` — wildcard pattern matching works
- `TestOutcomeFilesContain_Match` — file exists and contains expected string
- `TestOutcomeFilesContain_Missing` — expected string absent, fails
- `TestOutcomeFilesContain_NotContains` — not_contains string present, fails
- `TestOutcomeOutputContains_Match/Fail`
- `TestOutcomeOutputRegex_Match/Fail/InvalidRegex`
- `TestOutcomeStateKeyNotEmpty_Present/Empty/Missing`
- `TestOutcomeEucloMode_Match/Mismatch` — snapshot has euclo mode, assertion checks it
- `TestOutcomeAssertionResultIDs` — each assertion produces unique AssertionID

**Exit criteria:**
- All new unit tests pass
- `go test ./testsuite/agenttest/... -run TestOutcome` passes
- Running the existing `euclo.baseline.support` suite against a live agent: result is identical to before (same pass/fail outcome, backward compat confirmed)
- `assertion_results.json` artifact is written for cases that use the new `outcome:` block

---

### Phase 3 — Security Boundary Evaluation

**Goal:** Implement `evaluateSecurityExpectations`. This phase requires surfacing the audit log from the agent bootstrap chain to the runner.

**Deliverables:**
- `testsuite/agenttest/manifest_scope.go`: manifest permission matching utilities (specified in section 4.3)
- `testsuite/agenttest/audit_bridge.go`: new file
  - `extractAuditLog(boot appruntime.AgentBootstrapResult) []core.AuditRecord` — reads from the `InMemoryAuditLogger` created during bootstrap
  - This requires a small change to `ayenitd.BootstrapAgentRuntime` or its return type to expose the audit logger; OR the runner can retrieve it by casting from the PermissionManager. **Design decision**: add `AuditLog []core.AuditRecord` to `appruntime.AgentBootstrapResult` struct, populated after execution.
- New function `evaluateSecurityExpectations` in `runner_expectations.go` implementing the algorithms from section 4.2
- Runner wiring in `runner_case.go`:
  - After execution, extract audit records via `extractAuditLog`
  - If `expect.Security != nil`, call `evaluateSecurityExpectations`
  - Append `SecurityObservation` records (both violation and grant) to `CaseReport.SecurityObservations`
  - Write `{artifacts_dir}/security_observations.json`
  - Security assertion failures are prepended to `caseErr` with prefix `[security]`
- `classifyCaseFailure` updated to recognize `[security]` prefix as a new `"security"` failure kind
- `SuiteReport` gains `SecurityFailures int` counter alongside `InfraFailures`, `AssertFailures`

**Unit tests to implement** (`testsuite/agenttest/security_test.go`):
- `TestSecurityNoWritesOutsideScope_CleanRun` — no writes in transcript, assertion passes
- `TestSecurityNoWritesOutsideScope_InScopeWrite` — write within declared path, assertion passes
- `TestSecurityNoWritesOutsideScope_OutOfScopeWrite` — write to undeclared path, assertion fails
- `TestSecurityNoWritesOutsideScope_SandboxBlocked` — write was blocked by sandbox, still a SecurityObservation but test passes (violation was contained)
- `TestSecurityToolsMustNotCall_NotCalled` — tool absent from transcript, passes
- `TestSecurityToolsMustNotCall_Called` — tool present in transcript, fails
- `TestSecurityMutationEnforced_ProfileDisallowedNoWrites` — mutation_allowed:false + no writes, passes
- `TestSecurityMutationEnforced_ProfileDisallowedWithWrites` — mutation_allowed:false + write present, fails
- `TestSecurityMutationEnforced_ProfileAllowedWithWrites` — mutation_allowed:true, no assertion
- `TestSecurityExpectedViolation_Matches` — out-of-scope write is in expected_violations, test still passes
- `TestSecurityNoNetworkOutsideManifest_InManifest` — network call to declared host, passes
- `TestSecurityNoNetworkOutsideManifest_OutOfManifest` — network call to undeclared host, fails
- `TestSecurityObservationFormat` — SecurityObservation fields are populated correctly
- `TestAuditBridge_ExtractsRecords` — extractAuditLog returns correct records from InMemoryAuditLogger
- `TestManifestScopeGlob_Workspace` — `${workspace}/**` glob expansion works correctly

**Exit criteria:**
- All new unit tests pass
- `go test ./testsuite/agenttest/... -run TestSecurity` passes
- `go test ./testsuite/agenttest/... -run TestAuditBridge` passes
- A manually-written test case with `security.tools_must_not_call: [file_write]` and a run that calls `file_write` produces `FailureKind: "security"` in the report
- A clean run with no violations produces `SecurityObservations: []` in the report
- `security_observations.json` artifact is written for cases that use the `security:` block

---

### Phase 4 — Benchmark Recording

**Goal:** Implement `evaluateBenchmarkExpectations`. Mismatches are never failures; they are structured observations.

**Deliverables:**
- New function `evaluateBenchmarkExpectations` in `runner_expectations.go` implementing section 5
- Observation field path naming follows the dotted convention from section 5.2
- New function `aggregateBenchmarkSummary(cases []CaseReport) BenchmarkSummary` in `runner.go`
- `SuiteReport.BenchmarkSummary` populated after all cases complete
- `{artifacts_dir}/benchmark_observations.json` written per case
- `{output_dir}/benchmark_summary.json` written per suite run (companion to `report.json`)
- `dev-agent-cli agenttest benchmark` subcommand (or `report --benchmark`) that pretty-prints the summary
- Runner wiring: `evaluateBenchmarkExpectations` called if `expect.Benchmark != nil`; observations appended to `CaseReport.BenchmarkObservations`; test pass/fail is NEVER affected

**Unit tests to implement** (`testsuite/agenttest/benchmark_test.go`):
- `TestBenchmarkToolsExpected_Present` — tool in transcript, observation matched:true
- `TestBenchmarkToolsExpected_Absent` — tool not in transcript, observation matched:false, test passes
- `TestBenchmarkToolsNotExpected_Present` — unexpected tool present, observation matched:false, test passes
- `TestBenchmarkToolsNotExpected_Absent` — expected absent tool absent, observation matched:true
- `TestBenchmarkEucloRouting_AllMatch` — all EucloBenchmarkSpec fields match snapshot, all observations matched:true
- `TestBenchmarkEucloRouting_SomeMismatch` — some fields mismatch, observations correct, test still passes
- `TestBenchmarkTokenBudget_Under` — actual tokens under budget, observation matched:true
- `TestBenchmarkTokenBudget_Over` — actual tokens over budget, observation matched:false, test passes
- `TestBenchmarkNeverFails` — construct spec with every field mismatched, confirm no error returned
- `TestBenchmarkObservationFieldPaths` — observation Field values follow dotted convention
- `TestAggregateBenchmarkSummary_Empty` — no observations, summary is zero values
- `TestAggregateBenchmarkSummary_Mixed` — mix of matched/unmatched, rates computed correctly
- `TestAggregateBenchmarkSummary_WorstFields` — WorstFields sorted by ascending match rate

**Exit criteria:**
- All new unit tests pass
- `go test ./testsuite/agenttest/... -run TestBenchmark` passes
- Running the debug suite with `benchmark:` blocks in YAML: test passes regardless of observation match rate
- `benchmark_observations.json` artifact written; `benchmark_summary.json` written at suite level
- `dev-agent-cli agenttest benchmark` subcommand prints readable report

---

### Phase 5 — Full Runner Integration

**Goal:** Wire all three evaluators into a unified assertion pipeline in `runner_case.go`. Both the new OSB path and the legacy path produce `AssertionResult` records. Failure classification is updated. Report JSON is enriched.

**Deliverables:**
- Unified assertion pipeline in `runner_case.go`:
  ```
  if expect.Outcome != nil → evaluateOutcomeExpectations → AssertionResults, error
  else                     → evaluateExpectations (legacy) → error only
  
  if expect.Security != nil → evaluateSecurityExpectations → AssertionResults, SecurityObservations, error
  
  if expect.Benchmark != nil → evaluateBenchmarkExpectations → BenchmarkObservations (no error)
  ```
- Unified error string construction: outcome failures first, security failures second (with `[security]` prefix), joined by `;`
- `classifyCaseFailure` updated: `[security]` prefix → `"security"` kind; existing logic unchanged for other patterns
- `SuiteReport.SecurityFailures` counter incremented for `FailureKind == "security"`
- `report.json` now includes `security_observations`, `benchmark_observations`, `assertion_results` per case
- `SuiteReport.BenchmarkSummary` populated
- All existing `CaseReport` fields remain populated (backward compat for tooling that reads `report.json`)
- Legacy `evaluateExpectations` function preserved (not deleted until Phase 8)

**Integration tests to implement** (`testsuite/agenttest/integration_test.go`):
- `TestRunnerPipeline_OutcomeOnly` — case with only outcome: block; passes and produces AssertionResults
- `TestRunnerPipeline_SecurityOnly` — case with only security: block; violation fails with FailureKind "security"
- `TestRunnerPipeline_BenchmarkOnly` — case with only benchmark: block; passes regardless of observations
- `TestRunnerPipeline_LegacyFallback` — case with no new blocks; old evaluateExpectations runs unchanged
- `TestRunnerPipeline_AllThreeTiers` — case with all three blocks; outcome+security failures, benchmark observations
- `TestRunnerPipeline_SecurityFailureDoesNotAffectBenchmark` — security fails, benchmark still fully evaluated
- `TestRunnerPipeline_ReportJSON` — serialized report.json contains all new fields
- `TestRunnerPipeline_ArtifactFiles` — all new artifact JSON files written to artifacts_dir
- `TestFailureClassification_SecurityKind` — [security] prefix produces FailureKind "security"
- `TestSuiteReport_SecurityFailureCount` — SecurityFailures counter incremented correctly

**Exit criteria:**
- All new integration tests pass
- `go test ./testsuite/agenttest/...` passes
- A complete end-to-end run of `euclo.baseline.support` (the currently-passing suite): identical pass/fail outcome as before phase
- Report artifacts (`assertion_results.json`, `security_observations.json`, `benchmark_observations.json`) written for all cases
- `FailureKind` values in report are one of: `""`, `"infra"`, `"assertion"`, `"agent"`, `"security"`
- Existing CI scripts that parse `report.json` still work (backward compat of existing fields confirmed)

---

### Phase 6 — Migration Tooling

**Goal:** Build the `agenttest migrate` tool that mechanically converts existing YAML files to the new OSB schema. The tool must be deterministic, verifiable, and safe (never silently corrupt a test case).

**Deliverables:**
- New command `app/dev-agent-cli/cmd/agenttest_migrate.go` (or sub-package)
- Migration rule engine: implements the field classification table from section 2.2
- Handles all 57 YAML files and ~280 cases
- Produces well-formed YAML that parses correctly with the new schema
- `--dry-run` and `--diff` modes
- `--backup` creates `.bak` before overwriting
- `--verify` runs schema validation after write
- Migration log: writes `migration_report.json` summarizing fields migrated, warnings, and ambiguous cases
- Ambiguous `tool_calls_must_exclude` entries: the tool uses the classification function (`classifyToolExclusion`); uncertain cases emit a warning in the migration report and default to `benchmark.tools_not_expected`
- Handles the legacy `euclo:` block split: `euclo.mode` → `outcome.euclo_mode`; all other `euclo.*` → `benchmark.euclo.*`
- Idempotent: running migration twice on an already-migrated file produces no changes

**Unit tests to implement** (`app/dev-agent-cli/cmd/agenttest_migrate_test.go`):
- `TestMigrateExpectSpec_MustSucceed` — must_succeed migrates to outcome.must_succeed
- `TestMigrateExpectSpec_FilesChanged` — files_changed, files_contain migrate to outcome
- `TestMigrateExpectSpec_ToolExcludeFileWrite` — file_write in must_exclude → security.tools_must_not_call
- `TestMigrateExpectSpec_ToolExcludeGoTest` — go_test in must_exclude → benchmark.tools_not_expected
- `TestMigrateExpectSpec_ToolInclude` — tool_calls_must_include → benchmark.tools_expected
- `TestMigrateExpectSpec_EucloMode` — euclo.mode → outcome.euclo_mode
- `TestMigrateExpectSpec_EucloTelemetry` — euclo.profile, behavior_family etc. → benchmark.euclo.*
- `TestMigrateExpectSpec_TokenBudget` — max_*_tokens → benchmark.token_budget
- `TestMigrateExpectSpec_LLMCalls` — llm_calls → benchmark.llm_calls_expected
- `TestMigrateFullSuiteFile` — load a complete testsuite YAML, migrate, parse output, validate no data lost
- `TestMigrateIdempotent` — migrate twice, output identical
- `TestMigrateVerifyFlag` — --verify fails on corrupt output (injected by test)
- `TestMigrateBackupFlag` — .bak file created before overwrite
- `TestMigrateDryRun` — no file written in dry-run mode
- `TestMigrationReport` — migration_report.json contains correct counts

**Exit criteria:**
- All migration unit tests pass
- `dev-agent-cli agenttest migrate --dry-run testsuite/agenttests/` runs on all 57 YAML files without errors
- Output of dry-run parses correctly with the new schema for all 57 files
- Migration report shows 0 errors (warnings for ambiguous cases are acceptable)
- Running the tool with `--verify` on each file passes schema validation

---

### Phase 7 — Suite Migration

**Goal:** Apply the migration tool to all 57 YAML suite files. After migration, all previously-passing tests continue to pass. Previously-failing tests that failed due to over-strict assertions (behavioral rather than security/outcome) now pass with benchmark observations.

**Deliverables:**
- All 57 YAML suite files converted to OSB schema
- `.bak` backups created during migration
- Migration report reviewed and annotated for any ambiguous decisions
- Manual review of cases where `tool_calls_must_exclude` was ambiguous (flagged by migration report)
- Verification run: full suite execution against the live system (same model, same workspace) confirming:
  - All previously-passing cases still pass
  - Cases that previously failed due to `tool_calls_must_include`/`euclo.*` mismatches now pass with observations
  - Cases that previously failed due to genuine outcome/security violations still fail

**No new code in this phase.** All changes are YAML only.

**Manual review checklist (per YAML file):**
- [ ] `outcome:` block covers the same assertions as the old `expect:` hard fields
- [ ] `security:` block covers any `tool_calls_must_exclude` entries that are genuine security constraints
- [ ] `benchmark:` block captures all telemetry/routing fields
- [ ] No assertion was accidentally dropped during migration
- [ ] `euclo_mode` assertion is present for any test that previously had `euclo.mode`
- [ ] Test description is updated to reflect the outcome it is actually validating

**Regression test protocol:**
```bash
# 1. Baseline: run all suites on main branch before migration
dev-agent-cli agenttest run --suite euclo.baseline --output baseline_pre.json

# 2. Migrate all YAML files
dev-agent-cli agenttest migrate --backup --verify testsuite/agenttests/

# 3. Post-migration run
dev-agent-cli agenttest run --suite euclo.baseline --output baseline_post.json

# 4. Compare: pre-passing cases must still pass
dev-agent-cli agenttest compare baseline_pre.json baseline_post.json --mode regressions-only
```

**Exit criteria:**
- `dev-agent-cli agenttest migrate --verify` passes for all 57 files
- All previously-passing cases still pass in post-migration runs
- `euclo.baseline.debug` cases 1–3 (investigate and simple_repair) now pass with benchmark observations instead of failing
- Security failures in the report are 0 for all clean runs (no genuine security violations expected)
- `benchmark_summary.json` is produced and readable
- All `.bak` files deleted after successful verification

---

### Phase 8 — Cleanup

**Goal:** Remove all legacy assertion code, old YAML fields, backward-compatibility shims, and dead types. The codebase should contain only the OSB model with no vestigial dual-path code.

**Deliverables:**

**Go cleanup:**
- Remove `evaluateExpectations` (the old monolithic function) from `runner_expectations.go`
- Remove the old `evaluateEucloExpectations` function
- Remove legacy fields from `ExpectSpec` in `suite.go`:
  - `ToolCallsMustInclude`, `ToolCallsMustExclude`, `ToolCallsInOrder`
  - `LLMCalls`, `MaxToolCalls`, `MaxPromptTokens`, `MaxCompletionTokens`, `MaxTotalTokens`
  - `Euclo *EucloExpectSpec` (replaced by `Benchmark.Euclo *EucloBenchmarkSpec` and `Outcome.EucloMode`)
  - `ToolSuccessRate`, `DeterminismScore`, `LLMResponseStable`, `ToolCallLatencyMs`, `MaxTotalToolTimeMs`
  - `ToolsRequired`, `ToolRecoveryObserved`, `ToolDependencies`
- Remove `EucloExpectSpec` struct (replaced by `EucloBenchmarkSpec` + `outcome.euclo_mode`)
- Remove the legacy backward-compat path in `runner_case.go` (`if expect.Outcome == nil { ... old path ... }`)
- Remove `isAssertionFailure` fragment matching for old assertion messages that no longer apply
- Remove any `evaluateExpectations`-related test helpers

**YAML cleanup:**
- Delete all `.bak` files from Phase 7
- Remove any remaining legacy `expect:` fields that the migration tool left in place as comments
- Final `--verify` pass to confirm no legacy fields remain

**Documentation:**
- Update `testsuite/agenttest/README.md` (or create it) with the OSB model explanation, YAML reference, and migration notes
- Update any CI scripts that classify failures by kind to recognize `"security"` failure kind
- Update `CLAUDE.md` testsuite section to reference the new structure

**Final validation tests** (`testsuite/agenttest/cleanup_test.go`):
- `TestNoLegacyFieldsInSuiteYAML` — parse all YAML files; assert `ExpectSpec.ToolCallsMustInclude == nil`, `ExpectSpec.Euclo == nil`, etc. — all legacy fields are empty
- `TestNoLegacyEvaluatorCode` — compile-time: import `runner_expectations.go`; assert `evaluateExpectations` is not exported and not callable (enforced by deletion)
- `TestOSBFieldsPopulated` — for every case in every YAML file that has an `expect:` block, at least one of `outcome:`, `security:`, `benchmark:` is non-nil
- `TestReportSchemaStability` — serialize a CaseReport with all new fields; deserialize; round-trip is lossless

**Exit criteria:**
- `go build ./testsuite/agenttest/...` passes with no legacy types referenced
- `go test ./testsuite/agenttest/...` passes (all unit + integration tests)
- `TestNoLegacyFieldsInSuiteYAML` passes (all 57 files are clean)
- `go vet ./testsuite/...` produces no warnings
- All previously-passing live test cases still pass in a full suite run
- CI pipeline green on the cleaned branch

---

## 8. Phase Dependency Graph

```
Phase 1: Type System
    └── Phase 2: Outcome Evaluation
        └── Phase 5: Runner Integration ←─ Phase 3: Security Evaluation
                                        ←─ Phase 4: Benchmark Recording
    └── Phase 3: Security Evaluation
    └── Phase 4: Benchmark Recording

Phase 5: Runner Integration
    └── Phase 6: Migration Tooling (depends on final schema stability)
        └── Phase 7: Suite Migration
            └── Phase 8: Cleanup
```

Phases 2, 3, 4 are independent of each other and can proceed in parallel after Phase 1 completes. Phase 5 depends on all three. Phases 6–8 are strictly sequential.

---

## 9. Files Changed Summary

| File | Phase | Change Type |
|------|-------|-------------|
| `testsuite/agenttest/suite.go` | 1 | Add OutcomeSpec, SecuritySpec, BenchmarkSpec, ExpectedViolation, TokenBudgetHint, EucloBenchmarkSpec |
| `testsuite/agenttest/runner.go` | 1 | Add SecurityObservation, BenchmarkObservation, AssertionResult, BenchmarkSummary to report types |
| `testsuite/agenttest/manifest_scope.go` | 1 (new) | ManifestCoversFileAction, ManifestCoversExecutable, ManifestCoversNetworkCall |
| `testsuite/agenttest/runner_expectations.go` | 2,3,4 | Add evaluateOutcomeExpectations, evaluateSecurityExpectations, evaluateBenchmarkExpectations |
| `testsuite/agenttest/audit_bridge.go` | 3 (new) | extractAuditLog; bridge from bootstrap to runner |
| `app/relurpish/runtime/bootstrap.go` | 3 | Surface audit log in AgentBootstrapResult |
| `testsuite/agenttest/runner_case.go` | 5 | Wire all three evaluators; populate new report fields; write new artifact files |
| `testsuite/agenttest/runner.go` | 5 | Populate BenchmarkSummary; add SecurityFailures counter; write benchmark_summary.json |
| `testsuite/agenttest/failure_classification.go` | 5 | Add "security" failure kind; update isAssertionFailure fragments |
| `app/dev-agent-cli/cmd/agenttest_migrate.go` | 6 (new) | Migration CLI tool |
| `testsuite/agenttests/*.yaml` (57 files) | 7 | Convert to OSB schema |
| `testsuite/agenttest/suite.go` | 8 | Remove legacy ExpectSpec fields and EucloExpectSpec |
| `testsuite/agenttest/runner_expectations.go` | 8 | Remove evaluateExpectations, evaluateEucloExpectations |
| `testsuite/agenttest/runner_case.go` | 8 | Remove legacy fallback path |
| `testsuite/agenttest/failure_classification.go` | 8 | Remove stale isAssertionFailure fragments |

---

## 10. Key Design Constraints and Non-Goals

**Constraints:**
- The `manifest.AgentManifest` loaded for a test case is the ground truth for security assertions. Security assertions never invent rules that aren't reflected in the manifest.
- Benchmark observations do not affect test outcome under any circumstances. No configuration flag, no threshold, no mode makes benchmark observations into failures.
- Legacy YAML (all existing test files) must parse and run without error throughout Phases 1–6. Backward compat is never broken before Phase 8.
- The `extractAuditLog` bridge must not add overhead to production agent paths — audit logging is already in-memory and zero-cost during non-test operation.

**Non-goals:**
- This redesign does not change how agents are executed, how capabilities are dispatched, or how the framework enforces permissions at runtime. It only changes how the test runner observes and categorizes the results.
- This redesign does not add replay/determinism testing (that concern belongs to the existing `consistency.go` / `determinism.go` infrastructure).
- This redesign does not change the `report.json` schema version. New fields are additive.
