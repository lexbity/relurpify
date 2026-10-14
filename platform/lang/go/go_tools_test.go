package golang

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// scriptedRunner is a queued ports.CommandRunner that records every request and
// pops one canned outcome per Run call.
type scriptedRunner struct {
	err      error
	results  []*ports.CommandResult
	requests []ports.CommandRequest
}

func (r *scriptedRunner) Run(_ context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	r.requests = append(r.requests, req)
	if r.err != nil {
		return nil, r.err
	}
	if len(r.results) == 0 {
		return &ports.CommandResult{ExitCode: 0}, nil
	}
	res := r.results[0]
	if len(r.results) > 1 {
		r.results = r.results[1:]
	}
	return res, nil
}

func (r *scriptedRunner) argv() [][]string {
	out := make([][]string, 0, len(r.requests))
	for _, req := range r.requests {
		out = append(out, req.Args)
	}
	return out
}

func okRun(stdout string) *ports.CommandResult {
	return &ports.CommandResult{Stdout: stdout, ExitCode: 0}
}

// writeGoModule creates a directory containing a go.mod file.
func writeGoModule(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/probe\n\ngo 1.27\n"), 0o600))
}

func TestGoWorkspaceDetectTool_FindsNearestModule(t *testing.T) {
	base := t.TempDir()
	writeGoModule(t, filepath.Join(base, "mod"))
	inner := filepath.Join(base, "mod", "internal", "deep")
	require.NoError(t, os.MkdirAll(inner, 0o750))

	tool := &GoWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"path": "mod/internal/deep"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, filepath.Join(base, "mod"), res.Data["module_root"])
	assert.Equal(t, filepath.Join(base, "mod", "go.mod"), res.Data["module_path"])
	assert.Equal(t, "", res.Data["workspace_path"])
	assert.Contains(t, res.Data["summary"], "Go module detected at")
}

func TestGoWorkspaceDetectTool_FallsBackToWorkspaceFile(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(base, "go.work"), []byte("go 1.27\n"), 0o600))
	inner := filepath.Join(base, "svc")
	require.NoError(t, os.MkdirAll(inner, 0o750))

	tool := &GoWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, base, res.Data["module_root"], "go.work alone must anchor the module root")
	assert.Equal(t, filepath.Join(base, "go.work"), res.Data["workspace_path"])
	assert.Contains(t, res.Data["summary"], "workspace=")
}

func TestGoWorkspaceDetectTool_FilePathUsesEnclosingDirectory(t *testing.T) {
	base := t.TempDir()
	writeGoModule(t, base)
	target := filepath.Join(base, "main.go")
	require.NoError(t, os.WriteFile(target, []byte("package main\n"), 0o600))

	tool := &GoWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"path": "main.go"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, base, res.Data["module_root"])
}

func TestGoWorkspaceDetectTool_NilAndBlankPathsDefaultToBase(t *testing.T) {
	base := t.TempDir()
	writeGoModule(t, base)

	for name, args := range map[string]map[string]any{
		"explicit nil value": {"path": nil},
		"empty string":       {"path": ""},
		"sentinel <nil>":     {"path": "<nil>"},
		"whitespace":         {"path": "   "},
		"missing key":        {},
	} {
		t.Run(name, func(t *testing.T) {
			tool := &GoWorkspaceDetectTool{BasePath: base}
			res, err := tool.Execute(context.Background(), args)
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.True(t, res.Success)
			assert.Equal(t, base, res.Data["module_root"])
			assert.Equal(t, base, res.Data["path"])
		})
	}
}

func TestGoWorkspaceDetectTool_ReportsMissingModule(t *testing.T) {
	base := t.TempDir()
	tool := &GoWorkspaceDetectTool{BasePath: base}

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, "no Go module or workspace found", res.Error)
}

func TestGoWorkspaceDetectTool_PropagatesStatErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	tool := &GoWorkspaceDetectTool{BasePath: t.TempDir()}

	res, err := tool.Execute(context.Background(), map[string]any{"path": missing})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestGoWorkspaceDetectTool_Metadata(t *testing.T) {
	tool := &GoWorkspaceDetectTool{BasePath: "."}
	assert.Equal(t, "go_workspace_detect", tool.Name())
	assert.Equal(t, "go", tool.Category())
	assert.True(t, tool.IsAvailable(context.Background()))
	assert.Equal(t, []string{ports.TagReadOnly, "lang:go", "workspace-detect", "recovery"}, tool.Tags())
	assert.NotNil(t, tool.Permissions().Permissions)
	require.Len(t, tool.Parameters(), 1)
	assert.Equal(t, "path", tool.Parameters()[0].Name)
}

func TestGoModuleMetadataTool_ParsesModuleJSON(t *testing.T) {
	runner := &scriptedRunner{results: [](*ports.CommandResult){okRun(`{
		"Path": "example.com/probe",
		"Dir": "/ws/probe",
		"GoMod": "/ws/probe/go.mod",
		"GoVersion": "1.27",
		"Main": true
	}`)}}
	tool := NewGoModuleMetadataTool(t.TempDir())
	tool.SetCommandRunner(runner)
	require.True(t, tool.IsAvailable(context.Background()))

	res, err := tool.Execute(context.Background(), map[string]any{"working_directory": "sub"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "go module example.com/probe go=1.27", res.Data["summary"])
	assert.Equal(t, "example.com/probe", res.Data["module_name"])
	assert.Equal(t, "/ws/probe", res.Data["module_dir"])
	assert.Equal(t, "/ws/probe/go.mod", res.Data["go_mod"])
	assert.Equal(t, "1.27", res.Data["go_version"])
	assert.Equal(t, true, res.Data["is_main"])
	assert.Equal(t, [][]string{{"go", "list", "-m", "-json"}}, runner.argv())
	assert.Equal(t, "sub", runner.requests[0].Workdir)
}

func TestGoModuleMetadataTool_ToleratesMalformedOutput(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("not json at all\n")}}
	tool := NewGoModuleMetadataTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "go module metadata completed", res.Data["summary"])
	assert.NotContains(t, res.Data, "module_name")
}

func TestGoModuleMetadataTool_ReportsFailingGoList(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{{Stderr: "go: no module found", ExitCode: 1}}}
	tool := NewGoModuleMetadataTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, "go: no module found", res.Error)
}

func TestGoModuleMetadataTool_WithoutRunnerFails(t *testing.T) {
	tool := NewGoModuleMetadataTool(t.TempDir())
	assert.False(t, tool.IsAvailable(context.Background()))

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "command runner missing")
}

func TestGoTestTool_SuccessSummary(t *testing.T) {
	stdout := "goos: linux\nok  \texample.com/a\t0.5s\nok  \texample.com/b\t0.1s\n"
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun(stdout)}}
	tool := NewGoTestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "go test passed: 2 packages ok, 0 failed tests", res.Data["summary"])
	assert.Equal(t, 2, res.Data["passed"])
	assert.Equal(t, 0, res.Data["failed"])
	assert.Equal(t, []string(nil), res.Data["failed_tests"])
	assert.Equal(t, stdout, res.Data["stdout"])
}

func TestGoTestTool_CommandConstruction(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("ok  \texample.com/a\t0.5s\n")}}
	tool := NewGoTestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"working_directory": "pkg/auth",
		"package":           "./auth/...",
		"run":               "TestLogin",
		"extra_args":        []any{"-v", "-count=1"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, [][]string{{"go", "test", "./auth/...", "-run", "TestLogin", "-v", "-count=1"}}, runner.argv())
	assert.Equal(t, "pkg/auth", runner.requests[0].Workdir)
}

func TestGoTestTool_BlankAndInvalidOptionalArgs(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewGoTestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"package":    "   ",
		"run":        "",
		"extra_args": "not-an-array",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"go", "test", "./..."}}, runner.argv(), "blank package/run and malformed extra_args must be dropped")
}

func TestGoTestTool_FailureNamesFirstFailingTest(t *testing.T) {
	stdout := "=== RUN   TestGood\n--- PASS: TestGood (0.00s)\n--- FAIL: TestBad (0.01s)\nFAIL\nFAIL\texample.com/a\t0.2s\n"
	runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: stdout, ExitCode: 1}}}
	tool := NewGoTestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, 1, res.Data["failed"])
	assert.Equal(t, "TestBad", res.Data["first_failure"])
	assert.Equal(t, "go test failed: TestBad", res.Data["summary"])
	assert.Equal(t, []string{"TestBad"}, res.Data["failed_tests"])
}

func TestGoTestTool_FailureWithoutTestsFallsBackToOutput(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		wantSummary string
	}{
		{name: "stderr carries the reason", stdout: "", stderr: "build failed: no required module\n", wantSummary: "go test failed: build failed: no required module"},
		{name: "stdout carries the reason", stdout: "# example.com/a\n", stderr: "", wantSummary: "go test failed: # example.com/a"},
		{name: "no output at all", stdout: "", stderr: "", wantSummary: "go test completed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: 2}}}
			tool := NewGoTestTool(t.TempDir())
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.False(t, res.Success)
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestGoBuildTool_SuccessAndCommandConstruction(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewGoBuildTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"working_directory": ".",
		"package":           "./cmd/x",
		"extra_args":        []string{"-trimpath"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "go build passed: 0 errors", res.Data["summary"])
	assert.Equal(t, [][]string{{"go", "build", "./cmd/x", "-trimpath"}}, runner.argv())
}

func TestGoBuildTool_CountsCompilerErrors(t *testing.T) {
	stderr := "./a.go:10:2: undefined: missing\n./b.go:4:1: expected ';', found '}'\n"
	runner := &scriptedRunner{results: []*ports.CommandResult{{Stderr: stderr, ExitCode: 1}}}
	tool := NewGoBuildTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, 2, res.Data["error_count"])
	assert.Equal(t, "./a.go:10:2: undefined: missing", res.Data["first_message"])
	assert.Equal(t, "go build failed: ./a.go:10:2: undefined: missing", res.Data["summary"])
}

func TestGoBuildTool_FailureWithoutLocationLines(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		wantSummary string
	}{
		{name: "stderr fallback", stderr: "go: cannot find main module\n", wantSummary: "go build failed: go: cannot find main module"},
		{name: "stdout fallback", stdout: "nothing to build\n", wantSummary: "go build failed: nothing to build"},
		{name: "silent failure", wantSummary: "go build completed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: 1}}}
			tool := NewGoBuildTool(t.TempDir())
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.False(t, res.Success)
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestGoBuildTool_MalformedExtraArgsAreIgnored(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewGoBuildTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{"extra_args": 42})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"go", "build", "./..."}}, runner.argv())
}

func TestGoTools_MetadataAndAvailability(t *testing.T) {
	tests := []struct {
		name      string
		tool      ports.Tool
		wantTags  []string
		wantDescr string
		wantAvail bool
	}{
		{
			name:      "go_workspace_detect",
			tool:      &GoWorkspaceDetectTool{BasePath: "."},
			wantTags:  []string{ports.TagReadOnly, "lang:go", "workspace-detect", "recovery"},
			wantDescr: "Detects the nearest Go module or workspace for a file or directory.",
			wantAvail: true,
		},
		{
			name:      "go_test",
			tool:      NewGoTestTool("."),
			wantTags:  []string{ports.TagExecute, "lang:go", "test", "verification", "diagnostics"},
			wantDescr: "Runs go test and returns structured Go test results.",
		},
		{
			name:      "go_build",
			tool:      NewGoBuildTool("."),
			wantTags:  []string{ports.TagExecute, "lang:go", "build", "verification", "diagnostics"},
			wantDescr: "Runs go build and returns structured Go build results.",
		},
		{
			name:      "go_module_metadata",
			tool:      NewGoModuleMetadataTool("."),
			wantTags:  []string{ports.TagExecute, "lang:go", "metadata", "recovery"},
			wantDescr: "Runs go list -m -json and returns structured Go module metadata.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.name, tc.tool.Name())
			assert.Equal(t, tc.wantDescr, tc.tool.Description())
			assert.Equal(t, "go", tc.tool.Category())
			assert.Equal(t, tc.wantTags, tc.tool.Tags())
			assert.NotEmpty(t, tc.tool.Parameters())
			assert.NotNil(t, tc.tool.Permissions().Permissions)
			assert.Equal(t, tc.wantAvail, tc.tool.IsAvailable(context.Background()))
		})
	}
}

func TestGoTools_RunnerErrorSurfaces(t *testing.T) {
	runner := &scriptedRunner{err: errors.New("sandbox denied")}
	for name, tool := range map[string]ports.Tool{
		"go_test":            NewGoTestTool("."),
		"go_build":           NewGoBuildTool("."),
		"go_module_metadata": NewGoModuleMetadataTool("."),
	} {
		t.Run(name, func(t *testing.T) {
			tool.(interface{ SetCommandRunner(ports.CommandRunner) }).SetCommandRunner(runner)
			res, err := tool.Execute(context.Background(), map[string]any{})
			require.Error(t, err)
			assert.Nil(t, res)
			assert.Contains(t, err.Error(), "subprocess execution failed")
		})
	}
}

func TestDetectGoProject_StopsAtBasePath(t *testing.T) {
	base := t.TempDir()
	root, modulePath, workspace := detectGoProject(base, base)
	assert.Empty(t, root)
	assert.Empty(t, modulePath)
	assert.Empty(t, workspace)
}

func TestParseGoModuleMetadata(t *testing.T) {
	t.Run("full payload", func(t *testing.T) {
		summary, parsed := parseGoModuleMetadata(`{"Path":"x/y","Dir":"/d","GoMod":"/d/go.mod","GoVersion":"1.26","Main":false}`)
		assert.Equal(t, "go module x/y go=1.26", summary)
		assert.Equal(t, "x/y", parsed["module_name"])
		assert.Equal(t, false, parsed["is_main"])
	})
	t.Run("no go version", func(t *testing.T) {
		summary, parsed := parseGoModuleMetadata(`{"Path":"x/y"}`)
		assert.Equal(t, "go module x/y", summary)
		assert.Equal(t, "", parsed["go_version"])
	})
	t.Run("invalid json", func(t *testing.T) {
		summary, parsed := parseGoModuleMetadata("{oops")
		assert.Equal(t, "go module metadata completed", summary)
		assert.Empty(t, parsed)
	})
}

func TestSummarizeGoTest(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		success     bool
		wantSummary string
		wantPassed  int
		wantFailed  int
		wantFirst   string
	}{
		{
			name:        "success counts ok packages",
			stdout:      "ok  \ta\t0.1s\nok  \tb\t0.2s\n",
			success:     true,
			wantSummary: "go test passed: 2 packages ok, 0 failed tests",
			wantPassed:  2,
		},
		{
			name:        "failure names first failing test",
			stdout:      "--- FAIL: TestA (0.0s)\n--- FAIL: TestB (0.0s)\nFAIL\n",
			wantSummary: "go test failed: TestA",
			wantFailed:  2,
			wantFirst:   "TestA",
		},
		{
			name:        "failure with only stderr",
			stderr:      "flag provided but not defined\n",
			wantSummary: "go test failed: flag provided but not defined",
		},
		{
			name:        "silent failure keeps default summary",
			wantSummary: "go test completed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeGoTest(tc.stdout, tc.stderr, tc.success)
			assert.Equal(t, tc.wantSummary, got.Summary)
			assert.Equal(t, tc.wantPassed, got.Passed)
			assert.Equal(t, tc.wantFailed, got.Failed)
			assert.Equal(t, tc.wantFirst, got.FirstFailure)
		})
	}
}

func TestSummarizeGoBuild(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		success     bool
		wantSummary string
		wantCount   int
		wantFirst   string
	}{
		{
			name:        "clean success",
			success:     true,
			wantSummary: "go build passed: 0 errors",
		},
		{
			name:        "success still counts located errors",
			stderr:      "x.go:1:1: syntax error\n",
			success:     true,
			wantSummary: "go build passed: 1 errors",
			wantCount:   1,
			wantFirst:   "x.go:1:1: syntax error",
		},
		{
			name:        "failure reports first located error",
			stderr:      "y.go:3:5: cannot use x\n",
			wantSummary: "go build failed: y.go:3:5: cannot use x",
			wantCount:   1,
			wantFirst:   "y.go:3:5: cannot use x",
		},
		{
			name:        "failure without locations uses stderr",
			stderr:      "missing go directive\n",
			wantSummary: "go build failed: missing go directive",
			wantFirst:   "missing go directive",
		},
		{
			name:        "failure without locations uses stdout",
			stdout:      "first stdout line\n",
			wantSummary: "go build failed: first stdout line",
			wantFirst:   "first stdout line",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeGoBuild(tc.stdout, tc.stderr, tc.success)
			assert.Equal(t, tc.wantSummary, got.Summary)
			assert.Equal(t, tc.wantCount, got.ErrorCount)
			assert.Equal(t, tc.wantFirst, got.FirstMessage)
		})
	}
}

func FuzzSummarizeGoTest(f *testing.F) {
	f.Add("ok  \ta\t0.1s\n--- FAIL: TestX\n", "", false)
	f.Add("", "boom", true)
	f.Add("--- FAIL: Test A ---\nok  \tb\n", "--- FAIL: TestC\n", false)
	f.Fuzz(func(t *testing.T, stdout, stderr string, success bool) {
		summary := summarizeGoTest(stdout, stderr, success)
		if summary.Failed != len(summary.FailedTests) {
			t.Errorf("failed count %d must match FailedTests length %d", summary.Failed, len(summary.FailedTests))
		}
		if success && summary.FirstFailure == "" && summary.Passed < 0 {
			t.Errorf("passing runs must not report negative counts")
		}
		if summary.FirstFailure != "" && summary.Failed == 0 {
			t.Errorf("first failure %q recorded without failing tests", summary.FirstFailure)
		}
	})
}

func FuzzSummarizeGoBuild(f *testing.F) {
	f.Add("./a.go:1:1: boom\n", "", true)
	f.Add("", "not go output", false)
	f.Fuzz(func(t *testing.T, stdout, stderr string, success bool) {
		summary := summarizeGoBuild(stdout, stderr, success)
		if summary.ErrorCount < 0 {
			t.Errorf("error count must never be negative, got %d", summary.ErrorCount)
		}
		if summary.FirstMessage == "" && summary.ErrorCount > 0 {
			t.Errorf("%d located errors must produce a first message", summary.ErrorCount)
		}
	})
}

func FuzzParseGoModuleMetadata(f *testing.F) {
	f.Add(`{"Path":"m","GoVersion":"1.27","Main":true}`)
	f.Add(`{}`)
	f.Add(`not json`)
	f.Fuzz(func(t *testing.T, payload string) {
		summary, parsed := parseGoModuleMetadata(payload)
		if summary == "" {
			t.Errorf("summary must never be empty")
		}
		if _, ok := parsed["module_name"]; ok {
			if _, ok := parsed["go_mod"]; !ok {
				t.Errorf("parsed module metadata must be all-or-nothing")
			}
		}
	})
}
