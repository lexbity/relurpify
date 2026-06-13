//go:build live
// +build live

package agenttest

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	api_example_com                             = "api.example.com"
	etc_passwd                                  = "/etc/passwd"
	expected_nil_manifest_to_not_cover_anything = "Expected nil manifest to not cover anything"
	home_user                                   = "/home/user"
	var_log_log                                 = "/var/log/*.log"
)

// TestOutcomeSpecRoundTrip verifies marshal/unmarshal preserves the generic fields.
func TestOutcomeSpecRoundTrip(t *testing.T) {
	original := &OutcomeSpec{
		NoFileChanges:        false,
		FilesChanged:         []string{"file1.go", "file2.go"},
		FilesContain:         []FileContentExpectation{{Path: test_go, Contains: []string{"func"}, NotContains: []string{"panic"}}},
		OutputContains:       []string{success},
		OutputRegex:          []string{"^done$"},
		StateKeyNotEmpty:     []string{key1},
		StateKeysMustExist:   []string{key2},
		MemoryRecordsCreated: 5,
		WorkflowStateUpdated: true,
		Verify: &VerifySpec{
			Steps: []VerifyStepSpec{
				{Tool: go_test, Args: map[string]any{"package": "./...", "working_directory": "."}},
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
		ToolsMustNotCall:         []string{file_write, "file_delete"},
		MutationEnforced:         true,
		NoNetworkOutsideManifest: true,
		NoExecOutsideManifest:    true,
		ExpectedViolations: []ExpectedViolation{
			{Kind: file_write, Resource: etc_passwd, Reason: "expected block"},
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

// TestBenchmarkSpecRoundTrip verifies marshal/unmarshal preserves the generic fields.
func TestBenchmarkSpecRoundTrip(t *testing.T) {
	original := &BenchmarkSpec{
		ToolsExpected:          []string{file_read, file_search},
		ToolsNotExpected:       []string{go_test},
		ToolSequenceExpected:   []string{file_read, file_write},
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
}

// TestLoadSuiteRejectsLegacyExpectFields verifies legacy schema fields fail strict loading.
func TestLoadSuiteRejectsLegacyExpectFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "suite.yaml")
	err := fs.WriteFileSecure(path, []byte(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: sample
spec:
  agent_name: coding
  manifest: relurpify_cfg/agent.yaml
  cases:
    - name: smoke
      prompt: summarize
      expect:
        must_succeed: true
`))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := LoadSuite(path); err == nil {
		t.Fatal("expected legacy expect.must_succeed to fail load")
	}
}

func TestLoadSuiteRejectsLegacyEucloAndControlFlowFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "suite.yaml")
	err := fs.WriteFileSecure(path, []byte(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: sample
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agent.yaml
  cases:
    - name: smoke
      prompt: summarize
      expect:
        outcome:
          euclo_mode: debug
        benchmark:
          euclo:
            profile: trace_execute_analyze
      overrides:
        control_flow: pipeline
`))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := LoadSuite(path); err == nil {
		t.Fatal("expected legacy euclo/control_flow fields to fail load")
	}
}

func testDocumentWithPermissions(t *testing.T) *config.Document {
	t.Helper()
	doc := &config.Document{
		APIVersion: "relurpify.io/v1",
		Kind:       "AgentManifest",
		Metadata:   config.DocumentMetadata{Name: "coverage-agent"},
		Spec:       map[string]yaml.Node{},
	}
	var node yaml.Node
	if err := node.Encode(permissions.PermissionSet{
		FileSystem: []permissions.FileSystemPermission{
			{Action: permissions.FileSystemWrite, Path: "${workspace}/**"},
			{Action: permissions.FileSystemRead, Path: "/tmp/*.log"},
			{Action: permissions.FileSystemDelete, Path: "/var/data/*"},
		},
		Executables: []permissions.ExecutablePermission{
			{Binary: "go"},
			{Binary: "git"},
			{Binary: "python*"},
		},
		Network: []permissions.NetworkPermission{
			{Host: api_example_com, Port: 443},
			{Host: "*.local", Port: 0},
			{Host: "localhost", Port: 8080},
		},
	}); err != nil {
		t.Fatalf("encode permissions: %v", err)
	}
	doc.Spec["permissions"] = node
	return doc
}

// TestDocumentCoversFileAction verifies permission checking for file actions
func TestDocumentCoversFileAction(t *testing.T) {
	m := testDocumentWithPermissions(t)

	workspace := "/home/user/project"

	// Test: write within workspace should be covered
	if !DocumentCoversFileAction(m, permissions.FileSystemWrite, "file.go", workspace) {
		t.Error("Expected write to file.go to be covered by ${workspace}/**")
	}

	// Test: write to absolute path within workspace
	if !DocumentCoversFileAction(m, permissions.FileSystemWrite, "/home/user/project/src/main.go", workspace) {
		t.Error("Expected write to /home/user/project/src/main.go to be covered")
	}

	// Test: read from /tmp with matching pattern
	if !DocumentCoversFileAction(m, permissions.FileSystemRead, "/tmp/app.log", workspace) {
		t.Error("Expected read of /tmp/app.log to be covered")
	}

	// Test: read from /tmp with non-matching pattern
	if DocumentCoversFileAction(m, permissions.FileSystemRead, "/tmp/app.txt", workspace) {
		t.Error("Expected read of /tmp/app.txt to NOT be covered (wrong extension)")
	}

	// Test: action not matching (write vs read)
	if DocumentCoversFileAction(m, permissions.FileSystemWrite, "/tmp/app.log", workspace) {
		t.Error("Expected write to /tmp/app.log to NOT be covered (pattern is read-only)")
	}

	// Test: nil manifest
	if DocumentCoversFileAction(nil, permissions.FileSystemWrite, "file.go", workspace) {
		t.Error(expected_nil_manifest_to_not_cover_anything)
	}
}

// TestDocumentCoversExecutable verifies binary permission checking
func TestDocumentCoversExecutable(t *testing.T) {
	m := testDocumentWithPermissions(t)

	// Test: declared binary
	if !DocumentCoversExecutable(m, "go") {
		t.Error("Expected 'go' to be covered")
	}

	// Test: declared binary with path
	if !DocumentCoversExecutable(m, "/usr/bin/git") {
		t.Error("Expected '/usr/bin/git' to be covered (basename matches)")
	}

	// Test: glob match
	if !DocumentCoversExecutable(m, "python3") {
		t.Error("Expected 'python3' to be covered by 'python*' glob")
	}

	// Test: undeclared binary
	if DocumentCoversExecutable(m, "rm") {
		t.Error("Expected 'rm' to NOT be covered")
	}

	// Test: nil manifest
	if DocumentCoversExecutable(nil, "go") {
		t.Error(expected_nil_manifest_to_not_cover_anything)
	}
}

// TestDocumentCoversNetworkCall verifies network permission checking
func TestDocumentCoversNetworkCall(t *testing.T) {
	m := testDocumentWithPermissions(t)

	// Test: exact host and port match
	if !DocumentCoversNetworkCall(m, api_example_com, 443) {
		t.Error("Expected api.example.com:443 to be covered")
	}

	// Test: wrong port
	if DocumentCoversNetworkCall(m, api_example_com, 80) {
		t.Error("Expected api.example.com:80 to NOT be covered (wrong port)")
	}

	// Test: glob host with any port
	if !DocumentCoversNetworkCall(m, "server.local", 1234) {
		t.Error("Expected server.local:1234 to be covered by *.local with any port")
	}

	// Test: undeclared host
	if DocumentCoversNetworkCall(m, "evil.com", 443) {
		t.Error("Expected evil.com to NOT be covered")
	}

	// Test: nil manifest
	if DocumentCoversNetworkCall(nil, "localhost", 8080) {
		t.Error(expected_nil_manifest_to_not_cover_anything)
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
    resource: etc_passwd
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
	if first.Kind != file_write {
		t.Errorf("First violation Kind: got %q, want %q", first.Kind, file_write)
	}
	if first.Resource != etc_passwd {
		t.Errorf("First violation Resource: got %q, want %q", first.Resource, etc_passwd)
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
		{"/var/log/app.log", var_log_log, true},
		{"/var/log/subdir/app.log", var_log_log, false},
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
		{"${workspace}/**", home_user, "/home/user/**"},
		{"${workspace}/src/*.go", home_user, "/home/user/src/*.go"},
		{var_log_log, home_user, var_log_log},
		{"${workspace}", "", "${workspace}"},
	}

	for _, tc := range tests {
		got := expandPathPattern(tc.pattern, tc.workspace)
		if got != tc.want {
			t.Errorf("expandPathPattern(%q, %q) = %q, want %q", tc.pattern, tc.workspace, got, tc.want)
		}
	}
}
