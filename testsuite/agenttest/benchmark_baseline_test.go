package agenttest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// benchmarkSuite builds a minimal benchmark-classified suite whose single
// case succeeds, so the produced overall score is deterministic.
func benchmarkSuite(t *testing.T) *Suite {
	t.Helper()
	suite := &Suite{}
	suite.Metadata.Name = "euclo.bench.testsuite.yaml"
	suite.Metadata.Classification = "benchmark"
	suite.Spec.AgentName = "euclo"
	suite.Spec.Recording.Mode = "off"
	suite.Spec.Models = []ModelSpec{{Name: "model", Endpoint: "http://127.0.0.1:11434"}}
	suite.Spec.Cases = []CaseSpec{{Name: "case_a", Prompt: "edit"}}
	return suite
}

func benchmarkReport(t *testing.T, suite *Suite) *SuiteReport {
	t.Helper()
	report := &SuiteReport{
		RunID:        "run-1",
		BaselinesDir: BaselinesDir(t.TempDir()),
	}
	report.Cases = []CaseReport{sampleCaseReport("case_a", "model", 1234)}
	return report
}

// TestBenchmarkSuiteComparisonRecordsAbsence proves D10.4 at the suite
// level: an absent baseline is absence — never the old
// "missing → within variance" default that made the comparison unfalsifiable.
func TestBenchmarkSuiteComparisonRecordsAbsence(t *testing.T) {
	suite := benchmarkSuite(t)
	suite.Metadata.Benchmark.VarianceThreshold = 5
	report := benchmarkReport(t, suite)

	built, err := BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.False(t, built.Comparison.BaselineFound)
	require.False(t, built.Comparison.WithinVariance, "absence must not read as within variance")
	require.Equal(t, "baseline missing", built.Comparison.Message)
	require.Equal(t, SuiteBaselinePath(report.BaselinesDir, suite.Metadata.Name), built.Comparison.BaselinePath)
}

// TestBenchmarkSuiteComparisonWithinAndOutside proves the suite comparison
// is real once a baseline exists: within threshold vs outside threshold.
func TestBenchmarkSuiteComparisonWithinAndOutside(t *testing.T) {
	suite := benchmarkSuite(t)
	suite.Metadata.Benchmark.VarianceThreshold = 5
	report := benchmarkReport(t, suite)

	path := SuiteBaselinePath(report.BaselinesDir, suite.Metadata.Name)
	produced, err := BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	within := produced.Summary.OverallScore - 1
	outside := produced.Summary.OverallScore - 20
	require.NoError(t, WriteSuiteBaseline(path, &SuiteBaselineFile{
		Schema:     BaselineSchemaV1,
		Suite:      suite.Metadata.Name,
		CapturedAt: time.Now().UTC(),
		SourceRun:  "run-0",
		Score:      BaselineScore{OverallScore: within, VarianceThreshold: 5},
	}))

	built, err := BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.True(t, built.Comparison.BaselineFound)
	require.True(t, built.Comparison.WithinVariance, "a one-point drift is inside a 5-point threshold")
	require.Equal(t, "within variance threshold", built.Comparison.Message)

	require.NoError(t, WriteSuiteBaseline(path, &SuiteBaselineFile{
		Schema:     BaselineSchemaV1,
		Suite:      suite.Metadata.Name,
		CapturedAt: time.Now().UTC(),
		SourceRun:  "run-0",
		Score:      BaselineScore{OverallScore: outside, VarianceThreshold: 5},
	}))
	built, err = BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.True(t, built.Comparison.BaselineFound)
	require.False(t, built.Comparison.WithinVariance)
	require.Equal(t, "outside variance threshold", built.Comparison.Message)
	require.InDelta(t, 20.0, built.Comparison.Delta, 0.0001)
}

// TestBenchmarkSuiteComparisonWithoutThreshold proves a found baseline with
// no configured threshold says so instead of claiming variance either way.
func TestBenchmarkSuiteComparisonWithoutThreshold(t *testing.T) {
	suite := benchmarkSuite(t)
	report := benchmarkReport(t, suite)
	require.NoError(t, WriteSuiteBaseline(SuiteBaselinePath(report.BaselinesDir, suite.Metadata.Name), &SuiteBaselineFile{
		Schema: BaselineSchemaV1,
		Suite:  suite.Metadata.Name,
		Score:  BaselineScore{OverallScore: 42},
	}))

	built, err := BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.True(t, built.Comparison.BaselineFound)
	require.False(t, built.Comparison.WithinVariance)
	require.Equal(t, "variance threshold not configured", built.Comparison.Message)
}

// TestBenchmarkCaseScoreReadsSameBaselineDocument proves the per-case score
// comparison reads the SAME per-case baseline the metrics compare reads —
// the parallel score-baseline family is gone.
func TestBenchmarkCaseScoreReadsSameBaselineDocument(t *testing.T) {
	suite := benchmarkSuite(t)
	suite.Metadata.Benchmark.VarianceThreshold = 5
	report := benchmarkReport(t, suite)

	// Absence first.
	built, err := BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.Len(t, built.Cases, 1)
	require.False(t, built.Cases[0].BaselineFound)
	require.False(t, built.Cases[0].WithinVariance, "an absent case baseline is not \"within variance\"")
	require.Equal(t, "baseline missing", built.Cases[0].Message)
	require.Equal(t, BaselinePath(report.BaselinesDir, suite.Metadata.Name, "case_a", "model"), built.Cases[0].BaselinePath)

	// A baseline without a score block compares as absent for the score.
	require.NoError(t, WriteBaseline(built.Cases[0].BaselinePath,
		BuildBaselineFile(suite.Metadata.Name, sampleCaseReport("case_a", "model", 1234), "run-0", time.Now().UTC())))
	built, err = BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.False(t, built.Cases[0].BaselineFound)
	require.Equal(t, "baseline has no score", built.Cases[0].Message)

	// A baseline with a score block compares: one point inside the
	// threshold, then twenty points outside it.
	produced, err := BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	runScore := produced.Cases[0].OverallScore
	withScore := BuildBaselineFile(suite.Metadata.Name, sampleCaseReport("case_a", "model", 1234), "run-0", time.Now().UTC())
	withScore.Score = &BaselineScore{OverallScore: runScore - 1, VarianceThreshold: 5}
	require.NoError(t, WriteBaseline(produced.Cases[0].BaselinePath, withScore))

	built, err = BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.True(t, built.Cases[0].BaselineFound)
	require.InDelta(t, runScore-1, built.Cases[0].BaselineScore, 0.0001)
	require.True(t, built.Cases[0].WithinVariance)
	require.Equal(t, "within case variance", built.Cases[0].Message)

	withScore.Score.OverallScore = runScore - 20
	require.NoError(t, WriteBaseline(built.Cases[0].BaselinePath, withScore))
	built, err = BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.True(t, built.Cases[0].BaselineFound)
	require.False(t, built.Cases[0].WithinVariance)
	require.Equal(t, "outside case variance", built.Cases[0].Message)
}

// TestBenchmarkCaseArtifactsWriteBesideTheCase proves the per-case benchmark
// artifacts land inside that case's artifacts dir — and that a case with no
// artifacts dir writes nothing (a join against "" would otherwise drop the
// file into the process CWD and pollute the tree).
func TestBenchmarkCaseArtifactsWriteBesideTheCase(t *testing.T) {
	suite := benchmarkSuite(t)
	report := benchmarkReport(t, suite)
	artifacts := t.TempDir()
	report.Cases[0].ArtifactsDir = artifacts

	built, err := BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.Len(t, built.Cases, 1)
	require.FileExists(t, filepath.Join(artifacts, BenchmarkCaseScoreFileName))
	require.FileExists(t, filepath.Join(artifacts, BenchmarkCaseComparisonName))

	data, err := os.ReadFile(filepath.Join(artifacts, BenchmarkCaseScoreFileName))
	require.NoError(t, err)
	var score BenchmarkCaseScore
	require.NoError(t, json.Unmarshal(data, &score))
	require.Equal(t, "case_a", score.Name)
}

// TestBenchmarkReportWorkspaceAndSuccessAreHonest proves the report's
// Workspace is the case's workspace (not the baselines dir) and Success
// reflects the cases rather than being a constant true.
func TestBenchmarkReportWorkspaceAndSuccessAreHonest(t *testing.T) {
	suite := benchmarkSuite(t)
	report := benchmarkReport(t, suite)
	report.Cases[0].Workspace = filepath.Join(t.TempDir(), "derived")

	built, err := BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.Equal(t, report.Cases[0].Workspace, built.Workspace)
	require.True(t, built.Success)

	report.Cases[0].Success = false
	report.FailedCases = 1
	built, err = BuildBenchmarkReport(suite, report)
	require.NoError(t, err)
	require.False(t, built.Success, "a suite with a failed case is not a successful benchmark report")
}
