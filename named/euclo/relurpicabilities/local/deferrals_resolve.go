package local

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type deferralsResolveCapability struct {
	env agentenv.AgentEnvironment
}

// DeferralsResolveRoutine resolves a deferred issue and synchronizes the
// persisted workspace record, in-memory plan, and guidance event stream.
type DeferralsResolveRoutine struct {
	DeferralPlan   *guidance.DeferralPlan
	GuidanceBroker *guidance.GuidanceBroker
}

func NewDeferralsResolveCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &deferralsResolveCapability{env: env}
}

func (c *deferralsResolveCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            euclorelurpic.CapabilityDeferralsResolve,
		Name:          "Deferrals Resolve",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "deferrals", "resolve"},
		Annotations: map[string]any{
			"supported_profiles": []string{"chat_ask_respond", "review_suggest_implement", "plan_stage_execute"},
		},
	}
}

func (c *deferralsResolveCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindDeferralResolved,
		},
	}
}

func (c *deferralsResolveCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read tools required for deferrals resolve"}
	}
	if !artifacts.Has(euclotypes.ArtifactKindIntake) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "deferrals resolve requires intake", MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake}}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "intake and read tools available"}
}

func (c *deferralsResolveCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	result, _, err := resolveDeferredIssue(env.Task, env.State, nil, nil)
	if err != nil {
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "failed to resolve deferred issue",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "deferrals_resolve_failed",
				Message:      err.Error(),
				Recoverable:  true,
				FailedPhase:  "deferrals_resolve",
				ParadigmUsed: "planner",
			},
		}
	}
	return result
}

func (r DeferralsResolveRoutine) ID() string { return euclorelurpic.CapabilityDeferralsResolve }

func (r DeferralsResolveRoutine) IsPrimary() bool { return false }

func (r DeferralsResolveRoutine) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	_, artifact, err := resolveDeferredIssue(in.Task, in.State, r.DeferralPlan, r.GuidanceBroker)
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": []euclotypes.Artifact{artifact}},
	}, nil
}

func (r DeferralsResolveRoutine) Execute(ctx context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	_, artifact, err := resolveDeferredIssue(in.Task, in.State, r.DeferralPlan, r.GuidanceBroker)
	if err != nil {
		return nil, err
	}
	if in.State != nil {
		in.State.Set("euclo.deferral_resolved", artifact.Payload)
	}
	mergeStateArtifactsToContext(in.State, []euclotypes.Artifact{artifact})
	return []euclotypes.Artifact{artifact}, nil
}

func resolveDeferredIssue(task *core.Task, state *core.Context, plan *guidance.DeferralPlan, broker *guidance.GuidanceBroker) (euclotypes.ExecutionResult, euclotypes.Artifact, error) {
	if state == nil {
		return euclotypes.ExecutionResult{}, euclotypes.Artifact{}, fmt.Errorf("deferral resolution requires state")
	}
	input, ok := deferralResolveInputFromState(state)
	if !ok {
		return euclotypes.ExecutionResult{}, euclotypes.Artifact{}, fmt.Errorf("euclo.deferral_resolve_input missing or invalid")
	}
	if strings.TrimSpace(input.IssueID) == "" {
		return euclotypes.ExecutionResult{}, euclotypes.Artifact{}, fmt.Errorf("deferral resolution requires issue_id")
	}
	if !knownDeferralResolution(input.Resolution) {
		return euclotypes.ExecutionResult{}, euclotypes.Artifact{}, fmt.Errorf("unknown deferral resolution %q", input.Resolution)
	}

	if plan != nil {
		plan.ResolveObservation(input.IssueID)
	}

	workspaceDir := deferralsWorkspace(task, state)
	issues := deferredIssuesFromState(state)
	if len(issues) == 0 && workspaceDir != "" {
		issues = eucloruntime.LoadDeferredIssuesFromWorkspace(workspaceDir)
	}
	updatedIssues := append([]eucloruntime.DeferredExecutionIssue(nil), issues...)
	var target *eucloruntime.DeferredExecutionIssue
	for i := range updatedIssues {
		if strings.TrimSpace(updatedIssues[i].IssueID) != strings.TrimSpace(input.IssueID) {
			continue
		}
		updatedIssues[i].Status = eucloruntime.DeferredIssueStatusResolved
		target = &updatedIssues[i]
		break
	}
	if target == nil {
		return euclotypes.ExecutionResult{}, euclotypes.Artifact{}, fmt.Errorf("deferred issue %q not found", input.IssueID)
	}

	path := strings.TrimSpace(target.WorkspaceArtifactPath)
	if path == "" && workspaceDir != "" {
		path = deferredIssueWorkspacePath(workspaceDir, input.IssueID)
	}
	if path == "" {
		return euclotypes.ExecutionResult{}, euclotypes.Artifact{}, fmt.Errorf("deferred issue %q has no workspace artifact path", input.IssueID)
	}
	if err := eucloruntime.RewriteDeferredIssueMarkdown(path, input); err != nil {
		return euclotypes.ExecutionResult{}, euclotypes.Artifact{}, err
	}
	target.WorkspaceArtifactPath = path

	eucloruntime.SeedDeferredIssueState(state, updatedIssues)
	state.Set("euclo.deferral_resolve_input", input)
	if broker != nil {
		broker.EmitResolution(input.IssueID, euclorelurpic.CapabilityDeferralsResolve)
	}

	payload := map[string]any{
		"issue_id":                input.IssueID,
		"resolution":              strings.TrimSpace(input.Resolution),
		"note":                    strings.TrimSpace(input.Note),
		"workspace_artifact_path": target.WorkspaceArtifactPath,
		"status":                  string(eucloruntime.DeferredIssueStatusResolved),
	}
	artifact := euclotypes.Artifact{
		ID:         "deferral_resolved_" + sanitizeDeferredFilename(input.IssueID),
		Kind:       euclotypes.ArtifactKindDeferralResolved,
		Summary:    fmt.Sprintf("resolved deferred issue %s", input.IssueID),
		Payload:    payload,
		ProducerID: euclorelurpic.CapabilityDeferralsResolve,
		Status:     "produced",
	}
	result := euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   fmt.Sprintf("resolved deferred issue %s", input.IssueID),
		Artifacts: []euclotypes.Artifact{artifact},
	}
	return result, artifact, nil
}

func deferralResolveInputFromState(state *core.Context) (eucloruntime.DeferralResolveInput, bool) {
	if state == nil {
		return eucloruntime.DeferralResolveInput{}, false
	}
	raw, ok := state.Get("euclo.deferral_resolve_input")
	if !ok || raw == nil {
		return eucloruntime.DeferralResolveInput{}, false
	}
	switch typed := raw.(type) {
	case eucloruntime.DeferralResolveInput:
		return typed, true
	case *eucloruntime.DeferralResolveInput:
		if typed == nil {
			return eucloruntime.DeferralResolveInput{}, false
		}
		return *typed, true
	case map[string]any:
		return eucloruntime.DeferralResolveInput{
			IssueID:    stringValue(typed["issue_id"]),
			Resolution: stringValue(typed["resolution"]),
			Note:       stringValue(typed["note"]),
		}, true
	default:
		return eucloruntime.DeferralResolveInput{}, false
	}
}

func knownDeferralResolution(value string) bool {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "accept", "reject", "defer_again", "escalate":
		return true
	default:
		return false
	}
}

func deferredIssueWorkspacePath(workspaceDir, issueID string) string {
	return filepath.Join(workspaceDir, "relurpify_cfg", "artifacts", "euclo", "deferred", sanitizeDeferredFilename(issueID)+".md")
}

func sanitizeDeferredFilename(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "deferred-issue"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-", "\n", "-")
	value = replacer.Replace(value)
	return strings.Trim(value, "-")
}
