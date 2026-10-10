package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
	"github.com/spf13/cobra"
)

type fakeAgentTestRunner struct {
	calls int
}

const rootAgentTestName = "agenttest"
const (
	runCmdName        = "run"
	promoteCmdName    = "promote"
	reportCmdName     = "report"
	rerecordCmdName   = "rerecord"
	workspaceFlagName = "--workspace"
	suiteFlagName     = "--suite"
)

func (f *fakeAgentTestRunner) RunSuite(_ context.Context, suite *agenttest.Suite, opts agenttest.RunOptions) (*agenttest.SuiteReport, error) {
	f.calls++
	if suite == nil {
		return nil, context.Canceled
	}
	if opts.TargetWorkspace == "" {
		return nil, context.Canceled
	}
	return &agenttest.SuiteReport{
		SuitePath:   suite.SourcePath,
		Profile:     opts.Profile,
		PassedCases: len(suite.Spec.Cases),
	}, nil
}

func TestNewRootCmdIncludesAgentTestRun(t *testing.T) {
	cmd := NewRootCmd()
	if cmd == nil {
		t.Fatal("root command is nil")
	}
	var agenttestCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == rootAgentTestName {
			agenttestCmd = c
			break
		}
	}
	if agenttestCmd == nil {
		t.Fatal("expected agenttest command to be registered")
	}
	want := map[string]bool{
		runCmdName:      true,
		promoteCmdName:  true,
		reportCmdName:   true,
		rerecordCmdName: true,
	}
	for _, c := range agenttestCmd.Commands() {
		delete(want, c.Name())
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for name := range want {
			missing = append(missing, name)
		}
		t.Fatalf("expected commands not registered: %v", missing)
	}
}

func TestAgentTestRunExecutesSuiteThroughRunner(t *testing.T) {
	ws := t.TempDir()
	suiteDir := filepath.Join(ws, "testsuite", "agenttests")
	if err := os.MkdirAll(suiteDir, 0o750); err != nil {
		t.Fatal(err)
	}
	suitePath := filepath.Join(suiteDir, "demo.testsuite.yaml")
	if err := os.WriteFile(suitePath, []byte(strings.TrimSpace(`
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
`)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fake := &fakeAgentTestRunner{}
	prev := newAgentTestRunnerFn
	newAgentTestRunnerFn = func() agentTestRunner { return fake }
	defer func() { newAgentTestRunnerFn = prev }()

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{workspaceFlagName, ws, rootAgentTestName, runCmdName, suiteFlagName, suitePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", fake.calls)
	}
}
