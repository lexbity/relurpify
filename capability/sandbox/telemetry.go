package sandbox

import (
	"context"
	"errors"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	capruntime "codeburg.org/lexbit/relurpify/capability/runtime"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// emitOutcome reports the outcome of one command through the sandbox chain
// (FR-15). The three outcomes are mutually exclusive:
//
//   - sandbox.command_denied: enforcement blocked execution before the sandbox
//     ran it. The matched policy surface travels as "rule" (the
//     ExecutionDeniedError.Policy source, e.g. "sandbox policy") and the
//     concrete reason travels as "reason".
//   - sandbox.failure: enforcement passed but execution failed (start error,
//     container failure, etc.).
//   - sandbox.command_executed: a command result was produced, with exit code
//     and duration.
//
// Command payloads are clipped and secret-scrubbed via capability/runtime
// redaction (NFR-7): secrets embedded in arguments are replaced before the
// event reaches the sink.
func (a *AuthorizedRunner) emitOutcome(ctx context.Context, req CommandRequest, res *ports.CommandResult, err error, started time.Time) {
	if a == nil || a.telemetry == nil {
		return
	}
	command := strings.Join(req.Args, " ")
	base := map[string]any{
		"command":     clipCommand(command),
		"duration_ms": time.Since(started).Milliseconds(),
	}

	var denied *ExecutionDeniedError
	switch {
	case errors.As(err, &denied):
		metadata := capruntime.RedactMetadataMap(base)
		metadata["rule"] = denied.Policy
		metadata["reason"] = denied.Reason
		emitCommandEvent(ctx, a.telemetry, telemetry.EventSandboxCommandDenied, "sandbox command denied", metadata)
	case err != nil:
		metadata := capruntime.RedactMetadataMap(base)
		metadata["error"] = redactDetail(err.Error())
		emitCommandEvent(ctx, a.telemetry, telemetry.EventSandboxFailure, "sandbox execution failed", metadata)
	default:
		metadata := capruntime.RedactMetadataMap(base)
		metadata["exit_code"] = 0
		if res != nil {
			metadata["exit_code"] = res.ExitCode
			metadata["stdout_bytes"] = res.StdoutBytes
			metadata["stderr_bytes"] = res.StderrBytes
			metadata["timed_out"] = res.TimedOut
			metadata["oom_killed"] = res.OOMKilled
		}
		emitCommandEvent(ctx, a.telemetry, telemetry.EventSandboxCommandExecuted, "sandbox command executed", metadata)
	}
}

// clipCommand bounds the serialized command payload so a single invocation
// cannot balloon the durable record (it is also the first line of NFR-7
// defence for secrets that do not match redaction patterns).
func clipCommand(command string) string {
	const max = 512
	if len(command) <= max {
		return command
	}
	return command[:max] + "...(truncated)"
}

// redactDetail scrubs a free-form error string for secret-bearing patterns
// (NFR-7), reusing the shared capability/runtime redaction vocabulary by
// routing the value through the standard metadata scrubbing path.
func redactDetail(detail string) string {
	if detail == "" {
		return ""
	}
	scrubbed := capruntime.RedactMetadataMap(map[string]any{"detail": detail})
	value, _ := scrubbed["detail"].(string)
	return value
}

func emitCommandEvent(ctx context.Context, sink telemetry.Telemetry, eventType telemetry.EventType, message string, metadata map[string]any) {
	if sink == nil {
		return
	}
	ev := telemetry.Event{
		Type:      eventType,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	sink.Emit(ev)
}
