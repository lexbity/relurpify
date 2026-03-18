# Euclo Capabilities

Euclo currently exposes 18 concrete coding capabilities through the internal capability registry.

## Capability List

| Capability | ID | Profiles | Purpose |
|---|---|---|---|
| Edit-Verify-Repair | `euclo:edit_verify_repair` | `edit_verify_repair` | General-purpose code change flow with explore/edit/verify phases. |
| Reproduce-Localize-Patch | `euclo:reproduce_localize_patch` | `reproduce_localize_patch` | Debugging flow built around reproduction, localization, patching, and verification. |
| Test-Driven Generation | `euclo:test_driven_generation` | `test_driven_generation` | Test-first implementation path. |
| Planner Plan | `euclo:planner.plan` | `plan_stage_execute`, `edit_verify_repair` | Produces implementation plans. |
| Verify Change | `euclo:verify.change` | `edit_verify_repair`, `reproduce_localize_patch`, `test_driven_generation` | Produces verification artifacts. |
| Final Coding Report | `euclo:report.final_coding` | all primary profiles | Compiles final execution summaries. |
| Investigate Regression | `euclo:debug.investigate_regression` | `reproduce_localize_patch` | Blackboard-assisted regression narrowing and root-cause seeding. |
| Design Alternatives | `euclo:design.alternatives` | `plan_stage_execute`, `edit_verify_repair` | Produces plan candidates and a selected plan. |
| Execution Profile Select | `euclo:execution_profile.select` | multiple | Answers explicit “which profile/approach” questions. |
| Trace Analyze | `euclo:trace.analyze` | `trace_execute_analyze` | Produces trace and analyze artifacts from trace-oriented intake. |
| Review Findings | `euclo:review.findings` | `review_suggest_implement` | Produces structured review findings. |
| Review Compatibility | `euclo:review.compatibility` | `review_suggest_implement` | Produces compatibility assessments for API-surface changes. |
| Review Implement If Safe | `euclo:review.implement_if_safe` | `review_suggest_implement` | Runs findings first, then delegates safe fixes into edit/verify. |
| API-Compatible Refactor | `euclo:refactor.api_compatible` | `edit_verify_repair`, `plan_stage_execute` | HTN-guided refactoring with strict compatibility gating. |
| Artifact Diff Summary | `euclo:artifact.diff_summary` | multiple | Summarizes edit intent into structured diff summaries. |
| Artifact Trace To Root Cause | `euclo:artifact.trace_to_root_cause` | `trace_execute_analyze`, `reproduce_localize_patch` | Ranks root-cause candidates from trace artifacts. |
| Artifact Verification Summary | `euclo:artifact.verification_summary` | multiple | Normalizes verification artifacts into accept/investigate/reject summaries. |
| Migration Execute | `euclo:migration.execute` | `plan_stage_execute` | Structured migration execution with step checks and rollback support. |

## Profile Routing

| Profile | Primary capabilities |
|---|---|
| `edit_verify_repair` | `euclo:edit_verify_repair`, `euclo:planner.plan`, `euclo:verify.change`, `euclo:design.alternatives`, `euclo:refactor.api_compatible`, `euclo:artifact.diff_summary`, `euclo:artifact.verification_summary`, `euclo:execution_profile.select`, `euclo:report.final_coding` |
| `reproduce_localize_patch` | `euclo:reproduce_localize_patch`, `euclo:debug.investigate_regression`, `euclo:artifact.trace_to_root_cause`, `euclo:artifact.diff_summary`, `euclo:artifact.verification_summary`, `euclo:verify.change`, `euclo:report.final_coding` |
| `test_driven_generation` | `euclo:test_driven_generation`, `euclo:verify.change`, `euclo:artifact.verification_summary`, `euclo:report.final_coding` |
| `review_suggest_implement` | `euclo:review.findings`, `euclo:review.compatibility`, `euclo:review.implement_if_safe`, `euclo:artifact.diff_summary`, `euclo:artifact.verification_summary`, `euclo:report.final_coding` |
| `plan_stage_execute` | `euclo:planner.plan`, `euclo:design.alternatives`, `euclo:migration.execute`, `euclo:execution_profile.select`, `euclo:refactor.api_compatible`, `euclo:report.final_coding` |
| `trace_execute_analyze` | `euclo:trace.analyze`, `euclo:artifact.trace_to_root_cause`, `euclo:artifact.verification_summary`, `euclo:report.final_coding` |

## Artifact Registry

Key capability-produced artifacts added in phases 2 through 8:

- `euclo.regression_analysis`
- `euclo.plan_candidates`
- `euclo.profile_selection`
- `euclo.trace`
- `euclo.review_findings`
- `euclo.compatibility_assessment`
- `euclo.diff_summary`
- `euclo.root_cause_candidates`
- `euclo.verification_summary`
- `euclo.migration_plan`

## Composition Patterns

- `euclo:debug.investigate_regression` seeds artifacts consumed by `euclo:reproduce_localize_patch`.
- `euclo:review.implement_if_safe` runs `euclo:review.findings` before delegating to `euclo:edit_verify_repair`.
- `euclo:refactor.api_compatible` uses compatibility assessment logic as a hard gate around refactor steps.
- `euclo:artifact.trace_to_root_cause` consumes trace output from `euclo:trace.analyze`.
- `euclo:migration.execute` combines migration planning artifacts with deterministic pipeline stage checks.
