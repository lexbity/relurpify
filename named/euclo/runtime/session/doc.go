// Package session provides session indexing, selection, and resume capabilities
// for Euclo interactive workflows.
//
// The core flow is:
//  1. SessionResumeTrigger fires when a user expresses intent to resume.
//  2. SessionSelectPhase lists recent sessions and awaits selection.
//  3. SessionResumeResolver resolves the selected workflow to a full
//     SessionResumeContext (workflow ID, run ID, BKC chunk IDs, plan version,
//     phase state, and semantic summary).
//  4. applySessionResumeContext (managed_execution.go) injects the context
//     into the task and state before initializeManagedExecution runs.
//  5. ExecutorSemanticContext is seeded from the resumed semantic summary so
//     executors start warm, not cold.
//
// Key components:
//   - SessionRecord: A summarized view of a past session for selection
//   - SessionList: An ordered list of sessions for selection UI
//   - SessionIndex: Service that queries WorkflowStateStore and enriches
//     records with plan and phase information
//   - SessionResumeTrigger: AgencyTrigger that fires on "resume session" phrases
//   - SessionSelectPhase: Interactive phase that lists sessions and collects
//     user selection via AwaitResponse
//   - SessionResumeResolver: Resolves workflow ID to full resume context
//   - SessionResumeContext: Complete context needed to restore a session
//   - SessionSemanticSummary: Pre-resolved archaeology content (tensions,
//     patterns, learning interactions)
//
// The session package supports filtering by workspace, recency, and mode,
// enabling users to resume previous coding sessions with restored semantic
// context. It is wired into all interactive mode machines via
// Agent.createInteractionRegistry() using ModeMachineRegistry.WrapFactory().
package session
