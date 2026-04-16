package agenttest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/perfstats"
)

type RunOptions struct {
	TargetWorkspace  string
	OutputDir        string
	Sandbox          bool
	Timeout          time.Duration
	BootstrapTimeout time.Duration
	SkipASTIndex     bool
	Profile          string
	Strict           bool
	MaxRetries       int

	ModelOverride    string
	EndpointOverride string

	MaxIterations int
	DebugLLM      bool
	DebugAgent    bool

	BackendReset        string   // none|model|server
	BackendBinary       string   // default: ollama
	BackendService      string   // default: ollama
	BackendResetOn      []string // regexes matched against error to trigger reset+retry
	BackendResetBetween bool     // reset before each case

	Lane string // optional tag-lane filter for multi-suite runs
}

type SuiteReport struct {
	SuitePath        string
	RunID            string
	Profile          string
	Strict           bool
	StartedAt        time.Time
	FinishedAt       time.Time
	DurationMS       int64
	PassedCases      int
	FailedCases      int
	SkippedCases     int
	InfraFailures    int
	AssertFailures   int
	SecurityFailures int `json:"security_failures,omitempty"` // OSB: security assertion failures
	Cases            []CaseReport
	Performance      PerformanceSummary   `json:"performance,omitempty"`
	Benchmark        *BenchmarkReport     `json:"benchmark,omitempty"`
	OSBBenchmark     *OSBBenchmarkSummary `json:"osb_benchmark,omitempty"` // OSB: benchmark summary
}

type CaseReport struct {
	Name                 string
	Model                string
	ModelDigest          string
	ModelLoadedAs        string
	ModelSource          string
	Provider             string
	ManifestModel        string
	Endpoint             string
	RecordingMode        string
	BackendResetStrategy string
	TapePath             string
	Workspace            string
	ArtifactsDir         string
	StartedAt            time.Time
	FinishedAt           time.Time
	DurationMS           int64

	Skipped    bool
	SkipReason string

	Success             bool
	Error               string
	FailureKind         string
	Attempts            int
	RetryCount          int
	RetryTriggeredBy    []string
	Output              string
	ChangedFiles        []string
	ToolCalls           map[string]int
	TokenUsage          TokenUsageReport
	MemoryOutcome       MemoryOutcomeReport
	FrameworkPerf       perfstats.Snapshot   `json:"framework_perf,omitempty"`
	PhaseMetrics        []PhaseMetric        `json:"phase_metrics,omitempty"`
	BaselinePath        string               `json:"baseline_path,omitempty"`
	BaselineFound       bool                 `json:"baseline_found,omitempty"`
	PerformanceWarnings []PerformanceWarning `json:"performance_warnings,omitempty"`
	// NEW: Latency tracking (Phase 5)
	ToolLatencies   map[string]LatencyStats `json:"tool_latencies,omitempty"`
	TotalToolTimeMs int64                   `json:"total_tool_time_ms,omitempty"`

	// OSB Model: New report fields (Phase 1)
	SecurityObservations  []SecurityObservation  `json:"security_observations,omitempty"`
	BenchmarkObservations []BenchmarkObservation `json:"benchmark_observations,omitempty"`
	AssertionResults      []AssertionResult      `json:"assertion_results,omitempty"`
}

// SecurityObservation records one security-relevant event observed during the run.
// Used for both violations (unexpected boundary crossings) and grants
// (expected boundary crossings that were permitted per manifest).
type SecurityObservation struct {
	Kind       string `json:"kind"`     // "file_write", "exec", "network", "read"
	Resource   string `json:"resource"` // affected path, binary, or endpoint
	Action     string `json:"action"`   // "write", "execute", "connect", "read"
	InScope    bool   `json:"in_scope"` // true if within manifest-declared permissions
	Blocked    bool   `json:"blocked"`  // true if the sandbox blocked it
	Expected   bool   `json:"expected"` // true if listed in security.expected_violations
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

// AssertionResult captures one outcome or security assertion result.
type AssertionResult struct {
	AssertionID string `json:"assertion_id"` // e.g. "outcome.must_succeed"
	Tier        string `json:"tier"`         // "outcome" or "security"
	Passed      bool   `json:"passed"`
	Message     string `json:"message,omitempty"`
}

// OSBBenchmarkSummary aggregates benchmark observations across cases (Outcome-Security-Benchmark model).
type OSBBenchmarkSummary struct {
	TotalObservations   int                        `json:"total_observations"`
	MatchedObservations int                        `json:"matched_observations"`
	MatchRate           float64                    `json:"match_rate"` // matched/total
	ByCategory          map[string]CategorySummary `json:"by_category"`
	ByField             map[string]FieldSummary    `json:"by_field"`
	WorstFields         []FieldSummary             `json:"worst_fields"` // lowest match rate
}

// CategorySummary summarizes observations for one category.
type CategorySummary struct {
	Total   int     `json:"total"`
	Matched int     `json:"matched"`
	Rate    float64 `json:"rate"`
}

// FieldSummary summarizes observations for one field.
type FieldSummary struct {
	Field        string   `json:"field"`
	Total        int      `json:"total"`
	Matched      int      `json:"matched"`
	Rate         float64  `json:"rate"`
	ActualValues []string `json:"actual_values,omitempty"` // distinct observed values
}

type TokenUsageReport struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LLMCalls         int
}

type MemoryOutcomeReport struct {
	DeclarativeRecordsCreated int
	ProceduralRecordsCreated  int
	MemoryRecordsCreated      int
	WorkflowRowsCreated       int
	WorkflowStateUpdated      bool
}

type Runner struct {
	Logger *log.Logger
}

type runCaseLayout struct {
	ArtifactsDir        string
	TmpDir              string
	TelemetryPath       string
	LogPath             string
	TapePath            string
	InteractionTapePath string
	WorkspaceDir        string
}

func (r *Runner) RunSuite(ctx context.Context, suite *Suite, opts RunOptions) (*SuiteReport, error) {
	if suite == nil {
		return nil, errors.New("suite required")
	}
	if opts.TargetWorkspace == "" {
		return nil, errors.New("target workspace required")
	}
	targetWorkspace, err := filepath.Abs(opts.TargetWorkspace)
	if err != nil {
		return nil, err
	}
	workspacePaths := config.New(targetWorkspace)
	runID := time.Now().UTC().Format("20060102-150405.000")
	outDir := opts.OutputDir
	if outDir == "" {
		outDir = workspacePaths.TestRunDir(suite.Spec.AgentName, runID)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	report := &SuiteReport{
		SuitePath: suite.SourcePath,
		RunID:     runID,
		Profile:   suite.EffectiveProfile(opts.Profile),
		Strict:    suite.IsStrictRun(opts.Profile, opts.Strict),
		StartedAt: time.Now().UTC(),
	}

	models := suite.Spec.Models
	if len(models) == 0 {
		models = []ModelSpec{{Name: "", Endpoint: ""}}
	}
	matrixModels := expandSuiteModelMatrix(models, suite.Spec.Providers, suite.Spec.Execution.MatrixOrder)
	if err := r.preflightSuite(suite, opts, targetWorkspace, models); err != nil {
		return nil, err
	}

	for _, c := range suite.Spec.Cases {
		caseModels := matrixModels
		if c.Overrides.Model != nil {
			caseModels = expandSuiteModelMatrix([]ModelSpec{*c.Overrides.Model}, suite.Spec.Providers, suite.Spec.Execution.MatrixOrder)
		}
		for _, model := range caseModels {
			cr := r.runCase(ctx, suite, c, model, opts, targetWorkspace, outDir)
			report.Cases = append(report.Cases, cr)
		}
	}

	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	for _, c := range report.Cases {
		switch {
		case c.Skipped:
			report.SkippedCases++
		case c.Success:
			report.PassedCases++
		default:
			report.FailedCases++
			switch c.FailureKind {
			case "infra":
				report.InfraFailures++
			case "security":
				report.SecurityFailures++
			default:
				report.AssertFailures++
			}
		}
	}
	report.Performance = SummarizePerformance(report.Cases)

	// OSB Model: Populate benchmark summary (Phase 5)
	report.OSBBenchmark = aggregateBenchmarkSummary(report.Cases)

	if strings.EqualFold(strings.TrimSpace(suite.Metadata.Classification), "benchmark") || suite.Metadata.Benchmark.ScoreFamily != "" || len(suite.Metadata.Benchmark.ScoreDimensions) > 0 || strings.TrimSpace(suite.Metadata.Benchmark.ComparisonWindow) != "" || suite.Metadata.Benchmark.VarianceThreshold > 0 {
		if benchmarkReport, err := BuildBenchmarkReport(suite, report); err == nil && benchmarkReport != nil {
			report.Benchmark = benchmarkReport
			if data, marshalErr := json.MarshalIndent(benchmarkReport, "", "  "); marshalErr == nil {
				_ = os.WriteFile(BenchmarkReportFilePath(outDir), data, 0o644)
			}
		}
	}

	// OSB Model: Write benchmark_summary.json (Phase 5)
	if report.OSBBenchmark != nil {
		if data, err := json.MarshalIndent(report.OSBBenchmark, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(outDir, "benchmark_summary.json"), data, 0o644)
		}
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "report.json"), data, 0o644)
	return report, nil
}

func (r *Runner) preflightSuite(suite *Suite, opts RunOptions, targetWorkspace string, models []ModelSpec) error {
	manifestModel := ""
	if suite != nil {
		suiteManifestAbs := suite.ResolvePath(suite.Spec.Manifest)
		suiteManifestAbs = resolveAgainstWorkspace(targetWorkspace, suiteManifestAbs, suite.Spec.Manifest)
		suiteManifestAbs = fallbackManifestPath(suiteManifestAbs, targetWorkspace)
		if loadedManifest, err := manifest.LoadAgentManifest(suiteManifestAbs); err == nil && loadedManifest.Spec.Agent != nil {
			manifestModel = loadedManifest.Spec.Agent.Model.Name
		}
	}
	checked := map[string]struct{}{}
	layout := newRunCaseLayout(filepath.Join(targetWorkspace, "relurpify_cfg", "test_runs_preflight"), "preflight", "preflight")
	matrixModels := expandSuiteModelMatrix(models, suite.Spec.Providers, suite.Spec.Execution.MatrixOrder)
	for _, c := range suite.Spec.Cases {
		caseModels := matrixModels
		if c.Overrides.Model != nil {
			caseModels = expandSuiteModelMatrix([]ModelSpec{*c.Overrides.Model}, suite.Spec.Providers, suite.Spec.Execution.MatrixOrder)
		}
		for _, model := range caseModels {
			exec, err := resolveCaseExecution(suite, c, model, manifestModel, opts, layout, targetWorkspace, targetWorkspace)
			if err != nil {
				return err
			}
			if !shouldPreflightBackend(exec.RecordingMode) {
				continue
			}
			key := strings.TrimSpace(exec.Endpoint) + "|" + strings.TrimSpace(exec.Model)
			if _, ok := checked[key]; ok {
				continue
			}
			checked[key] = struct{}{}
			if err := preflightBackend(exec.Endpoint, exec.Model); err != nil {
				return fmt.Errorf("inference backend preflight failed for suite %s case %s: %w", filepath.Base(suite.SourcePath), c.Name, err)
			}
		}
	}
	return nil
}

func expandSuiteModelMatrix(models []ModelSpec, providers []ProviderSpec, order string) []ModelSpec {
	if len(models) == 0 {
		models = []ModelSpec{{Name: "", Endpoint: ""}}
	}
	if len(providers) == 0 {
		return append([]ModelSpec(nil), models...)
	}
	if strings.TrimSpace(order) == "model-first" {
		return expandSuiteModelMatrixModelFirst(models, providers)
	}
	return expandSuiteModelMatrixProviderFirst(models, providers)
}

func expandSuiteModelMatrixProviderFirst(models []ModelSpec, providers []ProviderSpec) []ModelSpec {
	rows := make([]ModelSpec, 0, len(models)*len(providers))
	for _, provider := range providers {
		for _, model := range models {
			rows = append(rows, modelForProvider(model, provider))
		}
	}
	return rows
}

func expandSuiteModelMatrixModelFirst(models []ModelSpec, providers []ProviderSpec) []ModelSpec {
	rows := make([]ModelSpec, 0, len(models)*len(providers))
	for _, model := range models {
		for _, provider := range providers {
			rows = append(rows, modelForProvider(model, provider))
		}
	}
	return rows
}

func modelForProvider(model ModelSpec, provider ProviderSpec) ModelSpec {
	out := model
	if strings.TrimSpace(provider.Name) != "" {
		out.Provider = provider.Name
	}
	if strings.TrimSpace(provider.Endpoint) != "" {
		out.Endpoint = provider.Endpoint
	}
	if strings.TrimSpace(provider.ResetStrategy) != "" {
		out.ResetStrategy = provider.ResetStrategy
	}
	if provider.ResetBetween {
		out.ResetBetween = true
	}
	return out
}

func newRunCaseLayout(outDir, caseName, modelName string) runCaseLayout {
	caseKey := sanitizeName(caseName) + "__" + sanitizeName(modelName)
	artifactsDir := filepath.Join(outDir, "artifacts", caseKey)
	tmpDir := filepath.Join(outDir, "tmp", caseKey)
	return runCaseLayout{
		ArtifactsDir:        artifactsDir,
		TmpDir:              tmpDir,
		TelemetryPath:       filepath.Join(outDir, "telemetry", caseKey+".jsonl"),
		LogPath:             filepath.Join(outDir, "logs", caseKey+".log"),
		TapePath:            filepath.Join(artifactsDir, "tape.jsonl"),
		InteractionTapePath: filepath.Join(artifactsDir, "interaction.tape.jsonl"),
		WorkspaceDir:        filepath.Join(tmpDir, "workspace"),
	}
}

// aggregateBenchmarkSummary aggregates benchmark observations across all cases.
// Phase 4: OSB Model benchmark summary aggregation.
func aggregateBenchmarkSummary(cases []CaseReport) *OSBBenchmarkSummary {
	if len(cases) == 0 {
		return nil
	}

	summary := &OSBBenchmarkSummary{
		ByCategory: make(map[string]CategorySummary),
		ByField:    make(map[string]FieldSummary),
	}

	// Collect all observations from all cases
	for _, c := range cases {
		for _, obs := range c.BenchmarkObservations {
			summary.TotalObservations++
			if obs.Matched {
				summary.MatchedObservations++
			}

			// Update category summary
			cat := summary.ByCategory[obs.Category]
			cat.Total++
			if obs.Matched {
				cat.Matched++
			}
			summary.ByCategory[obs.Category] = cat

			// Update field summary
			field := summary.ByField[obs.Field]
			field.Field = obs.Field
			field.Total++
			if obs.Matched {
				field.Matched++
			}
			// Collect distinct actual values
			if obs.Actual != "" {
				found := false
				for _, v := range field.ActualValues {
					if v == obs.Actual {
						found = true
						break
					}
				}
				if !found {
					field.ActualValues = append(field.ActualValues, obs.Actual)
				}
			}
			summary.ByField[obs.Field] = field
		}
	}

	// Calculate rates
	if summary.TotalObservations > 0 {
		summary.MatchRate = float64(summary.MatchedObservations) / float64(summary.TotalObservations)
	}

	for catName, cat := range summary.ByCategory {
		if cat.Total > 0 {
			cat.Rate = float64(cat.Matched) / float64(cat.Total)
			summary.ByCategory[catName] = cat
		}
	}

	for fieldName, field := range summary.ByField {
		if field.Total > 0 {
			field.Rate = float64(field.Matched) / float64(field.Total)
			summary.ByField[fieldName] = field
		}
	}

	// Build worst fields list (lowest match rate)
	for _, field := range summary.ByField {
		summary.WorstFields = append(summary.WorstFields, field)
	}
	// Sort by rate ascending
	sort.Slice(summary.WorstFields, func(i, j int) bool {
		return summary.WorstFields[i].Rate < summary.WorstFields[j].Rate
	})

	return summary
}

// === Phase 8: Types from deleted latency.go ===

// latencyAccumulator tracks intermediate state for computing proper running averages
type latencyAccumulator struct {
	minMs  int64
	maxMs  int64
	sumMs  int64
	count  int64
	values []int64 // for p95 calculation
}

func (a *latencyAccumulator) add(duration int64) {
	if a.count == 0 || duration < a.minMs {
		a.minMs = duration
	}
	if duration > a.maxMs {
		a.maxMs = duration
	}
	a.sumMs += duration
	a.count++
	a.values = append(a.values, duration)
}

func (a *latencyAccumulator) toStats() LatencyStats {
	if a.count == 0 {
		return LatencyStats{}
	}
	stats := LatencyStats{
		MinMs: a.minMs,
		MaxMs: a.maxMs,
		AvgMs: a.sumMs / a.count,
	}
	if len(a.values) > 0 {
		stats.P95Ms = calculateP95(a.values)
	}
	return stats
}

func calculateP95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)-1) * 0.95)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// LatencyStats captures statistical information about tool latency
type LatencyStats struct {
	MinMs int64 `json:"min_ms"`
	MaxMs int64 `json:"max_ms"`
	AvgMs int64 `json:"avg_ms"`
	P95Ms int64 `json:"p95_ms"`
}

// ToolLatencyReport contains latency information for all tools
type ToolLatencyReport struct {
	ToolLatencies   map[string]LatencyStats `json:"tool_latencies"`
	TotalToolTimeMs int64                   `json:"total_tool_time_ms"`
}

// BuildLatencyReport builds a latency report from tool transcript
// Phase 8: Simplified implementation after latency.go removal
func BuildLatencyReport(transcript *ToolTranscriptArtifact) *ToolLatencyReport {
	if transcript == nil || len(transcript.Entries) == 0 {
		return nil
	}

	accumulators := make(map[string]*latencyAccumulator)
	var totalTime int64

	for _, entry := range transcript.Entries {
		duration := entry.DurationMS
		totalTime += duration

		acc, exists := accumulators[entry.Tool]
		if !exists {
			acc = &latencyAccumulator{}
			accumulators[entry.Tool] = acc
		}
		acc.add(duration)
	}

	// Convert accumulators to final stats
	latencies := make(map[string]LatencyStats)
	for tool, acc := range accumulators {
		latencies[tool] = acc.toStats()
	}

	return &ToolLatencyReport{
		ToolLatencies:   latencies,
		TotalToolTimeMs: totalTime,
	}
}
