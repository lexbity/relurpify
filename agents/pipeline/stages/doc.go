// Package stages provides pipeline stage implementations for coding tasks,
// used by PipelineAgent and CodingStageFactory.
//
// # CodingStageFactory
//
// CodingStageFactory (factory.go) constructs the appropriate stage sequence
// for a given task kind:
//
//   - Code tasks: ExploreStage → AnalyzeStage → PlanStage → CodeStage → VerifyStage
//   - Analysis tasks: ExploreStage → VerifyStage
//
// # Stage output types
//
// Each stage produces a typed result consumed by the next stage:
//
//   - FileSelection: the set of files relevant to the task.
//   - IssueList: problems identified during analysis.
//   - FixPlan: strategy, ordered steps, and risk assessment.
//   - EditPlan: concrete file edits with a change summary.
//   - VerificationReport: test results and remaining issues after edits are applied.
package stages
