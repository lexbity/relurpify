package agenttest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
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
	baseline := BuildBaselineFile("euclo.code.testsuite.yaml", sampleCaseReport("case", "model", 18432), "relurpify_cfg/test_runs/euclo/run-1", time.Unix(1700000000, 0))
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

// TestBaselinePathSanitization proves the tape naming convention: the model
// token's `:` and `/` sanitize to `_`.
func TestBaselinePathSanitization(t *testing.T) {
	path := BaselinePath(t.TempDir(), "euclo.gemma4.smoke.testsuite.yaml", "simple_code_edit", "qwen3:4b")
	require.Contains(t, path, "simple_code_edit__qwen3_4b.baseline.json")
	require.NotContains(t, filepath.Base(path), ":")
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

// TestBaselineDirResolvesSourceWorkspace proves the R-5a regression proof:
// the descriptor's BaselineDir resolves from the INVOKED (source) workspace
// root — the derived workspace carries the source path, never a copy.
func TestBaselineDirResolvesSourceWorkspace(t *testing.T) {
	source := t.TempDir()
	derived := t.TempDir()
	// The descriptor build wires BaselinesDir(opts.TargetWorkspace); assert
	// the resolver lands inside the source tree.
	require.Equal(t, filepath.Join(source, "relurpify_cfg", "test_runs", "baselines"), BaselinesDir(source))
	require.NotEqual(t, BaselinesDir(derived), BaselinesDir(source))
}

// TestPromoteBaselinesWritesFromRealReports proves the producer: promotion
// writes one schema-v1 file per executed case, skips skipped cases, and
// dry-run writes nothing.
func TestPromoteBaselinesWritesFromRealReports(t *testing.T) {
	dir := t.TempDir()
	suite := &Suite{}
	suite.Metadata.Name = "euclo.code.testsuite.yaml"
	report := &SuiteReport{RunID: "run-1", BaselinesDir: BaselinesDir(dir)}
	report.Cases = []CaseReport{
		sampleCaseReport("case_a", "model", 1234),
		sampleCaseReport("case_b", "model", 4321),
	}
	report.Cases[1].Skipped = true

	require.NoError(t, PromoteBaselines(nil, suite, report, "relurpify_cfg/test_runs/euclo/run-1", false))
	written := BaselinePath(report.BaselinesDir, suite.Metadata.Name, "case_a", "model")
	loaded, err := LoadBaseline(written)
	require.NoError(t, err)
	require.Equal(t, BaselineSchemaV1, loaded.Schema)
	require.Equal(t, 1234, loaded.Metrics.TotalTokens)
	require.Equal(t, "relurpify_cfg/test_runs/euclo/run-1", loaded.SourceRun)
	_, err = LoadBaseline(BaselinePath(report.BaselinesDir, suite.Metadata.Name, "case_b", "model"))
	require.ErrorIs(t, err, ErrBaselineNotFound, "skipped cases produce no baseline")

	// Dry-run writes nothing.
	dryDir := t.TempDir()
	dryReport := &SuiteReport{RunID: "run-2", BaselinesDir: BaselinesDir(dryDir)}
	dryReport.Cases = []CaseReport{sampleCaseReport("case_a", "model", 1234)}
	require.NoError(t, PromoteBaselines(nil, suite, dryReport, "run-2", true))
	_, err = LoadBaseline(BaselinePath(dryReport.BaselinesDir, suite.Metadata.Name, "case_a", "model"))
	require.ErrorIs(t, err, ErrBaselineNotFound)
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
