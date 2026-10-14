package js

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

// makeNodeProject writes marker files (and optional extra paths) under dir.
func makeNodeProject(t *testing.T, dir string, markers []string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	for _, marker := range markers {
		content := []byte("{}")
		if marker == "package.json" {
			content = []byte(`{"name": "probe", "scripts": {"test": "vitest run"}}`)
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, marker), content, 0o600))
	}
}

func TestNodeWorkspaceDetectTool_FindsNearestProjectRoot(t *testing.T) {
	base := t.TempDir()
	makeNodeProject(t, base, []string{"package.json", "package-lock.json"})
	nested := filepath.Join(base, "apps", "web")
	makeNodeProject(t, nested, []string{"pnpm-lock.yaml"})

	tool := &NodeWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"path": "apps/web"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, nested, res.Data["project_root"])
	assert.Equal(t, "pnpm-lock.yaml", filepath.Base(res.Data["manifest_path"].(string)))
	assert.Equal(t, "pnpm", res.Data["package_manager"])
	assert.Equal(t, []string{"pnpm-lock.yaml"}, res.Data["marker_files"])
	assert.Contains(t, res.Data["summary"], "Node project detected at")
	assert.Contains(t, res.Data["summary"], "package_manager=pnpm")
}

func TestNodeWorkspaceDetectTool_PrefersPackageJsonManifest(t *testing.T) {
	base := t.TempDir()
	makeNodeProject(t, base, []string{"package.json", "package-lock.json", "yarn.lock"})

	tool := &NodeWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "package.json", filepath.Base(res.Data["manifest_path"].(string)))
	assert.Equal(t, "yarn", res.Data["package_manager"], "yarn.lock outranks package-lock.json")
	assert.Contains(t, res.Data["summary"], "using package.json")
}

func TestNodeWorkspaceDetectTool_FilePathUsesEnclosingDirectory(t *testing.T) {
	base := t.TempDir()
	makeNodeProject(t, base, []string{"package.json"})
	require.NoError(t, os.WriteFile(filepath.Join(base, "index.js"), []byte("export {}\n"), 0o600))

	tool := &NodeWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"path": "index.js"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, base, res.Data["project_root"])
}

func TestNodeWorkspaceDetectTool_NilAndBlankPathsDefaultToBase(t *testing.T) {
	base := t.TempDir()
	makeNodeProject(t, base, []string{"package.json"})

	for name, args := range map[string]map[string]any{
		"nil value":   {"path": nil},
		"empty":       {"path": ""},
		"<nil>":       {"path": "<nil>"},
		"whitespace":  {"path": "  "},
		"missing key": {},
	} {
		t.Run(name, func(t *testing.T) {
			tool := &NodeWorkspaceDetectTool{BasePath: base}
			res, err := tool.Execute(context.Background(), args)
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, base, res.Data["project_root"])
		})
	}
}

func TestNodeWorkspaceDetectTool_ReportsMissingMarkers(t *testing.T) {
	tool := &NodeWorkspaceDetectTool{BasePath: t.TempDir()}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, "no Node project markers found", res.Error)
}

func TestNodeWorkspaceDetectTool_PropagatesStatErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	tool := &NodeWorkspaceDetectTool{BasePath: t.TempDir()}
	res, err := tool.Execute(context.Background(), map[string]any{"path": missing})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestNodeProjectMetadataTool_ParsesPackageJSON(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(base, 0o750))
	pkg := `{
		"name": "probe",
		"type": "module",
		"private": true,
		"packageManager": "pnpm@9.0.0",
		"scripts": {"test": "vitest run", "build": "tsc", "lint": "eslint ."},
		"dependencies": {"left-pad": "1.0.0", "other": "2.0.0"},
		"devDependencies": {"vitest": "3.0.0"}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(base, "package.json"), []byte(pkg), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(base, "tsmanifest.json"), []byte("{}"), 0o600))

	tool := &NodeProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "probe", res.Data["project_name"])
	assert.Equal(t, "module", res.Data["package_type"])
	assert.Equal(t, true, res.Data["private"])
	assert.Equal(t, "pnpm@9.0.0", res.Data["package_manager"], "manifest packageManager wins over lockfile inference")
	assert.Equal(t, []string{"build", "lint", "test"}, res.Data["script_names"])
	assert.Equal(t, true, res.Data["has_test_script"])
	assert.Equal(t, true, res.Data["has_build_script"])
	assert.Equal(t, true, res.Data["has_lint_script"])
	assert.Equal(t, true, res.Data["has_typecheck_script"], "tsmanifest.json marker implies typecheck")
	assert.Equal(t, "node_npm_test", res.Data["preferred_test_tool"])
	assert.Equal(t, true, res.Data["is_typescript"])
	assert.Equal(t, 3, res.Data["dependency_count"])
	assert.Contains(t, res.Data["summary"], "name=probe scripts=3")
}

func TestNodeProjectMetadataTool_WithoutTestScriptHasNoPreferredTestTool(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(base, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(base, "package.json"), []byte(`{"name": "lib"}`), 0o600))

	tool := &NodeProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "", res.Data["preferred_test_tool"])
	assert.Equal(t, false, res.Data["has_typecheck_script"])
	assert.Equal(t, false, res.Data["is_typescript"])
}

func TestNodeProjectMetadataTool_LockfileOnlyProjectSkipsManifestParse(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(base, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(base, "yarn.lock"), []byte("# yarn lockfile\n"), 0o600))

	tool := &NodeProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "yarn", res.Data["package_manager"])
	assert.NotContains(t, res.Data, "project_name", "non-package.json manifest must not be parsed as JSON")
	assert.Contains(t, res.Data["summary"], "Node project at")
}

func TestNodeProjectMetadataTool_MalformedPackageJSONFails(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(base, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(base, "package.json"), []byte("{broken"), 0o600))

	tool := &NodeProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Nil(t, res)
}

func TestNodeProjectMetadataTool_ReportsMissingMarkers(t *testing.T) {
	tool := &NodeProjectMetadataTool{BasePath: t.TempDir()}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, "no Node project markers found", res.Error)
}

func TestNodeNPMTestTool_SummarizesEachRunner(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		success     bool
		wantRunner  string
		wantPassed  int
		wantFailed  int
		wantFirst   string
		wantSummary string
	}{
		{
			name:        "vitest counts failed and passed",
			stdout:      " vitest v3.0.0\n\nFAIL  src/a.test.js > suite\nTests  2 failed | 10 passed (12)\n",
			wantRunner:  "vitest",
			wantPassed:  10,
			wantFailed:  2,
			wantFirst:   "src/a.test.js > suite",
			wantSummary: "npm test failed: src/a.test.js > suite",
		},
		{
			name:        "jest summary line",
			stdout:      "Tests: 1 failed, 4 passed\nTest Suites: 1 failed\n",
			success:     false,
			wantRunner:  "jest",
			wantPassed:  4,
			wantFailed:  1,
			wantFirst:   "Tests: 1 failed, 4 passed",
			wantSummary: "npm test failed: Tests: 1 failed, 4 passed",
		},
		{
			name:        "mocha counts passing and failing",
			stdout:      "mocha output\n\n  3 passing\n  1 failing\n",
			success:     true,
			wantRunner:  "mocha",
			wantPassed:  3,
			wantFailed:  1,
			wantSummary: "npm test passed: 3 passed, 1 failed",
		},
		{
			name:        "unknown runner falls back to mocha-style counts",
			stdout:      "  7 passing\n",
			success:     true,
			wantRunner:  "unknown",
			wantPassed:  7,
			wantSummary: "npm test passed: 7 passed, 0 failed",
		},
		{
			name:        "clean success without failure lines",
			stdout:      "all good\n",
			success:     true,
			wantRunner:  "unknown",
			wantFirst:   "all good",
			wantSummary: "npm test passed: 0 passed, 0 failed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, ExitCode: exitFor(tc.success)}}}
			tool := NewNodeNPMTestTool(t.TempDir())
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.success, res.Success)
			assert.Equal(t, tc.wantRunner, res.Data["runner"])
			assert.Equal(t, tc.wantPassed, res.Data["passed"])
			assert.Equal(t, tc.wantFailed, res.Data["failed"])
			if tc.wantFirst != "" {
				assert.Contains(t, res.Data["first_failure"], tc.wantFirst)
			}
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func exitFor(success bool) int {
	if success {
		return 0
	}
	return 1
}

func TestNodeNPMTestTool_FailureWithoutSpecFallsBackToOutput(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		wantSummary string
	}{
		{name: "stderr fallback", stderr: "npm ERR! missing script: test\n", wantSummary: "npm test failed: npm ERR! missing script: test"},
		{name: "stdout fallback", stdout: "> probe@1 test\n", wantSummary: "npm test failed: > probe@1 test"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: 1}}}
			tool := NewNodeNPMTestTool(t.TempDir())
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.False(t, res.Success)
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestNodeNPMTestTool_ForwardsExtraArgsAfterSeparator(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("ok\n")}}
	tool := NewNodeNPMTestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"working_directory": "apps/web",
		"extra_args":        []any{"--run", "src/a.test.js"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, [][]string{{"npm", "test", "--", "--run", "--", "src/a.test.js"}}, runner.argv())
	assert.Equal(t, "apps/web", runner.requests[0].Workdir)
}

func TestNodeNPMTestTool_MalformedExtraArgsAreIgnored(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewNodeNPMTestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{"extra_args": 7})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"npm", "test"}}, runner.argv())
}

func TestNodeSyntaxCheckTool_RequiresPath(t *testing.T) {
	tool := NewNodeSyntaxCheckTool(t.TempDir())
	tool.SetCommandRunner(&scriptedRunner{})

	for name, args := range map[string]map[string]any{
		"missing": {},
		"nil":     {"path": nil},
		"blank":   {"path": "   "},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := tool.Execute(context.Background(), args)
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.False(t, res.Success)
			assert.Equal(t, "path is required", res.Error)
		})
	}
}

func TestNodeSyntaxCheckTool_CountsSyntaxErrors(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{{
		Stderr:   "SyntaxError: Unexpected token '}'\n",
		ExitCode: 1,
	}}}
	tool := NewNodeSyntaxCheckTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{"path": "bad.js"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, 1, res.Data["error_count"])
	assert.Equal(t, "SyntaxError: Unexpected token '}'", res.Data["first_message"])
	assert.Equal(t, "node syntax check failed: SyntaxError: Unexpected token '}'", res.Data["summary"])
	assert.Equal(t, [][]string{{"node", "--check", "bad.js"}}, runner.argv())
}

func TestNodeSyntaxCheckTool_SuccessWithAndWithoutErrors(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		wantCount   int
		wantSummary string
	}{
		{name: "clean file", wantCount: 0, wantSummary: "node syntax check passed: 0 errors"},
		{name: "exit zero but suspicious output", stdout: "hint: missing semicolon\n", wantCount: 1, wantSummary: "node syntax check passed: 1 errors"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptedRunner{results: []*ports.CommandResult{okRun(tc.stdout)}}
			tool := NewNodeSyntaxCheckTool(t.TempDir())
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{"path": "ok.js"})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.True(t, res.Success)
			assert.Equal(t, tc.wantCount, res.Data["error_count"])
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestNodeSyntaxCheckTool_FailureWithoutPatternsFallsBack(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		wantSummary string
	}{
		{name: "stderr", stderr: "Cannot find module\n", wantSummary: "node syntax check failed: Cannot find module"},
		{name: "stdout", stdout: "first line\n", wantSummary: "node syntax check failed: first line"},
		{name: "silent", wantSummary: "node syntax check completed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: 1}}}
			tool := NewNodeSyntaxCheckTool(t.TempDir())
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{"path": "x.js"})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.False(t, res.Success)
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestNodeTools_RunnerErrorSurfaces(t *testing.T) {
	runner := &scriptedRunner{err: errors.New("sandbox denied")}
	for name, tool := range map[string]ports.Tool{
		"node_npm_test":     NewNodeNPMTestTool("."),
		"node_syntax_check": NewNodeSyntaxCheckTool("."),
	} {
		t.Run(name, func(t *testing.T) {
			tool.(interface{ SetCommandRunner(ports.CommandRunner) }).SetCommandRunner(runner)
			res, err := tool.Execute(context.Background(), map[string]any{"path": "x.js"})
			require.Error(t, err)
			assert.Nil(t, res)
		})
	}
}

func TestNodeTools_MetadataAndAvailability(t *testing.T) {
	tests := []struct {
		name      string
		tool      ports.Tool
		wantTags  []string
		wantDescr string
		wantAvail bool
	}{
		{
			name:      "node_workspace_detect",
			tool:      &NodeWorkspaceDetectTool{BasePath: "."},
			wantTags:  []string{ports.TagReadOnly, "lang:node", "workspace-detect", "recovery"},
			wantDescr: "Detects the nearest Node.js/JavaScript project root and marker files for a file or directory.",
			wantAvail: true,
		},
		{
			name:      "node_project_metadata",
			tool:      &NodeProjectMetadataTool{BasePath: "."},
			wantTags:  []string{ports.TagReadOnly, "lang:node", "metadata", "recovery"},
			wantDescr: "Reads Node project markers and returns structured package metadata.",
			wantAvail: true,
		},
		{
			name:      "node_npm_test",
			tool:      NewNodeNPMTestTool("."),
			wantTags:  []string{ports.TagExecute, "lang:node", "test", "verification", "diagnostics"},
			wantDescr: "Runs npm test and returns structured JavaScript/Node test results.",
		},
		{
			name:      "node_syntax_check",
			tool:      NewNodeSyntaxCheckTool("."),
			wantTags:  []string{ports.TagExecute, "lang:node", "syntax-check", "verification", "diagnostics"},
			wantDescr: "Runs node --check on a JavaScript file and returns structured syntax-check results.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.name, tc.tool.Name())
			assert.Equal(t, tc.wantDescr, tc.tool.Description())
			assert.Equal(t, "node", tc.tool.Category())
			assert.Equal(t, tc.wantTags, tc.tool.Tags())
			assert.NotEmpty(t, tc.tool.Parameters())
			assert.NotNil(t, tc.tool.Permissions().Permissions)
			assert.Equal(t, tc.wantAvail, tc.tool.IsAvailable(context.Background()))
		})
	}
}

func TestDetectNodeProject_StopsAtBasePath(t *testing.T) {
	base := t.TempDir()
	root, manifest, markers := detectNodeProject(base, base)
	assert.Empty(t, root)
	assert.Empty(t, manifest)
	assert.Nil(t, markers)
}

func TestInferNodePackageManager(t *testing.T) {
	tests := []struct {
		markers []string
		want    string
	}{
		{markers: []string{"pnpm-lock.yaml"}, want: "pnpm"},
		{markers: []string{"yarn.lock"}, want: "yarn"},
		{markers: []string{"package-lock.json"}, want: "npm"},
		{markers: []string{"package.json"}, want: ""},
		{markers: nil, want: ""},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, inferNodePackageManager(tc.markers), "markers=%v", tc.markers)
	}
}

func TestInferNodeTestRunner(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "RUNNING VITEST suite", want: "vitest"},
		{text: "Tests: 1 failed", want: "jest"},
		{text: "Test Suites: 2 passed", want: "jest"},
		{text: "mocha --reporter spec", want: "mocha"},
		{text: "nothing recognizable", want: "unknown"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, inferNodeTestRunner(tc.text), "text=%q", tc.text)
	}
}

func TestAtoiSafe(t *testing.T) {
	assert.Equal(t, 42, atoiSafe("42"))
	assert.Equal(t, 12, atoiSafe("12x"))
	assert.Equal(t, 0, atoiSafe("x12"))
	assert.Equal(t, 0, atoiSafe(""))
}

func TestContainsString(t *testing.T) {
	assert.True(t, containsString([]string{"a", "b"}, "b"))
	assert.False(t, containsString([]string{"a"}, "b"))
	assert.False(t, containsString(nil, "b"))
}

func TestHasScriptAndTypecheck(t *testing.T) {
	scripts := map[string]string{"test": "vitest"}
	assert.True(t, hasScript(scripts, "test"))
	assert.False(t, hasScript(scripts, "build"))
	assert.False(t, hasScript(nil, "test"))

	assert.True(t, hasTypecheck(map[string]string{"typecheck": "tsc"}, nil))
	assert.True(t, hasTypecheck(nil, []string{"tsmanifest.json"}))
	assert.False(t, hasTypecheck(nil, []string{"package.json"}))
}

func FuzzSummarizeNodeNPMTest(f *testing.F) {
	f.Add("Tests  2 failed | 10 passed (12)\nFAIL a.test.js\n", "", false)
	f.Add("Tests: 1 failed, 4 passed\n", "", false)
	f.Add("  3 passing\n  1 failing\n", "", true)
	f.Add("", "npm ERR!", true)
	f.Fuzz(func(t *testing.T, stdout, stderr string, success bool) {
		summary := summarizeNodeNPMTest(stdout, stderr, success)
		if summary.Passed < 0 || summary.Failed < 0 {
			t.Errorf("counts must never be negative: %+v", summary)
		}
		if summary.Runner == "" {
			t.Errorf("runner must never be empty")
		}
		if success && summary.Summary == "" {
			t.Errorf("successful runs must always have a summary")
		}
	})
}

func FuzzSummarizeNodeSyntaxCheck(f *testing.F) {
	f.Add("SyntaxError: Unexpected token\n", "", false)
	f.Add("", "cannot read file", true)
	f.Fuzz(func(t *testing.T, stdout, stderr string, success bool) {
		summary := summarizeNodeSyntaxCheck(stdout, stderr, success)
		if summary.ErrorCount < 0 {
			t.Errorf("error count must never be negative, got %d", summary.ErrorCount)
		}
		if summary.Summary == "" {
			t.Errorf("summary must never be empty")
		}
	})
}

func FuzzInferNodeTestRunner(f *testing.F) {
	f.Add("vitest run")
	f.Add("Tests: 1 failed")
	f.Add("mocha spec")
	f.Add("")
	f.Fuzz(func(t *testing.T, text string) {
		runner := inferNodeTestRunner(text)
		switch runner {
		case "vitest", "jest", "mocha", "unknown":
		default:
			t.Errorf("runner %q outside known set", runner)
		}
	})
}
