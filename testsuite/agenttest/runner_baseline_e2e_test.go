package agenttest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
)

// stubPreparedRun redirects the runner's executor + verifier seams for the
// duration of one test. The executor stub materialises a real case report
// from the descriptor the runner prepared (the same artifact shape the
// production executor writes) and bumps one framework-perf counter, standing
// in for that case's graph activity. The verifier stub passes.
func stubPreparedRun(t *testing.T, reportFor func(desc *PreparedRunDescriptor) CaseReport) {
	t.Helper()
	prevExec, prevVerify := PreparedRunExecutorFn, PreparedRunVerifierFn
	t.Cleanup(func() {
		PreparedRunExecutorFn, PreparedRunVerifierFn = prevExec, prevVerify
	})
	PreparedRunExecutorFn = func(_ context.Context, descriptorPath, _, _ string, _ io.Writer) error {
		desc, err := LoadPreparedRunDescriptor(descriptorPath)
		if err != nil {
			return err
		}
		report := reportFor(desc)
		// The executor snapshots the framework counters it owns at report
		// build time (execution.go buildCaseReport).
		perfstats.IncBranchClone()
		report.FrameworkPerf = perfstats.Get()
		return WriteCaseReport(caseReportPath(desc), report)
	}
	PreparedRunVerifierFn = func(_ context.Context, _ *PreparedRun, caseReport CaseReport, _ *Suite, _ CaseSpec, _ sandbox.CommandRunner) (*PreparedRunVerificationReport, error) {
		return &PreparedRunVerificationReport{
			Success: caseReport.Success,
			Checks:  []AssertionResult{{AssertionID: "stub.pass", Tier: "outcome", Passed: true, Message: "stubbed verification"}},
		}, nil
	}
}

// baselineE2EWorkspace materialises a workspace + suite whose recording plan
// is replay (so the runner's preflight never contacts a backend) and whose
// two cases exercise the runner loop end to end.
func baselineE2EWorkspace(t *testing.T) (string, *Suite) {
	t.Helper()
	ws := t.TempDir()
	suiteDir := filepath.Join(ws, "testsuite", "agenttests")
	require.NoError(t, os.MkdirAll(suiteDir, 0o750))
	tapePath := filepath.Join(ws, "tapes", "dummy.tape.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(tapePath), 0o750))
	require.NoError(t, os.WriteFile(tapePath, []byte("{\"kind\":\"request\"}\n"), 0o600))

	suitePath := filepath.Join(suiteDir, "euclo.baseline.testsuite.yaml")
	suiteYAML := strings.TrimSpace(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: euclo.baseline
  description: baseline workflow e2e
  tier: smoke
  quarantined: false
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agents/euclo.yaml
  execution:
    timeout: 90s
  models:
  - name: test-model
    endpoint: http://127.0.0.1:11434
  recording:
    mode: replay
    tape: tapes/dummy.tape.jsonl
  cases:
  - name: case_one
    prompt: first
  - name: case_two
    prompt: second
`) + "\n"
	require.NoError(t, os.WriteFile(suitePath, []byte(suiteYAML), 0o600))

	suite, err := LoadSuite(suitePath)
	require.NoError(t, err)
	return ws, suite
}

func e2eCaseReport(desc *PreparedRunDescriptor) CaseReport {
	return CaseReport{
		Name:         desc.CaseName,
		Model:        desc.ModelName,
		Success:      true,
		DurationMS:   4200,
		ToolCalls:    map[string]int{"file_write": 2},
		TokenUsage:   TokenUsageReport{TotalTokens: 10000, LLMCalls: 4},
		PhaseMetrics: []PhaseMetric{{Phase: "dispatch", DurationMS: 120}},
		Workspace:    desc.DerivedWorkspaceRoot,
		ArtifactsDir: caseArtifactsDir(desc),
	}
}

// TestRunSuiteBaselineWorkflowEndToEnd is the D10 acceptance proof through
// the real runner loop: the first run records absence, promotion writes
// schema-v1 baselines from that run, the second run compares against them
// (report-only), and the framework-perf snapshot is per-case.
func TestRunSuiteBaselineWorkflowEndToEnd(t *testing.T) {
	ws, suite := baselineE2EWorkspace(t)
	stubPreparedRun(t, e2eCaseReport)

	runner := &Runner{}
	ctx := context.Background()

	first, err := runner.RunSuite(ctx, suite, RunOptions{TargetWorkspace: ws})
	require.NoError(t, err)
	require.Len(t, first.Cases, 2)
	for _, cr := range first.Cases {
		require.False(t, cr.BaselineFound, "the first run has no baseline to compare")
		require.Empty(t, cr.PerformanceWarnings)
		require.True(t, cr.Success)
		require.Equal(t, int64(1), cr.FrameworkPerf.BranchClones, "FrameworkPerf is non-zero and per-case")
	}
	require.Equal(t, BaselinesDir(ws), first.BaselinesDir)
	require.NotEmpty(t, first.SourceRun)
	require.Equal(t, first.RunRoot, filepath.Join(ws, ".relurpify_state", "test_run"))

	// Promotion: one schema-v1 baseline per executed case plus the suite
	// score snapshot (the suite is not benchmark-classified, so metrics
	// only).
	require.NoError(t, PromoteBaselines(io.Discard, suite, first, first.SourceRun, false))
	for _, name := range []string{"case_one", "case_two"} {
		baseline, err := LoadBaseline(BaselinePath(first.BaselinesDir, suite.Metadata.Name, name, "test-model"))
		require.NoError(t, err)
		require.Equal(t, BaselineSchemaV1, baseline.Schema)
		require.Equal(t, 10000, baseline.Metrics.TotalTokens)
		require.Equal(t, first.SourceRun, baseline.SourceRun)
		require.Nil(t, baseline.Score)
	}
	_, err = LoadSuiteBaseline(SuiteBaselinePath(first.BaselinesDir, suite.Metadata.Name))
	require.ErrorIs(t, err, ErrBaselineNotFound, "a non-benchmark run writes no suite score snapshot")

	// Second run: the comparison is populated and stays report-only.
	second, err := runner.RunSuite(ctx, suite, RunOptions{TargetWorkspace: ws})
	require.NoError(t, err)
	require.Len(t, second.Cases, 2)
	for _, cr := range second.Cases {
		require.True(t, cr.BaselineFound, "the second run compares against the promoted baseline")
		require.NotNil(t, cr.Baseline)
		require.Empty(t, cr.PerformanceWarnings, "an identical rerun drifts nothing")
		require.True(t, cr.Success)
		require.Equal(t, int64(1), cr.FrameworkPerf.BranchClones,
			"the second case's snapshot excludes the first case's counters (D10.5)")
	}
	line, ok := FormatBaselineCompareLine(second)
	require.True(t, ok)
	require.Contains(t, line, "2 case(s) compared")
}

// TestRunSuiteBaselineWarnIsReportOnly proves a >50% drift warns and never
// fails the case (R-5c) — the compare block cannot gate a run.
func TestRunSuiteBaselineWarnIsReportOnly(t *testing.T) {
	ws, suite := baselineE2EWorkspace(t)
	tokens := 10000
	stubPreparedRun(t, func(desc *PreparedRunDescriptor) CaseReport {
		cr := e2eCaseReport(desc)
		cr.TokenUsage = TokenUsageReport{TotalTokens: tokens, LLMCalls: 4}
		return cr
	})

	runner := &Runner{}
	first, err := runner.RunSuite(context.Background(), suite, RunOptions{TargetWorkspace: ws})
	require.NoError(t, err)
	require.NoError(t, PromoteBaselines(io.Discard, suite, first, first.SourceRun, false))

	// A run far above the baseline warns on every case and still passes.
	tokens = 90000
	second, err := runner.RunSuite(context.Background(), suite, RunOptions{TargetWorkspace: ws})
	require.NoError(t, err)
	for _, cr := range second.Cases {
		require.True(t, cr.BaselineFound)
		require.NotEmpty(t, cr.PerformanceWarnings, "a 9x token drift is far past the 50% threshold")
		require.True(t, cr.Success, "warnings MUST NOT fail the case (R-5c)")
		require.Empty(t, cr.FailureKind)
	}
	require.Equal(t, 2, second.PassedCases)
	require.Equal(t, 0, second.FailedCases)
}
