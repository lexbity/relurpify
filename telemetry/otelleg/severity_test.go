package otelleg

import (
	"testing"

	"codeburg.org/lexbit/relurpify/telemetry"
	"go.opentelemetry.io/otel/log"
)

func TestSeverityOfTable(t *testing.T) {
	tests := []struct {
		name string
		et   telemetry.EventType
		want log.Severity
	}{
		{"failure suffix", telemetry.EventNodeError, log.SeverityError1},
		{"failed suffix", telemetry.EventJobFailed, log.SeverityError1},
		{"failure word", telemetry.EventSandboxFailure, log.SeverityError1},
		{"denied", telemetry.EventSandboxCommandDenied, log.SeverityError1},
		{"rejected", telemetry.EventTaskRejected, log.SeverityError1},
		{"abort", telemetry.EventInferenceAbort, log.SeverityError1},
		{"timeout", telemetry.EventInferenceTimeout, log.SeverityError1},
		{"doom", telemetry.EventDoomLoopDetected, log.SeverityError1},
		{"escape", telemetry.EventSandboxProtectedPathEscaped, log.SeverityError1},
		{"ceiling", telemetry.EventSandboxOutputCeilingExceeded, log.SeverityError1},
		{"degraded", telemetry.EventBootDegraded, log.SeverityError1},
		{"expired", telemetry.EventHITLExpired, log.SeverityError1},
		{"budget ceiling override", telemetry.EventCompilerBudgetExceeded, log.SeverityError1},
		{"skipped", telemetry.EventSchedulerJobSkipped, log.SeverityWarn1},
		{"staled", telemetry.EventChunkStaled, log.SeverityWarn1},
		{"dropped", telemetry.EventEventDropped, log.SeverityWarn1},
		{"quarantined", telemetry.EventCaptureQuarantined, log.SeverityWarn1},
		{"capped", telemetry.EventReflectionRevisionCapped, log.SeverityWarn1},
		{"malformed", telemetry.EventSpoolMalformed, log.SeverityWarn1},
		{"contented", telemetry.EventRunnerLockContented, log.SeverityWarn1},
		{"downgraded", telemetry.EventCaptureEpistemicsDowngraded, log.SeverityWarn1},
		{"abandoned", telemetry.EventShutdownAbandoned, log.SeverityWarn1},
		{"unwired", telemetry.EventGroundingPortsUnwired, log.SeverityWarn1},
		{"ignored", telemetry.EventJobDuplicateIgnored, log.SeverityWarn1},
		{"warning", telemetry.EventCompilerWarning, log.SeverityWarn1},
		{"lifecycle info", telemetry.EventGraphStart, log.SeverityInfo1},
		{"tool edited info", telemetry.EventToolEdited, log.SeverityInfo1},
		{"dynamic spelling defaults to info", telemetry.EventType("command.mystery"), log.SeverityInfo1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SeverityOf(tc.et); got != tc.want {
				t.Fatalf("SeverityOf(%q) = %v, want %v", tc.et, got, tc.want)
			}
		})
	}
}

// TestSeverityExhaustive proves every declared event type resolves explicitly
// through the curated table (FR-5, NFR-9). A type that only reaches the Info
// default fails.
func TestSeverityExhaustive(t *testing.T) {
	values := declaredEventTypeValues(t)
	if len(values) < 100 {
		t.Fatalf("taxonomy parse found only %d event types; expected the full set", len(values))
	}
	t.Logf("severity exhaustiveness: %d declared event types", len(values))
	for _, value := range values {
		if _, ok := severityRuleFor(telemetry.EventType(value)); !ok {
			t.Errorf("event type %q has no explicit severity; add an exact entry or a suffix rule", value)
		}
	}
	// The ok signal must be meaningful: an undeclared spelling does not resolve.
	if _, ok := severityRuleFor(telemetry.EventType("command.mystery")); ok {
		t.Fatal("an undeclared spelling must not resolve explicitly")
	}
}
