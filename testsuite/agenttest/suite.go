package agenttest

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"gopkg.in/yaml.v3"
)

type Suite struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   SuiteMeta `yaml:"metadata"`
	Spec       SuiteSpec `yaml:"spec"`
	SourcePath string    `yaml:"-"`
}

type SuiteMeta struct {
	Name           string        `yaml:"name"`
	Description    string        `yaml:"description,omitempty"`
	Owner          string        `yaml:"owner,omitempty"`
	Tier           string        `yaml:"tier,omitempty"`
	Classification string        `yaml:"classification,omitempty"`
	Benchmark      BenchmarkMeta `yaml:"benchmark,omitempty"`
	Quarantined    bool          `yaml:"quarantined"`
}

type BenchmarkMeta struct {
	ScoreFamily       string   `yaml:"score_family,omitempty"`
	ScoreDimensions   []string `yaml:"score_dimensions,omitempty"`
	ComparisonWindow  string   `yaml:"comparison_window,omitempty"`
	VarianceThreshold float64  `yaml:"variance_threshold,omitempty"`
}

type SuiteSpec struct {
	AgentName string `yaml:"agent_name"`
	Manifest  string `yaml:"manifest"`

	Execution SuiteExecutionSpec `yaml:"execution,omitempty"`
	Workspace WorkspaceSpec      `yaml:"workspace"`
	Memory    MemorySpec         `yaml:"memory,omitempty"`
	Models    []ModelSpec        `yaml:"models,omitempty"`
	Providers []ProviderSpec     `yaml:"providers,omitempty"`
	Recording RecordingSpec      `yaml:"recording,omitempty"`
	Cases     []CaseSpec         `yaml:"cases"`
}

type SuiteExecutionSpec struct {
	Profile     string `yaml:"profile,omitempty"`
	Strict      bool   `yaml:"strict,omitempty"`
	Timeout     string `yaml:"timeout,omitempty"`
	MatrixOrder string `yaml:"matrix_order,omitempty"`
}

type WorkspaceSpec struct {
	Strategy        string          `yaml:"strategy,omitempty"` // derived
	TemplateProfile string          `yaml:"template_profile,omitempty"`
	Exclude         []string        `yaml:"exclude,omitempty"`
	IgnoreChanges   []string        `yaml:"ignore_changes,omitempty"`
	Files           []SetupFileSpec `yaml:"files,omitempty"`
}

type ModelSpec struct {
	Name          string `yaml:"name"`
	Endpoint      string `yaml:"endpoint,omitempty"`
	Provider      string `yaml:"provider,omitempty"`
	ResetStrategy string `yaml:"reset_strategy,omitempty"`
	ResetBetween  bool   `yaml:"reset_between,omitempty"`
}

type ProviderSpec struct {
	Name          string `yaml:"name"`
	Endpoint      string `yaml:"endpoint,omitempty"`
	ResetStrategy string `yaml:"reset_strategy,omitempty"`
	ResetBetween  bool   `yaml:"reset_between,omitempty"`
}

type RecordingSpec struct {
	Strategy string `yaml:"strategy,omitempty"` // live|replay-if-golden|replay-only
	Mode     string `yaml:"mode,omitempty"`     // off|record|replay
	Tape     string `yaml:"tape,omitempty"`
}

type CaseSpec struct {
	Name              string                        `yaml:"name"`
	Description       string                        `yaml:"description,omitempty"`
	Timeout           string                        `yaml:"timeout,omitempty"`
	TaskType          string                        `yaml:"task_type,omitempty"`
	Prompt            string                        `yaml:"prompt"`
	InteractionScript []InteractionScriptStep       `yaml:"interaction_script,omitempty"`
	Context           map[string]any                `yaml:"context,omitempty"`
	Metadata          map[string]string             `yaml:"metadata,omitempty"`
	BrowserFixtures   map[string]BrowserFixtureSpec `yaml:"browser_fixtures,omitempty"`
	Setup             SetupSpec                     `yaml:"setup,omitempty"`
	Requires          RequiresSpec                  `yaml:"requires,omitempty"`
	Expect            ExpectSpec                    `yaml:"expect,omitempty"`
	Overrides         CaseOverrideSpec              `yaml:"overrides,omitempty"`
	Tags              []string                      `yaml:"tags,omitempty"`
	// Phase 4: CapabilityDirectRun bypasses full agent loop for direct capability testing
	CapabilityDirectRun *CapabilityDirectRunSpec `yaml:"capability_direct_run,omitempty"`
}

type InteractionScriptStep struct {
	Phase  string `yaml:"phase,omitempty"`
	Kind   string `yaml:"kind,omitempty"`
	Action string `yaml:"action"`
	Text   string `yaml:"text,omitempty"`
}

type BrowserFixtureSpec struct {
	Path        string            `yaml:"path,omitempty"`
	File        string            `yaml:"file,omitempty"`
	Content     string            `yaml:"content,omitempty"`
	ContentType string            `yaml:"content_type,omitempty"`
	Status      int               `yaml:"status,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty"`
}

type RequiresSpec struct {
	Executables []string `yaml:"executables,omitempty"`
	Tools       []string `yaml:"tools,omitempty"`
	// NEW: ToolsAvailable checks if tools exist in registry before running (fails fast)
	ToolsAvailable []string `yaml:"tools_available,omitempty"`
	// NEW: ToolsRequired ensures tools are actually invoked during test
	ToolsRequired []string `yaml:"tools_required,omitempty"`
	// NEW: ToolsDisable removes tools from the registry so agent cannot use them
	ToolsDisable []string `yaml:"tools_disable,omitempty"`
}

type SetupSpec struct {
	Files         []SetupFileSpec        `yaml:"files,omitempty"`
	GitInit       bool                   `yaml:"git_init,omitempty"`
	Memory        MemorySeedSpec         `yaml:"memory,omitempty"`
	Workflows     []WorkflowSeedSpec     `yaml:"workflows,omitempty"`
	ToolOverrides []ToolResponseOverride `yaml:"tool_overrides,omitempty"`
	// Phase 4: StateKeys injects values into core.Context before agent execution
	StateKeys map[string]any `yaml:"state_keys,omitempty"`
}

// CapabilityDirectRunSpec defines direct capability invocation for testing supporting-only capabilities
type CapabilityDirectRunSpec struct {
	CapabilityID    string `yaml:"capability_id"`
	InvokingPrimary string `yaml:"invoking_primary,omitempty"`
}

type SetupFileSpec struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
	Mode    string `yaml:"mode,omitempty"`
}

type ExpectSpec struct {
	MustSucceed bool `yaml:"must_succeed,omitempty"`

	OutputContains []string                 `yaml:"output_contains,omitempty"`
	OutputRegex    []string                 `yaml:"output_regex,omitempty"`
	FilesContain   []FileContentExpectation `yaml:"files_contain,omitempty"`

	NoFileChanges bool     `yaml:"no_file_changes,omitempty"`
	FilesChanged  []string `yaml:"files_changed,omitempty"`

	MemoryRecordsCreated int      `yaml:"memory_records_created,omitempty"`
	WorkflowStateUpdated bool     `yaml:"workflow_state_updated,omitempty"`
	StateKeysMustExist   []string `yaml:"state_keys_must_exist,omitempty"`
	StateKeysNotEmpty    []string `yaml:"state_key_not_empty,omitempty"`
	WorkflowHasTensions  []string `yaml:"workflow_has_tensions,omitempty"`

	// OSB Model: Outcome, Security, Benchmark blocks (Phase 1)
	Outcome   *OutcomeSpec   `yaml:"outcome,omitempty"`
	Security  *SecuritySpec  `yaml:"security,omitempty"`
	Benchmark *BenchmarkSpec `yaml:"benchmark,omitempty"`
}

type ArtifactChainSpec struct {
	Kind            string   `yaml:"kind"`
	ProducedByPhase string   `yaml:"produced_by_phase,omitempty"`
	ConsumedByPhase string   `yaml:"consumed_by_phase,omitempty"`
	ContentContains []string `yaml:"content_contains,omitempty"`
}

type FileContentExpectation struct {
	Path        string   `yaml:"path"`
	Contains    []string `yaml:"contains,omitempty"`
	NotContains []string `yaml:"not_contains,omitempty"`
}

// OutcomeSpec defines hard pass/fail assertions about goal achievement.
type OutcomeSpec struct {
	MustSucceed          bool                     `yaml:"must_succeed,omitempty"`
	NoFileChanges        bool                     `yaml:"no_file_changes,omitempty"`
	FilesChanged         []string                 `yaml:"files_changed,omitempty"`
	FilesContain         []FileContentExpectation `yaml:"files_contain,omitempty"`
	OutputContains       []string                 `yaml:"output_contains,omitempty"`
	OutputRegex          []string                 `yaml:"output_regex,omitempty"`
	StateKeyNotEmpty     []string                 `yaml:"state_key_not_empty,omitempty"`
	StateKeysMustExist   []string                 `yaml:"state_keys_must_exist,omitempty"`
	MemoryRecordsCreated int                      `yaml:"memory_records_created,omitempty"`
	WorkflowStateUpdated bool                     `yaml:"workflow_state_updated,omitempty"`
	EucloMode            string                   `yaml:"euclo_mode,omitempty"`
}

// SecuritySpec defines hard pass/fail assertions about sandbox contract enforcement.
// Assertions here are cross-referenced against the agent manifest's PermissionSet.
type SecuritySpec struct {
	// Filesystem scope
	NoWritesOutsideScope bool `yaml:"no_writes_outside_scope,omitempty"`
	NoReadsOutsideScope  bool `yaml:"no_reads_outside_scope,omitempty"`

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
	ToolsExpected        []string         `yaml:"tools_expected,omitempty"`
	ToolsNotExpected     []string         `yaml:"tools_not_expected,omitempty"`
	ToolSequenceExpected []string         `yaml:"tool_sequence_expected,omitempty"`
	ToolSuccessRate      map[string]int   `yaml:"tool_success_rate,omitempty"`
	ToolCallLatencyMs    map[string]int   `yaml:"tool_call_latency_ms,omitempty"`
	ToolDependencies     []ToolDependency `yaml:"tool_dependencies,omitempty"`
	ToolRecoveryObserved bool             `yaml:"tool_recovery_observed,omitempty"`

	// LLM usage hints
	LLMCallsExpected       int    `yaml:"llm_calls_expected,omitempty"`
	MaxToolCallsHint       int    `yaml:"max_tool_calls_hint,omitempty"`
	MaxTotalToolTimeHintMs int    `yaml:"max_total_tool_time_hint_ms,omitempty"`
	LLMResponseStableHint  bool   `yaml:"llm_response_stable_hint,omitempty"`
	DeterminismScoreHint   string `yaml:"determinism_score_hint,omitempty"`

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
	BehaviorFamily                 string              `yaml:"behavior_family,omitempty"`
	Profile                        string              `yaml:"profile,omitempty"`
	PrimaryRelurpicCapability      string              `yaml:"primary_relurpic_capability,omitempty"`
	SupportingRelurpicCapabilities []string            `yaml:"supporting_relurpic_capabilities,omitempty"`
	SpecializedCapabilityIDs       []string            `yaml:"specialized_capability_ids,omitempty"`
	RecipeIDs                      []string            `yaml:"recipe_ids,omitempty"`
	ArtifactsProduced              []string            `yaml:"artifacts_produced,omitempty"`
	PhasesExecuted                 []string            `yaml:"phases_executed,omitempty"`
	PhasesSkipped                  []string            `yaml:"phases_skipped,omitempty"`
	ResultClass                    string              `yaml:"result_class,omitempty"`
	AssuranceClass                 string              `yaml:"assurance_class,omitempty"`
	RecoveryStatus                 string              `yaml:"recovery_status,omitempty"`
	RecoveryAttempted              bool                `yaml:"recovery_attempted,omitempty"`
	DegradationMode                string              `yaml:"degradation_mode,omitempty"`
	SuccessGateReason              string              `yaml:"success_gate_reason,omitempty"`
	MinTransitionsProposed         int                 `yaml:"min_transitions_proposed,omitempty"`
	MaxTransitionsProposed         int                 `yaml:"max_transitions_proposed,omitempty"`
	MinFramesEmitted               int                 `yaml:"min_frames_emitted,omitempty"`
	MaxFramesEmitted               int                 `yaml:"max_frames_emitted,omitempty"`
	FrameKindsEmitted              []string            `yaml:"frame_kinds_emitted,omitempty"`
	FrameKindsNotExpected          []string            `yaml:"frame_kinds_not_expected,omitempty"`
	FrameKindsMustExclude          []string            `yaml:"frame_kinds_must_exclude,omitempty"`
	ArtifactChain                  []ArtifactChainSpec `yaml:"artifact_chain,omitempty"`
	ArtifactKindProduced           []string            `yaml:"artifact_kind_produced,omitempty"`
	RecoveryStrategies             []string            `yaml:"recovery_strategies,omitempty"`
}

type CaseOverrideSpec struct {
	MaxIterations        int                       `yaml:"max_iterations,omitempty"`
	BootstrapTimeout     string                    `yaml:"bootstrap_timeout,omitempty"`
	Model                *ModelSpec                `yaml:"model,omitempty"`
	Recording            *RecordingSpec            `yaml:"recording,omitempty"`
	Workspace            *WorkspaceSpec            `yaml:"workspace,omitempty"`
	Memory               *MemorySpec               `yaml:"memory,omitempty"`
	ExtraEnv             map[string]string         `yaml:"extra_env,omitempty"`
	AllowedCapabilities  []core.CapabilitySelector `yaml:"allowed_capabilities,omitempty"`
	RestrictCapabilities bool                      `yaml:"restrict_capabilities,omitempty"`
	ControlFlow          string                    `yaml:"control_flow,omitempty"`
}

type MemorySpec struct {
	Backend   string              `yaml:"backend,omitempty"` // hybrid|sqlite_runtime
	Retrieval MemoryRetrievalSpec `yaml:"retrieval,omitempty"`
}

type MemoryRetrievalSpec struct {
	Embedder string `yaml:"embedder,omitempty"` // "", "test"
}

type MemorySeedSpec struct {
	Declarative []DeclarativeMemorySeedSpec `yaml:"declarative,omitempty"`
	Procedural  []ProceduralMemorySeedSpec  `yaml:"procedural,omitempty"`
}

type DeclarativeMemorySeedSpec struct {
	RecordID    string         `yaml:"record_id"`
	Scope       string         `yaml:"scope,omitempty"`
	Kind        string         `yaml:"kind,omitempty"`
	Title       string         `yaml:"title,omitempty"`
	Content     string         `yaml:"content,omitempty"`
	Summary     string         `yaml:"summary,omitempty"`
	WorkflowID  string         `yaml:"workflow_id,omitempty"`
	TaskID      string         `yaml:"task_id,omitempty"`
	ProjectID   string         `yaml:"project_id,omitempty"`
	ArtifactRef string         `yaml:"artifact_ref,omitempty"`
	Tags        []string       `yaml:"tags,omitempty"`
	Metadata    map[string]any `yaml:"metadata,omitempty"`
	Verified    bool           `yaml:"verified,omitempty"`
}

type ProceduralMemorySeedSpec struct {
	RoutineID              string                    `yaml:"routine_id"`
	Scope                  string                    `yaml:"scope,omitempty"`
	Kind                   string                    `yaml:"kind,omitempty"`
	Name                   string                    `yaml:"name,omitempty"`
	Description            string                    `yaml:"description,omitempty"`
	Summary                string                    `yaml:"summary,omitempty"`
	WorkflowID             string                    `yaml:"workflow_id,omitempty"`
	TaskID                 string                    `yaml:"task_id,omitempty"`
	ProjectID              string                    `yaml:"project_id,omitempty"`
	BodyRef                string                    `yaml:"body_ref,omitempty"`
	InlineBody             string                    `yaml:"inline_body,omitempty"`
	CapabilityDependencies []core.CapabilitySelector `yaml:"capability_dependencies,omitempty"`
	VerificationMetadata   map[string]any            `yaml:"verification_metadata,omitempty"`
	PolicySnapshotID       string                    `yaml:"policy_snapshot_id,omitempty"`
	Verified               bool                      `yaml:"verified,omitempty"`
	Version                int                       `yaml:"version,omitempty"`
	ReuseCount             int                       `yaml:"reuse_count,omitempty"`
}

type WorkflowSeedSpec struct {
	Workflow    WorkflowRecordSeedSpec       `yaml:"workflow"`
	Runs        []WorkflowRunSeedSpec        `yaml:"runs,omitempty"`
	Knowledge   []WorkflowKnowledgeSeedSpec  `yaml:"knowledge,omitempty"`
	Checkpoints []WorkflowCheckpointSeedSpec `yaml:"checkpoints,omitempty"`
}

type WorkflowRecordSeedSpec struct {
	WorkflowID   string         `yaml:"workflow_id"`
	TaskID       string         `yaml:"task_id,omitempty"`
	TaskType     string         `yaml:"task_type,omitempty"`
	Instruction  string         `yaml:"instruction,omitempty"`
	Status       string         `yaml:"status,omitempty"`
	CursorStepID string         `yaml:"cursor_step_id,omitempty"`
	Metadata     map[string]any `yaml:"metadata,omitempty"`
}

type WorkflowRunSeedSpec struct {
	RunID          string         `yaml:"run_id"`
	WorkflowID     string         `yaml:"workflow_id,omitempty"`
	Status         string         `yaml:"status,omitempty"`
	AgentName      string         `yaml:"agent_name,omitempty"`
	AgentMode      string         `yaml:"agent_mode,omitempty"`
	RuntimeVersion string         `yaml:"runtime_version,omitempty"`
	Metadata       map[string]any `yaml:"metadata,omitempty"`
}

type WorkflowKnowledgeSeedSpec struct {
	RecordID   string         `yaml:"record_id"`
	WorkflowID string         `yaml:"workflow_id,omitempty"`
	StepRunID  string         `yaml:"step_run_id,omitempty"`
	StepID     string         `yaml:"step_id,omitempty"`
	Kind       string         `yaml:"kind,omitempty"`
	Title      string         `yaml:"title,omitempty"`
	Content    string         `yaml:"content,omitempty"`
	Status     string         `yaml:"status,omitempty"`
	Metadata   map[string]any `yaml:"metadata,omitempty"`
}

type WorkflowCheckpointSeedSpec struct {
	CheckpointID string         `yaml:"checkpoint_id"`
	TaskID       string         `yaml:"task_id"`
	WorkflowID   string         `yaml:"workflow_id,omitempty"`
	RunID        string         `yaml:"run_id,omitempty"`
	StageName    string         `yaml:"stage_name"`
	StageIndex   int            `yaml:"stage_index,omitempty"`
	ContextState map[string]any `yaml:"context_state,omitempty"`
	ResultData   map[string]any `yaml:"result_data,omitempty"`
}

func LoadSuite(path string) (*Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var suite Suite
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&suite); err != nil {
		return nil, err
	}
	suite.SourcePath = path
	if err := suite.Validate(); err != nil {
		return nil, err
	}
	return &suite, nil
}

func (s *Suite) Validate() error {
	if s.APIVersion == "" {
		return fmt.Errorf("suite missing apiVersion")
	}
	if s.Kind == "" {
		return fmt.Errorf("suite missing kind")
	}
	if s.Spec.AgentName == "" {
		return fmt.Errorf("suite missing spec.agent_name")
	}
	if s.Spec.Manifest == "" {
		return fmt.Errorf("suite missing spec.manifest")
	}
	if s.Metadata.Tier == "" {
		s.Metadata.Tier = "stable"
	}
	if err := validateSuiteTier(s.Metadata.Tier); err != nil {
		return err
	}
	if err := validateSuiteClassification(s.Metadata.Classification); err != nil {
		return err
	}
	if err := validateBenchmarkMeta(s.Metadata.Benchmark); err != nil {
		return err
	}
	if s.Spec.Execution.Profile == "" {
		s.Spec.Execution.Profile = "live"
	}
	if err := validateExecutionProfile(s.Spec.Execution.Profile); err != nil {
		return err
	}
	if err := validateMatrixOrder(s.Spec.Execution.MatrixOrder); err != nil {
		return err
	}
	if _, err := parseCaseTimeout(s.Spec.Execution.Timeout); err != nil {
		return fmt.Errorf("suite spec.execution.timeout invalid: %w", err)
	}
	if err := validateRecordingSpec(s.Spec.Recording, "suite spec.recording"); err != nil {
		return err
	}
	strategy := s.Spec.Workspace.Strategy
	if strategy == "" {
		s.Spec.Workspace.Strategy = "derived"
	} else if strategy != "derived" {
		return fmt.Errorf("suite workspace.strategy %q unsupported; use derived", strategy)
	}
	if s.Spec.Workspace.TemplateProfile == "" {
		s.Spec.Workspace.TemplateProfile = "default"
	}
	if err := validateMemorySpec(s.Spec.Memory, "suite spec.memory"); err != nil {
		return err
	}
	if len(s.Spec.Cases) == 0 {
		return fmt.Errorf("suite missing spec.cases")
	}
	for i, c := range s.Spec.Cases {
		if c.Name == "" {
			return fmt.Errorf("suite case[%d] missing name", i)
		}
		if c.Prompt == "" {
			return fmt.Errorf("suite case[%s] missing prompt", c.Name)
		}
		if _, err := parseCaseTimeout(c.Timeout); err != nil {
			return fmt.Errorf("suite case[%s] timeout invalid: %w", c.Name, err)
		}
		for j, step := range c.InteractionScript {
			if strings.TrimSpace(step.Action) == "" {
				return fmt.Errorf("suite case[%s] interaction_script[%d] missing action", c.Name, j)
			}
		}
		if c.Overrides.Memory != nil {
			if err := validateMemorySpec(*c.Overrides.Memory, fmt.Sprintf("suite case[%s] overrides.memory", c.Name)); err != nil {
				return err
			}
		}
		if c.Overrides.MaxIterations < 0 {
			return fmt.Errorf("suite case[%s] overrides.max_iterations must be >= 0", c.Name)
		}
		if _, err := parseCaseTimeout(c.Overrides.BootstrapTimeout); err != nil {
			return fmt.Errorf("suite case[%s] overrides.bootstrap_timeout invalid: %w", c.Name, err)
		}
		if strings.TrimSpace(c.Overrides.ControlFlow) != "" {
			return fmt.Errorf("suite case[%s] overrides.control_flow %q unsupported", c.Name, c.Overrides.ControlFlow)
		}
		if c.Overrides.Recording != nil {
			if err := validateRecordingSpec(*c.Overrides.Recording, fmt.Sprintf("suite case[%s] overrides.recording", c.Name)); err != nil {
				return err
			}
		}
		for fixtureName, fixture := range c.BrowserFixtures {
			if fixtureName == "" {
				return fmt.Errorf("suite case[%s] has browser fixture with empty name", c.Name)
			}
			if fixture.File == "" && fixture.Content == "" {
				return fmt.Errorf("suite case[%s] browser fixture[%s] missing file or content", c.Name, fixtureName)
			}
		}
		if err := validateSetup(c.Setup, c.Name); err != nil {
			return err
		}
	}
	for i, provider := range s.Spec.Providers {
		if strings.TrimSpace(provider.Name) == "" {
			return fmt.Errorf("suite spec.providers[%d] missing name", i)
		}
		if err := validateBackendResetStrategy(provider.ResetStrategy, fmt.Sprintf("suite spec.providers[%d].reset_strategy", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateSuiteTier(raw string) error {
	switch raw {
	case "smoke", "stable", "live-flaky", "quarantined":
		return nil
	default:
		return fmt.Errorf("suite metadata.tier %q unsupported", raw)
	}
}

func validateSuiteClassification(raw string) error {
	switch strings.TrimSpace(raw) {
	case "", "capability", "journey", "benchmark":
		return nil
	default:
		return fmt.Errorf("suite metadata.classification %q unsupported", raw)
	}
}

func validateBenchmarkMeta(meta BenchmarkMeta) error {
	if strings.TrimSpace(meta.ScoreFamily) == "" && len(meta.ScoreDimensions) == 0 && strings.TrimSpace(meta.ComparisonWindow) == "" && meta.VarianceThreshold == 0 {
		return nil
	}
	for i, dimension := range meta.ScoreDimensions {
		if strings.TrimSpace(dimension) == "" {
			return fmt.Errorf("suite metadata.benchmark.score_dimensions[%d] is empty", i)
		}
	}
	switch strings.TrimSpace(meta.ComparisonWindow) {
	case "", "case", "suite", "run", "matrix":
	default:
		return fmt.Errorf("suite metadata.benchmark.comparison_window %q unsupported", meta.ComparisonWindow)
	}
	if meta.VarianceThreshold < 0 {
		return fmt.Errorf("suite metadata.benchmark.variance_threshold must be >= 0")
	}
	return nil
}

func validateExecutionProfile(raw string) error {
	switch raw {
	case "live", "record", "replay", "developer-live", "ci-live", "ci-replay":
		return nil
	default:
		return fmt.Errorf("suite spec.execution.profile %q unsupported", raw)
	}
}

func validateMatrixOrder(raw string) error {
	switch strings.TrimSpace(raw) {
	case "", "provider-first", "model-first":
		return nil
	default:
		return fmt.Errorf("suite spec.execution.matrix_order %q unsupported", raw)
	}
}

func validateBackendResetStrategy(raw, field string) error {
	switch strings.TrimSpace(raw) {
	case "", "none", "model", "server":
		return nil
	default:
		return fmt.Errorf("%s %q unsupported", field, raw)
	}
}

func parseCaseTimeout(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if dur <= 0 {
		return 0, fmt.Errorf("must be > 0")
	}
	return dur, nil
}

func (s *Suite) EffectiveProfile(override string) string {
	if override != "" {
		return override
	}
	if s != nil && s.Spec.Execution.Profile != "" {
		return s.Spec.Execution.Profile
	}
	return "live"
}

func (s *Suite) IsStrictRun(overrideProfile string, strict bool) bool {
	if strict {
		return true
	}
	profile := s.EffectiveProfile(overrideProfile)
	if profile == "ci-live" || profile == "ci-replay" {
		return true
	}
	if s != nil && s.Spec.Execution.Strict {
		return true
	}
	return false
}

func (s *Suite) MatchesTier(tier string) bool {
	if strings.TrimSpace(tier) == "" {
		return true
	}
	if s == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(s.Metadata.Tier), strings.TrimSpace(tier))
}

func (c CaseSpec) MatchesAnyTag(tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	if len(c.Tags) == 0 {
		return false
	}
	for _, want := range tags {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		for _, have := range c.Tags {
			if strings.EqualFold(strings.TrimSpace(have), want) {
				return true
			}
		}
	}
	return false
}

func FilterSuiteCasesByTags(suite *Suite, tags []string) *Suite {
	if suite == nil {
		return nil
	}
	if len(tags) == 0 {
		return suite
	}
	filtered := *suite
	filtered.Spec = suite.Spec
	filtered.Spec.Cases = nil
	for _, c := range suite.Spec.Cases {
		if c.MatchesAnyTag(tags) {
			filtered.Spec.Cases = append(filtered.Spec.Cases, c)
		}
	}
	return &filtered
}

func (s *Suite) MatchesProfile(profile string) bool {
	if strings.TrimSpace(profile) == "" {
		return true
	}
	if s == nil {
		return false
	}
	return strings.EqualFold(s.EffectiveProfile(""), strings.TrimSpace(profile))
}

func validateMemorySpec(spec MemorySpec, location string) error {
	switch spec.Backend {
	case "", "hybrid", "sqlite_runtime":
	default:
		return fmt.Errorf("%s backend %q unsupported", location, spec.Backend)
	}
	switch spec.Retrieval.Embedder {
	case "", "test":
	default:
		return fmt.Errorf("%s retrieval.embedder %q unsupported", location, spec.Retrieval.Embedder)
	}
	return nil
}

func validateSetup(setup SetupSpec, caseName string) error {
	for _, record := range setup.Memory.Declarative {
		if record.RecordID == "" {
			return fmt.Errorf("suite case[%s] setup.memory.declarative missing record_id", caseName)
		}
	}
	for _, file := range setup.Files {
		if strings.TrimSpace(file.Path) == "" {
			return fmt.Errorf("suite case[%s] setup.files missing path", caseName)
		}
		if _, err := parseSetupFileMode(file.Mode); err != nil {
			return fmt.Errorf("suite case[%s] setup.files[%s] invalid mode: %w", caseName, file.Path, err)
		}
	}
	for _, record := range setup.Memory.Procedural {
		if record.RoutineID == "" {
			return fmt.Errorf("suite case[%s] setup.memory.procedural missing routine_id", caseName)
		}
	}
	for _, workflow := range setup.Workflows {
		if workflow.Workflow.WorkflowID == "" {
			return fmt.Errorf("suite case[%s] setup.workflows missing workflow.workflow_id", caseName)
		}
		for _, run := range workflow.Runs {
			if run.RunID == "" {
				return fmt.Errorf("suite case[%s] setup.workflows[%s] run missing run_id", caseName, workflow.Workflow.WorkflowID)
			}
		}
		for _, knowledge := range workflow.Knowledge {
			if knowledge.RecordID == "" {
				return fmt.Errorf("suite case[%s] setup.workflows[%s] knowledge missing record_id", caseName, workflow.Workflow.WorkflowID)
			}
		}
		for _, checkpoint := range workflow.Checkpoints {
			if checkpoint.CheckpointID == "" {
				return fmt.Errorf("suite case[%s] setup.workflows[%s] checkpoint missing checkpoint_id", caseName, workflow.Workflow.WorkflowID)
			}
			if checkpoint.TaskID == "" {
				return fmt.Errorf("suite case[%s] setup.workflows[%s] checkpoint missing task_id", caseName, workflow.Workflow.WorkflowID)
			}
			if checkpoint.StageName == "" {
				return fmt.Errorf("suite case[%s] setup.workflows[%s] checkpoint missing stage_name", caseName, workflow.Workflow.WorkflowID)
			}
		}
	}
	return nil
}

func validateRecordingSpec(spec RecordingSpec, location string) error {
	switch strings.TrimSpace(spec.Strategy) {
	case "", "live", "replay-if-golden", "replay-only":
	default:
		return fmt.Errorf("%s strategy %q unsupported", location, spec.Strategy)
	}
	switch strings.TrimSpace(spec.Mode) {
	case "", "off", "record", "replay":
		return nil
	default:
		return fmt.Errorf("%s mode %q unsupported", location, spec.Mode)
	}
}

func parseSetupFileMode(raw string) (fs.FileMode, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0o644, nil
	}
	value, err := strconv.ParseUint(raw, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("mode must be an octal string like 0644 or 0755")
	}
	mode := fs.FileMode(value)
	if mode > 0o777 {
		return 0, fmt.Errorf("mode %q exceeds permission bits", raw)
	}
	return mode, nil
}

func (s *Suite) ResolvePath(rel string) string {
	if rel == "" {
		return ""
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	base := "."
	if s != nil && s.SourcePath != "" {
		base = filepath.Dir(s.SourcePath)
	}
	return filepath.Clean(filepath.Join(base, rel))
}
