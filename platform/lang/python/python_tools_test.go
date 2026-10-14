package python

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

func exitFor(success bool) int {
	if success {
		return 0
	}
	return 1
}

func writeMarker(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

func TestPythonWorkspaceDetectTool_FindsNearestProjectRoot(t *testing.T) {
	base := t.TempDir()
	writeMarker(t, filepath.Join(base, "outer"), "requirements.txt", "flask\n")
	nested := filepath.Join(base, "outer", "pkg")
	writeMarker(t, nested, "pyproject.toml", "[project]\nname = \"pkg\"\n")

	tool := &PythonWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"path": "outer/pkg"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, nested, res.Data["project_root"])
	assert.Equal(t, "pyproject.toml", filepath.Base(res.Data["manifest_path"].(string)))
	assert.Equal(t, []string{"pyproject.toml"}, res.Data["marker_files"])
	assert.Contains(t, res.Data["summary"], "Python project detected at")
	assert.Contains(t, res.Data["summary"], "using pyproject.toml")
}

func TestPythonWorkspaceDetectTool_FilePathUsesEnclosingDirectory(t *testing.T) {
	base := t.TempDir()
	writeMarker(t, base, "pyproject.toml", "[project]\nname = \"probe\"\n")
	require.NoError(t, os.WriteFile(filepath.Join(base, "mod.py"), []byte("x = 1\n"), 0o600))

	tool := &PythonWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"path": "mod.py"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, base, res.Data["project_root"])
}

func TestPythonWorkspaceDetectTool_NilAndBlankPathsDefaultToBase(t *testing.T) {
	base := t.TempDir()
	writeMarker(t, base, "setup.py", "from setuptools import setup\n")

	for name, args := range map[string]map[string]any{
		"nil value":   {"path": nil},
		"empty":       {"path": ""},
		"<nil>":       {"path": "<nil>"},
		"whitespace":  {"path": " \t"},
		"missing key": {},
	} {
		t.Run(name, func(t *testing.T) {
			tool := &PythonWorkspaceDetectTool{BasePath: base}
			res, err := tool.Execute(context.Background(), args)
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, base, res.Data["project_root"])
		})
	}
}

func TestPythonWorkspaceDetectTool_ReportsMissingMarkers(t *testing.T) {
	tool := &PythonWorkspaceDetectTool{BasePath: t.TempDir()}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, "no Python project markers found", res.Error)
}

func TestPythonWorkspaceDetectTool_PropagatesStatErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	tool := &PythonWorkspaceDetectTool{BasePath: t.TempDir()}
	res, err := tool.Execute(context.Background(), map[string]any{"path": missing})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestPythonProjectMetadataTool_ParsesPyprojectIdentity(t *testing.T) {
	base := t.TempDir()
	pyproject := "[project]\nname = \"probe\"\nrequires-python = \">=3.12\"\n"
	writeMarker(t, base, "pyproject.toml", pyproject)
	writeMarker(t, base, "requirements.txt", "pytest>=8\n")
	writeMarker(t, base, "tox.ini", "")

	tool := &PythonProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "probe", res.Data["project_name"])
	assert.Equal(t, ">=3.12", res.Data["requires_python"])
	assert.Equal(t, "python_pytest", res.Data["preferred_test_tool"], "pytest in requirements.txt implies pytest")
	assert.Equal(t, true, res.Data["has_pytest_config"])
	assert.Equal(t, true, res.Data["has_requirements_txt"])
	assert.Equal(t, []string{"pyproject.toml", "requirements.txt"}, res.Data["dependency_files"])
	assert.Contains(t, res.Data["summary"], "name=probe")
	assert.Contains(t, res.Data["summary"], "test_tool=python_pytest")
}

func TestPythonProjectMetadataTool_SetupCfgFallbackName(t *testing.T) {
	base := t.TempDir()
	// The identity pattern only accepts quoted values; an unquoted setup.cfg
	// name (common in configparser-style metadata) is not recognized. This is
	// documented in the direct parse test below.
	writeMarker(t, base, "setup.cfg", "[metadata]\nname = \"cfgprobe\"\n")

	tool := &PythonProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "cfgprobe", res.Data["project_name"])
	assert.Equal(t, "", res.Data["requires_python"])
	assert.Equal(t, "python_unittest", res.Data["preferred_test_tool"])
	assert.Equal(t, false, res.Data["has_pytest_config"])
	assert.Equal(t, []string{"setup.cfg"}, res.Data["dependency_files"])
}

func TestPythonProjectMetadataTool_PyprojectSectionTriggersPytest(t *testing.T) {
	base := t.TempDir()
	writeMarker(t, base, "pyproject.toml", "[tool.pytest.ini_options]\naddopts = \"-q\"\n")

	tool := &PythonProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "python_pytest", res.Data["preferred_test_tool"])
	assert.Equal(t, true, res.Data["has_pytest_config"])
}

func TestPythonProjectMetadataTool_PytestIniMarkerWins(t *testing.T) {
	base := t.TempDir()
	writeMarker(t, base, "pytest.ini", "[pytest]\n")
	// tox.ini is also a marker; declaration order makes pytest.ini second, but
	// both are read.
	writeMarker(t, base, "tox.ini", "[tox]\n")

	tool := &PythonProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "python_pytest", res.Data["preferred_test_tool"])
	assert.Equal(t, true, res.Data["has_pytest_config"])
}

func TestPythonProjectMetadataTool_UnreadableMarkerIsSkipped(t *testing.T) {
	base := t.TempDir()
	writeMarker(t, base, "pyproject.toml", "[project]\nname = \"probe\"\n")
	// A marker that exists but cannot be read as a file (a directory) must not
	// abort metadata collection.
	require.NoError(t, os.MkdirAll(filepath.Join(base, "setup.py"), 0o750))

	tool := &PythonProjectMetadataTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "probe", res.Data["project_name"])
}

func TestPythonProjectMetadataTool_ReportsMissingMarkers(t *testing.T) {
	tool := &PythonProjectMetadataTool{BasePath: t.TempDir()}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, "no Python project markers found", res.Error)
}

func TestPythonPytestTool_CommandConstruction(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewPythonPytestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"working_directory": "pkg/api",
		"test_path":         "tests/test_api.py",
		"extra_args":        []any{"-x", "--maxfail=1"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"python3", "-m", "pytest", "-q", "tests/test_api.py", "-x", "--maxfail=1"}}, runner.argv())
	assert.Equal(t, "pkg/api", runner.requests[0].Workdir)
}

func TestPythonPytestTool_SummarizesOutput(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		success     bool
		wantPassed  int
		wantFailed  int
		wantErrors  int
		wantFirst   string
		wantSummary string
	}{
		{
			name:        "success counts",
			stdout:      "5 passed, 1 skipped in 0.2s\n",
			success:     true,
			wantPassed:  5,
			wantSummary: "pytest passed: 5 passed, 0 failed, 0 errors",
		},
		{
			name:        "failure names first FAILED entry",
			stdout:      "2 failed, 3 passed in 0.4s\nFAILED tests/test_a.py::test_x - assert 1 == 2\nFAILED tests/test_b.py::test_y\n",
			wantPassed:  3,
			wantFailed:  2,
			wantFirst:   "tests/test_a.py::test_x",
			wantSummary: "pytest failed: tests/test_a.py::test_x",
		},
		{
			name:        "failure names first ERROR entry without an errors count",
			stderr:      "ERROR tests/test_broken.py - ImportError\n",
			wantErrors:  0,
			wantFirst:   "tests/test_broken.py",
			wantSummary: "pytest failed: tests/test_broken.py",
		},
		{
			name:        "failure without markers falls back to stderr",
			stderr:      "pytest: unrecognized arguments\n",
			wantSummary: "pytest failed: pytest: unrecognized arguments",
		},
		{
			name:        "failure without markers falls back to stdout",
			stdout:      "no tests ran\n",
			wantSummary: "pytest failed: no tests ran",
		},
		{
			name:        "silent failure keeps default summary",
			wantSummary: "pytest completed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: exitFor(tc.success)}}}
			tool := NewPythonPytestTool(t.TempDir())
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.success, res.Success)
			assert.Equal(t, tc.wantPassed, res.Data["passed"])
			assert.Equal(t, tc.wantFailed, res.Data["failed"])
			assert.Equal(t, tc.wantErrors, res.Data["errors"])
			assert.Equal(t, tc.wantFirst, res.Data["first_failure"])
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestPythonPytestTool_MalformedExtraArgsAreIgnored(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewPythonPytestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{"extra_args": "nope"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"python3", "-m", "pytest", "-q"}}, runner.argv())
}

func TestPythonUnittestTool_CommandConstruction(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewPythonUnittestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"working_directory": "pkg",
		"start_directory":   "tests",
		"pattern":           "test_*.py",
		"extra_args":        []string{"-v"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"python3", "-m", "unittest", "discover", "-s", "tests", "-p", "test_*.py", "-v"}}, runner.argv())
}

func TestPythonUnittestTool_BlankOptionalArgsAreDropped(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewPythonUnittestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"start_directory": "  ",
		"pattern":         "",
		"extra_args":      3,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"python3", "-m", "unittest", "discover"}}, runner.argv())
}

func TestPythonUnittestTool_SummarizesOutput(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		success     bool
		wantPassed  int
		wantFailed  int
		wantErrors  int
		wantFirst   string
		wantSummary string
	}{
		{
			name:        "all passing",
			stderr:      "Ran 4 tests in 0.01s\n\nOK\n",
			success:     true,
			wantPassed:  4,
			wantSummary: "unittest passed: 4 passed, 0 failed, 0 errors",
		},
		{
			name:        "failures reduce passed count",
			stderr:      "Ran 3 tests in 0.01s\n\nFAIL: test_broken (probe.ProbeTest)\nFAIL: test_other (probe.ProbeTest)\nFAILED (failures=2)\n",
			wantPassed:  1,
			wantFailed:  2,
			wantFirst:   "test_broken (probe.ProbeTest)",
			wantSummary: "unittest failed: test_broken (probe.ProbeTest)",
		},
		{
			name:        "errors are counted separately",
			stderr:      "Ran 2 tests in 0.01s\n\nERROR: test_boom (probe.ProbeTest)\nFAILED (errors=1)\n",
			wantPassed:  1,
			wantErrors:  1,
			wantFirst:   "test_boom (probe.ProbeTest)",
			wantSummary: "unittest failed: test_boom (probe.ProbeTest)",
		},
		{
			name:        "failure without structured output falls back",
			stderr:      "No module named probe\n",
			wantSummary: "unittest failed: No module named probe",
		},
		{
			name:        "silent failure keeps default summary",
			wantSummary: "unittest completed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: exitFor(tc.success)}}}
			tool := NewPythonUnittestTool(t.TempDir())
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.success, res.Success)
			assert.Equal(t, tc.wantPassed, res.Data["passed"])
			assert.Equal(t, tc.wantFailed, res.Data["failed"])
			assert.Equal(t, tc.wantErrors, res.Data["errors"])
			assert.Equal(t, tc.wantFirst, res.Data["first_failure"])
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestPythonUnittestTool_PassedNeverGoesNegative(t *testing.T) {
	// More FAIL lines than the Ran count must clamp passed to zero.
	stderr := "Ran 1 test in 0.0s\n\nFAIL: test_a\nFAIL: test_b\nFAIL: test_c\n"
	runner := &scriptedRunner{results: []*ports.CommandResult{{Stderr: stderr, ExitCode: 1}}}
	tool := NewPythonUnittestTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 0, res.Data["passed"])
	assert.Equal(t, 3, res.Data["failed"])
}

func TestPythonCompileCheckTool_CommandConstruction(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewPythonCompileCheckTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"working_directory": "src",
		"target":            "pkg",
		"extra_args":        []string{"-j", "2"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"python3", "-m", "compileall", "-q", "pkg", "-j", "2"}}, runner.argv())
}

func TestPythonCompileCheckTool_DefaultTarget(t *testing.T) {
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewPythonCompileCheckTool(t.TempDir())
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{"target": "  "})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"python3", "-m", "compileall", "-q", "."}}, runner.argv())
}

func TestPythonCompileCheckTool_SummarizesOutput(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		success     bool
		wantCount   int
		wantFirst   string
		wantSummary string
	}{
		{
			name:        "clean success",
			success:     true,
			wantSummary: "compile check passed: 0 errors",
		},
		{
			name:        "counts syntax errors case-insensitively",
			stderr:      "  File \"m.py\", line 3\nIndentationError: unexpected indent\nsyntaxerror: invalid syntax\n",
			wantCount:   2,
			wantFirst:   "IndentationError: unexpected indent",
			wantSummary: "compile check failed: IndentationError: unexpected indent",
		},
		{
			name:        "name errors count too",
			stdout:      "NameError: name 'x' is not defined\n",
			success:     true,
			wantCount:   1,
			wantFirst:   "NameError: name 'x' is not defined",
			wantSummary: "compile check passed: 1 errors",
		},
		{
			name:        "failure without patterns falls back to stderr",
			stderr:      "compileall: cannot list directory\n",
			wantFirst:   "compileall: cannot list directory",
			wantSummary: "compile check failed: compileall: cannot list directory",
		},
		{
			name:        "failure without patterns falls back to stdout",
			stdout:      "first line\n",
			wantFirst:   "first line",
			wantSummary: "compile check failed: first line",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: exitFor(tc.success)}}}
			tool := NewPythonCompileCheckTool(t.TempDir())
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.success, res.Success)
			assert.Equal(t, tc.wantCount, res.Data["error_count"])
			assert.Equal(t, tc.wantFirst, res.Data["first_message"])
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestPythonTools_RunnerErrorSurfaces(t *testing.T) {
	runner := &scriptedRunner{err: errors.New("sandbox denied")}
	for name, tool := range map[string]ports.Tool{
		"python_pytest":        NewPythonPytestTool("."),
		"python_unittest":      NewPythonUnittestTool("."),
		"python_compile_check": NewPythonCompileCheckTool("."),
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

func TestPythonTools_MetadataAndAvailability(t *testing.T) {
	tests := []struct {
		name      string
		tool      ports.Tool
		wantTags  []string
		wantDescr string
		wantAvail bool
	}{
		{
			name:      "python_workspace_detect",
			tool:      &PythonWorkspaceDetectTool{BasePath: "."},
			wantTags:  []string{ports.TagReadOnly, "lang:python", "workspace-detect", "recovery"},
			wantDescr: "Detects the nearest Python project root and marker files for a file or directory.",
			wantAvail: true,
		},
		{
			name:      "python_project_metadata",
			tool:      &PythonProjectMetadataTool{BasePath: "."},
			wantTags:  []string{ports.TagReadOnly, "lang:python", "metadata", "recovery"},
			wantDescr: "Reads Python project markers and returns structured project metadata.",
			wantAvail: true,
		},
		{
			name:      "python_pytest",
			tool:      NewPythonPytestTool("."),
			wantTags:  []string{ports.TagExecute, "lang:python", "test", "verification", "diagnostics"},
			wantDescr: "Runs python -m pytest and returns structured Python test results.",
		},
		{
			name:      "python_unittest",
			tool:      NewPythonUnittestTool("."),
			wantTags:  []string{ports.TagExecute, "lang:python", "test", "verification", "diagnostics"},
			wantDescr: "Runs python -m unittest discover and returns structured Python test results.",
		},
		{
			name:      "python_compile_check",
			tool:      NewPythonCompileCheckTool("."),
			wantTags:  []string{ports.TagExecute, "lang:python", "syntax-check", "verification", "diagnostics"},
			wantDescr: "Runs python -m compileall and returns structured Python syntax-check results.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.name, tc.tool.Name())
			assert.Equal(t, tc.wantDescr, tc.tool.Description())
			assert.Equal(t, "python", tc.tool.Category())
			assert.Equal(t, tc.wantTags, tc.tool.Tags())
			assert.NotEmpty(t, tc.tool.Parameters())
			assert.NotNil(t, tc.tool.Permissions().Permissions)
			assert.Equal(t, tc.wantAvail, tc.tool.IsAvailable(context.Background()))
		})
	}
}

func TestDetectPythonProject_StopsAtBasePath(t *testing.T) {
	base := t.TempDir()
	root, manifest, markers := detectPythonProject(base, base)
	assert.Empty(t, root)
	assert.Empty(t, manifest)
	assert.Nil(t, markers)
}

func TestDetectPythonProject_StopsAtFilesystemRoot(t *testing.T) {
	// A base path above the filesystem root lets the walk escape the workspace
	// boundary; it must still terminate at "/" without finding anything.
	empty := t.TempDir()
	root, manifest, markers := detectPythonProject(empty, string(filepath.Separator))
	assert.Empty(t, root)
	assert.Empty(t, manifest)
	assert.Nil(t, markers)
}

func TestFindPythonMarkers_PreservesDeclarationOrder(t *testing.T) {
	dir := t.TempDir()
	for _, marker := range []string{"Pipfile", "requirements.txt", "pyproject.toml"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, marker), []byte("x"), 0o600))
	}
	assert.Equal(t, []string{"pyproject.toml", "requirements.txt", "Pipfile"}, findPythonMarkers(dir))
}

func TestParsePythonProjectIdentity(t *testing.T) {
	tests := []struct {
		name         string
		files        map[string]string
		wantName     string
		wantRequires string
	}{
		{
			name:         "pyproject name and requires",
			files:        map[string]string{"pyproject.toml": "name = 'a'\nrequires-python = \">=3.11\"\n"},
			wantName:     "a",
			wantRequires: ">=3.11",
		},
		{
			name:     "pyproject without identity falls through to setup.cfg",
			files:    map[string]string{"pyproject.toml": "[tool.black]\n", "setup.cfg": "name = \"b\"\n"},
			wantName: "b",
		},
		{
			// The pattern requires quotes; unquoted setup.cfg names are missed.
			name:     "unquoted setup.cfg name is not recognized",
			files:    map[string]string{"setup.cfg": "name = b\n"},
			wantName: "",
		},
		{
			name:     "setup.cfg only",
			files:    map[string]string{"setup.cfg": "  name = \"spaced\"\n"},
			wantName: "spaced",
		},
		{name: "nothing", files: map[string]string{}, wantName: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, requires := parsePythonProjectIdentity(tc.files)
			assert.Equal(t, tc.wantName, name)
			assert.Equal(t, tc.wantRequires, requires)
		})
	}
}

func TestPythonDependencyFiles(t *testing.T) {
	assert.Equal(t,
		[]string{"pyproject.toml", "requirements.txt"},
		pythonDependencyFiles([]string{"pyproject.toml", "pytest.ini", "requirements.txt", "tox.ini"}))
	assert.Nil(t, pythonDependencyFiles([]string{"pytest.ini"}))
}

func TestFirstCapture(t *testing.T) {
	assert.Equal(t, "abc", firstCapture(pythonProjectNamePattern, "name = \"abc\"\n"))
	assert.Equal(t, "", firstCapture(pythonProjectNamePattern, "nothing here"))
}

func TestAtoiSafe(t *testing.T) {
	assert.Equal(t, 42, atoiSafe("42"))
	assert.Equal(t, 3, atoiSafe("3x"))
	assert.Equal(t, 0, atoiSafe("q"))
}

func FuzzSummarizePythonPytest(f *testing.F) {
	f.Add("5 passed, 1 skipped\nFAILED t::test_x\n", "", true)
	f.Add("", "ERROR t.py - boom\n", false)
	f.Add("2 failed 1 error", "", false)
	f.Fuzz(func(t *testing.T, stdout, stderr string, success bool) {
		summary := summarizePythonPytest(stdout, stderr, success)
		if summary.Passed < 0 || summary.Failed < 0 || summary.Errors < 0 {
			t.Errorf("counts must never be negative: %+v", summary)
		}
		if success && summary.Summary == "" {
			t.Errorf("successful runs must always have a summary")
		}
	})
}

func FuzzSummarizePythonUnittest(f *testing.F) {
	f.Add("", "Ran 3 tests in 0.01s\nFAIL: test_a\n", false)
	f.Add("ok\n", "Ran 2 tests in 0.0s\nOK\n", true)
	f.Fuzz(func(t *testing.T, stdout, stderr string, success bool) {
		summary := summarizePythonUnittest(stdout, stderr, success)
		if summary.Passed < 0 || summary.Failed < 0 || summary.Errors < 0 {
			t.Errorf("counts must never be negative: %+v", summary)
		}
	})
}

func FuzzSummarizePythonCompileCheck(f *testing.F) {
	f.Add("IndentationError: bad\n", "", false)
	f.Add("clean", "", true)
	f.Fuzz(func(t *testing.T, stdout, stderr string, success bool) {
		summary := summarizePythonCompileCheck(stdout, stderr, success)
		if summary.ErrorCount < 0 {
			t.Errorf("error count must never be negative, got %d", summary.ErrorCount)
		}
		if summary.Summary == "" {
			t.Errorf("summary must never be empty")
		}
	})
}

func FuzzParsePythonProjectIdentity(f *testing.F) {
	f.Add("name = \"a\"\nrequires-python = \">=3.11\"")
	f.Add("[tool.pytest]\nname=x")
	f.Add("")
	f.Fuzz(func(t *testing.T, pyproject string) {
		name, requires := parsePythonProjectIdentity(map[string]string{"pyproject.toml": pyproject})
		if name == "" && requires != "" {
			t.Errorf("requires %q without a project name is inconsistent", requires)
		}
	})
}
