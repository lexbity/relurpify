package blackboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

const (
	capabilityPlannerPlan   = "planner.plan"
	capabilityReviewerReview = "reviewer.review"
	capabilityVerifierVerify = "verifier.verify"
	capabilityExecutorInvoke = "executor.invoke"
	capabilitySummarizerSummarize = "summarizer.summarize"
	defaultAgentDispatch     = "agent:react"
)

// ExplorerKS gathers facts from the codebase when none are present yet.
type ExplorerKS struct{}

func (k *ExplorerKS) Name() string     { return "Explorer" }
func (k *ExplorerKS) Priority() int    { return 100 }
func (k *ExplorerKS) CanActivate(bb *Blackboard) bool {
	return len(bb.Facts) == 0
}
func (k *ExplorerKS) Execute(_ context.Context, bb *Blackboard, _ *capability.Registry, _ core.LanguageModel) error {
	// Explorer remains lightweight until a dedicated retrieval strategy lands.
	// It still records normalized goal facts that downstream capability-routed
	// specialists can consume.
	bb.AddFact("exploration.status", "explored", k.Name())
	if len(bb.Goals) > 0 {
		bb.AddFact("task.goal", bb.Goals[0], k.Name())
	}
	return nil
}

// AnalyzerKS identifies issues once facts are available.
type AnalyzerKS struct{}

func (k *AnalyzerKS) Name() string     { return "Analyzer" }
func (k *AnalyzerKS) Priority() int    { return 90 }
func (k *AnalyzerKS) CanActivate(bb *Blackboard) bool {
	return len(bb.Facts) > 0 && len(bb.Issues) == 0
}
func (k *AnalyzerKS) Execute(ctx context.Context, bb *Blackboard, tools *capability.Registry, _ core.LanguageModel) error {
	if res, ok, err := invokeCapabilityIfPresent(ctx, tools, capabilityReviewerReview, map[string]any{
		"instruction":         firstGoal(bb),
		"artifact_summary":    factsSummary(bb),
		"acceptance_criteria": append([]string(nil), bb.Goals...),
	}); err != nil {
		return err
	} else if ok {
		if added, err := addFindingsAsIssues(bb, res, k.Name()); err != nil {
			return err
		} else if added > 0 {
			return nil
		}
	}
	return bb.AddIssue(
		fmt.Sprintf("issue-%d", time.Now().UnixNano()),
		analysisSummary(bb),
		"low",
		k.Name(),
	)
}

// PlannerKS creates action requests for each identified issue.
type PlannerKS struct{}

func (k *PlannerKS) Name() string     { return "Planner" }
func (k *PlannerKS) Priority() int    { return 80 }
func (k *PlannerKS) CanActivate(bb *Blackboard) bool {
	return len(bb.Issues) > 0 && len(bb.PendingActions) == 0 && len(bb.CompletedActions) == 0
}
func (k *PlannerKS) Execute(ctx context.Context, bb *Blackboard, tools *capability.Registry, _ core.LanguageModel) error {
	if res, ok, err := invokeCapabilityIfPresent(ctx, tools, capabilityPlannerPlan, map[string]any{
		"instruction": plannerInstruction(bb),
	}); err != nil {
		return err
	} else if ok {
		if planned, err := enqueuePlannedActions(bb, res, k.Name()); err != nil {
			return err
		} else if planned > 0 {
			return nil
		}
	}
	for _, issue := range bb.Issues {
		if err := bb.EnqueueAction(ActionRequest{
			ID:          fmt.Sprintf("action-%s", issue.ID),
			ToolOrAgent: defaultAgentDispatch,
			Args:        map[string]any{"instruction": issue.Description},
			Description: fmt.Sprintf("Resolve: %s", issue.Description),
			RequestedBy: k.Name(),
			CreatedAt:   time.Now().UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// ReviewKS performs a second-pass review over produced artifacts before final verification.
type ReviewKS struct{}

func (k *ReviewKS) Name() string  { return "Review" }
func (k *ReviewKS) Priority() int { return 75 }
func (k *ReviewKS) CanActivate(bb *Blackboard) bool {
	return len(bb.Artifacts) > 0 && !bb.HasUnverifiedArtifacts() && len(bb.Issues) == 0
}
func (k *ReviewKS) Execute(ctx context.Context, bb *Blackboard, tools *capability.Registry, _ core.LanguageModel) error {
	if res, ok, err := invokeCapabilityIfPresent(ctx, tools, capabilityReviewerReview, map[string]any{
		"instruction":      firstGoal(bb),
		"artifact_summary": artifactSummary(bb),
		"mode":             "artifact_review",
	}); err != nil {
		return err
	} else if ok {
		_, err := addFindingsAsIssues(bb, res, k.Name())
		return err
	}
	return nil
}

// ExecutorKS invokes pending tool/agent actions and records results.
type ExecutorKS struct{}

func (k *ExecutorKS) Name() string     { return "Executor" }
func (k *ExecutorKS) Priority() int    { return 70 }
func (k *ExecutorKS) CanActivate(bb *Blackboard) bool {
	return len(bb.PendingActions) > 0
}
func (k *ExecutorKS) Execute(ctx context.Context, bb *Blackboard, tools *capability.Registry, _ core.LanguageModel) error {
	// Drain pending actions and produce artifacts.
	pending := append([]ActionRequest(nil), bb.PendingActions...)
	for _, req := range pending {
		outcome, err := executeActionRequest(ctx, tools, req)
		if err := bb.CompleteAction(ActionResult{
			RequestID: req.ID,
			Success:   err == nil,
			Output:    outcome,
			Error:     errorString(err),
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if err != nil {
			_ = bb.AddIssue(
				fmt.Sprintf("exec-%s", req.ID),
				fmt.Sprintf("action %s failed: %v", req.ID, err),
				"high",
				k.Name(),
			)
			return err
		}
		if err := bb.AddArtifact(
			fmt.Sprintf("artifact-%s", req.ID),
			"result",
			outcome,
			k.Name(),
		); err != nil {
			return err
		}
	}
	return nil
}

// VerifierKS marks artifacts verified after checking them.
type VerifierKS struct{}

func (k *VerifierKS) Name() string     { return "Verifier" }
func (k *VerifierKS) Priority() int    { return 60 }
func (k *VerifierKS) CanActivate(bb *Blackboard) bool {
	return bb.HasUnverifiedArtifacts()
}
func (k *VerifierKS) Execute(ctx context.Context, bb *Blackboard, tools *capability.Registry, _ core.LanguageModel) error {
	if res, ok, err := invokeCapabilityIfPresent(ctx, tools, capabilityVerifierVerify, map[string]any{
		"instruction":           firstGoal(bb),
		"artifact_summary":      artifactSummary(bb),
		"verification_criteria": append([]string(nil), bb.Goals...),
	}); err != nil {
		return err
	} else if ok {
		if verified, _ := res.Data["verified"].(bool); verified {
			bb.VerifyAllArtifacts()
			return nil
		}
		if missing := stringSliceFromAny(res.Data["missing_items"]); len(missing) > 0 {
			for idx, item := range missing {
				if err := bb.AddIssue(fmt.Sprintf("verify-missing-%d", idx), item, "medium", k.Name()); err != nil && !strings.Contains(err.Error(), "already exists") {
					return err
				}
			}
			return fmt.Errorf("verification failed: %s", strings.Join(missing, "; "))
		}
		return fmt.Errorf("verification failed")
	}
	bb.VerifyAllArtifacts()
	return nil
}

// FailureTriageKS turns failed action results into actionable issues and retries.
type FailureTriageKS struct{}

func (k *FailureTriageKS) Name() string  { return "FailureTriage" }
func (k *FailureTriageKS) Priority() int { return 65 }
func (k *FailureTriageKS) CanActivate(bb *Blackboard) bool {
	if bb == nil || len(bb.PendingActions) > 0 {
		return false
	}
	for _, result := range bb.CompletedActions {
		if !result.Success {
			return true
		}
	}
	return false
}
func (k *FailureTriageKS) Execute(_ context.Context, bb *Blackboard, _ *capability.Registry, _ core.LanguageModel) error {
	for _, result := range bb.CompletedActions {
		if result.Success {
			continue
		}
		issueID := fmt.Sprintf("triage-%s", result.RequestID)
		if !bb.HasIssue(issueID) {
			if err := bb.AddIssue(issueID, fmt.Sprintf("retry required after failed action %s: %s", result.RequestID, strings.TrimSpace(result.Error)), "high", k.Name()); err != nil {
				return err
			}
		}
		retryID := "retry-" + result.RequestID
		if bb.HasPendingAction(retryID) || bb.HasCompletedAction(retryID) {
			continue
		}
		if err := bb.EnqueueAction(ActionRequest{
			ID:          retryID,
			ToolOrAgent: defaultAgentDispatch,
			Args:        map[string]any{"instruction": fmt.Sprintf("Recover from failed action %s", result.RequestID)},
			Description: fmt.Sprintf("Recover from failed action %s", result.RequestID),
			RequestedBy: k.Name(),
			CreatedAt:   time.Now().UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// SummarizerKS produces a final summary artifact once work has been verified.
type SummarizerKS struct{}

func (k *SummarizerKS) Name() string  { return "Summarizer" }
func (k *SummarizerKS) Priority() int { return 55 }
func (k *SummarizerKS) CanActivate(bb *Blackboard) bool {
	if bb == nil || bb.HasUnverifiedArtifacts() || len(bb.CompletedActions) == 0 {
		return false
	}
	return !bb.HasArtifact("blackboard-summary")
}
func (k *SummarizerKS) Execute(ctx context.Context, bb *Blackboard, tools *capability.Registry, _ core.LanguageModel) error {
	summary := buildBlackboardCompletionSummary(bb)
	if res, ok, err := invokeCapabilityIfPresent(ctx, tools, capabilitySummarizerSummarize, map[string]any{
		"instruction":      firstGoal(bb),
		"artifact_summary": artifactSummary(bb),
		"fact_summary":     factsSummary(bb),
		"issue_summary":    issuesSummary(bb),
	}); err != nil {
		return err
	} else if ok {
		if candidate := strings.TrimSpace(fmt.Sprint(res.Data["summary"])); candidate != "" && candidate != "<nil>" {
			summary = candidate
		}
	}
	return bb.AddArtifact("blackboard-summary", "summary", summary, k.Name())
}

// DefaultKnowledgeSources returns the five built-in KS in priority order.
func DefaultKnowledgeSources() []KnowledgeSource {
	return []KnowledgeSource{
		&ExplorerKS{},
		&AnalyzerKS{},
		&PlannerKS{},
		&ReviewKS{},
		&ExecutorKS{},
		&FailureTriageKS{},
		&VerifierKS{},
		&SummarizerKS{},
	}
}

func invokeCapabilityIfPresent(ctx context.Context, tools *capability.Registry, name string, args map[string]any) (*core.ToolResult, bool, error) {
	if tools == nil {
		return nil, false, nil
	}
	if _, ok := tools.GetCapability(name); !ok {
		return nil, false, nil
	}
	result, err := tools.InvokeCapability(ctx, core.NewContext(), name, args)
	return result, true, err
}

func enqueuePlannedActions(bb *Blackboard, result *core.ToolResult, source string) (int, error) {
	if result == nil || len(result.Data) == 0 {
		return 0, nil
	}
	rawSteps, ok := result.Data["steps"]
	if !ok {
		return 0, nil
	}
	steps, ok := rawSteps.([]any)
	if !ok {
		return 0, nil
	}
	count := 0
	for idx, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			if converted, ok := raw.(map[string]interface{}); ok {
				step = make(map[string]any, len(converted))
				for key, value := range converted {
					step[key] = value
				}
			} else {
				continue
			}
		}
		stepID := strings.TrimSpace(fmt.Sprint(step["id"]))
		if stepID == "" {
			stepID = fmt.Sprintf("planned-%d", idx)
		}
		toolOrAgent := strings.TrimSpace(fmt.Sprint(step["tool"]))
		if toolOrAgent == "" {
			toolOrAgent = defaultAgentDispatch
		}
		description := strings.TrimSpace(fmt.Sprint(step["description"]))
		if description == "" {
			description = fmt.Sprintf("Planned action %d", idx+1)
		}
		args := mapFromAny(step["params"])
		if strings.HasPrefix(toolOrAgent, "agent:") && strings.TrimSpace(fmt.Sprint(args["instruction"])) == "" {
			args["instruction"] = description
		}
		if err := bb.EnqueueAction(ActionRequest{
			ID:          "action-" + stepID,
			ToolOrAgent: toolOrAgent,
			Args:        args,
			Description: description,
			RequestedBy: source,
			CreatedAt:   time.Now().UTC(),
		}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func executeActionRequest(ctx context.Context, tools *capability.Registry, req ActionRequest) (string, error) {
	if tools == nil {
		return fmt.Sprintf("completed: %s", req.Description), nil
	}
	target := strings.TrimSpace(req.ToolOrAgent)
	if target == "" {
		return "", fmt.Errorf("action target required")
	}
	if _, ok := tools.GetCoordinationTarget(target); ok || strings.HasPrefix(target, "agent:") {
		if _, exists := tools.GetCapability(target); !exists {
			return fmt.Sprintf("completed: %s", req.Description), nil
		}
		result, err := tools.InvokeCapability(ctx, core.NewContext(), target, req.Args)
		if err != nil {
			return "", err
		}
		return summarizeCapabilityResult(result, req.Description), nil
	}
	if _, exists := tools.GetCapability(target); !exists {
		return fmt.Sprintf("completed: %s", req.Description), nil
	}
	if _, ok := tools.GetCapability(capabilityExecutorInvoke); ok {
		result, err := tools.InvokeCapability(ctx, core.NewContext(), capabilityExecutorInvoke, map[string]any{
			"capability": target,
			"args":       req.Args,
		})
		if err != nil {
			return "", err
		}
		return summarizeCapabilityResult(result, req.Description), nil
	}
	result, err := tools.InvokeCapability(ctx, core.NewContext(), target, req.Args)
	if err != nil {
		return "", err
	}
	return summarizeCapabilityResult(result, req.Description), nil
}

func summarizeCapabilityResult(result *core.ToolResult, fallback string) string {
	if result == nil {
		return fallback
	}
	if summary := strings.TrimSpace(fmt.Sprint(result.Data["summary"])); summary != "" {
		return summary
	}
	if output := strings.TrimSpace(fmt.Sprint(result.Data["output"])); output != "" {
		return output
	}
	if text := strings.TrimSpace(fmt.Sprint(result.Data["result"])); text != "" && text != "<nil>" {
		return text
	}
	return fallback
}

func mapFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[key] = val
		}
		return out
	default:
		return map[string]any{}
	}
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text == "" {
				continue
			}
			out = append(out, text)
		}
		return out
	default:
		return nil
	}
}

func artifactSummary(bb *Blackboard) string {
	parts := make([]string, 0, len(bb.Artifacts))
	for _, artifact := range bb.Artifacts {
		parts = append(parts, fmt.Sprintf("%s:%s", artifact.Kind, artifact.Content))
	}
	return strings.Join(parts, "\n")
}

func analysisSummary(bb *Blackboard) string {
	if len(bb.Facts) == 0 {
		return "analysis complete"
	}
	parts := make([]string, 0, len(bb.Facts))
	for _, fact := range bb.Facts {
		parts = append(parts, fmt.Sprintf("%s=%s", fact.Key, fact.Value))
	}
	return "analysis from facts: " + strings.Join(parts, ", ")
}

func issuesSummary(bb *Blackboard) string {
	if bb == nil || len(bb.Issues) == 0 {
		return ""
	}
	parts := make([]string, 0, len(bb.Issues))
	for _, issue := range bb.Issues {
		parts = append(parts, fmt.Sprintf("%s:%s", issue.Severity, issue.Description))
	}
	return strings.Join(parts, "\n")
}

func factsSummary(bb *Blackboard) string {
	parts := make([]string, 0, len(bb.Facts))
	for _, fact := range bb.Facts {
		parts = append(parts, fmt.Sprintf("%s=%s", fact.Key, fact.Value))
	}
	return strings.Join(parts, "\n")
}

func plannerInstruction(bb *Blackboard) string {
	if len(bb.Issues) == 0 {
		return firstGoal(bb)
	}
	parts := make([]string, 0, len(bb.Issues))
	for _, issue := range bb.Issues {
		parts = append(parts, issue.Description)
	}
	if goal := firstGoal(bb); goal != "" {
		return goal + "\n\nIssues:\n- " + strings.Join(parts, "\n- ")
	}
	return strings.Join(parts, "\n")
}

func firstGoal(bb *Blackboard) string {
	if bb == nil || len(bb.Goals) == 0 {
		return ""
	}
	return bb.Goals[0]
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func buildBlackboardCompletionSummary(bb *Blackboard) string {
	if bb == nil {
		return ""
	}
	goal := firstGoal(bb)
	artifacts := make([]string, 0, len(bb.Artifacts))
	for _, artifact := range bb.Artifacts {
		if artifact.ID == "blackboard-summary" {
			continue
		}
		artifacts = append(artifacts, fmt.Sprintf("%s=%s", artifact.Kind, artifact.Content))
	}
	if goal == "" {
		return strings.Join(artifacts, "; ")
	}
	if len(artifacts) == 0 {
		return fmt.Sprintf("Completed %q", goal)
	}
	return fmt.Sprintf("Completed %q with artifacts: %s", goal, strings.Join(artifacts, "; "))
}

func addFindingsAsIssues(bb *Blackboard, result *core.ToolResult, source string) (int, error) {
	if result == nil || len(result.Data) == 0 {
		return 0, nil
	}
	rawFindings, ok := result.Data["findings"]
	if !ok {
		return 0, nil
	}
	findings, ok := rawFindings.([]any)
	if !ok {
		return 0, nil
	}
	count := 0
	for idx, raw := range findings {
		finding, ok := raw.(map[string]any)
		if !ok {
			if converted, ok := raw.(map[string]interface{}); ok {
				finding = make(map[string]any, len(converted))
				for key, value := range converted {
					finding[key] = value
				}
			} else {
				continue
			}
		}
		description := strings.TrimSpace(fmt.Sprint(finding["description"]))
		if description == "" {
			continue
		}
		severity := strings.TrimSpace(fmt.Sprint(finding["severity"]))
		if severity == "" {
			severity = "medium"
		}
		if err := bb.AddIssue(fmt.Sprintf("finding-%d", idx), description, severity, source); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
