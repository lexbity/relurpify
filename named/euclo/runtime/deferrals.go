package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
)

func BuildDeferredExecutionIssues(plan *guidance.DeferralPlan, uow UnitOfWork, state *core.Context, now time.Time) []DeferredExecutionIssue {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	issues := buildWaiverDeferredExecutionIssues(uow, state, now)
	if plan == nil {
		return issues
	}
	pending := plan.PendingObservations()
	if len(pending) == 0 {
		return issues
	}
	issues = append(issues, make([]DeferredExecutionIssue, 0, len(pending))...)
	for _, obs := range pending {
		issue := DeferredExecutionIssue{
			IssueID:               firstNonEmpty(strings.TrimSpace(obs.ID), fmt.Sprintf("defer-%d", now.UnixNano())),
			WorkflowID:            uow.WorkflowID,
			RunID:                 uow.RunID,
			ExecutionID:           uow.ExecutionID,
			ActivePlanID:          activePlanIDForIssue(uow),
			ActivePlanVersion:     activePlanVersionForIssue(uow),
			StepID:                activeStepIDForIssue(uow, state),
			RelatedStepIDs:        relatedStepIDsForIssue(uow, state),
			Kind:                  deferredKindFromObservation(obs),
			Severity:              deferredSeverityFromBlastRadius(obs.BlastRadius),
			Status:                DeferredIssueStatusOpen,
			Title:                 strings.TrimSpace(obs.Title),
			Summary:               strings.TrimSpace(obs.Description),
			WhyNotResolvedInline:  whyNotResolvedInline(obs),
			RecommendedReentry:    "archaeology",
			RecommendedNextAction: recommendedNextAction(obs),
			Evidence:              evidenceFromObservation(obs, state),
			ArchaeoRefs:           archaeoRefsFromObservation(obs, uow),
			CreatedAt:             nonZeroTime(obs.CreatedAt, now),
			UpdatedAt:             now,
		}
		if issue.Title == "" {
			issue.Title = strings.ReplaceAll(string(issue.Kind), "_", " ")
		}
		issues = append(issues, issue)
	}
	return issues
}

func buildWaiverDeferredExecutionIssues(uow UnitOfWork, state *core.Context, now time.Time) []DeferredExecutionIssue {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.execution_waiver")
	if !ok || raw == nil {
		return nil
	}
	waiver, ok := raw.(ExecutionWaiver)
	if !ok {
		return nil
	}
	issueID := strings.TrimSpace(waiver.WaiverID)
	if issueID == "" {
		issueID = fmt.Sprintf("waiver-%d", now.UnixNano())
	}
	title := "Operator waiver applied"
	if strings.TrimSpace(string(waiver.Kind)) != "" {
		title = "Operator waiver applied: " + strings.ReplaceAll(string(waiver.Kind), "_", " ")
	}
	summary := strings.TrimSpace(waiver.Reason)
	if summary == "" {
		summary = "Execution completed under an explicit operator waiver."
	}
	issue := DeferredExecutionIssue{
		IssueID:               issueID,
		WorkflowID:            uow.WorkflowID,
		RunID:                 firstNonEmpty(strings.TrimSpace(waiver.RunID), uow.RunID),
		ExecutionID:           uow.ExecutionID,
		ActivePlanID:          activePlanIDForIssue(uow),
		ActivePlanVersion:     activePlanVersionForIssue(uow),
		StepID:                activeStepIDForIssue(uow, state),
		RelatedStepIDs:        relatedStepIDsForIssue(uow, state),
		Kind:                  DeferredIssueWaiver,
		Severity:              DeferredIssueSeverityMedium,
		Status:                DeferredIssueStatusAcknowledged,
		Title:                 title,
		Summary:               summary,
		WhyNotResolvedInline:  "operator explicitly accepted downgraded assurance for this run",
		RecommendedReentry:    "archaeology",
		RecommendedNextAction: "revisit the waived verification or review gap in a follow-up run without the waiver",
		ArchaeoRefs:           waiverArchaeoRefs(waiver),
		Evidence: DeferredExecutionEvidence{
			RelevantProvenanceRefs: waiverProvenanceRefs(waiver),
			ShortReasoningSummary:  summary,
		},
		CreatedAt: nonZeroTime(waiver.CreatedAt, now),
		UpdatedAt: now,
	}
	return []DeferredExecutionIssue{issue}
}

func waiverArchaeoRefs(waiver ExecutionWaiver) map[string][]string {
	if strings.TrimSpace(waiver.ArchaeoRef) == "" {
		return nil
	}
	return map[string][]string{
		"waiver": {strings.TrimSpace(waiver.ArchaeoRef)},
	}
}

func waiverProvenanceRefs(waiver ExecutionWaiver) []string {
	if strings.TrimSpace(waiver.ArchaeoRef) == "" {
		return nil
	}
	return []string{strings.TrimSpace(waiver.ArchaeoRef)}
}

func PersistDeferredExecutionIssuesToWorkspace(task *core.Task, state *core.Context, issues []DeferredExecutionIssue) []DeferredExecutionIssue {
	workspace := workspacePathFromTaskState(task, state)
	if workspace == "" || len(issues) == 0 {
		return issues
	}
	dir := filepath.Join(workspace, "relurpify_cfg", "artifacts", "euclo", "deferred")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return issues
	}
	out := make([]DeferredExecutionIssue, 0, len(issues))
	for _, issue := range issues {
		filename := sanitizeFilename(firstNonEmpty(issue.IssueID, "deferred-issue")) + ".md"
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, []byte(renderDeferredIssueMarkdown(issue)), 0o644); err == nil {
			issue.WorkspaceArtifactPath = path
		}
		out = append(out, issue)
	}
	return out
}

func DeferredIssueIDs(issues []DeferredExecutionIssue) []string {
	if len(issues) == 0 {
		return nil
	}
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		if id := strings.TrimSpace(issue.IssueID); id != "" {
			out = append(out, id)
		}
	}
	return uniqueStrings(out)
}

func SeedDeferredIssueState(state *core.Context, issues []DeferredExecutionIssue) {
	if state == nil {
		return
	}
	if len(issues) == 0 {
		state.Set("euclo.deferred_execution_issues", []DeferredExecutionIssue{})
		state.Set("euclo.deferred_issue_ids", []string{})
		return
	}
	state.Set("euclo.deferred_execution_issues", issues)
	state.Set("euclo.deferred_issue_ids", DeferredIssueIDs(issues))
}

func workspacePathFromTaskState(task *core.Task, state *core.Context) string {
	if state != nil {
		if path := strings.TrimSpace(state.GetString("euclo.workspace")); path != "" {
			return path
		}
	}
	if task != nil && task.Context != nil {
		if path := strings.TrimSpace(stringValue(task.Context["workspace"])); path != "" {
			return path
		}
	}
	return ""
}

func activePlanIDForIssue(uow UnitOfWork) string {
	if uow.PlanBinding == nil {
		return ""
	}
	return strings.TrimSpace(uow.PlanBinding.PlanID)
}

func activePlanVersionForIssue(uow UnitOfWork) int {
	if uow.PlanBinding == nil {
		return 0
	}
	return uow.PlanBinding.PlanVersion
}

func activeStepIDForIssue(uow UnitOfWork, state *core.Context) string {
	if uow.PlanBinding != nil && strings.TrimSpace(uow.PlanBinding.ActiveStepID) != "" {
		return strings.TrimSpace(uow.PlanBinding.ActiveStepID)
	}
	if state != nil {
		return strings.TrimSpace(state.GetString("euclo.current_plan_step_id"))
	}
	return ""
}

func relatedStepIDsForIssue(uow UnitOfWork, state *core.Context) []string {
	steps := make([]string, 0, 2)
	if stepID := activeStepIDForIssue(uow, state); stepID != "" {
		steps = append(steps, stepID)
	}
	if uow.PlanBinding != nil && len(uow.PlanBinding.StepIDs) > 0 {
		steps = append(steps, uow.PlanBinding.StepIDs...)
	}
	return uniqueStrings(steps)
}

func deferredKindFromObservation(obs guidance.EngineeringObservation) DeferredIssueKind {
	if isProviderConstraintObservation(obs) {
		return DeferredIssueProviderConstraint
	}
	switch obs.GuidanceKind {
	case guidance.GuidanceAmbiguity:
		return DeferredIssueAmbiguity
	case guidance.GuidanceConfidence:
		return DeferredIssueStaleAssumption
	case guidance.GuidanceContradiction:
		return DeferredIssueVerificationConcern
	case guidance.GuidanceScopeExpansion:
		return DeferredIssuePatternTension
	case guidance.GuidanceRecovery:
		return DeferredIssueNonfatalFailure
	case guidance.GuidanceApproach:
		return DeferredIssuePatternTension
	default:
		return DeferredIssueAmbiguity
	}
}

func isProviderConstraintObservation(obs guidance.EngineeringObservation) bool {
	source := strings.ToLower(strings.TrimSpace(obs.Source))
	if strings.Contains(source, "provider") || strings.Contains(source, "llm") {
		return true
	}
	if obs.Evidence == nil {
		return false
	}
	if flag, ok := obs.Evidence["provider_constraint"].(bool); ok && flag {
		return true
	}
	if _, ok := obs.Evidence["provider_state_snapshot"]; ok {
		return true
	}
	return false
}

func deferredSeverityFromBlastRadius(radius int) DeferredIssueSeverity {
	switch {
	case radius >= 12:
		return DeferredIssueSeverityCritical
	case radius >= 7:
		return DeferredIssueSeverityHigh
	case radius >= 3:
		return DeferredIssueSeverityMedium
	default:
		return DeferredIssueSeverityLow
	}
}

func whyNotResolvedInline(obs guidance.EngineeringObservation) string {
	if len(obs.Questions) > 0 {
		return "requires user-guided archaeology follow-up"
	}
	switch obs.GuidanceKind {
	case guidance.GuidanceRecovery:
		return "recovery remained non-fatal and execution continued"
	case guidance.GuidanceContradiction:
		return "verification concern preserved for later review"
	default:
		return "execution preserved the concern as deferred knowledge"
	}
}

func recommendedNextAction(obs guidance.EngineeringObservation) string {
	switch obs.GuidanceKind {
	case guidance.GuidanceRecovery:
		return "inspect failure evidence and revisit archaeology before the next run"
	case guidance.GuidanceContradiction:
		return "review verification evidence and decide whether replanning is needed"
	default:
		return "review the deferred issue and restart archaeology if the concern remains material"
	}
}

func evidenceFromObservation(obs guidance.EngineeringObservation, state *core.Context) DeferredExecutionEvidence {
	evidence := DeferredExecutionEvidence{
		ShortReasoningSummary: strings.TrimSpace(obs.Description),
	}
	if obs.Evidence == nil {
		return evidence
	}
	evidence.TouchedSymbols = uniqueStrings(stringSliceAny(obs.Evidence["touched_symbols"]))
	evidence.RelevantPatternRefs = uniqueStrings(stringSliceAny(obs.Evidence["pattern_refs"]))
	evidence.RelevantTensionRefs = uniqueStrings(stringSliceAny(obs.Evidence["tension_refs"]))
	evidence.RelevantAnchorRefs = uniqueStrings(stringSliceAny(obs.Evidence["anchor_refs"]))
	evidence.RelevantProvenanceRefs = uniqueStrings(stringSliceAny(obs.Evidence["provenance_refs"]))
	evidence.RelevantRequestRefs = uniqueStrings(stringSliceAny(obs.Evidence["request_refs"]))
	evidence.VerificationRefs = uniqueStrings(stringSliceAny(obs.Evidence["verification_refs"]))
	evidence.CheckpointRefs = uniqueStrings(stringSliceAny(obs.Evidence["checkpoint_refs"]))
	if provider, ok := obs.Evidence["provider_state_snapshot"].(map[string]any); ok && provider != nil {
		evidence.ProviderStateSnapshot = cloneMapAny(provider)
	} else if state != nil {
		if raw, ok := state.Get("euclo.provider_state_snapshot"); ok {
			if provider, ok := raw.(map[string]any); ok && provider != nil {
				evidence.ProviderStateSnapshot = cloneMapAny(provider)
			}
		}
	}
	if evidence.ShortReasoningSummary == "" {
		evidence.ShortReasoningSummary = strings.TrimSpace(obs.Title)
	}
	return evidence
}

func archaeoRefsFromObservation(obs guidance.EngineeringObservation, uow UnitOfWork) map[string][]string {
	out := map[string][]string{}
	if uow.PlanBinding != nil && len(uow.PlanBinding.ArchaeoRefs) > 0 {
		for key, refs := range uow.PlanBinding.ArchaeoRefs {
			out[key] = append([]string(nil), refs...)
		}
	}
	if obs.Evidence != nil {
		for _, key := range []string{"tension_refs", "provenance_refs", "request_refs", "learning_refs", "phase_refs"} {
			if refs := uniqueStrings(stringSliceAny(obs.Evidence[key])); len(refs) > 0 {
				out[key] = append(out[key], refs...)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	for key, refs := range out {
		out[key] = uniqueStrings(refs)
	}
	return out
}

func nonZeroTime(value, fallback time.Time) time.Time {
	if !value.IsZero() {
		return value
	}
	return fallback
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "deferred-issue"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-", "\n", "-")
	value = replacer.Replace(value)
	return strings.Trim(value, "-")
}

func renderDeferredIssueMarkdown(issue DeferredExecutionIssue) string {
	var b strings.Builder
	b.WriteString("---\n")
	writeYAMLField(&b, "issue_id", issue.IssueID)
	writeYAMLField(&b, "workflow_id", issue.WorkflowID)
	writeYAMLField(&b, "run_id", issue.RunID)
	writeYAMLField(&b, "execution_id", issue.ExecutionID)
	writeYAMLField(&b, "plan_id", issue.ActivePlanID)
	if issue.ActivePlanVersion > 0 {
		b.WriteString(fmt.Sprintf("plan_version: %d\n", issue.ActivePlanVersion))
	}
	writeYAMLField(&b, "step_id", issue.StepID)
	writeYAMLField(&b, "kind", string(issue.Kind))
	writeYAMLField(&b, "severity", string(issue.Severity))
	writeYAMLField(&b, "status", string(issue.Status))
	writeYAMLField(&b, "created_at", issue.CreatedAt.Format(time.RFC3339))
	writeYAMLField(&b, "recommended_reentry", issue.RecommendedReentry)
	if len(issue.RelatedStepIDs) > 0 {
		b.WriteString("related_step_ids:\n")
		for _, stepID := range issue.RelatedStepIDs {
			b.WriteString(fmt.Sprintf("  - %q\n", stepID))
		}
	}
	if len(issue.ArchaeoRefs) > 0 {
		b.WriteString("archaeo_refs:\n")
		keys := make([]string, 0, len(issue.ArchaeoRefs))
		for key := range issue.ArchaeoRefs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			b.WriteString(fmt.Sprintf("  %s:\n", key))
			for _, ref := range issue.ArchaeoRefs[key] {
				b.WriteString(fmt.Sprintf("    - %q\n", ref))
			}
		}
	}
	b.WriteString("---\n\n")
	b.WriteString("# ")
	b.WriteString(issue.Title)
	b.WriteString("\n\n")
	b.WriteString(issue.Summary)
	b.WriteString("\n\n")
	b.WriteString("## Why Execution Continued\n\n")
	b.WriteString(issue.WhyNotResolvedInline)
	b.WriteString("\n\n")
	b.WriteString("## Recommended Next Action\n\n")
	b.WriteString(issue.RecommendedNextAction)
	b.WriteString("\n\n")
	b.WriteString("## Evidence\n\n")
	if issue.Evidence.ShortReasoningSummary != "" {
		b.WriteString(issue.Evidence.ShortReasoningSummary)
		b.WriteString("\n\n")
	}
	writeMarkdownList(&b, "Touched Symbols", issue.Evidence.TouchedSymbols)
	writeMarkdownList(&b, "Pattern Refs", issue.Evidence.RelevantPatternRefs)
	writeMarkdownList(&b, "Tension Refs", issue.Evidence.RelevantTensionRefs)
	writeMarkdownList(&b, "Provenance Refs", issue.Evidence.RelevantProvenanceRefs)
	writeMarkdownList(&b, "Request Refs", issue.Evidence.RelevantRequestRefs)
	return b.String()
}

func writeYAMLField(b *strings.Builder, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	b.WriteString(fmt.Sprintf("%s: %q\n", key, value))
}

func writeMarkdownList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, value := range values {
		b.WriteString("- ")
		b.WriteString(value)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func cloneMapAny(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
