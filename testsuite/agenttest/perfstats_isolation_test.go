package agenttest

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/telemetry/perfstats"
)

// perfBumpingAgent stands in for the graph's branch accounting: each
// executed case bumps exactly one framework counter, the way a real graph
// clone does (execution/agentgraph graph.go). The hermetic agent executes
// no real branch, so the counter is the deterministic witness.
type perfBumpingAgent struct{}

func (perfBumpingAgent) Execute(_ context.Context, _ *execution.Task, _ *contextdata.Envelope) (*execution.Result, error) {
	perfstats.IncBranchClone()
	return &execution.Result{Success: true}, nil
}

func (perfBumpingAgent) Initialize(_ *execution.Config) error { return nil }
func (perfBumpingAgent) Capabilities() []string               { return nil }
func (perfBumpingAgent) BuildGraph(_ context.Context, _ *execution.Task) (*agentgraph.Graph, error) {
	return nil, nil
}

// TestFrameworkPerfIsPerCaseTruth proves D10.5 through the real executor:
// the counters the runner resets before each case make the report snapshot —
// and the framework_perf.json artifact that documents it — the activity of
// THAT case, not the process lifetime.
func TestFrameworkPerfIsPerCaseTruth(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	desc.ExecutionArtifactsDir = filepath.Join(desc.ExecutionDir, "artifacts")
	desc.MaxRetries = 0

	runCase := func() CaseReport {
		t.Helper()
		// The runner resets the process counters before each case executes
		// (runner.go runPreparedCase); the executor snapshots at report
		// build. Repeat the runner's reset here to prove the pair.
		perfstats.Reset()
		exec := (&PreparedRunExecutor{}).
			WithRunnerOverride(fakeRunner{}).
			WithAgentOverride(perfBumpingAgent{})
		require.NoError(t, exec.Execute(context.Background(), desc, io.Discard))
		data, err := os.ReadFile(caseReportPath(desc))
		require.NoError(t, err)
		var report CaseReport
		require.NoError(t, json.Unmarshal(data, &report))
		return report
	}

	first := runCase()
	require.Equal(t, int64(1), first.FrameworkPerf.BranchClones, "the first case's snapshot is non-zero and its own")

	// The artifact the benchmark scorer reads carries the SAME snapshot the
	// report carries — not a second, later read of the process counters.
	artifactData, err := os.ReadFile(filepath.Join(desc.ExecutionArtifactsDir, "framework_perf.json"))
	require.NoError(t, err)
	var artifactSnapshot perfstats.Snapshot
	require.NoError(t, json.Unmarshal(artifactData, &artifactSnapshot))
	require.Equal(t, first.FrameworkPerf, artifactSnapshot)

	second := runCase()
	require.Equal(t, int64(1), second.FrameworkPerf.BranchClones,
		"the second case's snapshot excludes the first case's counters (per-case truth, D10.5)")

	// Without the reset the accumulator would keep climbing — the defect
	// the per-case reset exists to kill.
	perfstats.Reset()
	perfstats.IncBranchClone()
	perfstats.IncBranchClone()
	require.Equal(t, int64(2), perfstats.Get().BranchClones)
}
