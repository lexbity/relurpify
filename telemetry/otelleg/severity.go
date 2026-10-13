package otelleg

import (
	"strings"

	"codeburg.org/lexbit/relurpify/telemetry"
	"go.opentelemetry.io/otel/log"
)

// This file is the curated severity table of spec D12. Rules are applied in
// order: exact match first, then the suffix rules, then Info. Every declared
// EventType constant must resolve explicitly (exact match or a rule); the
// exhaustiveness test walks the taxonomy source and fails on any constant that
// only reaches the Info default. The Info default exists solely for runtime
// event types that are not declared constants (for example the dynamic
// "command.<kind>" spelling).
//
// Rule order matters: "error"/"failed"/"failure" are checked before the broader
// "downgraded"/"degraded" classes so e.g. a failure is never softened.

// severityRule is one suffix rule: an event type whose string value contains
// substr resolves to severity.
type severityRule struct {
	substr   string
	severity log.Severity
}

// severityRules are the D12 suffix rules, in evaluation order.
var severityRules = []severityRule{ //nolint:gochecknoglobals // immutable severity-rule table
	// Error: explicit failure.
	{substr: "error", severity: log.SeverityError1},
	{substr: "failed", severity: log.SeverityError1},
	{substr: "failure", severity: log.SeverityError1},
	// Error: hard refusals and resource ceilings.
	{substr: "denied", severity: log.SeverityError1},
	{substr: "rejected", severity: log.SeverityError1},
	{substr: "abort", severity: log.SeverityError1},
	{substr: "timeout", severity: log.SeverityError1},
	{substr: "doom", severity: log.SeverityError1},
	{substr: "escape", severity: log.SeverityError1},
	{substr: "ceiling", severity: log.SeverityError1},
	// Error: degraded/expired conditions.
	{substr: "degraded", severity: log.SeverityError1},
	{substr: "expired", severity: log.SeverityError1},
	// Warn: recoverable or discarded.
	{substr: "skipped", severity: log.SeverityWarn1},
	{substr: "staled", severity: log.SeverityWarn1},
	{substr: "stale", severity: log.SeverityWarn1},
	{substr: "dropped", severity: log.SeverityWarn1},
	{substr: "quarantined", severity: log.SeverityWarn1},
	{substr: "capped", severity: log.SeverityWarn1},
	{substr: "malformed", severity: log.SeverityWarn1},
	{substr: "contented", severity: log.SeverityWarn1},
	{substr: "downgraded", severity: log.SeverityWarn1},
	{substr: "abandoned", severity: log.SeverityWarn1},
	{substr: "unwired", severity: log.SeverityWarn1},
	{substr: "ignored", severity: log.SeverityWarn1},
	// Warn: explicitly labelled warnings.
	{substr: "warning", severity: log.SeverityWarn1},
}

// severityExact pins the severities of constants that no suffix rule resolves,
// plus deliberate overrides. Every entry here is required for exhaustiveness.
var severityExact = map[telemetry.EventType]log.Severity{ //nolint:gochecknoglobals // immutable severity table
	// A budget ceiling is a hard limit, matching the "ceiling" rule even
	// though the value spells "exceeded".
	telemetry.EventCompilerBudgetExceeded: log.SeverityError1,

	// Informational lifecycle and domain events.
	telemetry.EventAgentFinish:                log.SeverityInfo1,
	telemetry.EventAgentStart:                 log.SeverityInfo1,
	telemetry.EventAgentTestBaselineCompared:  log.SeverityInfo1,
	telemetry.EventBackendClose:               log.SeverityInfo1,
	telemetry.EventBackendRestart:             log.SeverityInfo1,
	telemetry.EventBackendStateChange:         log.SeverityInfo1,
	telemetry.EventBackendWarm:                log.SeverityInfo1,
	telemetry.EventBlackboardSourceExecuted:   log.SeverityInfo1,
	telemetry.EventBlackboardWriteGrounded:    log.SeverityInfo1,
	telemetry.EventBootstrapComplete:          log.SeverityInfo1,
	telemetry.EventBudgetSnapshot:             log.SeverityInfo1,
	telemetry.EventCapabilityCall:             log.SeverityInfo1,
	telemetry.EventCapabilityResult:           log.SeverityInfo1,
	telemetry.EventCaptureGrounded:            log.SeverityInfo1,
	telemetry.EventChunkCommitted:             log.SeverityInfo1,
	telemetry.EventChunkInvalidated:           log.SeverityInfo1,
	telemetry.EventCommandOpaqueConstructor:   log.SeverityInfo1,
	telemetry.EventCommandWrapperUnwrapped:    log.SeverityInfo1,
	telemetry.EventCompilerCacheHit:           log.SeverityInfo1,
	telemetry.EventCompilerCacheMiss:          log.SeverityInfo1,
	telemetry.EventCompilerCompleted:          log.SeverityInfo1,
	telemetry.EventCompilerStarted:            log.SeverityInfo1,
	telemetry.EventCompilerSummarySubstituted: log.SeverityInfo1,
	telemetry.EventContextPolicyReloaded:      log.SeverityInfo1,
	telemetry.EventContextStreamInjected:      log.SeverityInfo1,
	telemetry.EventDelegationCancel:           log.SeverityInfo1,
	telemetry.EventDelegationFinish:           log.SeverityInfo1,
	telemetry.EventDelegationStart:            log.SeverityInfo1,
	telemetry.EventEpochClosed:                log.SeverityInfo1,
	telemetry.EventGraphBranchMerged:          log.SeverityInfo1,
	telemetry.EventGraphFinish:                log.SeverityInfo1,
	telemetry.EventGraphStart:                 log.SeverityInfo1,
	telemetry.EventHITLRequested:              log.SeverityInfo1,
	telemetry.EventHITLResolved:               log.SeverityInfo1,
	telemetry.EventHTNExecutionCompleted:      log.SeverityInfo1,
	telemetry.EventHTNPlanStarted:             log.SeverityInfo1,
	telemetry.EventHTNStepCompleted:           log.SeverityInfo1,
	telemetry.EventHTNStepStarted:             log.SeverityInfo1,
	telemetry.EventIntentFamilyDisambiguation: log.SeverityInfo1,
	telemetry.EventJobClaimed:                 log.SeverityInfo1,
	telemetry.EventJobCompleted:               log.SeverityInfo1,
	telemetry.EventJobInterrupted:             log.SeverityInfo1,
	telemetry.EventJobRetried:                 log.SeverityInfo1,
	telemetry.EventJobSubmitted:               log.SeverityInfo1,
	telemetry.EventLLMPrompt:                  log.SeverityInfo1,
	telemetry.EventLLMResponse:                log.SeverityInfo1,
	telemetry.EventNodeFinish:                 log.SeverityInfo1,
	telemetry.EventNodeStart:                  log.SeverityInfo1,
	telemetry.EventPlannerPhase:               log.SeverityInfo1,
	telemetry.EventPlannerPlanCompleted:       log.SeverityInfo1,
	telemetry.EventPlannerPlanStarted:         log.SeverityInfo1,
	telemetry.EventPolicyConflictShadowed:     log.SeverityInfo1,
	telemetry.EventPolicyEvaluated:            log.SeverityInfo1,
	telemetry.EventProviderSessionEnded:       log.SeverityInfo1,
	telemetry.EventReflectionCompleted:        log.SeverityInfo1,
	telemetry.EventReflectionIteration:        log.SeverityInfo1,
	telemetry.EventReflectionReviewed:         log.SeverityInfo1,
	telemetry.EventRollbackTokenStored:        log.SeverityInfo1,
	telemetry.EventRunnerStarted:              log.SeverityInfo1,
	telemetry.EventRunnerStopped:              log.SeverityInfo1,
	telemetry.EventSandboxCommandExecuted:     log.SeverityInfo1,
	telemetry.EventSandboxImagePinned:         log.SeverityInfo1,
	telemetry.EventSandboxImageUnpinned:       log.SeverityInfo1,
	telemetry.EventSandboxOrphanReaped:        log.SeverityInfo1,
	telemetry.EventSandboxProtectedPathSelf:   log.SeverityInfo1,
	telemetry.EventSchedulerJobCompleted:      log.SeverityInfo1,
	telemetry.EventSchedulerJobStarted:        log.SeverityInfo1,
	telemetry.EventSessionResetRequired:       log.SeverityInfo1,
	telemetry.EventShutdownDrain:              log.SeverityInfo1,
	telemetry.EventStateChange:                log.SeverityInfo1,
	telemetry.EventStepFallbackActivated:      log.SeverityInfo1,
	telemetry.EventSummaryCommitted:           log.SeverityInfo1,
	telemetry.EventTombstonePreserved:         log.SeverityInfo1,
	telemetry.EventToolCall:                   log.SeverityInfo1,
	telemetry.EventToolEdited:                 log.SeverityInfo1,
	telemetry.EventToolResult:                 log.SeverityInfo1,
}

// SeverityOf resolves the OTel severity for an event type. It is the single
// mapping used by the log emitter; unknown runtime spellings fall back to Info.
func SeverityOf(et telemetry.EventType) log.Severity {
	sev, _ := severityRuleFor(et)
	return sev
}

// severityRuleFor resolves an event type through the curated table. The boolean
// reports whether the type was resolved explicitly (exact match or suffix
// rule); false means the spelling is not in the declared taxonomy.
func severityRuleFor(et telemetry.EventType) (log.Severity, bool) {
	if sev, ok := severityExact[et]; ok {
		return sev, true
	}
	name := string(et)
	for _, rule := range severityRules {
		if strings.Contains(name, rule.substr) {
			return rule.severity, true
		}
	}
	return log.SeverityInfo1, false
}
