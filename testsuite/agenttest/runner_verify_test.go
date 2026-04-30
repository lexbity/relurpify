package agenttest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuildVerifyToolIndex_ContainsGoPythonRust(t *testing.T) {
	workspace := t.TempDir()
	index := buildVerifyToolIndex(workspace, sandbox.NewLocalCommandRunner(workspace, nil))
	for _, name := range []string{"go_test", "python_pytest", "rust_cargo_test"} {
		if _, ok := index[name]; !ok {
			t.Fatalf("expected %s in verify tool index, got %v", name, keysOfToolIndex(index))
		}
	}
}

func TestRunVerificationSteps_AllPass(t *testing.T) {
	workspace := writeVerifyGoWorkspace(t)
	runner := sandbox.NewLocalCommandRunner(workspace, nil)
	spec := VerifySpec{
		Steps: []VerifyStepSpec{
			{Tool: "go_build", Args: map[string]any{"working_directory": ".", "package": "./good"}},
			{Tool: "go_test", Args: map[string]any{"working_directory": ".", "package": "./good"}},
		},
	}

	results := runVerificationSteps(context.Background(), spec, workspace, runner)
	require.Len(t, results, 2)
	require.True(t, results[0].Passed)
	require.True(t, results[1].Passed)
	require.Equal(t, "outcome", results[0].Tier)
	require.Equal(t, "outcome", results[1].Tier)
}

func TestRunVerificationSteps_StopsOnFirstFailure(t *testing.T) {
	workspace := writeVerifyGoWorkspace(t)
	runner := sandbox.NewLocalCommandRunner(workspace, nil)
	spec := VerifySpec{
		Steps: []VerifyStepSpec{
			{Tool: "go_test", Args: map[string]any{"working_directory": ".", "package": "./bad"}},
			{Tool: "go_test", Args: map[string]any{"working_directory": ".", "package": "./good"}},
		},
	}

	results := runVerificationSteps(context.Background(), spec, workspace, runner)
	require.Len(t, results, 1)
	require.False(t, results[0].Passed)
	require.NotEmpty(t, results[0].Message)
}

func TestRunVerificationSteps_ContinueOnFailure(t *testing.T) {
	workspace := writeVerifyGoWorkspace(t)
	runner := sandbox.NewLocalCommandRunner(workspace, nil)
	spec := VerifySpec{
		Steps: []VerifyStepSpec{
			{Tool: "go_test", Args: map[string]any{"working_directory": ".", "package": "./bad"}, ContinueOnFailure: true},
			{Tool: "go_test", Args: map[string]any{"working_directory": ".", "package": "./good"}},
		},
	}

	results := runVerificationSteps(context.Background(), spec, workspace, runner)
	require.Len(t, results, 2)
	require.False(t, results[0].Passed)
	require.True(t, results[1].Passed)
}

func TestRunVerificationSteps_PythonAndRust(t *testing.T) {
	workspace := t.TempDir()
	runner := &verifySpyRunner{}
	spec := VerifySpec{
		Steps: []VerifyStepSpec{
			{
				Tool: "python_pytest",
				Args: map[string]any{
					"working_directory": filepath.Join(workspace, "pysuite"),
					"test_path":         "test_calc.py",
				},
			},
			{
				Tool: "rust_cargo_test",
				Args: map[string]any{
					"working_directory": filepath.Join(workspace, "rustsuite"),
				},
			},
		},
	}

	results := runVerificationSteps(context.Background(), spec, workspace, runner)
	require.Len(t, results, 2)
	require.True(t, results[0].Passed)
	require.True(t, results[1].Passed)
	require.Len(t, runner.requests, 2)
	require.Equal(t, "python3", runner.requests[0].Args[0])
	require.Equal(t, "cargo", runner.requests[1].Args[0])
}

func TestRunVerificationSteps_UnknownTool(t *testing.T) {
	workspace := t.TempDir()
	runner := sandbox.NewLocalCommandRunner(workspace, nil)
	spec := VerifySpec{
		Steps: []VerifyStepSpec{{Tool: "missing_tool"}},
	}

	results := runVerificationSteps(context.Background(), spec, workspace, runner)
	require.Len(t, results, 1)
	require.False(t, results[0].Passed)
	require.Contains(t, results[0].Message, "missing_tool")
}

func TestRunVerifyScript_Pass(t *testing.T) {
	workspace := t.TempDir()
	script := filepath.Join(workspace, "verify.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/usr/bin/env bash\nset -eu\nexit 0\n"), 0o644))

	result := runVerifyScript(context.Background(), "verify.sh", workspace, sandbox.NewLocalCommandRunner(workspace, nil))
	require.True(t, result.Passed)
	require.NotContains(t, result.Message, "exit status")
}

func TestRunVerifyScript_Fail(t *testing.T) {
	workspace := t.TempDir()
	script := filepath.Join(workspace, "verify.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/usr/bin/env bash\nset -eu\nprintf 'boom\\n' >&2\nexit 1\n"), 0o644))

	result := runVerifyScript(context.Background(), "verify.sh", workspace, sandbox.NewLocalCommandRunner(workspace, nil))
	require.False(t, result.Passed)
	require.Contains(t, result.Message, "boom")
}

func TestVerifySpecYAMLRoundTrip(t *testing.T) {
	original := VerifySpec{
		Steps: []VerifyStepSpec{
			{
				Tool: "go_test",
				Args: map[string]any{
					"working_directory": ".",
					"package":           "./good",
				},
				ContinueOnFailure: true,
			},
		},
		Script: "testsuite/agenttest_fixtures/gosuite/verify.sh",
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var roundtripped VerifySpec
	require.NoError(t, yaml.Unmarshal(data, &roundtripped))
	require.Len(t, roundtripped.Steps, 1)
	require.Equal(t, original.Steps[0].Tool, roundtripped.Steps[0].Tool)
	require.Equal(t, original.Steps[0].ContinueOnFailure, roundtripped.Steps[0].ContinueOnFailure)
	require.Equal(t, original.Script, roundtripped.Script)
}

func TestOutcomeSpecWithVerifyLoadsCorrectly(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte("kind: test\n"), 0o644))

	suitePath := filepath.Join(workspace, "suite.yaml")
	suiteYAML := strings.TrimSpace(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: verify-suite
spec:
  agent_name: euclo
  manifest: manifest.yaml
  workspace:
    strategy: derived
  cases:
    - name: verify_case
      prompt: verify this
      expect:
        outcome:
          verify:
            steps:
              - tool: go_test
                args:
                  working_directory: .
                  package: ./good
            script: testsuite/agenttest_fixtures/gosuite/verify.sh
`)
	require.NoError(t, os.WriteFile(suitePath, []byte(suiteYAML+"\n"), 0o644))

	suite, err := LoadSuite(suitePath)
	require.NoError(t, err)
	require.Len(t, suite.Spec.Cases, 1)
	require.NotNil(t, suite.Spec.Cases[0].Expect.Outcome)
	require.NotNil(t, suite.Spec.Cases[0].Expect.Outcome.Verify)
	require.Len(t, suite.Spec.Cases[0].Expect.Outcome.Verify.Steps, 1)
	require.Equal(t, "go_test", suite.Spec.Cases[0].Expect.Outcome.Verify.Steps[0].Tool)
	require.Equal(t, "testsuite/agenttest_fixtures/gosuite/verify.sh", suite.Spec.Cases[0].Expect.Outcome.Verify.Script)
}

func writeVerifyGoWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/verify\n\ngo 1.22\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "good"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "bad"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(workspace, "good", "good.go"), []byte("package good\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "good", "good_test.go"), []byte("package good\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif got := Add(2, 3); got != 5 {\n\t\tt.Fatalf(\"Add(2,3) = %d, want 5\", got)\n\t}\n}\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(workspace, "bad", "bad.go"), []byte("package bad\n\nfunc Add(a, b int) int { return a - b }\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "bad", "bad_test.go"), []byte("package bad\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif got := Add(2, 3); got != 5 {\n\t\tt.Fatalf(\"Add(2,3) = %d, want 5\", got)\n\t}\n}\n"), 0o644))

	return workspace
}

func keysOfToolIndex(index map[string]core.Tool) []string {
	keys := make([]string, 0, len(index))
	for key := range index {
		keys = append(keys, key)
	}
	return keys
}

type verifySpyRunner struct {
	requests []sandbox.CommandRequest
}

func (r *verifySpyRunner) Run(_ context.Context, req sandbox.CommandRequest) (string, string, error) {
	r.requests = append(r.requests, req)
	if len(req.Args) == 0 {
		return "", "", nil
	}
	switch req.Args[0] {
	case "python3":
		return "1 passed in 0.01s\n", "", nil
	case "cargo":
		return "test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out\n", "", nil
	default:
		return "", "", nil
	}
}
