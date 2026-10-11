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
)

// BaselineSchemaV1 is the schema tag of the agenttest performance baseline
// (owner ruling R-5): baselines are a deliberate snapshot of one executed
// run, written into workspace state, and compared report-only.
const BaselineSchemaV1 = "relurpify/agenttest-baseline/v1"

// baselineWarnDelta is the per-metric delta fraction that appends a
// PerformanceWarning (D10.4). Comparison is REPORT-ONLY: a warning MUST NOT
// affect Success, exit codes, or OSB evaluation.
const baselineWarnDelta = 0.5

// ErrBaselineNotFound reports the absence of a baseline file. Absence is
// DATA, not an error condition of the run: it records BaselineFound=false.
var ErrBaselineNotFound = errors.New("agenttest: baseline not found")

// BaselineMetrics are the stable v1 comparison metrics: token count,
// tool-call count, wall duration, the framework counters, and the per-phase
// duration p50s. Anything unstable stays out of baselines v1.
type BaselineMetrics struct {
	TotalTokens   int                `json:"total_tokens"`
	ToolCalls     int                `json:"tool_calls"`
	DurationMS    int64              `json:"duration_ms"`
	PhaseMSP50    map[string]float64 `json:"phase_ms_p50,omitempty"`
	FrameworkPerf map[string]int64   `json:"framework_perf,omitempty"`
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
}

// BaselinesDirName is the workspace-state directory baselines live under —
// inside the SOURCE workspace's state tree (R-5a: workspace state, not repo,
// not the derived per-case copy).
const BaselinesDirName = "relurpify_cfg/test_runs/baselines"

// BaselinesDir resolves the baselines directory from the invoked workspace
// root.
func BaselinesDir(workspaceRoot string) string {
	return filepath.Join(strings.TrimSpace(workspaceRoot), BaselinesDirName)
}

// BaselinePath builds the baseline file path for a (suite, case, model)
// triple: <baselines-dir>/<suite>/<case>__<model>.baseline.json. The model
// token is sanitized with the tape naming convention (`:`,`/` → `_`) so
// tooling stays uniform.
func BaselinePath(baselinesDir, suiteName, caseName, modelName string) string {
	key := SanitizeName(caseName) + "__" + SanitizeName(modelName) + ".baseline.json"
	return filepath.Join(baselinesDir, SanitizeName(suiteName), key)
}

// BuildBaselineFile promotes one executed case report into a baseline
// document.
func BuildBaselineFile(suiteName string, cr CaseReport, sourceRun string, capturedAt time.Time) *BaselineFile {
	phaseMSP50 := map[string]float64{}
	for _, phase := range cr.PhaseMetrics {
		if strings.TrimSpace(phase.Phase) == "" {
			continue
		}
		phaseMSP50[phase.Phase] = float64(phase.DurationMS)
	}
	framework := map[string]int64{}
	if v := cr.FrameworkPerf; v.BranchClones != 0 || v.BranchMergeCount != 0 || v.ArtifactBudgetRescanCount != 0 {
		framework["branch_clones"] = v.BranchClones
		framework["branch_merges"] = v.BranchMergeCount
		framework["artifact_budget_rescans"] = v.ArtifactBudgetRescanCount
		framework["progressive_file_reads"] = v.ProgressiveFileReadCount
		framework["runtime_memory_searches"] = v.RuntimeMemorySearchCount
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
			DurationMS:    cr.DurationMS,
			PhaseMSP50:    phaseMSP50,
			FrameworkPerf: framework,
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
	if baseline.Schema != BaselineSchemaV1 {
		return fmt.Errorf("agenttest: baseline schema %q unsupported (want %s)", baseline.Schema, BaselineSchemaV1)
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

// BaselineComparison is the report-only compare block on a case report.
type BaselineComparison struct {
	BaselineFound bool                  `json:"baseline_found"`
	BaselinePath  string                `json:"baseline_path,omitempty"`
	Metrics       []BaselineMetricDelta `json:"metrics,omitempty"`
	Warnings      []PerformanceWarning  `json:"warnings,omitempty"`
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
// let this affect Success.
func CompareAgainstBaseline(cr *CaseReport, path string, tel baselineEventEmitter) {
	if cr == nil {
		return
	}
	cr.BaselinePath = path
	baseline, err := LoadBaseline(path)
	if err != nil {
		// Absence (or an unreadable file) is recorded as absence; the
		// "missing → within variance" default is deleted (D10.4).
		cr.BaselineFound = false
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
	compare("duration_ms", float64(baseline.Metrics.DurationMS), float64(cr.DurationMS))
	baselineSorted := make([]string, 0, len(baseline.Metrics.PhaseMSP50))
	for phase := range baseline.Metrics.PhaseMSP50 {
		baselineSorted = append(baselineSorted, phase)
	}
	sort.Strings(baselineSorted)
	for _, phase := range baselineSorted {
		compare("phase_ms_p50."+phase, baseline.Metrics.PhaseMSP50[phase], float64(phaseDurationMS(cr, phase)))
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

// BaselinesSourceRun derives the source-run reference recorded in a baseline:
// the suite run's root under the output directory.
func BaselinesSourceRun(outDir, runID string) string {
	outDir = strings.TrimSpace(outDir)
	runID = strings.TrimSpace(runID)
	if outDir == "" || runID == "" {
		return ""
	}
	return filepath.Join("relurpify_cfg/test_runs", filepath.Base(outDir), runID)
}
