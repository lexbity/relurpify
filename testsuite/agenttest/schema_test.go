package agenttest

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"gopkg.in/yaml.v3"
)

// TestOutcomeSpecRoundTrip verifies marshal/unmarshal preserves all fields
func TestOutcomeSpecRoundTrip(t *testing.T) {
	original := &OutcomeSpec{
		MustSucceed:          true,
		NoFileChanges:        false,
		FilesChanged:         []string{"file1.go", "file2.go"},
		FilesContain:         []FileContentExpectation{{Path: "test.go", Contains: []string{"func"}, NotContains: []string{"panic"}}},
		OutputContains:       []string{"success"},
		OutputRegex:          []string{"^done$"},
		StateKeyNotEmpty:     []string{"key1"},
		StateKeysMustExist:   []string{"key2"},
		MemoryRecordsCreated: 5,
		WorkflowStateUpdated: true,
		EucloMode:            "debug",
		Verify: &VerifySpec{
			Steps: []VerifyStepSpec{
				{Tool: "go_test", Args: map[string]any{"package": "./...", "working_directory": "."}},
			},
			Script: "testsuite/agenttest_fixtures/gosuite/verify.sh",
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtripped OutcomeSpec
	if err := yaml.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.MustSucceed != original.MustSucceed {
		t.Errorf("MustSucceed: got %v, want %v", roundtripped.MustSucceed, original.MustSucceed)
	}
	if roundtripped.NoFileChanges != original.NoFileChanges {
		t.Errorf("NoFileChanges: got %v, want %v", roundtripped.NoFileChanges, original.NoFileChanges)
	}
	if len(roundtripped.FilesChanged) != len(original.FilesChanged) {
		t.Errorf("FilesChanged length: got %d, want %d", len(roundtripped.FilesChanged), len(original.FilesChanged))
	}
	if len(roundtripped.FilesContain) != len(original.FilesContain) {
		t.Errorf("FilesContain length: got %d, want %d", len(roundtripped.FilesContain), len(original.FilesContain))
	}
	if len(roundtripped.FilesContain) > 0 {
		if len(roundtripped.FilesContain[0].NotContains) != len(original.FilesContain[0].NotContains) {
			t.Errorf("NotContains length: got %d, want %d", len(roundtripped.FilesContain[0].NotContains), len(original.FilesContain[0].NotContains))
		}
	}
	if roundtripped.EucloMode != original.EucloMode {
		t.Errorf("EucloMode: got %q, want %q", roundtripped.EucloMode, original.EucloMode)
	}
	if roundtripped.Verify == nil {
		t.Fatal("Verify should not be nil")
	}
	if len(roundtripped.Verify.Steps) != len(original.Verify.Steps) {
		t.Fatalf("Verify.Steps length: got %d, want %d", len(roundtripped.Verify.Steps), len(original.Verify.Steps))
	}
	if roundtripped.Verify.Steps[0].Tool != original.Verify.Steps[0].Tool {
		t.Errorf("Verify.Steps[0].Tool: got %q, want %q", roundtripped.Verify.Steps[0].Tool, original.Verify.Steps[0].Tool)
	}
	if roundtripped.Verify.Script != original.Verify.Script {
		t.Errorf("Verify.Script: got %q, want %q", roundtripped.Verify.Script, original.Verify.Script)
	}
}

// TestSecuritySpecRoundTrip verifies marshal/unmarshal preserves all fields
func TestSecuritySpecRoundTrip(t *testing.T) {
	original := &SecuritySpec{
		NoWritesOutsideScope:     true,
		NoReadsOutsideScope:      false,
		ToolsMustNotCall:         []string{"file_write", "file_delete"},
		MutationEnforced:         true,
		NoNetworkOutsideManifest: true,
		NoExecOutsideManifest:    true,
		ExpectedViolations: []ExpectedViolation{
			{Kind: "file_write", Resource: "/etc/passwd", Reason: "expected block"},
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtripped SecuritySpec
	if err := yaml.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if roundtripped.NoWritesOutsideScope != original.NoWritesOutsideScope {
		t.Errorf("NoWritesOutsideScope: got %v, want %v", roundtripped.NoWritesOutsideScope, original.NoWritesOutsideScope)
	}
	if len(roundtripped.ToolsMustNotCall) != len(original.ToolsMustNotCall) {
		t.Errorf("ToolsMustNotCall length: got %d, want %d", len(roundtripped.ToolsMustNotCall), len(original.ToolsMustNotCall))
	}
	if roundtripped.MutationEnforced != original.MutationEnforced {
		t.Errorf("MutationEnforced: got %v, want %v", roundtripped.MutationEnforced, original.MutationEnforced)
	}
	if len(roundtripped.ExpectedViolations) != len(original.ExpectedViolations) {
		t.Errorf("ExpectedViolations length: got %d, want %d", len(roundtripped.ExpectedViolations), len(original.ExpectedViolations))
	}
}

// TestBenchmarkSpecRoundTrip verifies marshal/unmarshal preserves all fields
func TestBenchmarkSpecRoundTrip(t *testing.T) {
	original := &BenchmarkSpec{
		ToolsExpected:          []string{"file_read", "file_search"},
		ToolsNotExpected:       []string{"go_test"},
		ToolSequenceExpected:   []string{"file_read", "file_write"},
		LLMCallsExpected:       10,
		MaxToolCallsHint:       20,
		MaxTotalToolTimeHintMs: 5000,
		LLMResponseStableHint:  true,
		DeterminismScoreHint:   "high",
		TokenBudget: &TokenBudgetHint{
			MaxPrompt:     50000,
			MaxCompletion: 8000,
			MaxTotal:      58000,
		},
		Euclo: &EucloBenchmarkSpec{
			BehaviorFamily:                 "stale_assumption_detection",
			Profile:                        "trace_execute_analyze",
			PrimaryRelurpicCapability:      "euclo:debug.investigate-repair",
			SupportingRelurpicCapabilities: []string{"euclo:debug.root-cause"},
			RecipeIDs:                      []string{"debug.investigate-repair.reproduce"},
			ArtifactsProduced:              []string{"euclo.explore"},
			PhasesExecuted:                 []string{"analyze", "trace"},
			ResultClass:                    "localization_complete",
			AssuranceClass:                 "medium",
			MinTransitionsProposed:         0,
			MaxTransitionsProposed:         2,
			FrameKindsEmitted:              []string{"artifact", "transition"},
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtripped BenchmarkSpec
	if err := yaml.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(roundtripped.ToolsExpected) != len(original.ToolsExpected) {
		t.Errorf("ToolsExpected length: got %d, want %d", len(roundtripped.ToolsExpected), len(original.ToolsExpected))
	}
	if roundtripped.LLMCallsExpected != original.LLMCallsExpected {
		t.Errorf("LLMCallsExpected: got %d, want %d", roundtripped.LLMCallsExpected, original.LLMCallsExpected)
	}
	if roundtripped.TokenBudget == nil {
		t.Error("TokenBudget is nil, expected value")
	} else if roundtripped.TokenBudget.MaxPrompt != original.TokenBudget.MaxPrompt {
		t.Errorf("TokenBudget.MaxPrompt: got %d, want %d", roundtripped.TokenBudget.MaxPrompt, original.TokenBudget.MaxPrompt)
	}
	if roundtripped.Euclo == nil {
		t.Error("Euclo is nil, expected value")
	} else if roundtripped.Euclo.BehaviorFamily != original.Euclo.BehaviorFamily {
		t.Errorf("Euclo.BehaviorFamily: got %q, want %q", roundtripped.Euclo.BehaviorFamily, original.Euclo.BehaviorFamily)
	}
}

// TestExpectSpecBackwardCompat verifies existing YAML parses with nil new blocks
func TestExpectSpecBackwardCompat(t *testing.T) {
	// Legacy YAML without the new blocks
	legacyYAML := `
must_succeed: true
no_file_changes: false
output_contains:
  - "success"
files_changed:
  - "test.go"
memory_records_created: 3
`

	var expect ExpectSpec
	if err := yaml.Unmarshal([]byte(legacyYAML), &expect); err != nil {
		t.Fatalf("unmarshal of legacy YAML failed: %v", err)
	}

	// Verify legacy fields parsed correctly
	if !expect.MustSucceed {
		t.Error("MustSucceed should be true")
	}
	if expect.NoFileChanges {
		t.Error("NoFileChanges should be false")
	}
	if len(expect.OutputContains) != 1 || expect.OutputContains[0] != "success" {
		t.Error("OutputContains not parsed correctly")
	}

	// Verify new blocks are nil
	if expect.Outcome != nil {
		t.Error("Outcome should be nil for legacy YAML")
	}
	if expect.Security != nil {
		t.Error("Security should be nil for legacy YAML")
	}
	if expect.Benchmark != nil {
		t.Error("Benchmark should be nil for legacy YAML")
	}
}

// TestManifestCoversFileAction verifies permission checking for file actions
func TestManifestCoversFileAction(t *testing.T) {
	m := &manifest.AgentManifest{
		Spec: manifest.ManifestSpec{
			Permissions: core.PermissionSet{
				FileSystem: []core.FileSystemPermission{
					{Action: core.FileSystemWrite, Path: "${workspace}/**"},
					{Action: core.FileSystemRead, Path: "/tmp/*.log"},
					{Action: core.FileSystemDelete, Path: "/var/data/*"},
				},
			},
		},
	}

	workspace := "/home/user/project"

	// Test: write within workspace should be covered
	if !ManifestCoversFileAction(m, core.FileSystemWrite, "file.go", workspace) {
		t.Error("Expected write to file.go to be covered by ${workspace}/**")
	}

	// Test: write to absolute path within workspace
	if !ManifestCoversFileAction(m, core.FileSystemWrite, "/home/user/project/src/main.go", workspace) {
		t.Error("Expected write to /home/user/project/src/main.go to be covered")
	}

	// Test: read from /tmp with matching pattern
	if !ManifestCoversFileAction(m, core.FileSystemRead, "/tmp/app.log", workspace) {
		t.Error("Expected read of /tmp/app.log to be covered")
	}

	// Test: read from /tmp with non-matching pattern
	if ManifestCoversFileAction(m, core.FileSystemRead, "/tmp/app.txt", workspace) {
		t.Error("Expected read of /tmp/app.txt to NOT be covered (wrong extension)")
	}

	// Test: action not matching (write vs read)
	if ManifestCoversFileAction(m, core.FileSystemWrite, "/tmp/app.log", workspace) {
		t.Error("Expected write to /tmp/app.log to NOT be covered (pattern is read-only)")
	}

	// Test: nil manifest
	if ManifestCoversFileAction(nil, core.FileSystemWrite, "file.go", workspace) {
		t.Error("Expected nil manifest to not cover anything")
	}
}

// TestManifestCoversExecutable verifies binary permission checking
func TestManifestCoversExecutable(t *testing.T) {
	m := &manifest.AgentManifest{
		Spec: manifest.ManifestSpec{
			Permissions: core.PermissionSet{
				Executables: []core.ExecutablePermission{
					{Binary: "go"},
					{Binary: "git"},
					{Binary: "python*"},
				},
			},
		},
	}

	// Test: declared binary
	if !ManifestCoversExecutable(m, "go") {
		t.Error("Expected 'go' to be covered")
	}

	// Test: declared binary with path
	if !ManifestCoversExecutable(m, "/usr/bin/git") {
		t.Error("Expected '/usr/bin/git' to be covered (basename matches)")
	}

	// Test: glob match
	if !ManifestCoversExecutable(m, "python3") {
		t.Error("Expected 'python3' to be covered by 'python*' glob")
	}

	// Test: undeclared binary
	if ManifestCoversExecutable(m, "rm") {
		t.Error("Expected 'rm' to NOT be covered")
	}

	// Test: nil manifest
	if ManifestCoversExecutable(nil, "go") {
		t.Error("Expected nil manifest to not cover anything")
	}
}

// TestManifestCoversNetworkCall verifies network permission checking
func TestManifestCoversNetworkCall(t *testing.T) {
	m := &manifest.AgentManifest{
		Spec: manifest.ManifestSpec{
			Permissions: core.PermissionSet{
				Network: []core.NetworkPermission{
					{Host: "api.example.com", Port: 443},
					{Host: "*.local", Port: 0}, // any port
					{Host: "localhost", Port: 8080},
				},
			},
		},
	}

	// Test: exact host and port match
	if !ManifestCoversNetworkCall(m, "api.example.com", 443) {
		t.Error("Expected api.example.com:443 to be covered")
	}

	// Test: wrong port
	if ManifestCoversNetworkCall(m, "api.example.com", 80) {
		t.Error("Expected api.example.com:80 to NOT be covered (wrong port)")
	}

	// Test: glob host with any port
	if !ManifestCoversNetworkCall(m, "server.local", 1234) {
		t.Error("Expected server.local:1234 to be covered by *.local with any port")
	}

	// Test: undeclared host
	if ManifestCoversNetworkCall(m, "evil.com", 443) {
		t.Error("Expected evil.com to NOT be covered")
	}

	// Test: nil manifest
	if ManifestCoversNetworkCall(nil, "localhost", 8080) {
		t.Error("Expected nil manifest to not cover anything")
	}
}

// TestExpectedViolationParsing verifies YAML parsing of expected_violations
func TestExpectedViolationParsing(t *testing.T) {
	yamlContent := `
no_writes_outside_scope: true
tools_must_not_call:
  - file_write
  - file_delete
expected_violations:
  - kind: file_write
    resource: "/etc/passwd"
    reason: "expected sandbox block"
  - kind: exec
    resource: "sudo"
    reason: "should be blocked"
`

	var spec SecuritySpec
	if err := yaml.Unmarshal([]byte(yamlContent), &spec); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(spec.ExpectedViolations) != 2 {
		t.Fatalf("Expected 2 expected violations, got %d", len(spec.ExpectedViolations))
	}

	first := spec.ExpectedViolations[0]
	if first.Kind != "file_write" {
		t.Errorf("First violation Kind: got %q, want %q", first.Kind, "file_write")
	}
	if first.Resource != "/etc/passwd" {
		t.Errorf("First violation Resource: got %q, want %q", first.Resource, "/etc/passwd")
	}

	second := spec.ExpectedViolations[1]
	if second.Kind != "exec" {
		t.Errorf("Second violation Kind: got %q, want %q", second.Kind, "exec")
	}
}

// TestPathMatchesGlob verifies glob pattern matching
func TestPathMatchesGlob(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
	}{
		{"/home/user/file.go", "/home/user/*.go", true},
		{"/home/user/file.txt", "/home/user/*.go", false},
		{"/home/user/a/b/c/file.go", "/home/user/**/*.go", true},
		{"/home/user/file.go", "/home/user/**", true},
		{"/var/log/app.log", "/var/log/*.log", true},
		{"/var/log/subdir/app.log", "/var/log/*.log", false},
		{"/var/log/subdir/app.log", "/var/log/**/*.log", true},
	}

	for _, tc := range tests {
		got := pathMatchesGlob(tc.path, tc.pattern)
		if got != tc.want {
			t.Errorf("pathMatchesGlob(%q, %q) = %v, want %v", tc.path, tc.pattern, got, tc.want)
		}
	}
}

// TestExpandPathPattern verifies variable expansion in path patterns
func TestExpandPathPattern(t *testing.T) {
	tests := []struct {
		pattern   string
		workspace string
		want      string
	}{
		{"${workspace}/**", "/home/user", "/home/user/**"},
		{"${workspace}/src/*.go", "/home/user", "/home/user/src/*.go"},
		{"/var/log/*.log", "/home/user", "/var/log/*.log"},
		{"${workspace}", "", "${workspace}"},
	}

	for _, tc := range tests {
		got := expandPathPattern(tc.pattern, tc.workspace)
		if got != tc.want {
			t.Errorf("expandPathPattern(%q, %q) = %q, want %q", tc.pattern, tc.workspace, got, tc.want)
		}
	}
}
