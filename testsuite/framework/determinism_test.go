package framework

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestRepeatedFixtureEquivalence validates that fixture creation produces
// stable, repeatable output across multiple invocations.
func TestRepeatedFixtureEquivalence(t *testing.T) {
	t.Run("workspace fixture equivalence", func(t *testing.T) {
		builder1 := SmallWorkspace(NewTempWorkspaceBuilder(t).basePath)
		builder2 := SmallWorkspace(NewTempWorkspaceBuilder(t).basePath)

		BuildWorkspace(t, builder1)
		BuildWorkspace(t, builder2)

		files1 := NormalizePaths(collectFiles(t, builder1.basePath))
		files2 := NormalizePaths(collectFiles(t, builder2.basePath))
		if !reflect.DeepEqual(files1, files2) {
			t.Fatalf("workspace file sets differ:\n got: %#v\nwant: %#v", files1, files2)
		}
	})

	t.Run("manifest fixture equivalence", func(t *testing.T) {
		m1 := ValidManifest().Build()
		m2 := ValidManifest().Build()

		AssertNormalizedFileSystemPermissionsEqual(t, m1.Policy.Permissions.FileSystem, m2.Policy.Permissions.FileSystem)
	})

	t.Run("policy rule fixture equivalence", func(t *testing.T) {
		rules1 := AllowAllPolicy().Build()
		rules2 := AllowAllPolicy().Build()

		AssertNormalizedPolicyRulesEqual(t, rules1, rules2)
	})

	t.Run("audit record fixture equivalence", func(t *testing.T) {
		record1 := GrantedAuditRecord().Build()
		record2 := GrantedAuditRecord().Build()

		AssertNormalizedAuditRecordsEqual(t, []policy.AuditRecord{record1}, []policy.AuditRecord{record2})
	})

	t.Run("envelope fixture equivalence", func(t *testing.T) {
		env1 := MinimalEnvelope().Build()
		env2 := MinimalEnvelope().Build()

		if env1 == nil || env2 == nil {
			t.Fatal("minimal envelope should not be nil")
		}
		if !reflect.DeepEqual(env1.WorkingDataSnapshot(), env2.WorkingDataSnapshot()) {
			t.Fatalf("envelope working data mismatch: %#v vs %#v", env1.WorkingDataSnapshot(), env2.WorkingDataSnapshot())
		}
		if !reflect.DeepEqual(env1.WorkingMemoryKeys(), env2.WorkingMemoryKeys()) {
			t.Fatalf("envelope working keys mismatch: %#v vs %#v", env1.WorkingMemoryKeys(), env2.WorkingMemoryKeys())
		}
	})
}

// TestWorkspaceIsolation validates that one test's workspace cannot observe
// artifacts from another test.
func TestWorkspaceIsolation(t *testing.T) {
	t.Run("isolated temp directories", func(t *testing.T) {
		env1 := NewTestEnvironment(t)
		env2 := NewTestEnvironment(t)

		file1 := filepath.Join(env1.WorkspacePath, "isolation_test.txt")
		content1 := "env1 content"
		fullPath1 := file1
		if err := os.WriteFile(fullPath1, []byte(content1), 0o644); err != nil {
			t.Fatalf("failed to write file in env1: %v", err)
		}

		fullPath2 := filepath.Join(env2.WorkspacePath, "isolation_test.txt")
		if _, err := os.Stat(fullPath2); err == nil {
			t.Error("env2 should not see file from env1")
		}

		if _, err := os.Stat(fullPath1); err != nil {
			t.Errorf("env1's file should exist: %v", err)
		}
		if got := NormalizePaths(collectFiles(t, env1.WorkspacePath)); !reflect.DeepEqual(got, []string{"isolation_test.txt"}) {
			t.Fatalf("unexpected env1 file set: %#v", got)
		}
		if got := NormalizePaths(collectFiles(t, env2.WorkspacePath)); len(got) != 0 {
			t.Fatalf("unexpected env2 file set: %#v", got)
		}
	})

	t.Run("isolated telemetry sinks", func(t *testing.T) {
		env1 := NewTestEnvironment(t)
		env2 := NewTestEnvironment(t)

		// Emit an event in env1
		env1.TelemetrySink.Emit(telemetry.Event{Type: telemetry.EventNodeFinish, TaskID: "env1-task", Message: "env1 message"})

		// Emit an event in env2
		env2.TelemetrySink.Emit(telemetry.Event{Type: telemetry.EventNodeFinish, TaskID: "env2-task", Message: "env2 message"})

		// Verify env1 only has its own event
		events1 := env1.TelemetrySink.Events()
		AssertNormalizedTelemetryEventsEqual(t, events1, []telemetry.Event{{Type: telemetry.EventNodeFinish, TaskID: "env1-task", Message: "env1 message"}})

		// Verify env2 only has its own event
		events2 := env2.TelemetrySink.Events()
		AssertNormalizedTelemetryEventsEqual(t, events2, []telemetry.Event{{Type: telemetry.EventNodeFinish, TaskID: "env2-task", Message: "env2 message"}})
	})

	t.Run("isolated audit sinks", func(t *testing.T) {
		env1 := NewTestEnvironment(t)
		env2 := NewTestEnvironment(t)

		// Log a record in env1
		env1.AuditSink.Log(context.Background(), policy.AuditRecord{
			AgentID: "env1-agent",
			Action:  "env1-action",
		})

		// Log a record in env2
		env2.AuditSink.Log(context.Background(), policy.AuditRecord{
			AgentID: "env2-agent",
			Action:  "env2-action",
		})

		// Verify env1 only has its own record
		records1 := env1.AuditSink.Records()
		AssertNormalizedAuditRecordsEqual(t, records1, []policy.AuditRecord{{AgentID: "env1-agent", Action: "env1-action"}})

		// Verify env2 only has its own record
		records2 := env2.AuditSink.Records()
		AssertNormalizedAuditRecordsEqual(t, records2, []policy.AuditRecord{{AgentID: "env2-agent", Action: "env2-action"}})
	})
}

// TestOrderingStability validates that normalization helpers produce
// deterministic ordering regardless of input order.
func TestOrderingStability(t *testing.T) {
	t.Run("string sorting stability", func(t *testing.T) {
		input1 := []string{"zebra", "apple", "banana", "cherry"}
		input2 := []string{"banana", "zebra", "apple", "cherry"}
		input3 := []string{"cherry", "banana", "apple", "zebra"}

		sorted1 := SortStrings(input1)
		sorted2 := SortStrings(input2)
		sorted3 := SortStrings(input3)

		if len(sorted1) != len(sorted2) || len(sorted1) != len(sorted3) {
			t.Error("sorted arrays have different lengths")
		}

		for i := range sorted1 {
			if sorted1[i] != sorted2[i] || sorted1[i] != sorted3[i] {
				t.Errorf("ordering instability at index %d", i)
			}
		}
	})

	t.Run("path normalization stability", func(t *testing.T) {
		inputs := []string{
			"./test/file.txt",
			"./test/./file.txt",
			"test/../test/file.txt",
			"test/file.txt",
		}

		// All should normalize to the same result
		expected := "test/file.txt"
		for _, input := range inputs {
			result := NormalizePath(input)
			if result != expected {
				t.Errorf("NormalizePath(%q) = %q, want %q", input, result, expected)
			}
		}
	})

	t.Run("filesystem permission sorting stability", func(t *testing.T) {
		perms1 := []permissions.FileSystemPermission{
			{Action: permissions.FileSystemWrite, Path: "/a"},
			{Action: permissions.FileSystemRead, Path: "/b"},
			{Action: permissions.FileSystemList, Path: "/c"},
		}
		perms2 := []permissions.FileSystemPermission{
			{Action: permissions.FileSystemList, Path: "/c"},
			{Action: permissions.FileSystemWrite, Path: "/a"},
			{Action: permissions.FileSystemRead, Path: "/b"},
		}

		normalized1 := NormalizeFileSystemPermissions(perms1)
		normalized2 := NormalizeFileSystemPermissions(perms2)

		AssertNormalizedFileSystemPermissionsEqual(t, normalized1, normalized2)
	})

	t.Run("network permission sorting stability", func(t *testing.T) {
		perms1 := []permissions.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
			{Direction: "ingress", Protocol: "udp", Host: "test.com", Port: 80},
			{Direction: "egress", Protocol: "tcp", Host: "api.com", Port: 8080},
		}
		perms2 := []permissions.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "api.com", Port: 8080},
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
			{Direction: "ingress", Protocol: "udp", Host: "test.com", Port: 80},
		}

		normalized1 := NormalizeNetworkPermissions(perms1)
		normalized2 := NormalizeNetworkPermissions(perms2)

		AssertNormalizedNetworkPermissionsEqual(t, normalized1, normalized2)
	})

	t.Run("policy rule sorting stability", func(t *testing.T) {
		rules1 := []policy.PolicyRule{
			{ID: "rule-3", Effect: policy.PolicyEffect{Action: "allow"}},
			{ID: "rule-1", Effect: policy.PolicyEffect{Action: "deny"}},
			{ID: "rule-2", Effect: policy.PolicyEffect{Action: "allow"}},
		}
		rules2 := []policy.PolicyRule{
			{ID: "rule-2", Effect: policy.PolicyEffect{Action: "allow"}},
			{ID: "rule-3", Effect: policy.PolicyEffect{Action: "allow"}},
			{ID: "rule-1", Effect: policy.PolicyEffect{Action: "deny"}},
		}

		normalized1 := NormalizePolicyRules(rules1)
		normalized2 := NormalizePolicyRules(rules2)

		AssertNormalizedPolicyRulesEqual(t, normalized1, normalized2)
	})

	t.Run("chunk ID sorting stability", func(t *testing.T) {
		ids1 := []string{"chunk-3", "chunk-1", "chunk-2"}
		ids2 := []string{"chunk-2", "chunk-3", "chunk-1"}
		ids3 := []string{"chunk-1", "chunk-2", "chunk-3"}

		normalized1 := NormalizeChunkIDs(ids1)
		normalized2 := NormalizeChunkIDs(ids2)
		normalized3 := NormalizeChunkIDs(ids3)

		if !reflect.DeepEqual(normalized1, normalized2) || !reflect.DeepEqual(normalized1, normalized3) {
			t.Fatalf("chunk normalization mismatch:\n %#v\n %#v\n %#v", normalized1, normalized2, normalized3)
		}
	})
}

// TestNormalizationHelpersContract validates that normalization helpers
// handle edge cases correctly and maintain the determinism contract.
func TestNormalizationHelpersContract(t *testing.T) {
	t.Run("empty inputs return nil", func(t *testing.T) {
		if NormalizeTelemetryEvents(nil) != nil {
			t.Error("nil events should return nil")
		}
		if NormalizeTelemetryEvents([]telemetry.Event{}) != nil {
			t.Error("empty events should return nil")
		}
		if NormalizeAuditRecords(nil) != nil {
			t.Error("nil records should return nil")
		}
		if NormalizeAuditRecords([]policy.AuditRecord{}) != nil {
			t.Error("empty records should return nil")
		}
		if NormalizeFileSystemPermissions(nil) != nil {
			t.Error("nil permissions should return nil")
		}
		if NormalizeFileSystemPermissions([]permissions.FileSystemPermission{}) != nil {
			t.Error("empty permissions should return nil")
		}
		if NormalizeNetworkPermissions(nil) != nil {
			t.Error("nil permissions should return nil")
		}
		if NormalizeNetworkPermissions([]permissions.NetworkPermission{}) != nil {
			t.Error("empty permissions should return nil")
		}
		if NormalizePolicyRules(nil) != nil {
			t.Error("nil rules should return nil")
		}
		if NormalizePolicyRules([]policy.PolicyRule{}) != nil {
			t.Error("empty rules should return nil")
		}
		if NormalizeChunkIDs(nil) != nil {
			t.Error("nil chunk IDs should return nil")
		}
		if NormalizeChunkIDs([]string{}) != nil {
			t.Error("empty chunk IDs should return nil")
		}
	})

	t.Run("helpers do not modify original input", func(t *testing.T) {
		original := []string{"zebra", "apple", "banana"}
		originalCopy := make([]string, len(original))
		copy(originalCopy, original)

		_ = SortStrings(original)

		// Verify original was not modified
		for i := range original {
			if original[i] != originalCopy[i] {
				t.Errorf("original array was modified at index %d", i)
			}
		}
	})
}
