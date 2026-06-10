package framework

import (
	"reflect"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// AssertAuditRecordExists verifies that at least one audit record matching the given filter exists.
func AssertAuditRecordExists(t *testing.T, env *TestEnvironment, filter policy.AuditQuery) {
	t.Helper()

	records := env.AuditSink.Records()
	if len(records) == 0 {
		t.Error("expected audit records to exist, but found none")
		return
	}

	matched := false
	for _, record := range records {
		if matchesAuditFilter(record, filter) {
			matched = true
			break
		}
	}

	if !matched {
		t.Errorf("expected to find audit record matching filter, but found none in %d records", len(records))
	}
}

// AssertNormalizedAuditRecordsEqual verifies that the provided audit records match
// after canonical normalization.
func AssertNormalizedAuditRecordsEqual(t *testing.T, got, want []policy.AuditRecord) {
	t.Helper()

	normalizedGot := NormalizeAuditRecords(got)
	normalizedWant := NormalizeAuditRecords(want)
	if !reflect.DeepEqual(normalizedGot, normalizedWant) {
		t.Fatalf("normalized audit records mismatch:\n got: %#v\nwant: %#v", normalizedGot, normalizedWant)
	}
}

// AssertNormalizedTelemetryEventsEqual verifies that the provided telemetry
// events match after canonical normalization.
func AssertNormalizedTelemetryEventsEqual(t *testing.T, got, want []telemetry.Event) {
	t.Helper()

	normalizedGot := NormalizeTelemetryEvents(got)
	normalizedWant := NormalizeTelemetryEvents(want)
	if !reflect.DeepEqual(normalizedGot, normalizedWant) {
		t.Fatalf("normalized telemetry events mismatch:\n got: %#v\nwant: %#v", normalizedGot, normalizedWant)
	}
}

// AssertNormalizedFileSystemPermissionsEqual verifies that filesystem permissions
// match after canonical normalization.
func AssertNormalizedFileSystemPermissionsEqual(t *testing.T, got, want []permissions.FileSystemPermission) {
	t.Helper()

	normalizedGot := NormalizeFileSystemPermissions(got)
	normalizedWant := NormalizeFileSystemPermissions(want)
	if !reflect.DeepEqual(normalizedGot, normalizedWant) {
		t.Fatalf("normalized filesystem permissions mismatch:\n got: %#v\nwant: %#v", normalizedGot, normalizedWant)
	}
}

// AssertNormalizedNetworkPermissionsEqual verifies that network permissions
// match after canonical normalization.
func AssertNormalizedNetworkPermissionsEqual(t *testing.T, got, want []permissions.NetworkPermission) {
	t.Helper()

	normalizedGot := NormalizeNetworkPermissions(got)
	normalizedWant := NormalizeNetworkPermissions(want)
	if !reflect.DeepEqual(normalizedGot, normalizedWant) {
		t.Fatalf("normalized network permissions mismatch:\n got: %#v\nwant: %#v", normalizedGot, normalizedWant)
	}
}

// AssertNormalizedPolicyRulesEqual verifies that policy rules match after
// canonical normalization.
func AssertNormalizedPolicyRulesEqual(t *testing.T, got, want []policy.PolicyRule) {
	t.Helper()

	normalizedGot := NormalizePolicyRules(got)
	normalizedWant := NormalizePolicyRules(want)
	if !reflect.DeepEqual(normalizedGot, normalizedWant) {
		t.Fatalf("normalized policy rules mismatch:\n got: %#v\nwant: %#v", normalizedGot, normalizedWant)
	}
}

// AssertAuditRecordCount verifies that the exact number of audit records matching the filter exist.
func AssertAuditRecordCount(t *testing.T, env *TestEnvironment, filter policy.AuditQuery, expectedCount int) {
	t.Helper()

	records := env.AuditSink.Records()
	count := 0
	for _, record := range records {
		if matchesAuditFilter(record, filter) {
			count++
		}
	}

	if count != expectedCount {
		t.Errorf("expected %d audit records matching filter, got %d", expectedCount, count)
	}
}

// AssertTelemetryEventExists verifies that at least one telemetry event of the given type exists.
func AssertTelemetryEventExists(t *testing.T, env *TestEnvironment, eventType telemetry.EventType) {
	t.Helper()

	events := env.TelemetrySink.Events()
	if len(events) == 0 {
		t.Error("expected telemetry events to exist, but found none")
		return
	}

	found := false
	for _, event := range events {
		if event.Type == eventType {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected to find telemetry event of type %s, but found none in %d events", eventType, len(events))
	}
}

// AssertTelemetryEventCount verifies that the exact number of telemetry events of the given type exist.
func AssertTelemetryEventCount(t *testing.T, env *TestEnvironment, eventType telemetry.EventType, expectedCount int) {
	t.Helper()

	events := env.TelemetrySink.Events()
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}

	if count != expectedCount {
		t.Errorf("expected %d telemetry events of type %s, got %d", expectedCount, eventType, count)
	}
}

// AssertTelemetryEventMetadata verifies that a telemetry event has specific metadata.
func AssertTelemetryEventMetadata(t *testing.T, env *TestEnvironment, eventType telemetry.EventType, key string, expectedValue any) {
	t.Helper()

	events := env.TelemetrySink.Events()
	found := false
	for _, event := range events {
		if event.Type == eventType {
			if event.Metadata != nil {
				if value, ok := event.Metadata[key]; ok && value == expectedValue {
					found = true
					break
				}
			}
		}
	}

	if !found {
		t.Errorf("expected to find telemetry event of type %s with metadata[%s]=%v", eventType, key, expectedValue)
	}
}

// AssertPermissionGranted verifies that a permission was granted in the audit records.
func AssertPermissionGranted(t *testing.T, env *TestEnvironment, permissionType, resource string) {
	t.Helper()

	filter := policy.AuditQuery{
		Type:       permissionType,
		Permission: resource,
		Result:     "granted",
	}

	AssertAuditRecordExists(t, env, filter)
}

// AssertPermissionDenied verifies that a permission was denied in the audit records.
func AssertPermissionDenied(t *testing.T, env *TestEnvironment, permissionType, resource string) {
	t.Helper()

	filter := policy.AuditQuery{
		Type:       permissionType,
		Permission: resource,
		Result:     "denied",
	}

	AssertAuditRecordExists(t, env, filter)
}

// matchesAuditFilter checks if an audit record matches the given filter.
func matchesAuditFilter(record policy.AuditRecord, filter policy.AuditQuery) bool {
	if filter.AgentID != "" && record.AgentID != filter.AgentID {
		return false
	}
	if filter.Type != "" && record.Type != filter.Type {
		return false
	}
	if filter.Action != "" && record.Action != filter.Action {
		return false
	}
	if filter.Permission != "" && record.Permission != filter.Permission {
		return false
	}
	if filter.Result != "" && record.Result != filter.Result {
		return false
	}
	if !filter.TimeStart.IsZero() && record.Timestamp.Before(filter.TimeStart) {
		return false
	}
	if !filter.TimeEnd.IsZero() && record.Timestamp.After(filter.TimeEnd) {
		return false
	}
	return true
}

// AssertEventOrder verifies that events occurred in the expected order.
func AssertEventOrder(t *testing.T, env *TestEnvironment, expectedTypes []telemetry.EventType) {
	t.Helper()

	events := env.TelemetrySink.Events()
	if len(events) < len(expectedTypes) {
		t.Fatalf("expected at least %d events, got %d", len(expectedTypes), len(events))
	}

	for i, expectedType := range expectedTypes {
		if events[i].Type != expectedType {
			t.Errorf("event %d: expected type %s, got %s", i, expectedType, events[i].Type)
		}
	}
}

// AssertNoAuditRecords verifies that no audit records exist matching the filter.
func AssertNoAuditRecords(t *testing.T, env *TestEnvironment, filter policy.AuditQuery) {
	t.Helper()

	records := env.AuditSink.Records()
	for _, record := range records {
		if matchesAuditFilter(record, filter) {
			t.Errorf("expected no audit records matching filter, but found one: %+v", record)
			return
		}
	}
}

// AssertNoTelemetryEvents verifies that no telemetry events of the given type exist.
func AssertNoTelemetryEvents(t *testing.T, env *TestEnvironment, eventType telemetry.EventType) {
	t.Helper()

	events := env.TelemetrySink.Events()
	for _, event := range events {
		if event.Type == eventType {
			t.Errorf("expected no telemetry events of type %s, but found one: %+v", eventType, event)
			return
		}
	}
}

// AssertEventWithinTimeRange verifies that an event occurred within the specified time range.
func AssertEventWithinTimeRange(t *testing.T, env *TestEnvironment, eventType telemetry.EventType, start, end time.Time) {
	t.Helper()

	events := env.TelemetrySink.Events()
	found := false
	for _, event := range events {
		if event.Type == eventType {
			if event.Timestamp.After(start) && event.Timestamp.Before(end) {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("expected to find telemetry event of type %s between %s and %s", eventType, start, end)
	}
}

// LogAuditRecords logs all audit records for debugging purposes.
func LogAuditRecords(t *testing.T, env *TestEnvironment) {
	t.Helper()

	records := env.AuditSink.Records()
	t.Logf("=== Audit Records (%d) ===", len(records))
	for i, record := range records {
		t.Logf("[%d] Type: %s, Action: %s, Permission: %s, Result: %s, Agent: %s, Timestamp: %s",
			i, record.Type, record.Action, record.Permission, record.Result, record.AgentID, record.Timestamp.Format(time.RFC3339))
	}
}

// LogTelemetryEvents logs all telemetry events for debugging purposes.
func LogTelemetryEvents(t *testing.T, env *TestEnvironment) {
	t.Helper()

	events := env.TelemetrySink.Events()
	t.Logf("=== Telemetry Events (%d) ===", len(events))
	for i, event := range events {
		t.Logf("[%d] Type: %s, NodeID: %s, TaskID: %s, Message: %s, Timestamp: %s",
			i, event.Type, event.NodeID, event.TaskID, event.Message, event.Timestamp.Format(time.RFC3339))
	}
}
