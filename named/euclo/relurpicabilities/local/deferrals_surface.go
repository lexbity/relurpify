package local

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type deferralsSurfaceCapability struct {
	env agentenv.AgentEnvironment
}

// DeferralsSurfaceRoutine is a SupportingRoutine that loads persisted deferred
// issues for the current workflow and writes a structured summary to state. It
// does not mutate any deferred issue status.
type DeferralsSurfaceRoutine struct{}

func NewDeferralsSurfaceCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &deferralsSurfaceCapability{env: env}
}

func (c *deferralsSurfaceCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            euclorelurpic.CapabilityDeferralsSurface,
		Name:          "Deferrals Surface",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "deferrals", "surface"},
		Annotations: map[string]any{
			"supported_profiles": []string{"chat_ask_respond", "review_suggest_implement", "plan_stage_execute"},
		},
	}
}

func (c *deferralsSurfaceCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindDeferralsSurface,
		},
	}
}

func (c *deferralsSurfaceCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read tools required for deferrals surface"}
	}
	if !artifacts.Has(euclotypes.ArtifactKindIntake) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "deferrals surface requires intake", MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake}}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "intake and read tools available"}
}

func (c *deferralsSurfaceCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	summary, artifact := buildDeferralsSurfaceArtifact(env.Task, env.State, deferralsWorkspace(env.Task, env.State))
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   fmt.Sprintf("surfaced %d deferred issue(s)", summary.TotalOpen),
		Artifacts: []euclotypes.Artifact{artifact},
	}
}

func (r DeferralsSurfaceRoutine) ID() string {
	return euclorelurpic.CapabilityDeferralsSurface
}

func (r DeferralsSurfaceRoutine) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	summary, artifact := buildDeferralsSurfaceArtifact(in.Task, in.State, deferralsWorkspace(in.Task, in.State))
	if in.State != nil {
		in.State.Set("euclo.deferrals_surface", summary)
	}
	mergeStateArtifactsToContext(in.State, []euclotypes.Artifact{artifact})
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": []euclotypes.Artifact{artifact}},
	}, nil
}

func (r DeferralsSurfaceRoutine) IsPrimary() bool { return false }

func (r DeferralsSurfaceRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	summary, artifact := buildDeferralsSurfaceArtifact(in.Task, in.State, deferralsWorkspace(in.Task, in.State))
	if in.State != nil {
		in.State.Set("euclo.deferrals_surface", summary)
	}
	mergeStateArtifactsToContext(in.State, []euclotypes.Artifact{artifact})
	return []euclotypes.Artifact{artifact}, nil
}

func buildDeferralsSurfaceArtifact(task *core.Task, state *core.Context, workspaceDir string) (eucloruntime.DeferralsSurfaceSummary, euclotypes.Artifact) {
	issues := deferredIssuesFromState(state)
	if len(issues) == 0 {
		issues = eucloruntime.LoadDeferredIssuesFromWorkspace(workspaceDir)
	}
	workflowID := deferralsWorkflowID(task, state, issues)
	summary := eucloruntime.BuildDeferralsSurfaceSummary(workflowID, issues)
	artifact := euclotypes.Artifact{
		ID:         "deferrals_surface",
		Kind:       euclotypes.ArtifactKindDeferralsSurface,
		Summary:    fmt.Sprintf("surfaced %d deferred issue(s)", summary.TotalOpen),
		Payload:    summary,
		ProducerID: euclorelurpic.CapabilityDeferralsSurface,
		Status:     "produced",
	}
	if state != nil {
		state.Set("euclo.deferrals_surface", summary)
	}
	return summary, artifact
}

func deferredIssuesFromState(state *core.Context) []eucloruntime.DeferredExecutionIssue {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.deferred_execution_issues")
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []eucloruntime.DeferredExecutionIssue:
		return append([]eucloruntime.DeferredExecutionIssue(nil), typed...)
	case []any:
		out := make([]eucloruntime.DeferredExecutionIssue, 0, len(typed))
		for _, item := range typed {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			issue := eucloruntime.DeferredExecutionIssue{
				IssueID:               stringValue(record["issue_id"]),
				WorkflowID:            stringValue(record["workflow_id"]),
				RunID:                 stringValue(record["run_id"]),
				ExecutionID:           stringValue(record["execution_id"]),
				ActivePlanID:          stringValue(record["active_plan_id"]),
				StepID:                stringValue(record["step_id"]),
				Kind:                  eucloruntime.DeferredIssueKind(stringValue(record["kind"])),
				Severity:              eucloruntime.DeferredIssueSeverity(stringValue(record["severity"])),
				Status:                eucloruntime.DeferredIssueStatus(stringValue(record["status"])),
				Title:                 stringValue(record["title"]),
				Summary:               stringValue(record["summary"]),
				WhyNotResolvedInline:  stringValue(record["why_not_resolved_inline"]),
				RecommendedReentry:    stringValue(record["recommended_reentry"]),
				RecommendedNextAction: stringValue(record["recommended_next_action"]),
			}
			if path := stringValue(record["workspace_artifact_path"]); path != "" {
				issue.WorkspaceArtifactPath = path
			}
			out = append(out, issue)
		}
		return out
	default:
		return nil
	}
}

func deferralsWorkspace(task *core.Task, state *core.Context) string {
	if state != nil {
		if value := strings.TrimSpace(state.GetString("euclo.workspace")); value != "" {
			return value
		}
	}
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(stringValue(task.Context["workspace"])); value != "" {
			return value
		}
		if value := strings.TrimSpace(stringValue(task.Context["cwd"])); value != "" {
			return value
		}
	}
	return ""
}

func deferralsWorkflowID(task *core.Task, state *core.Context, issues []eucloruntime.DeferredExecutionIssue) string {
	if state != nil {
		if value := strings.TrimSpace(state.GetString("euclo.workflow_id")); value != "" {
			return value
		}
	}
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(stringValue(task.Context["workflow_id"])); value != "" {
			return value
		}
	}
	for _, issue := range issues {
		if value := strings.TrimSpace(issue.WorkflowID); value != "" {
			return value
		}
	}
	return ""
}
