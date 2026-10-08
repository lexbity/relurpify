package agenttest

import (
	"sort"
	"strings"
	"time"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// applyRecordedTelemetry fills the measurement and evidence fields of a
// CaseReport from the events captured by the recording sink. It is the bridge
// that turns the harness from a noop emitter into a real telemetry consumer
// (FR-8).
//
// Only fields that have no authoritative source elsewhere are overwritten:
// ToolCalls, TokenUsage, ToolLatencies, TotalToolTimeMs, PhaseMetrics,
// ChangedFiles, and the raw SecurityObservations derived from telemetry.
// Callers that already recorded stronger evidence (for example an executor
// report loaded from disk) remain the source of truth for the rest.
func applyRecordedTelemetry(report *CaseReport, events []telemetry.Event) {
	if report == nil || len(events) == 0 {
		return
	}
	_, byTool := CountToolCalls(events)
	if len(byTool) > 0 {
		report.ToolCalls = byTool
	}
	if usage := CountTokenUsage(events); usage.LLMCalls > 0 || usage.TotalTokens > 0 {
		report.TokenUsage = usage
	}
	if transcript := BuildToolTranscript(events); transcript != nil {
		if latency := BuildLatencyReport(transcript); latency != nil {
			report.ToolLatencies = latency.ToolLatencies
			report.TotalToolTimeMs = latency.TotalToolTimeMs
		}
	}
	if phases := extractPhaseMetrics(events); len(phases) > 0 {
		report.PhaseMetrics = phases
	}
	if changed := changedFilesFromEvents(events); len(changed) > 0 {
		report.ChangedFiles = changed
	}
	if observed := securityObservationsFromEvents(events); len(observed) > 0 {
		report.SecurityObservations = append(report.SecurityObservations, observed...)
	}
}

// extractPhaseMetrics attributes events to a running phase label and aggregates
// per-phase duration, LLM call count, and token usage. The phase label is taken
// from the event itself when present ("phase", "paradigm", or
// "execution_paradigm"); otherwise events are attributed to the most recently
// seen phase, which is how euclo.step.started/euclo.step.completed bracket the
// work of a thought recipe step.
func extractPhaseMetrics(events []telemetry.Event) []PhaseMetric {
	type accumulator struct {
		first    time.Time
		last     time.Time
		llmCalls int
		tokens   int
	}
	accumulators := make(map[string]*accumulator)
	var order []string
	current := ""

	touch := func(phase string) *accumulator {
		acc, ok := accumulators[phase]
		if !ok {
			acc = &accumulator{}
			accumulators[phase] = acc
			order = append(order, phase)
		}
		return acc
	}

	for _, ev := range events {
		if label := phaseLabelFromEvent(ev); label != "" {
			current = label
		}
		if current == "" {
			continue
		}
		acc := touch(current)
		if !ev.Timestamp.IsZero() {
			if acc.first.IsZero() || ev.Timestamp.Before(acc.first) {
				acc.first = ev.Timestamp
			}
			if acc.last.IsZero() || ev.Timestamp.After(acc.last) {
				acc.last = ev.Timestamp
			}
		}
		if ev.Type == telemetry.EventLLMResponse {
			acc.llmCalls++
			if _, _, total, ok := tokenUsageFromRaw(ev.Metadata["usage"]); ok {
				acc.tokens += total
			}
		}
	}

	if len(order) == 0 {
		return nil
	}
	sort.Strings(order)
	metrics := make([]PhaseMetric, 0, len(order))
	for _, phase := range order {
		acc := accumulators[phase]
		var duration int64
		if !acc.first.IsZero() && !acc.last.IsZero() {
			duration = acc.last.Sub(acc.first).Milliseconds()
		}
		metrics = append(metrics, PhaseMetric{
			Phase:      phase,
			DurationMS: duration,
			LLMCalls:   acc.llmCalls,
			TokensUsed: acc.tokens,
		})
	}
	return metrics
}

func phaseLabelFromEvent(ev telemetry.Event) string {
	for _, key := range []string{"phase", "paradigm", "execution_paradigm"} {
		if value, ok := ev.Metadata[key].(string); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// changedFilesFromEvents collects the distinct paths of write-class edits
// reported by the capability registry (EventToolEdited).
func changedFilesFromEvents(events []telemetry.Event) []string {
	seen := map[string]struct{}{}
	var files []string
	for _, ev := range events {
		if ev.Type != telemetry.EventToolEdited {
			continue
		}
		path, _ := ev.Metadata["path"].(string)
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

// securityObservationsFromEvents derives raw security evidence from telemetry:
// successful write-class edits, capability/tool denials folded into tool
// results, and explicit capability security events.
func securityObservationsFromEvents(events []telemetry.Event) []SecurityObservation {
	var observations []SecurityObservation
	for _, ev := range events {
		switch ev.Type {
		case telemetry.EventToolEdited:
			path, _ := ev.Metadata["path"].(string)
			origin, _ := ev.Metadata["origin"].(string)
			observations = append(observations, SecurityObservation{
				Kind:      stringValue(origin),
				Resource:  strings.TrimSpace(path),
				Action:    "write",
				InScope:   true,
				Timestamp: eventTimestamp(ev),
				AgentID:   stringValue(ev.Metadata["agent_id"]),
			})
		case telemetry.EventToolResult, telemetry.EventCapabilityResult:
			tool := firstNonEmpty(stringValue(ev.Metadata["tool"]), stringValue(ev.Metadata["capability"]))
			errText := firstNonEmpty(stringValue(ev.Metadata["tool_error"]), stringValue(ev.Metadata["error"]), stringValue(ev.Metadata["capability_error"]))
			if tool == "" || errText == "" || !looksLikeDenial(errText) {
				continue
			}
			observations = append(observations, SecurityObservation{
				Kind:       securityKindForTool(tool),
				Resource:   tool,
				Action:     securityActionForTool(tool),
				InScope:    false,
				Blocked:    true,
				Timestamp:  eventTimestamp(ev),
				AgentID:    stringValue(ev.Metadata["agent_id"]),
				PolicyRule: "capability_policy",
			})
		case telemetry.EventSandboxCommandDenied:
			command := firstNonEmpty(stringValue(ev.Metadata["command"]), "command")
			observations = append(observations, SecurityObservation{
				Kind:       securityKindForTool(command),
				Resource:   command,
				Action:     "execute",
				InScope:    false,
				Blocked:    true,
				Timestamp:  eventTimestamp(ev),
				AgentID:    stringValue(ev.Metadata["agent_id"]),
				PolicyRule: firstNonEmpty(stringValue(ev.Metadata["rule"]), stringValue(ev.Metadata["reason"])),
			})
		case telemetry.EventStateChange:
			if !hasSecurityEvent(ev) {
				continue
			}
			resource := firstNonEmpty(stringValue(ev.Metadata["capability_id"]), stringValue(ev.Metadata["tool"]))
			observations = append(observations, SecurityObservation{
				Kind:       stringValue(ev.Metadata["security_event"]),
				Resource:   resource,
				Action:     "evaluate",
				InScope:    false,
				Blocked:    true,
				Timestamp:  eventTimestamp(ev),
				AgentID:    stringValue(ev.Metadata["agent_id"]),
				PolicyRule: firstNonEmpty(stringValue(ev.Metadata["reason"]), stringValue(ev.Metadata["security_event"])),
			})
		}
	}
	return observations
}

func hasSecurityEvent(ev telemetry.Event) bool {
	_, ok := ev.Metadata["security_event"].(string)
	return ok
}

func looksLikeDenial(message string) bool {
	lower := strings.ToLower(message)
	for _, needle := range []string{"blocked", "denied", "deny", "not permitted", "permission denied", "outside scope", "unauthorized"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func eventTimestamp(ev telemetry.Event) string {
	if ev.Timestamp.IsZero() {
		return ""
	}
	return ev.Timestamp.UTC().Format(time.RFC3339Nano)
}

// securityKindForTool maps a capability/tool identifier to an OSB observation
// kind. It is deliberately permissive: unknown tools keep their own name so the
// observation remains actionable.
func securityKindForTool(tool string) string {
	lower := strings.ToLower(tool)
	switch {
	case containsAny(lower, "write", "edit", "patch", "save", "create", "delete", "remove", "move", "rename", "mkdir", "chmod"):
		return "file_write"
	case containsAny(lower, "read", "search", "glob", "list", "inspect", "cat", "view"):
		return "read"
	case containsAny(lower, "exec", "shell", "bash", "command", "subprocess", "run", "test", "build", "compile"):
		return "exec"
	case containsAny(lower, "network", "http", "https", "fetch", "curl", "socket", "connect"):
		return "network"
	default:
		return tool
	}
}

func securityActionForTool(tool string) string {
	switch securityKindForTool(tool) {
	case "file_write":
		return "write"
	case "read":
		return "read"
	case "exec":
		return "execute"
	case "network":
		return "connect"
	default:
		return "call"
	}
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
