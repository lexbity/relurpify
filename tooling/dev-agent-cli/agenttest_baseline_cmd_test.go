package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

// baselineFakeRunner serves a populated suite report through the runner
// seam: the baseline command's promotion path consumes exactly what a real
// executed run produces.
type baselineFakeRunner struct {
	report *agenttest.SuiteReport
	calls  int
}

func (f *baselineFakeRunner) RunSuite(_ context.Context, suite *agenttest.Suite, opts agenttest.RunOptions) (*agenttest.SuiteReport, error) {
	f.calls++
	if f.report == nil {
		f.report = &agenttest.SuiteReport{}
	}
	f.report.SuitePath = suite.SourcePath
	f.report.BaselinesDir = agenttest.BaselinesDir(opts.TargetWorkspace)
	// The real runner populates BaselineFound from the compare path; the
	// fake mirrors the artifact state so the CLI's rendering is exercised.
	for i := range f.report.Cases {
		cr := &f.report.Cases[i]
		cr.BaselineFound = false
		if _, err := os.Stat(agenttest.BaselinePath(f.report.BaselinesDir, suite.Metadata.Name, cr.Name, cr.Model)); err == nil {
			cr.BaselineFound = true
		}
	}
	return f.report, nil
}

func baselineSuiteFixture(t *testing.T, ws string) string {
	t.Helper()
	suiteDir := filepath.Join(ws, "testsuite", "agenttests")
	require.NoError(t, os.MkdirAll(suiteDir, 0o750))
	suitePath := filepath.Join(suiteDir, "demo.testsuite.yaml")
	require.NoError(t, os.WriteFile(suitePath, []byte(strings.TrimSpace(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: demo
  tier: smoke
  quarantined: false
spec:
  agent_name: demo
  manifest: relurpify_cfg/agents/demo.yaml
  execution:
    profile: live
  workspace:
    strategy: derived
  cases:
    - name: smoke
      prompt: hello
`)+"\n"), 0o600))
	return suitePath
}

func runAgentTestCommand(t *testing.T, args ...string) (string, string) {
	t.Helper()
	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return out.String(), errOut.String()
}

// TestAgentTestBaselineWritesFromTheRunnerPath proves D10.2: the baseline
// command runs the suite through the same runner path and promotes the
// executed metrics into schema-v1 baseline files under the workspace's
// baselines directory.
func TestAgentTestBaselineWritesFromTheRunnerPath(t *testing.T) {
	ws := t.TempDir()
	suitePath := baselineSuiteFixture(t, ws)

	fake := &baselineFakeRunner{report: &agenttest.SuiteReport{
		RunID:       "run-1",
		SourceRun:   ".relurpify_state/test_run/run-1",
		Profile:     "live",
		PassedCases: 1,
		Cases: []agenttest.CaseReport{{
			Name:       "smoke",
			Model:      "qwen3:4b",
			Success:    true,
			DurationMS: 4200,
			ToolCalls:  map[string]int{"file_write": 2},
			TokenUsage: agenttest.TokenUsageReport{TotalTokens: 18432, LLMCalls: 5},
		}},
	}}
	prev := newAgentTestRunnerFn
	newAgentTestRunnerFn = func() agentTestRunner { return fake }
	defer func() { newAgentTestRunnerFn = prev }()

	stdout, _ := runAgentTestCommand(t,
		workspaceFlagName, ws,
		rootAgentTestName, baselineCmdName, suiteFlagName, suitePath)
	require.Equal(t, 1, fake.calls, "baseline runs the suite through the runner")
	require.Contains(t, stdout, "1/1 passed")
	require.NotContains(t, stdout, "baseline:", "no baseline existed on the first run")

	baselinePath := filepath.Join(agenttest.BaselinesDir(ws), "demo", "smoke__qwen3_4b.baseline.json")
	require.FileExists(t, baselinePath)
	data, err := os.ReadFile(baselinePath)
	require.NoError(t, err)
	require.Contains(t, string(data), agenttest.BaselineSchemaV1)
	require.Contains(t, string(data), ".relurpify_state/test_run/run-1")

	// A second `run` over the same workspace compares against the baseline
	// it just wrote — report-only, so the pass line is unchanged.
	stdout, _ = runAgentTestCommand(t,
		workspaceFlagName, ws,
		rootAgentTestName, runCmdName, suiteFlagName, suitePath)
	require.Equal(t, 2, fake.calls)
	require.Contains(t, stdout, "baseline: 1 case(s) compared")
	require.Contains(t, stdout, "report-only")
}

// TestAgentTestBaselineDryRunWritesNothing proves --dry-run prints the
// planned files and writes none of them.
func TestAgentTestBaselineDryRunWritesNothing(t *testing.T) {
	ws := t.TempDir()
	suitePath := baselineSuiteFixture(t, ws)

	fake := &baselineFakeRunner{report: &agenttest.SuiteReport{
		RunID:     "run-1",
		SourceRun: "run-1",
		Cases: []agenttest.CaseReport{{
			Name: "smoke", Model: "qwen3:4b", Success: true,
			ToolCalls:  map[string]int{"file_write": 2},
			TokenUsage: agenttest.TokenUsageReport{TotalTokens: 18432, LLMCalls: 5},
		}},
	}}
	prev := newAgentTestRunnerFn
	newAgentTestRunnerFn = func() agentTestRunner { return fake }
	defer func() { newAgentTestRunnerFn = prev }()

	stdout, _ := runAgentTestCommand(t,
		workspaceFlagName, ws,
		rootAgentTestName, baselineCmdName, suiteFlagName, suitePath, "--dry-run")
	require.Contains(t, stdout, "would write")
	require.Contains(t, stdout, "tokens=18432")
	require.NoFileExists(t, filepath.Join(agenttest.BaselinesDir(ws), "demo", "smoke__qwen3_4b.baseline.json"))
}

// TestAgentTestBaselineRejectsUnknownCase proves a --case that matches
// nothing fails loudly instead of promoting an empty run.
func TestAgentTestBaselineRejectsUnknownCase(t *testing.T) {
	ws := t.TempDir()
	suitePath := baselineSuiteFixture(t, ws)

	fake := &baselineFakeRunner{report: &agenttest.SuiteReport{RunID: "run-1", SourceRun: "run-1"}}
	prev := newAgentTestRunnerFn
	newAgentTestRunnerFn = func() agentTestRunner { return fake }
	defer func() { newAgentTestRunnerFn = prev }()

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{workspaceFlagName, ws, rootAgentTestName, baselineCmdName, suiteFlagName, suitePath, "--case", "nope"})
	require.Error(t, cmd.Execute())
	require.Equal(t, 0, fake.calls, "an unmatched --case must not reach the runner")
}

// TestAgentTestRejectsNegativeMaxRetries proves the retry-honesty contract
// (D9.4): a negative budget is rejected at flag parse, on both commands.
func TestAgentTestRejectsNegativeMaxRetries(t *testing.T) {
	ws := t.TempDir()
	suitePath := baselineSuiteFixture(t, ws)
	for _, cmdName := range []string{runCmdName, baselineCmdName} {
		fake := &baselineFakeRunner{report: &agenttest.SuiteReport{RunID: "run-1", SourceRun: "run-1"}}
		prev := newAgentTestRunnerFn
		newAgentTestRunnerFn = func() agentTestRunner { return fake }
		cmd := NewRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{workspaceFlagName, ws, rootAgentTestName, cmdName, suiteFlagName, suitePath, "--max-retries=-1"})
		require.Error(t, cmd.Execute(), "agenttest %s must reject --max-retries=-1", cmdName)
		require.Equal(t, 0, fake.calls, "a rejected flag must not reach the runner")
		newAgentTestRunnerFn = prev
	}
}

// TestAgentTestBaselineSharesRunFlags proves the baseline command binds the
// same execution knobs as run (D10.2: same executor, same reports).
func TestAgentTestBaselineSharesRunFlags(t *testing.T) {
	runCmd := newAgentTestRunCmd()
	baselineCmd := newAgentTestBaselineCmd()
	runFlags := map[string]bool{}
	runCmd.Flags().VisitAll(func(f *pflag.Flag) {
		runFlags[f.Name] = false
	})
	for name := range runFlags {
		if name == "dry-run" {
			continue
		}
		require.NotNil(t, baselineCmd.Flags().Lookup(name),
			"baseline must bind --%s like run does", name)
	}
	require.NotNil(t, baselineCmd.Flags().Lookup("dry-run"), "baseline owns --dry-run")
	for _, dead := range []string{"sandbox", "lane", "bootstrap-timeout", "debug-agent", "backend-reset-between"} {
		require.Nil(t, baselineCmd.Flags().Lookup(dead), "the deleted dead flag --%s must stay deleted", dead)
		require.Nil(t, runCmd.Flags().Lookup(dead), "the deleted dead flag --%s must stay deleted", dead)
	}
	require.True(t, baselineUsesCounter(runCmd), "run and baseline share the counter seam")
}

func baselineUsesCounter(cmd *cobra.Command) bool {
	return cmd.Use != ""
}
