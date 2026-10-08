package sandbox

import (
	"context"
	"errors"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
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
func (a *AuthorizedRunner) emitOutcome(ctx context.Context, req CommandRequest, res *ports.CommandResult, err error, started time.Time) {
	if a == nil || a.telemetry == nil {
		return
	}
	command := strings.Join(req.Args, " ")
	base := map[string]any{
		"command":     command,
		"args":        append([]string(nil), req.Args...),
		"duration_ms": time.Since(started).Milliseconds(),
	}

	var denied *ExecutionDeniedError
	switch {
	case errors.As(err, &denied):
		metadata := cloneAnyMap(base)
		metadata["rule"] = denied.Policy
		metadata["reason"] = denied.Reason
		emitCommandEvent(ctx, a.telemetry, telemetry.EventSandboxCommandDenied, "sandbox command denied", metadata)
	case err != nil:
		metadata := cloneAnyMap(base)
		metadata["error"] = err.Error()
		emitCommandEvent(ctx, a.telemetry, telemetry.EventSandboxFailure, "sandbox execution failed", metadata)
	default:
		metadata := cloneAnyMap(base)
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

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
