package framework

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestDeterministicFixtureGeneration validates that fixture generation produces
// stable, repeatable output across multiple invocations.
func TestDeterministicFixtureGeneration(t *testing.T) {
	t.Run("workspace fixture", func(t *testing.T) {
		builder1 := NewTempWorkspaceBuilder(t).
			WithFile("src/helper.go", "package src\n\nfunc Helper() {}\n").
			WithFile("README.md", "# Test Project\n").
			WithFile("main.go", "package main\n\nfunc main() {}\n").
			WithFile("config.yaml", "key: value\n")
		builder2 := NewTempWorkspaceBuilder(t).
			WithFile("config.yaml", "key: value\n").
			WithFile("main.go", "package main\n\nfunc main() {}\n").
			WithFile("README.md", "# Test Project\n").
			WithFile("src/helper.go", "package src\n\nfunc Helper() {}\n")

		BuildWorkspace(t, builder1)
		BuildWorkspace(t, builder2)

		files1 := NormalizePaths(collectFiles(t, builder1.basePath))
		files2 := NormalizePaths(collectFiles(t, builder2.basePath))
		if !reflect.DeepEqual(files1, files2) {
			t.Fatalf("workspace file sets differ:\n got: %#v\nwant: %#v", files1, files2)
		}
	})

	t.Run("permission set fixture", func(t *testing.T) {
		env1 := NewTestEnvironment(t)
		env2 := NewTestEnvironment(t)

		if env1.PermissionManager == nil || env2.PermissionManager == nil {
			t.Fatal("permission managers not initialized")
		}
		if env1.PermissionManager == env2.PermissionManager {
			t.Fatal("permission managers should be isolated per environment")
		}
		if env1.PermissionManager.DefaultPolicy() == "" {
			t.Fatal("expected a usable default permission policy")
		}
	})

	t.Run("telemetry fixture", func(t *testing.T) {
		env := NewTestEnvironment(t)

		event := telemetry.Event{Type: telemetry.EventNodeFinish, TaskID: "test-task", NodeID: "test-node", Message: "deterministic test message"}
		env.TelemetrySink.Emit(event)

		events := env.TelemetrySink.Events()
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		AssertNormalizedTelemetryEventsEqual(t, events, []telemetry.Event{event})
	})

	t.Run("audit fixture", func(t *testing.T) {
		env := NewTestEnvironment(t)

		record := NewAuditRecordBuilder().WithAgentID("test-agent").WithAction("test_action").WithType("test_type").WithPermission("test_permission").WithResult("granted").WithCorrelation("test-correlation").Build()
		if err := env.AuditSink.Log(context.TODO(), record); err != nil {
			t.Fatalf("failed to log audit record: %v", err)
		}

		records := env.AuditSink.Records()
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		AssertNormalizedAuditRecordsEqual(t, records, []policy.AuditRecord{record})
	})
}

// TestFixtureIsolation validates that fixtures from different tests do not interfere.
func TestFixtureIsolation(t *testing.T) {
	env1 := NewTestEnvironment(t)
	env2 := NewTestEnvironment(t)

	// Create a file in env1's workspace
	file1 := filepath.Join(env1.WorkspacePath, "test.txt")
	if err := os.WriteFile(file1, []byte("env1"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Create a file in env2's workspace
	file2 := filepath.Join(env2.WorkspacePath, "test.txt")
	if err := os.WriteFile(file2, []byte("env2"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if file1 == file2 {
		t.Fatal("files should be in different workspace paths")
	}

	// Verify file contents are different
	content1, err := os.ReadFile(file1)
	if err != nil {
		t.Fatalf("failed to read file1: %v", err)
	}
	content2, err := os.ReadFile(file2)
	if err != nil {
		t.Fatalf("failed to read file2: %v", err)
	}
	if string(content1) == string(content2) {
		t.Fatal("file contents should differ")
	}

	if got := NormalizePaths(collectFiles(t, env1.WorkspacePath)); !reflect.DeepEqual(got, []string{"test.txt"}) {
		t.Fatalf("unexpected env1 file set: %#v", got)
	}
	if got := NormalizePaths(collectFiles(t, env2.WorkspacePath)); !reflect.DeepEqual(got, []string{"test.txt"}) {
		t.Fatalf("unexpected env2 file set: %#v", got)
	}
}

// collectFiles recursively collects all file paths under a directory.
func collectFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// Store relative path from root for comparison
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk directory: %v", err)
	}

	return files
}

// TestWorkspaceFixtureBuilder validates the workspace fixture builder.
func TestWorkspaceFixtureBuilder(t *testing.T) {
	t.Run("small workspace", func(t *testing.T) {
		builder := SmallWorkspace(NewTempWorkspaceBuilder(t).basePath)
		BuildWorkspace(t, builder)

		want := []string{"README.md", "config.yaml", "main.go"}
		if got := NormalizePaths(collectFiles(t, builder.basePath)); !reflect.DeepEqual(got, want) {
			t.Fatalf("small workspace file set mismatch:\n got: %#v\nwant: %#v", got, want)
		}
		for _, path := range want {
			fullPath := filepath.Join(builder.basePath, path)
			if _, err := os.Stat(fullPath); err != nil {
				t.Errorf("file %s not created: %v", path, err)
			}
		}
	})

	t.Run("mixed language workspace", func(t *testing.T) {
		builder := MixedLanguageWorkspace(NewTempWorkspaceBuilder(t).basePath)
		BuildWorkspace(t, builder)

		for _, dir := range []string{"src", "python"} {
			fullPath := filepath.Join(builder.basePath, dir)
			if _, err := os.Stat(fullPath); err != nil {
				t.Errorf("directory %s not created: %v", dir, err)
			}
		}

		want := []string{"README.md", "config.json", "main.go", "python/script.py", "src/helper.go"}
		if got := NormalizePaths(collectFiles(t, builder.basePath)); !reflect.DeepEqual(got, want) {
			t.Fatalf("mixed workspace file set mismatch:\n got: %#v\nwant: %#v", got, want)
		}
		for _, path := range want {
			fullPath := filepath.Join(builder.basePath, path)
			if _, err := os.Stat(fullPath); err != nil {
				t.Errorf("file %s not created: %v", path, err)
			}
		}
	})

	t.Run("custom workspace", func(t *testing.T) {
		builder := NewTempWorkspaceBuilder(t).
			WithDirectory("custom/dir").
			WithFile("custom/file.txt", "custom content")
		BuildWorkspace(t, builder)

		dirPath := filepath.Join(builder.basePath, "custom/dir")
		if _, err := os.Stat(dirPath); err != nil {
			t.Errorf("custom directory not created: %v", err)
		}

		filePath := filepath.Join(builder.basePath, "custom/file.txt")
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("custom file not created: %v", err)
		}
		if string(content) != "custom content" {
			t.Errorf("custom file content mismatch: got %s", string(content))
		}
		if got := NormalizePaths(collectFiles(t, builder.basePath)); !reflect.DeepEqual(got, []string{"custom/file.txt"}) {
			t.Fatalf("custom workspace file set mismatch: %#v", got)
		}
	})
}

// TestManifestFixtureBuilder validates the manifest fixture builder.
func TestManifestFixtureBuilder(t *testing.T) {
	t.Run("valid manifest", func(t *testing.T) {
		builder := ValidManifest()
		m := builder.Build()
		if m == nil {
			t.Fatal("valid manifest should not be nil")
		}

		if err := m.Validate(); err != nil {
			t.Errorf("valid manifest should validate: %v", err)
		}
		if m.APIVersion != "relurpify/v1alpha1" || m.Kind != "AgentManifest" || m.Metadata.Name != "test-agent" {
			t.Fatalf("unexpected manifest identity: %+v", m.Metadata)
		}
		AssertNormalizedFileSystemPermissionsEqual(t, m.Spec.Policy.Permissions.FileSystem, []contracts.FileSystemPermission{
			{Action: contracts.FileSystemRead, Path: "${workspace}/**"},
			{Action: contracts.FileSystemWrite, Path: "${workspace}/**"},
		})
		clone := builder.Build()
		if clone == m {
			t.Fatal("manifest builder should return a new manifest instance on each build")
		}
	})

	t.Run("manifest with custom name", func(t *testing.T) {
		builder := ValidManifest().WithName("custom-agent")
		m := builder.Build()

		if m.Metadata.Name != "custom-agent" {
			t.Errorf("name not set: got %s", m.Metadata.Name)
		}
	})

	t.Run("manifest with custom version", func(t *testing.T) {
		builder := ValidManifest().WithVersion("2.0.0")
		m := builder.Build()

		if m.Metadata.Version != "2.0.0" {
			t.Errorf("version not set: got %s", m.Metadata.Version)
		}
	})

	t.Run("manifest with filesystem permission", func(t *testing.T) {
		builder := ValidManifest().
			WithFileSystemPermission(contracts.FileSystemRead, "${workspace}/src/**")
		m := builder.Build()

		AssertNormalizedFileSystemPermissionsEqual(t, m.Spec.Policy.Permissions.FileSystem, []contracts.FileSystemPermission{
			{Action: contracts.FileSystemRead, Path: "${workspace}/**"},
			{Action: contracts.FileSystemWrite, Path: "${workspace}/**"},
			{Action: contracts.FileSystemRead, Path: "${workspace}/src/**"},
		})
	})

	t.Run("manifest with network permission", func(t *testing.T) {
		builder := ValidManifest().
			WithNetworkPermission("egress", "tcp", "example.com", 443)
		m := builder.Build()

		AssertNormalizedNetworkPermissionsEqual(t, m.Spec.Policy.Permissions.Network, []contracts.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443}})
	})

	t.Run("manifest with HITL required", func(t *testing.T) {
		builder := ValidManifest().
			WithFileSystemPermission(contracts.FileSystemRead, "${workspace}/sensitive/**").
			WithHITLRequired()
		m := builder.Build()

		perms := m.Spec.Policy.Permissions.FileSystem
		if len(perms) == 0 {
			t.Fatal("no filesystem permissions found")
		}
		if !perms[len(perms)-1].HITLRequired {
			t.Error("HITLRequired not set on last permission")
		}
	})

	t.Run("invalid manifest missing apiVersion", func(t *testing.T) {
		builder := InvalidManifestMissingAPIVersion()
		m := builder.Build()

		if err := m.Validate(); err == nil {
			t.Error("manifest without apiVersion should fail validation")
		}
	})

	t.Run("invalid manifest missing kind", func(t *testing.T) {
		builder := InvalidManifestMissingKind()
		m := builder.Build()

		if err := m.Validate(); err == nil {
			t.Error("manifest without kind should fail validation")
		}
	})
}

// TestPolicyRuleFixtureBuilder validates the policy rule fixture builder.
func TestPolicyRuleFixtureBuilder(t *testing.T) {
	t.Run("allow all policy", func(t *testing.T) {
		builder := AllowAllPolicy()
		rules := builder.Build()

		AssertNormalizedPolicyRulesEqual(t, rules, []policy.PolicyRule{{ID: "allow-1", Name: "allow-1", Enabled: true, Priority: 100, Conditions: policy.PolicyConditions{Capabilities: []string{"tool:*"}}, Effect: policy.PolicyEffect{Action: "allow"}}})
	})

	t.Run("deny specific policy", func(t *testing.T) {
		builder := DenySpecificPolicy("dangerous_tool")
		rules := builder.Build()

		AssertNormalizedPolicyRulesEqual(t, rules, []policy.PolicyRule{
			{ID: "allow-all", Name: "allow-all", Enabled: true, Priority: 100, Conditions: policy.PolicyConditions{Capabilities: []string{"tool:*"}}, Effect: policy.PolicyEffect{Action: "allow"}},
			{ID: "deny-specific", Name: "deny-specific", Enabled: true, Priority: 100, Conditions: policy.PolicyConditions{Capabilities: []string{"dangerous_tool"}}, Effect: policy.PolicyEffect{Action: "deny", Reason: "security restriction"}},
		})
	})

	t.Run("custom policy rules", func(t *testing.T) {
		builder := NewPolicyRuleBuilder().
			WithAllowRule("rule-1", "tool:read").
			WithDenyRule("rule-2", "tool:delete", "data protection")
		rules := builder.Build()

		AssertNormalizedPolicyRulesEqual(t, rules, []policy.PolicyRule{
			{ID: "rule-1", Name: "rule-1", Enabled: true, Priority: 100, Conditions: policy.PolicyConditions{Capabilities: []string{"tool:read"}}, Effect: policy.PolicyEffect{Action: "allow"}},
			{ID: "rule-2", Name: "rule-2", Enabled: true, Priority: 100, Conditions: policy.PolicyConditions{Capabilities: []string{"tool:delete"}}, Effect: policy.PolicyEffect{Action: "deny", Reason: "data protection"}},
		})
	})
}

// TestAuditRecordFixtureBuilder validates the audit record fixture builder.
func TestAuditRecordFixtureBuilder(t *testing.T) {
	t.Run("granted audit record", func(t *testing.T) {
		builder := GrantedAuditRecord()
		record := builder.Build()

		if record.Result != "granted" {
			t.Errorf("expected result 'granted', got %s", record.Result)
		}
	})

	t.Run("denied audit record", func(t *testing.T) {
		builder := DeniedAuditRecord("permission denied")
		record := builder.Build()

		if record.Result != "denied" {
			t.Errorf("expected result 'denied', got %s", record.Result)
		}
		if record.Metadata["reason"] != "permission denied" {
			t.Errorf("reason metadata mismatch: got %#v", record.Metadata)
		}
	})

	t.Run("custom audit record", func(t *testing.T) {
		builder := NewAuditRecordBuilder().
			WithAgentID("custom-agent").
			WithAction("custom_action").
			WithType("custom_type").
			WithPermission("custom_permission").
			WithResult("custom_result").
			WithMetadata("source", "fixture").
			WithCorrelation("custom-correlation")
		record := builder.Build()

		AssertNormalizedAuditRecordsEqual(t, []policy.AuditRecord{record}, []policy.AuditRecord{{AgentID: "custom-agent", Action: "custom_action", Type: "custom_type", Permission: "custom_permission", Result: "custom_result", Metadata: map[string]interface{}{"source": "fixture"}, Correlation: "custom-correlation"}})
	})
}

// TestEnvelopeFixtureBuilder validates the envelope fixture builder.
func TestEnvelopeFixtureBuilder(t *testing.T) {
	t.Run("minimal envelope", func(t *testing.T) {
		builder := MinimalEnvelope()
		env := builder.Build()

		if env == nil {
			t.Fatal("envelope should not be nil")
		}
		if env.TaskID != "test-task" || env.SessionID != "test-session" || env.NodeID != "test-node" {
			t.Fatalf("unexpected minimal envelope identity: %+v", env)
		}
		if !env.IsEmpty() {
			t.Fatal("minimal envelope should be empty")
		}
	})

	t.Run("envelope with working values", func(t *testing.T) {
		builder := NewEnvelopeBuilder().
			WithTaskID("custom-task").
			WithSessionID("custom-session").
			WithNodeID("custom-node").
			WithWorkingValue("key1", "value1", contextdata.MemoryClassTask).
			WithWorkingValue("key2", "value2", contextdata.MemoryClassTask)
		env := builder.Build()

		if env.TaskID != "custom-task" {
			t.Errorf("task ID mismatch: got %s", env.TaskID)
		}
		if env.SessionID != "custom-session" {
			t.Errorf("session ID mismatch: got %s", env.SessionID)
		}
		if env.NodeID != "custom-node" {
			t.Errorf("node ID mismatch: got %s", env.NodeID)
		}

		if keys := env.WorkingMemoryKeys(); !reflect.DeepEqual(keys, []string{"key1", "key2"}) {
			t.Fatalf("unexpected working memory keys: %#v", keys)
		}

		if snapshot := env.WorkingDataSnapshot(); snapshot["key1"] != "value1" || snapshot["key2"] != "value2" {
			t.Fatalf("unexpected working data snapshot: %#v", snapshot)
		}
	})
}

// TestNormalizationHelpers validates the normalization helper functions.
func TestNormalizationHelpers(t *testing.T) {
	t.Run("sort strings", func(t *testing.T) {
		input := []string{"zebra", "apple", "banana"}
		sorted := SortStrings(input)
		if !reflect.DeepEqual(input, []string{"zebra", "apple", "banana"}) {
			t.Fatalf("SortStrings should not mutate input: %#v", input)
		}

		expected := []string{"apple", "banana", "zebra"}
		if !reflect.DeepEqual(sorted, expected) {
			t.Fatalf("sorted strings mismatch:\n got: %#v\nwant: %#v", sorted, expected)
		}
	})

	t.Run("normalize path", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"./test/file.txt", "test/file.txt"},
			{"./test/./file.txt", "test/file.txt"},
			{"test/../test/file.txt", "test/file.txt"},
			{"test/file.txt", "test/file.txt"},
		}

		for _, tt := range tests {
			result := NormalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		}
	})

	t.Run("normalize paths", func(t *testing.T) {
		input := []string{"./zebra", "apple", "./banana"}
		normalized := NormalizePaths(input)

		expected := []string{"apple", "banana", "zebra"}
		if !reflect.DeepEqual(normalized, expected) {
			t.Fatalf("normalized path slice mismatch:\n got: %#v\nwant: %#v", normalized, expected)
		}
	})

	t.Run("audit and telemetry normalization", func(t *testing.T) {
		timestamp := policy.AuditRecord{}.Timestamp
		audit := []policy.AuditRecord{{AgentID: "agent", Action: "action", Type: "type", Permission: "permission", Result: "granted", Timestamp: timestamp, Metadata: map[string]interface{}{"reason": "fixture"}}}
		telemetry := []telemetry.Event{{Type: telemetry.EventNodeFinish, TaskID: "task", NodeID: "node", Message: "msg", Timestamp: timestamp, Metadata: map[string]interface{}{"source": "fixture"}}}

		normalizedAudit := NormalizeAuditRecords(audit)
		normalizedTelemetry := NormalizeTelemetryEvents(telemetry)

		if normalizedAudit[0].Timestamp != (reflect.ValueOf(time.Time{}).Interface().(time.Time)) {
			t.Fatal("audit normalization should zero timestamps")
		}
		if normalizedTelemetry[0].Timestamp != (reflect.ValueOf(time.Time{}).Interface().(time.Time)) {
			t.Fatal("telemetry normalization should zero timestamps")
		}
		normalizedAudit[0].Metadata["reason"] = "changed"
		if audit[0].Metadata["reason"] != "fixture" {
			t.Fatal("audit metadata should be deep copied")
		}
		normalizedTelemetry[0].Metadata["source"] = "changed"
		if telemetry[0].Metadata["source"] != "fixture" {
			t.Fatal("telemetry metadata should be deep copied")
		}
	})

	t.Run("permission and rule normalization", func(t *testing.T) {
		fs := NormalizeFileSystemPermissions([]contracts.FileSystemPermission{{Action: contracts.FileSystemWrite, Path: "./b"}, {Action: contracts.FileSystemRead, Path: "./a"}})
		if !reflect.DeepEqual(fs, []contracts.FileSystemPermission{{Action: contracts.FileSystemRead, Path: "a"}, {Action: contracts.FileSystemWrite, Path: "b"}}) {
			t.Fatalf("filesystem normalization mismatch: %#v", fs)
		}

		net := NormalizeNetworkPermissions([]contracts.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "z.example", Port: 443}, {Direction: "egress", Protocol: "tcp", Host: "a.example", Port: 80}})
		if !reflect.DeepEqual(net, []contracts.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "a.example", Port: 80}, {Direction: "egress", Protocol: "tcp", Host: "z.example", Port: 443}}) {
			t.Fatalf("network normalization mismatch: %#v", net)
		}

		rules := NormalizePolicyRules([]policy.PolicyRule{{ID: "rule-2", Priority: 20, Effect: policy.PolicyEffect{Action: "deny"}}, {ID: "rule-1", Priority: 10, Effect: policy.PolicyEffect{Action: "allow"}}})
		if len(rules) != 2 || rules[0].ID != "rule-1" || rules[1].ID != "rule-2" {
			t.Fatalf("policy rule normalization mismatch: %#v", rules)
		}
	})
}
