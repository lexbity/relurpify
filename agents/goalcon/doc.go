// Package goalcon implements deterministic backward-chaining goal planning with support
// for ambiguity detection, human clarification, goal decomposition, and automatic recovery.
//
// GoalCon is organized into subpackages:
//
//   - analysis: Goal classification, ambiguity detection, clarification, and decomposition
//   - planning: Backward-chaining solver for plan generation
//   - execution: Step execution with retry logic and failure recovery
//   - operators: Operator registry, loading, and quality metrics
//   - audit: Audit trails, provenance tracking, and compliance logging
//
// Typical workflow:
//
//   1. Classify goal using analysis.GoalClassifierLLM
//   2. Detect ambiguities with analysis.AmbiguityAnalyzer
//   3. Clarify if needed via analysis.GoalClarifier (HITL integration)
//   4. Decompose into sub-goals using analysis.GoalDecomposer
//   5. Generate plan with planning.Solver (backward chaining)
//   6. Execute with execution.RetryExecutor (automatic retries + recovery)
//   7. Track provenance with audit.CapabilityAuditTrail
//
// All types are re-exported at the package root level for backward compatibility.
// For modular imports, use the subpackages directly.
package goalcon
