package agenttest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func sampleCaseReport(name, model string, tokens int) CaseReport {
	return CaseReport{
		Name:       name,
		Model:      model,
		Success:    true,
		DurationMS: 4200,
		ToolCalls:  map[string]int{"file_write": 3, "file_read": 4},
		TokenUsage: TokenUsageReport{TotalTokens: tokens, LLMCalls: 5},
		PhaseMetrics: []PhaseMetric{
			{Phase: "dispatch", DurationMS: 120},
			{Phase: "react_iter", DurationMS: 3100},
		},
		FrameworkPerf: perfSnapshotForBaseline(),
	}
}

// TestBaselineRoundTrip proves write → load deep-equals, and the schema tag
// is enforced on load.
func TestBaselineRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "euclo.code", "case__model.baseline.json")
	baseline := BuildBaselineFile("euclo.code.testsuite.yaml", sampleCaseReport("case", "model", 18432), ".relurpify_state/test_run/euclo/run-1", time.Unix(1700000000, 0))
	baseline.Schema = BaselineSchemaV1
	require.NoError(t, WriteBaseline(path, baseline))

	loaded, err := LoadBaseline(path)
	require.NoError(t, err)
	require.Equal(t, baseline, loaded)

	// A foreign schema is rejected, never silently accepted.
	baseline.Schema = "someone-elses/baseline/v9"
	foreign, marshalErr := json.MarshalIndent(baseline, "", "  ")
	require.NoError(t, marshalErr)
	require.NoError(t, os.WriteFile(path, foreign, 0o600))
	_, err = LoadBaseline(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema")
}

// TestComparePopulatesAllR5Axes proves the R-5 axes are all compared:
// framework perf, token count, and tool-call count (plus the phase timing
// signal) — each under the key it was recorded under.
func TestComparePopulatesAllR5Axes(t *testing.T) {
	dir := t.TempDir()
	path := BaselinePath(dir, "suite.yaml", "case", "model")
	baseline := BuildBaselineFile("suite.yaml", sampleCaseReport("case", "model", 10000), "run-0", time.Unix(1700000000, 0))
	require.NoError(t, WriteBaseline(path, baseline))

	// The same run shape, but 3x the framework branch clones.
	cr := sampleCaseReport("case", "model", 11000)
	cr.FrameworkPerf.BranchClones = baseline.Metrics.FrameworkPerf["branch_clones"] * 3
	CompareAgainstBaseline(&cr, path, nil)

	require.True(t, cr.BaselineFound)
	metrics := map[string]BaselineMetricDelta{}
	for _, m := range cr.Baseline.Metrics {
		metrics[m.Metric] = m
	}
	for _, axis := range []string{"total_tokens", "tool_calls", "framework_perf.branch_clones", "phase_ms_p50.dispatch"} {
		require.Contains(t, metrics, axis, "axis %s must be compared", axis)
	}
	require.InDelta(t, 2.0, metrics["framework_perf.branch_clones"].Delta, 0.0001)
	require.NotEmpty(t, cr.PerformanceWarnings, "a 3x framework counter is past the 50% threshold")
	require.Equal(t, "framework_perf.branch_clones", cr.PerformanceWarnings[len(cr.PerformanceWarnings)-1].Metric)
	require.True(t, cr.Success, "warnings MUST NOT affect Success (R-5c)")
}

// TestBaselineMetricsCarryNoWallClock proves the v1 schema carries only the
// stable axes: a wall-clock duration belongs to the run report, not to a
// baseline that must compare reproducibly.
func TestBaselineMetricsCarryNoWallClock(t *testing.T) {
	baseline := BuildBaselineFile("suite.yaml", sampleCaseReport("case", "model", 10), "run-0", time.Unix(1700000000, 0))
	data, err := json.Marshal(baseline)
	require.NoError(t, err)
	require.Contains(t, string(data), "total_tokens")
	require.Contains(t, string(data), "tool_calls")
	require.Contains(t, string(data), "phase_ms_p50")
	require.Contains(t, string(data), "framework_perf")
	require.NotContains(t, string(data), "duration_ms")
}

// TestBaselineScoreRoundTrip proves the benchmark score block survives the
// write → load round trip: the metrics and the score live in ONE document
// (one producer, one consumer), not in a parallel score-baseline family.
func TestBaselineScoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := BaselinePath(dir, "suite.yaml", "case", "model")
	baseline := BuildBaselineFile("suite.yaml", sampleCaseReport("case", "model", 18432), "run-1", time.Unix(1700000000, 0))
	baseline.Score = &BaselineScore{
		OverallScore:      87.5,
		DimensionScores:   map[string]float64{"completion": 1, "stability": 0.8},
		ScoreFamily:       "capability",
		VarianceThreshold: 5,
	}
	require.NoError(t, WriteBaseline(path, baseline))

	loaded, err := LoadBaseline(path)
	require.NoError(t, err)
	require.Equal(t, baseline, loaded)
	require.NotNil(t, loaded.Score)
	require.InDelta(t, 87.5, loaded.Score.OverallScore, 0.0001)
	require.InDelta(t, 0.8, loaded.Score.DimensionScores["stability"], 0.0001)
}

// TestSuiteBaselineRoundTrip proves the suite-level score snapshot round
// trips beside the per-case baselines of the same suite.
func TestSuiteBaselineRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := SuiteBaselinePath(dir, "euclo.gemma4.smoke.testsuite.yaml")
	baseline := &SuiteBaselineFile{
		Schema:     BaselineSchemaV1,
		Suite:      "euclo.gemma4.smoke.testsuite.yaml",
		CapturedAt: time.Unix(1700000000, 0).UTC(),
		SourceRun:  "run-9",
		Score: BaselineScore{
			OverallScore:      91.25,
			DimensionScores:   map[string]float64{"completion": 1},
			ScoreFamily:       "journey",
			VarianceThreshold: 10,
		},
	}
	require.NoError(t, WriteSuiteBaseline(path, baseline))

	loaded, err := LoadSuiteBaseline(path)
	require.NoError(t, err)
	require.Equal(t, baseline, loaded)

	// Absence is data, not an error.
	_, err = LoadSuiteBaseline(filepath.Join(dir, "absent", SuiteBaselineFilename))
	require.ErrorIs(t, err, ErrBaselineNotFound)
}

// TestBaselinePathSanitization proves the tape naming convention: the model
// token's `:` and `/` sanitize to `_`.
func TestBaselinePathSanitization(t *testing.T) {
	path := BaselinePath(t.TempDir(), "euclo.gemma4.smoke.testsuite.yaml", "simple_code_edit", "qwen3:4b")
	require.Contains(t, path, "simple_code_edit__qwen3_4b.baseline.json")
	require.NotContains(t, filepath.Base(path), ":")
	require.Equal(t, GoldenBaselineFilename("simple_code_edit", "qwen3:4b"), filepath.Base(path))
}

// TestBaselinesDirResolvesCanonicalStateTree proves R-5a: baselines live in
// the workspace's runtime state tree beside the runs they reference —
// resolved through the canonical config layout, never a second convention.
func TestBaselinesDirResolvesCanonicalStateTree(t *testing.T) {
	ws := t.TempDir()
	require.Equal(t,
		filepath.Join(config.New(ws).TestRunsDir(), BaselinesDirName),
		BaselinesDir(ws))
	require.True(t, filepath.IsAbs(BaselinesDir(ws)), "baselines dir must be absolute")
}

// TestComparePopulatesPerformance proves the compare path (D10.4): a found
// baseline populates BaselineFound + per-metric deltas, a >50% delta warns,
// and Success is NEVER affected.
func TestComparePopulatesPerformance(t *testing.T) {
	dir := t.TempDir()
	path := BaselinePath(dir, "suite.yaml", "case", "model")
	baseline := BuildBaselineFile("suite.yaml", sampleCaseReport("case", "model", 10000), "run-0", time.Unix(1700000000, 0))
	require.NoError(t, WriteBaseline(path, baseline))

	// Current run: 3x the tokens — far past the 50% warn threshold.
	cr := sampleCaseReport("case", "model", 30000)
	CompareAgainstBaseline(&cr, path, nil)

	require.True(t, cr.BaselineFound)
	require.NotNil(t, cr.Baseline)
	require.True(t, len(cr.Baseline.Metrics) >= 3, "tokens, tool_calls, duration compared")
	require.NotEmpty(t, cr.PerformanceWarnings, "a >50% delta appends a warning")
	var tokenDelta *BaselineMetricDelta
	for i := range cr.Baseline.Metrics {
		if cr.Baseline.Metrics[i].Metric == "total_tokens" {
			tokenDelta = &cr.Baseline.Metrics[i]
		}
	}
	require.NotNil(t, tokenDelta)
	require.InDelta(t, 2.0, tokenDelta.Delta, 0.0001, "delta = (30000-10000)/10000")

	// Report-only: the comparison never touches Success.
	require.True(t, cr.Success, "warnings MUST NOT affect Success (R-5c)")

	// A current run within variance warns nothing.
	closeCR := sampleCaseReport("case", "model", 11000)
	CompareAgainstBaseline(&closeCR, path, nil)
	require.True(t, closeCR.BaselineFound)
	require.Empty(t, closeCR.PerformanceWarnings, "a 10% drift is under the warn threshold")
}

// TestCompareAgainstBaselineEmitsPerMetricEvents proves the per-compared-metric
// telemetry contract (FR-28 observability).
func TestCompareAgainstBaselineEmitsPerMetricEvents(t *testing.T) {
	dir := t.TempDir()
	path := BaselinePath(dir, "suite.yaml", "case", "model")
	require.NoError(t, WriteBaseline(path, BuildBaselineFile("suite.yaml", sampleCaseReport("case", "model", 10000), "run-0", time.Unix(1700000000, 0))))

	sink := &recordingBaselineEmitter{}
	cr := sampleCaseReport("case", "model", 12345)
	CompareAgainstBaseline(&cr, path, sink)

	require.NotEmpty(t, sink.metrics)
	require.Contains(t, sink.metrics, "total_tokens")
	for metric, delta := range sink.metrics {
		require.False(t, delta != delta, "delta must be a number for %s", metric)
	}
}

type recordingBaselineEmitter struct {
	metrics map[string]float64
}

func (r *recordingBaselineEmitter) EmitBaselineCompared(metric string, delta float64) {
	if r.metrics == nil {
		r.metrics = map[string]float64{}
	}
	r.metrics[metric] = delta
}

// TestMissingBaselineRecordsAbsence proves the deleted default: a missing
// baseline records absence — no "missing → within variance" branch, no
// warnings, and BaselineFound stays false.
func TestMissingBaselineRecordsAbsence(t *testing.T) {
	cr := sampleCaseReport("case", "model", 1000)
	CompareAgainstBaseline(&cr, filepath.Join(t.TempDir(), "absent.baseline.json"), nil)
	require.False(t, cr.BaselineFound)
	require.Nil(t, cr.Baseline)
	require.Empty(t, cr.PerformanceWarnings)
	require.True(t, cr.Success)
	require.True(t, errors.Is(ErrBaselineNotFound, ErrBaselineNotFound))
}

// TestUnreadableBaselineIsRecordedAsUnreadable proves absence and corruption
// are different facts: a missing baseline is quiet absence, while a baseline
// file that exists and cannot be parsed is recorded with its reason.
func TestUnreadableBaselineIsRecordedAsUnreadable(t *testing.T) {
	dir := t.TempDir()
	path := BaselinePath(dir, "suite.yaml", "case", "model")
	require.NoError(t, fs.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

	cr := sampleCaseReport("case", "model", 1000)
	CompareAgainstBaseline(&cr, path, nil)
	require.False(t, cr.BaselineFound)
	require.NotNil(t, cr.Baseline, "a corrupt baseline must not hide as absence")
	require.Contains(t, cr.Baseline.Message, "unreadable")
	require.Empty(t, cr.PerformanceWarnings)
	require.True(t, cr.Success, "an unreadable baseline cannot fail the case (R-5c)")
}

// TestSkippedCaseComparesAsAbsence proves a skipped case carries no metrics
// and therefore records absence rather than a wall of false deltas.
func TestSkippedCaseComparesAsAbsence(t *testing.T) {
	dir := t.TempDir()
	path := BaselinePath(dir, "suite.yaml", "case", "model")
	require.NoError(t, WriteBaseline(path, BuildBaselineFile("suite.yaml", sampleCaseReport("case", "model", 10000), "run-0", time.Unix(1700000000, 0))))
	cr := sampleCaseReport("case", "model", 0)
	cr.Skipped = true
	CompareAgainstBaseline(&cr, path, nil)
	require.False(t, cr.BaselineFound)
	require.Empty(t, cr.PerformanceWarnings)
}

// TestBaselineDirResolvesSourceWorkspace proves the R-5a regression proof:
// the descriptor's BaselineDir resolves from the INVOKED (source) workspace
// root — the derived workspace carries the source path, never a copy.
func TestBaselineDirResolvesSourceWorkspace(t *testing.T) {
	source := t.TempDir()
	derived := filepath.Join(t.TempDir(), "derived-workspace")
	require.NotEqual(t, BaselinesDir(derived), BaselinesDir(source))

	suite := &Suite{}
	suite.Metadata.Name = "euclo.gemma4.smoke.testsuite.yaml"
	suite.SourcePath = filepath.Join(source, "euclo.gemma4.smoke.testsuite.yaml")
	suite.Spec.AgentName = "euclo"
	suite.Spec.Recording.Mode = "off"
	suite.Spec.Models = []ModelSpec{{Name: "qwen3:4b", Endpoint: "http://127.0.0.1:11434"}}
	suite.Spec.Cases = []CaseSpec{{Name: "simple_code_edit", Prompt: "edit"}}

	desc, err := BuildPreparedRunDescriptor(suite, suite.Spec.Cases[0], suite.Spec.Models[0],
		RunOptions{TargetWorkspace: source}, source, filepath.Join(source, "runroot"), "run-1")
	require.NoError(t, err)
	require.Equal(t, BaselinesDir(source), desc.BaselineDir)
	require.True(t, filepath.IsAbs(desc.BaselineDir))
	require.NotContains(t, desc.BaselineDir, derived)

	// A relative workspace invocation still yields an absolute baselines
	// path: the compare must never resolve against a CWD.
	relSuite := *suite
	descRel, err := BuildPreparedRunDescriptor(&relSuite, suite.Spec.Cases[0], suite.Spec.Models[0],
		RunOptions{TargetWorkspace: "."}, source, filepath.Join(source, "runroot"), "run-2")
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(descRel.BaselineDir), "relative workspace must not leak into BaselineDir")
}

// TestPromoteBaselinesWritesFromRealReports proves the producer: promotion
// writes one schema-v1 file per executed case, skips skipped cases, attaches
// the benchmark score when the run produced one, and dry-run writes nothing.
func TestPromoteBaselinesWritesFromRealReports(t *testing.T) {
	dir := t.TempDir()
	suite := &Suite{}
	suite.Metadata.Name = "euclo.code.testsuite.yaml"
	report := &SuiteReport{RunID: "run-1", BaselinesDir: BaselinesDir(dir), SourceRun: ".relurpify_state/test_run/run-1"}
	report.Cases = []CaseReport{
		sampleCaseReport("case_a", "model", 1234),
		sampleCaseReport("case_b", "model", 4321),
	}
	report.Cases[1].Skipped = true

	require.NoError(t, PromoteBaselines(nil, suite, report, report.SourceRun, false))
	written := BaselinePath(report.BaselinesDir, suite.Metadata.Name, "case_a", "model")
	loaded, err := LoadBaseline(written)
	require.NoError(t, err)
	require.Equal(t, BaselineSchemaV1, loaded.Schema)
	require.Equal(t, 1234, loaded.Metrics.TotalTokens)
	require.Equal(t, ".relurpify_state/test_run/run-1", loaded.SourceRun)
	require.Nil(t, loaded.Score, "a non-benchmark run baselines metrics only")
	_, err = LoadBaseline(BaselinePath(report.BaselinesDir, suite.Metadata.Name, "case_b", "model"))
	require.ErrorIs(t, err, ErrBaselineNotFound, "skipped cases produce no baseline")

	// Dry-run writes nothing.
	dryDir := t.TempDir()
	dryReport := &SuiteReport{RunID: "run-2", BaselinesDir: BaselinesDir(dryDir), SourceRun: "run-2"}
	dryReport.Cases = []CaseReport{sampleCaseReport("case_a", "model", 1234)}
	require.NoError(t, PromoteBaselines(nil, suite, dryReport, "run-2", true))
	_, err = LoadBaseline(BaselinePath(dryReport.BaselinesDir, suite.Metadata.Name, "case_a", "model"))
	require.ErrorIs(t, err, ErrBaselineNotFound)
}

// TestPromoteBaselinesWritesBenchmarkScores proves the benchmark-mode
// producer (R-5: fix, not delete): a benchmark-classified run promotes its
// per-case scores into the same baseline documents and writes the
// suite-level score snapshot.
func TestPromoteBaselinesWritesBenchmarkScores(t *testing.T) {
	dir := t.TempDir()
	suite := &Suite{}
	suite.Metadata.Name = "euclo.bench.testsuite.yaml"
	suite.Metadata.Benchmark = BenchmarkMeta{ScoreFamily: "capability", VarianceThreshold: 5}

	report := &SuiteReport{RunID: "run-1", BaselinesDir: BaselinesDir(dir), SourceRun: "run-1"}
	report.Cases = []CaseReport{sampleCaseReport("case_a", "model", 1234)}
	report.Benchmark = &BenchmarkReport{
		SuiteName:         suite.Metadata.Name,
		ScoreFamily:       "capability",
		VarianceThreshold: 5,
		Summary:           BenchmarkSummary{OverallScore: 91.5, DimensionScores: map[string]float64{"completion": 1}},
	}
	report.Benchmark.Cases = []BenchmarkCaseScore{{
		Name:         "case_a",
		Model:        "model",
		OverallScore: 87.5,
		DimensionScores: []BenchmarkCaseDimensionScore{
			{Name: "completion", Weight: 0.5, Score: 1},
		},
	}}

	require.NoError(t, PromoteBaselines(nil, suite, report, "run-1", false))

	caseBaseline, err := LoadBaseline(BaselinePath(report.BaselinesDir, suite.Metadata.Name, "case_a", "model"))
	require.NoError(t, err)
	require.NotNil(t, caseBaseline.Score, "the case score lands in the same baseline document")
	require.InDelta(t, 87.5, caseBaseline.Score.OverallScore, 0.0001)
	require.InDelta(t, 1.0, caseBaseline.Score.DimensionScores["completion"], 0.0001)
	require.InDelta(t, 5.0, caseBaseline.Score.VarianceThreshold, 0.0001)

	suiteBaseline, err := LoadSuiteBaseline(SuiteBaselinePath(report.BaselinesDir, suite.Metadata.Name))
	require.NoError(t, err)
	require.InDelta(t, 91.5, suiteBaseline.Score.OverallScore, 0.0001)
	require.Equal(t, "run-1", suiteBaseline.SourceRun)
}

// TestFormatBaselineCompareLine proves the rendered compare block: a line
// only when something was compared, silence when every baseline is absent.
func TestFormatBaselineCompareLine(t *testing.T) {
	empty := &SuiteReport{Cases: []CaseReport{sampleCaseReport("case", "model", 10)}}
	line, ok := FormatBaselineCompareLine(empty)
	require.False(t, ok, "absence must render nothing that reads as variance")
	require.Empty(t, line)

	compared := &SuiteReport{Cases: []CaseReport{sampleCaseReport("case", "model", 10)}}
	compared.Cases[0].BaselineFound = true
	compared.Cases[0].PerformanceWarnings = []PerformanceWarning{{Metric: "total_tokens"}}
	line, ok = FormatBaselineCompareLine(compared)
	require.True(t, ok)
	require.Contains(t, line, "1 case(s) compared")
	require.Contains(t, line, "1 with warning(s)")
	require.Contains(t, line, "report-only")

	line, ok = FormatBaselineCompareLine(nil)
	require.False(t, ok)
	require.Empty(t, line)
}

// TestSourceRunReferenceIsWorkspaceRelative proves the baseline provenance
// reference is relative to the invoking workspace and degrades to the
// absolute path only when the run root is outside it.
func TestSourceRunReferenceIsWorkspaceRelative(t *testing.T) {
	ws := t.TempDir()
	runRoot := filepath.Join(ws, ".relurpify_state", "test_run", "run-1")
	require.Equal(t,
		filepath.Join(".relurpify_state", "test_run", "run-1"),
		sourceRunReference(ws, runRoot))
	require.Equal(t, "", sourceRunReference(ws, ""))
	outside := filepath.Join(t.TempDir(), "run-1")
	require.Equal(t, outside, sourceRunReference(ws, outside))
	require.Equal(t, runRoot, sourceRunReference("", runRoot))
}

func perfSnapshotForBaseline() perfstats.Snapshot {
	return perfstats.Snapshot{
		BranchClones:              2,
		BranchMergeCount:          1,
		ArtifactBudgetRescanCount: 3,
		ProgressiveFileReadCount:  7,
		RuntimeMemorySearchCount:  4,
	}
}
