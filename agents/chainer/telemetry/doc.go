// Package telemetry provides observability and event tracking for ChainerAgent execution.
//
// # Event Types
//
// ChainerEvent represents significant execution moments:
//   - LinkStart: Link execution begins
//   - LinkFinish: Link execution completes successfully
//   - LinkError: Link execution fails
//   - ParsingFailure: LLM output parse fails
//   - RetryAttempt: Retry triggered after failure
//   - CompressionEvent: Budget compression triggered
//   - ResumeEvent: Execution resumed from checkpoint
//
// # Recording
//
// EventRecorder captures and stores events for later querying:
//   - Record(event) — emit event to telemetry system
//   - RecordedEvents(taskID, stageName) — retrieve events for task/stage
//   - AllEvents(taskID) — retrieve all events for task
//
// # Integration
//
// Events flow through the execution pipeline:
//   1. LinkStage.BuildPrompt() → LinkStart
//   2. LinkStage.Decode() → ParsingFailure (on parse error)
//   3. Recovery resumption → ResumeEvent
//   4. Compression trigger → CompressionEvent
//   5. LinkStage.Apply() → LinkFinish or LinkError
//
// EventRecorder delegates to framework/core.Telemetry if available,
// enabling integration with observability infrastructure.
package telemetry
