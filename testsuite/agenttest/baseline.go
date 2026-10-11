package agenttest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// BaselineSchemaV1 is the schema tag of the agenttest performance baseline
// (owner ruling R-5): baselines are a deliberate snapshot of one executed
// run, written into workspace state, and compared report-only.
const BaselineSchemaV1 = "relurpify/agenttest-baseline/v1"

// BaselinesDirName is the baselines leaf inside the canonical runtime state
// tree. Baselines live beside the test runs they reference — the workspace's
// runtime state root (config.Paths.TestRunsDir), never the repository and
// never a case's derived workspace copy (R-5a).
const BaselinesDirName = "baselines"

// SuiteBaselineFilename is the suite-level score snapshot inside one suite's
// baselines directory. It cannot collide with a per-case baseline: cases are
// stored as <case>__<model>.baseline.json.
const SuiteBaselineFilename = "suite.baseline.json"

// baselineWarnDelta is the per-metric delta fraction that appends a
// PerformanceWarning (D10.4). Comparison is REPORT-ONLY: a warning MUST NOT
// affect Success, exit codes, or OSB evaluation.
const baselineWarnDelta = 0.5

// ErrBaselineNotFound reports the absence of a baseline file. Absence is
// DATA, not an error condition of the run: it records BaselineFound=false.
var ErrBaselineNotFound = errors.New("agenttest: baseline not found")

// BaselineMetrics are the stable v1 comparison metrics — the R-5 axes plus
// the phase timing signal: token count, tool-call count, the per-phase
// durations, and the framework counters. Anything unstable (wall-clock
// totals, memory addresses, timestamps) stays out of baselines v1.
type BaselineMetrics struct {
	TotalTokens int                `json:"total_tokens"`
	ToolCalls   int                `json:"tool_calls"`
	PhaseMSP50  map[string]float64 `json:"phase_ms_p50,omitempty"`
	// FrameworkPerf carries the framework counters keyed by their stable
	// names; the compare path reads the same keys off the current run.
	FrameworkPerf map[string]int64 `json:"framework_perf,omitempty"`
}

// BaselineScore is the benchmark-mode score block of a baseline. It is
// present only when the producing run scored the case (benchmark-classified
// suites): the OSB comparison reads the SAME baseline file instead of a
// parallel score-baseline family (one producer, one consumer).
type BaselineScore struct {
	OverallScore      float64            `json:"overall_score"`
	DimensionScores   map[string]float64 `json:"dimension_scores,omitempty"`
	ScoreFamily       string             `json:"score_family,omitempty"`
	VarianceThreshold float64            `json:"variance_threshold,omitempty"`
}

// BaselineFile is one case's baseline document (schema v1).
type BaselineFile struct {
	Schema     string          `json:"schema"`
	Suite      string          `json:"suite"`
	Case       string          `json:"case"`
	Model      string          `json:"model"`
	CapturedAt time.Time       `json:"captured_at"`
	SourceRun  string          `json:"source_run"`
	Metrics    BaselineMetrics `json:"metrics"`
	// Score is the optional benchmark-mode compare block. Absent for
	// non-benchmark runs; absent scores compare as absent (never as
	// "within variance").
	Score *BaselineScore `json:"score,omitempty"`
}

// SuiteBaselineFile is one suite's score snapshot (schema v1): the
// suite-level half of the benchmark-mode comparison, kept beside the
// per-case baselines of the same suite.
type SuiteBaselineFile struct {
	Schema     string        `json:"schema"`
	Suite      string        `json:"suite"`
	CapturedAt time.Time     `json:"captured_at"`
	SourceRun  string        `json:"source_run"`
	Score      BaselineScore `json:"score"`
}

// BaselinesDir resolves the baselines directory from the invoked workspace
// root through the canonical runtime-state layout: every other runtime state
// consumer resolves the same way, so baselines sit beside the runs they
// reference (R-5a).
func BaselinesDir(workspaceRoot string) string {
	return filepath.Join(config.New(strings.TrimSpace(workspaceRoot)).TestRunsDir(), BaselinesDirName)
}

// BaselinePath builds the per-case baseline path:
// <baselines-dir>/<suite>/<case>__<model>.baseline.json. The file name uses
// the tape naming convention (GoldenBaselineFilename) so tooling stays
// uniform across the golden and workspace-state tiers.
func BaselinePath(baselinesDir, suiteName, caseName, modelName string) string {
	return filepath.Join(baselinesDir, SanitizeName(suiteName), GoldenBaselineFilename(caseName, modelName))
}

// SuiteBaselinePath builds the suite-level baseline path:
// <baselines-dir>/<suite>/suite.baseline.json.
func SuiteBaselinePath(baselinesDir, suiteName string) string {
	return filepath.Join(baselinesDir, SanitizeName(suiteName), SuiteBaselineFilename)
}

// frameworkPerfMetrics maps the process snapshot onto the baseline's stable
// counter names. BuildBaselineFile (the producer) and CompareAgainstBaseline
// (the consumer) both map through it, so a counter is compared under exactly
// the key it was recorded under.
func frameworkPerfMetrics(snapshot perfstats.Snapshot) map[string]int64 {
	return map[string]int64{
		"branch_clones":           snapshot.BranchClones,
		"branch_merges":           snapshot.BranchMergeCount,
		"artifact_budget_rescans": snapshot.ArtifactBudgetRescanCount,
		"progressive_file_reads":  snapshot.ProgressiveFileReadCount,
		"runtime_memory_searches": snapshot.RuntimeMemorySearchCount,
	}
}

// BuildBaselineFile promotes one executed case report into a per-case
// baseline document. The benchmark score block is attached by the promotion
// path from the run's produced BenchmarkCaseScore (benchmark.go) — one
// producer (the run), one consumer (the compare path).
func BuildBaselineFile(suiteName string, cr CaseReport, sourceRun string, capturedAt time.Time) *BaselineFile {
	phaseMSP50 := map[string]float64{}
	for _, phase := range cr.PhaseMetrics {
		if strings.TrimSpace(phase.Phase) == "" {
			continue
		}
		phaseMSP50[phase.Phase] = float64(phase.DurationMS)
	}
	return &BaselineFile{
		Schema:     BaselineSchemaV1,
		Suite:      strings.TrimSpace(suiteName),
		Case:       strings.TrimSpace(cr.Name),
		Model:      strings.TrimSpace(cr.Model),
		CapturedAt: capturedAt.UTC(),
		SourceRun:  strings.TrimSpace(sourceRun),
		Metrics: BaselineMetrics{
			TotalTokens:   cr.TokenUsage.TotalTokens,
			ToolCalls:     totalToolCalls(cr),
			PhaseMSP50:    phaseMSP50,
			FrameworkPerf: frameworkPerfMetrics(cr.FrameworkPerf),
		},
	}
}

func totalToolCalls(cr CaseReport) int {
	total := 0
	for _, count := range cr.ToolCalls {
		total += count
	}
	return total
}

// WriteBaseline stores the baseline document (overwrite-by-design: baselines
// are a deliberate snapshot, not an accumulator).
func WriteBaseline(path string, baseline *BaselineFile) error {
	if baseline == nil {
		return fmt.Errorf("agenttest: baseline required")
	}
	if err := validateBaselinePath(path, baseline.Schema); err != nil {
		return err
	}
	if strings.TrimSpace(baseline.Case) == "" {
		return fmt.Errorf("agenttest: baseline case required")
	}
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteFileSecure(path, data)
}

// LoadBaseline loads a baseline document. A missing file returns
// ErrBaselineNotFound — absence is data.
func LoadBaseline(path string) (*BaselineFile, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBaselineNotFound
		}
		return nil, err
	}
	var baseline BaselineFile
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("agenttest: parse baseline %s: %w", path, err)
	}
	if baseline.Schema != BaselineSchemaV1 {
		return nil, fmt.Errorf("agenttest: baseline %s has schema %q, want %s", path, baseline.Schema, BaselineSchemaV1)
	}
	return &baseline, nil
}

// WriteSuiteBaseline stores the suite-level score snapshot.
func WriteSuiteBaseline(path string, baseline *SuiteBaselineFile) error {
	if baseline == nil {
		return fmt.Errorf("agenttest: suite baseline required")
	}
	if err := validateBaselinePath(path, baseline.Schema); err != nil {
		return err
	}
	if strings.TrimSpace(baseline.Suite) == "" {
		return fmt.Errorf("agenttest: suite baseline suite required")
	}
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteFileSecure(path, data)
}

// LoadSuiteBaseline loads the suite-level score snapshot. A missing file
// returns ErrBaselineNotFound.
func LoadSuiteBaseline(path string) (*SuiteBaselineFile, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBaselineNotFound
		}
		return nil, err
	}
	var baseline SuiteBaselineFile
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("agenttest: parse suite baseline %s: %w", path, err)
	}
	if baseline.Schema != BaselineSchemaV1 {
		return nil, fmt.Errorf("agenttest: suite baseline %s has schema %q, want %s", path, baseline.Schema, BaselineSchemaV1)
	}
	return &baseline, nil
}

func validateBaselinePath(path, schema string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("agenttest: baseline path required")
	}
	if schema != BaselineSchemaV1 {
		return fmt.Errorf("agenttest: baseline schema %q unsupported (want %s)", schema, BaselineSchemaV1)
	}
	return nil
}

// BaselineComparison is the report-only compare block on a case report.
type BaselineComparison struct {
	BaselineFound bool   `json:"baseline_found"`
	BaselinePath  string `json:"baseline_path,omitempty"`
	// Message records why a comparison did not happen: empty when the
	// metrics below are the comparison; "unreadable: …" when a baseline file
	// exists but cannot be used. Absence is NOT recorded as a block — a
	// missing baseline is quiet absence (BaselineFound=false, no block).
	Message  string                `json:"message,omitempty"`
	Metrics  []BaselineMetricDelta `json:"metrics,omitempty"`
	Warnings []PerformanceWarning  `json:"warnings,omitempty"`
}

// BaselineMetricDelta is one compared metric: baseline value, current value,
// and the fractional delta (current-baseline)/baseline. A baseline of zero
// for a metric skips it (recorded n/a, not infinity).
type BaselineMetricDelta struct {
	Metric   string  `json:"metric"`
	Baseline float64 `json:"baseline"`
	Current  float64 `json:"current"`
	Delta    float64 `json:"delta"` // fraction; 0 when skipped
	Skipped  bool    `json:"skipped,omitempty"`
}

// baselineEventEmitter receives the per-metric compare events (nil-safe).
type baselineEventEmitter interface {
	EmitBaselineCompared(metric string, delta float64)
}

// CompareAgainstBaseline populates the case report's compare block from the
// baseline at path (when one exists) and appends a PerformanceWarning per
// metric off by more than baselineWarnDelta. Report-only: callers MUST NOT
// let this affect Success. A skipped case carries no metrics, so it is
// recorded as absence.
func CompareAgainstBaseline(cr *CaseReport, path string, tel baselineEventEmitter) {
	if cr == nil {
		return
	}
	cr.BaselinePath = path
	if cr.Skipped {
		cr.BaselineFound = false
		return
	}
	baseline, err := LoadBaseline(path)
	if err != nil {
		// Absence is recorded as quiet absence (BaselineFound=false, no
		// block): the "missing → within variance" default is deleted (D10.4).
		// An UNREADABLE baseline is different — it exists and is broken, so
		// it is recorded as such rather than hidden as absence.
		cr.BaselineFound = false
		if !errors.Is(err, ErrBaselineNotFound) {
			cr.Baseline = &BaselineComparison{
				BaselineFound: false,
				BaselinePath:  path,
				Message:       fmt.Sprintf("unreadable: %v", err),
			}
		}
		return
	}
	cr.BaselineFound = true
	comparison := BaselineComparison{
		BaselineFound: true,
		BaselinePath:  path,
	}
	compare := func(metric string, baseline, current float64) {
		delta := BaselineMetricDelta{Metric: metric, Baseline: baseline, Current: current}
		if baseline != 0 {
			delta.Delta = (current - baseline) / baseline
		} else {
			delta.Skipped = true
		}
		comparison.Metrics = append(comparison.Metrics, delta)
		if !delta.Skipped && absFloat64(delta.Delta) > baselineWarnDelta {
			warning := PerformanceWarning{
				Metric:   metric,
				Actual:   int64(current),
				Baseline: int64(baseline),
				Detail:   fmt.Sprintf("%s off baseline by %.0f%% (baseline %.0f, current %.0f)", metric, delta.Delta*100, baseline, current),
			}
			comparison.Warnings = append(comparison.Warnings, warning)
		}
		if tel != nil {
			tel.EmitBaselineCompared(metric, delta.Delta)
		}
	}
	compare("total_tokens", float64(baseline.Metrics.TotalTokens), float64(cr.TokenUsage.TotalTokens))
	compare("tool_calls", float64(baseline.Metrics.ToolCalls), float64(totalToolCalls(*cr)))
	baselineSorted := make([]string, 0, len(baseline.Metrics.PhaseMSP50))
	for phase := range baseline.Metrics.PhaseMSP50 {
		baselineSorted = append(baselineSorted, phase)
	}
	sort.Strings(baselineSorted)
	for _, phase := range baselineSorted {
		compare("phase_ms_p50."+phase, baseline.Metrics.PhaseMSP50[phase], float64(phaseDurationMS(cr, phase)))
	}
	// The framework counters compare under the same keys BuildBaselineFile
	// recorded them (R-5 axes: framework perf, token count, tool-call count).
	currentFramework := frameworkPerfMetrics(cr.FrameworkPerf)
	frameworkSorted := make([]string, 0, len(baseline.Metrics.FrameworkPerf))
	for counter := range baseline.Metrics.FrameworkPerf {
		frameworkSorted = append(frameworkSorted, counter)
	}
	sort.Strings(frameworkSorted)
	for _, counter := range frameworkSorted {
		compare("framework_perf."+counter, float64(baseline.Metrics.FrameworkPerf[counter]), float64(currentFramework[counter]))
	}
	cr.Baseline = &comparison
	cr.PerformanceWarnings = append(cr.PerformanceWarnings, comparison.Warnings...)
}

func phaseDurationMS(cr *CaseReport, phase string) int64 {
	for _, m := range cr.PhaseMetrics {
		if m.Phase == phase {
			return m.DurationMS
		}
	}
	return 0
}

// BaselineCompareSummary aggregates the report-only compare block across a
// run's cases: how many cases carried a baseline, and how many accrued
// warnings. It never affects a case outcome (R-5c).
type BaselineCompareSummary struct {
	ComparedCases int `json:"compared_cases"`
	WarnedCases   int `json:"warned_cases"`
	Warnings      int `json:"warnings"`
}

// SummarizeBaselineComparison aggregates BaselineFound / PerformanceWarnings
// over one suite report's cases.
func SummarizeBaselineComparison(report *SuiteReport) BaselineCompareSummary {
	if report == nil {
		return BaselineCompareSummary{}
	}
	var summary BaselineCompareSummary
	for _, cr := range report.Cases {
		if !cr.BaselineFound {
			continue
		}
		summary.ComparedCases++
		if len(cr.PerformanceWarnings) > 0 {
			summary.WarnedCases++
		}
		summary.Warnings += len(cr.PerformanceWarnings)
	}
	return summary
}

// FormatBaselineCompareLine renders the compare block for a suite report.
// The second return is false when nothing was compared — an absent baseline
// is absence, never a line claiming variance (D10.4).
func FormatBaselineCompareLine(report *SuiteReport) (string, bool) {
	summary := SummarizeBaselineComparison(report)
	if summary.ComparedCases == 0 {
		return "", false
	}
	return fmt.Sprintf("baseline: %d case(s) compared, %d with warning(s), %d warning(s) total — report-only, never affects the case outcome",
		summary.ComparedCases, summary.WarnedCases, summary.Warnings), true
}
