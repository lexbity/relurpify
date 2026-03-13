package testfu

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	runnerpkg "github.com/lexcodex/relurpify/named/testfu/runner"
)

type fakeRunner struct {
	lastSuite *runnerpkg.Suite
	lastOpts  runnerpkg.RunOptions
	report    *runnerpkg.SuiteReport
	err       error
}

func (r *fakeRunner) RunSuite(_ context.Context, suite *runnerpkg.Suite, opts runnerpkg.RunOptions) (*runnerpkg.SuiteReport, error) {
	r.lastSuite = suite
	r.lastOpts = opts
	return r.report, r.err
}

func TestParseRequestListSuites(t *testing.T) {
	req := parseRequest(&core.Task{Instruction: "list_suites"})
	if req.Action != actionListSuites {
		t.Fatalf("expected list_suites action, got %q", req.Action)
	}
}

func TestExecuteRunCaseStoresState(t *testing.T) {
	ws := t.TempDir()
	suitePath := filepath.Join(ws, "testsuite", "agenttests")
	if err := os.MkdirAll(suitePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(suitePath, "sample.testsuite.yaml"), []byte(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: sample
spec:
  agent_name: react
  manifest: relurpify_cfg/agents/react.yaml
  cases:
    - name: smoke
      prompt: summarize
`), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{report: &runnerpkg.SuiteReport{Cases: []runnerpkg.CaseReport{{Name: "smoke", Success: true}}}}
	agent := New(agentenv.AgentEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{},
	}, WithWorkspace(ws), WithRunner(runner))
	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		Context: map[string]any{
			"workspace":  ws,
			"suite_path": "testsuite/agenttests/sample.testsuite.yaml",
			"case_name":  "smoke",
		},
	}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success result: %+v", result)
	}
	if passed, ok := state.Get("testfu.passed"); !ok || passed != true {
		t.Fatalf("expected testfu.passed=true, got %v", passed)
	}
	if runner.lastSuite == nil || len(runner.lastSuite.Spec.Cases) != 1 {
		t.Fatalf("expected filtered suite run, got %+v", runner.lastSuite)
	}
}
