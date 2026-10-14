package rust

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func writeCrate(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"probe\"\nversion = \"0.1.0\"\n"), 0o600))
}

func TestRustWorkspaceDetectTool_FindsNearestAndWorkspaceManifests(t *testing.T) {
	base := t.TempDir()
	writeCrate(t, base)
	member := filepath.Join(base, "crates", "member")
	writeCrate(t, member)

	tool := &RustWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"path": "crates/member"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, filepath.Join(member, "Cargo.toml"), res.Data["manifest_path"])
	assert.Equal(t, member, res.Data["crate_root"])
	assert.Equal(t, filepath.Join(base, "Cargo.toml"), res.Data["workspace_manifest"])
}

func TestRustWorkspaceDetectTool_SingleCrateHasNoWorkspaceManifest(t *testing.T) {
	base := t.TempDir()
	writeCrate(t, base)

	tool := &RustWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, filepath.Join(base, "Cargo.toml"), res.Data["manifest_path"])
	// A lone crate reports itself as its own workspace manifest: the walk marks
	// every directory containing a Cargo.toml as the outermost manifest.
	assert.Equal(t, filepath.Join(base, "Cargo.toml"), res.Data["workspace_manifest"])
}

func TestRustWorkspaceDetectTool_FilePathUsesEnclosingDirectory(t *testing.T) {
	base := t.TempDir()
	writeCrate(t, base)
	require.NoError(t, os.WriteFile(filepath.Join(base, "lib.rs"), []byte("pub fn f() {}\n"), 0o600))

	tool := &RustWorkspaceDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"path": "lib.rs"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, filepath.Join(base, "Cargo.toml"), res.Data["manifest_path"])
}

func TestRustWorkspaceDetectTool_NilAndBlankPathsDefaultToBase(t *testing.T) {
	base := t.TempDir()
	writeCrate(t, base)

	for name, args := range map[string]map[string]any{
		"nil value":   {"path": nil},
		"empty":       {"path": ""},
		"<nil>":       {"path": "<nil>"},
		"whitespace":  {"path": " "},
		"missing key": {},
	} {
		t.Run(name, func(t *testing.T) {
			tool := &RustWorkspaceDetectTool{BasePath: base}
			res, err := tool.Execute(context.Background(), args)
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, filepath.Join(base, "Cargo.toml"), res.Data["manifest_path"])
		})
	}
}

func TestRustWorkspaceDetectTool_ReportsMissingManifest(t *testing.T) {
	tool := &RustWorkspaceDetectTool{BasePath: t.TempDir()}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, "no Cargo.toml found", res.Error)
}

func TestRustWorkspaceDetectTool_PropagatesStatErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	tool := &RustWorkspaceDetectTool{BasePath: t.TempDir()}
	res, err := tool.Execute(context.Background(), map[string]any{"path": missing})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRustCargoTestTool_CommandConstruction(t *testing.T) {
	base := t.TempDir()
	writeCrate(t, base)
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("test result: ok. 2 passed; 0 failed;\n")}}
	tool := NewRustCargoTestTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"working_directory": base,
		"test_name":         "probe::tests::works",
		"extra_args":        []any{"--nocapture", "--test-threads=1"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, [][]string{{"cargo", "test", "probe::tests::works", "--nocapture", "--test-threads=1"}}, runner.argv())
	assert.Equal(t, base, runner.requests[0].Workdir, "top-level crate must run without cargo isolation")
}

func TestRustCargoTestTool_BlankNameAndMalformedExtraArgsAreDropped(t *testing.T) {
	base := t.TempDir()
	writeCrate(t, base)
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("")}}
	tool := NewRustCargoTestTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"test_name":  "  ",
		"extra_args": 9,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"cargo", "test"}}, runner.argv())
}

func TestRustCargoTestTool_IsolatesNestedWorkspaceMember(t *testing.T) {
	base := t.TempDir()
	writeCrate(t, base)
	member := filepath.Join(base, "crates", "member")
	writeCrate(t, member)

	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("test result: ok. 1 passed; 0 failed;\n")}}
	tool := NewRustCargoTestTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{"working_directory": member})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)

	require.Len(t, runner.requests, 1)
	req := runner.requests[0]
	assert.Equal(t, base, req.Workdir, "isolated run must execute from the workspace root")
	got := req.Args
	require.GreaterOrEqual(t, len(got), 4)
	require.Equal(t, "cargo", got[0])
	require.Equal(t, "test", got[1])
	require.Equal(t, "--manifest-path", got[2], "manifest path must be injected after the subcommand")
	assert.Contains(t, got[3], "relurpify-cargo", "isolated manifest must live in a relurpify-cargo temp dir")
	assert.True(t, strings.HasSuffix(got[3], filepath.Join("member", "Cargo.toml")))
}

func TestRustCargoTestTool_SummarizesOutput(t *testing.T) {
	tests := []struct {
		name           string
		stdout         string
		stderr         string
		success        bool
		wantPassed     int
		wantFailed     int
		wantFirst      string
		wantFailedList []string
		wantSummary    string
	}{
		{
			name:        "success sums suites",
			stdout:      "test result: ok. 3 passed; 0 failed;\ntest result: ok. 2 passed; 1 failed;\n",
			success:     true,
			wantPassed:  5,
			wantFailed:  1,
			wantSummary: "cargo test passed: 5 passed, 1 failed",
		},
		{
			name:           "failure names first failing test",
			stdout:         "---- probe::tests::broken stdout ----\nthread panicked\n---- probe::tests::other stdout ----\ntest result: FAILED. 0 passed; 2 failed;\n",
			wantPassed:     0,
			wantFailed:     2,
			wantFirst:      "probe::tests::broken",
			wantFailedList: []string{"probe::tests::broken", "probe::tests::other"},
			wantSummary:    "cargo test failed: probe::tests::broken",
		},
		{
			name:        "failure without names falls back to stderr",
			stderr:      "error: could not compile\n",
			wantSummary: "cargo test failed: error: could not compile",
		},
		{
			name:        "failure without names falls back to stdout",
			stdout:      "Compiling probe v0.1.0\n",
			wantSummary: "cargo test failed: Compiling probe v0.1.0",
		},
		{
			name:        "silent failure keeps default summary",
			wantSummary: "cargo test completed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: exitFor(tc.success)}}}
			tool := NewRustCargoTestTool(base)
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.success, res.Success)
			assert.Equal(t, tc.wantPassed, res.Data["passed"])
			assert.Equal(t, tc.wantFailed, res.Data["failed"])
			assert.Equal(t, tc.wantFirst, res.Data["first_failure"])
			assert.Equal(t, tc.wantFailedList, res.Data["failed_tests"])
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestRustCargoCheckTool_CountsErrorsAndWarnings(t *testing.T) {
	tests := []struct {
		name         string
		stdout       string
		stderr       string
		success      bool
		wantErrors   int
		wantWarnings int
		wantFirst    string
		wantSummary  string
	}{
		{
			name:        "clean success",
			stdout:      "    Finished dev [unoptimized] target(s)\n",
			success:     true,
			wantSummary: "cargo check passed: 0 errors, 0 warnings",
		},
		{
			name:         "success still counts located diagnostics",
			stdout:       "warning: unused variable `x`\nerror[E0382]: use of moved value\n",
			success:      true,
			wantErrors:   1,
			wantWarnings: 1,
			wantFirst:    "warning: unused variable `x`",
			wantSummary:  "cargo check passed: 1 errors, 1 warnings",
		},
		{
			name:         "failure reports first diagnostic",
			stderr:       "error: could not compile `probe` due to 1 previous error\nwarning: build failed\n",
			wantErrors:   1,
			wantWarnings: 1,
			wantFirst:    "error: could not compile `probe` due to 1 previous error",
			wantSummary:  "cargo check failed: error: could not compile `probe` due to 1 previous error",
		},
		{
			name:        "failure without diagnostics falls back to stderr",
			stderr:      "cargo does not talk to the network\n",
			wantSummary: "cargo check failed: cargo does not talk to the network",
		},
		{
			name:        "failure without diagnostics falls back to stdout",
			stdout:      "Compiling probe\n",
			wantSummary: "cargo check failed: Compiling probe",
		},
		{
			name:        "silent failure keeps default summary",
			wantSummary: "cargo check completed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			runner := &scriptedRunner{results: []*ports.CommandResult{{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: exitFor(tc.success)}}}
			tool := NewRustCargoCheckTool(base)
			tool.SetCommandRunner(runner)

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.success, res.Success)
			assert.Equal(t, tc.wantErrors, res.Data["error_count"])
			assert.Equal(t, tc.wantWarnings, res.Data["warning_count"])
			assert.Equal(t, tc.wantFirst, res.Data["first_message"])
			assert.Equal(t, tc.wantSummary, res.Data["summary"])
		})
	}
}

func TestRustCargoCheckTool_CommandConstructionAndExtraArgs(t *testing.T) {
	base := t.TempDir()
	writeCrate(t, base)
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("    Finished\n")}}
	tool := NewRustCargoCheckTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{
		"working_directory": base,
		"extra_args":        []string{"--all-targets"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, [][]string{{"cargo", "check", "--all-targets"}}, runner.argv())
}

func TestRustCargoMetadataTool_ParsesWorkspaceJSON(t *testing.T) {
	base := t.TempDir()
	stdout := `{"workspace_root":"/ws","packages":[` +
		`{"name":"probe","manifest_path":"/ws/Cargo.toml"},` +
		`{"name":"member","manifest_path":"/ws/crates/member/Cargo.toml"}]}`
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun(stdout)}}
	tool := NewRustCargoMetadataTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{"working_directory": "crates/member"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "cargo metadata: 2 packages workspace=/ws", res.Data["summary"])
	assert.Equal(t, "/ws", res.Data["workspace_root"])
	assert.Equal(t, []string{"probe", "member"}, res.Data["package_names"])
	assert.Equal(t, 2, res.Data["package_count"])
	assert.Equal(t, [][]string{{"cargo", "metadata", "--format-version", "1", "--no-deps"}}, runner.argv())
	assert.Equal(t, "crates/member", runner.requests[0].Workdir)
}

func TestRustCargoMetadataTool_ToleratesMalformedOutput(t *testing.T) {
	base := t.TempDir()
	runner := &scriptedRunner{results: []*ports.CommandResult{okRun("cargo: not json\n")}}
	tool := NewRustCargoMetadataTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "cargo metadata completed", res.Data["summary"])
	assert.NotContains(t, res.Data, "workspace_root")
}

func TestRustCargoMetadataTool_ReportsFailingCargo(t *testing.T) {
	base := t.TempDir()
	runner := &scriptedRunner{results: []*ports.CommandResult{{Stderr: "error: no such subcommand", ExitCode: 101}}}
	tool := NewRustCargoMetadataTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Equal(t, "error: no such subcommand", res.Error)
}

func TestRustTools_RunnerErrorSurfaces(t *testing.T) {
	runner := &scriptedRunner{err: errors.New("sandbox denied")}
	for name, tool := range map[string]ports.Tool{
		"rust_cargo_test":     NewRustCargoTestTool("."),
		"rust_cargo_check":    NewRustCargoCheckTool("."),
		"rust_cargo_metadata": NewRustCargoMetadataTool("."),
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

func TestRustTools_MetadataAndAvailability(t *testing.T) {
	tests := []struct {
		name      string
		tool      ports.Tool
		wantTags  []string
		wantDescr string
		wantAvail bool
	}{
		{
			name:      "rust_workspace_detect",
			tool:      &RustWorkspaceDetectTool{BasePath: "."},
			wantTags:  []string{ports.TagReadOnly, "lang:rust", "workspace-detect", "recovery"},
			wantDescr: "Detects the nearest Rust crate/workspace manifest for a file or directory.",
			wantAvail: true,
		},
		{
			name:      "rust_cargo_test",
			tool:      NewRustCargoTestTool("."),
			wantTags:  []string{ports.TagExecute, "lang:rust", "test", "verification", "diagnostics"},
			wantDescr: "Runs cargo test and returns structured Rust test results.",
		},
		{
			name:      "rust_cargo_check",
			tool:      NewRustCargoCheckTool("."),
			wantTags:  []string{ports.TagExecute, "lang:rust", "build", "verification", "diagnostics"},
			wantDescr: "Runs cargo check and returns structured Rust compile results.",
		},
		{
			name:      "rust_cargo_metadata",
			tool:      NewRustCargoMetadataTool("."),
			wantTags:  []string{ports.TagExecute, "lang:rust", "metadata", "recovery"},
			wantDescr: "Runs cargo metadata and returns structured Rust workspace data.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.name, tc.tool.Name())
			assert.Equal(t, tc.wantDescr, tc.tool.Description())
			assert.Equal(t, "rust", tc.tool.Category())
			assert.Equal(t, tc.wantTags, tc.tool.Tags())
			assert.NotEmpty(t, tc.tool.Parameters())
			assert.NotNil(t, tc.tool.Permissions().Permissions)
			assert.Equal(t, tc.wantAvail, tc.tool.IsAvailable(context.Background()))
		})
	}
}

func TestDetectRustManifests_StopsAtBasePath(t *testing.T) {
	base := t.TempDir()
	nearest, workspace := detectRustManifests(base, base)
	assert.Empty(t, nearest)
	assert.Empty(t, workspace)
}

func TestAtoiSafe(t *testing.T) {
	assert.Equal(t, 42, atoiSafe("42"))
	assert.Equal(t, 5, atoiSafe("5;"))
	assert.Equal(t, 0, atoiSafe("v5"))
}

func TestToStringSliceValue(t *testing.T) {
	got, err := toStringSliceValue([]any{"a", 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "2"}, got)

	_, err = toStringSliceValue("scalar")
	assert.Error(t, err)
}

func FuzzSummarizeRustCargoTest(f *testing.F) {
	f.Add("test result: ok. 3 passed; 0 failed;\n", "", true)
	f.Add("---- a::b stdout ----\ntest result: FAILED. 0 passed; 2 failed;\n", "", false)
	f.Add("", "error: could not compile", false)
	f.Fuzz(func(t *testing.T, stdout, stderr string, success bool) {
		summary := summarizeRustCargoTest(stdout, stderr, success)
		if summary.Passed < 0 || summary.Failed < 0 {
			t.Errorf("counts must never be negative: %+v", summary)
		}
		if summary.FirstFailure != "" && summary.Failed == 0 {
			t.Errorf("first failure %q without failed tests", summary.FirstFailure)
		}
		if summary.FirstFailure != "" && len(summary.FailedTests) == 0 {
			t.Errorf("first failure %q must come from FailedTests", summary.FirstFailure)
		}
	})
}

func FuzzSummarizeRustCargoCheck(f *testing.F) {
	f.Add("warning: unused\nerror: broken\n", "", false)
	f.Add("    Finished\n", "", true)
	f.Fuzz(func(t *testing.T, stdout, stderr string, success bool) {
		summary := summarizeRustCargoCheck(stdout, stderr, success)
		if summary.ErrorCount < 0 || summary.WarningCount < 0 {
			t.Errorf("counts must never be negative: %+v", summary)
		}
		if summary.Summary == "" {
			t.Errorf("summary must never be empty")
		}
	})
}

func FuzzParseRustCargoMetadata(f *testing.F) {
	f.Add(`{"workspace_root":"/w","packages":[{"name":"p","manifest_path":"/w/Cargo.toml"}]}`)
	f.Add(`{}`)
	f.Add(`garbage`)
	f.Fuzz(func(t *testing.T, payload string) {
		summary, parsed := parseRustCargoMetadata(payload)
		if summary == "" {
			t.Errorf("summary must never be empty")
		}
		if parsed == nil {
			t.Errorf("parsed map must never be nil")
		}
	})
}
