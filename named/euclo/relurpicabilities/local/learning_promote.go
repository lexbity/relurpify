package local

import (
	"context"
	"fmt"
	"strings"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type learningPromoteCapability struct {
	env agentenv.AgentEnvironment
}

// LearningPromoteRoutine creates a pending learning interaction from a
// user-described insight or a behavior-trace-backed session state.
type LearningPromoteRoutine struct {
	LearningService  archaeolearning.Service
	WorkflowResolver func(state *core.Context) (workflowID, explorationID string)
}

func NewLearningPromoteCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &learningPromoteCapability{env: env}
}

func (c *learningPromoteCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            euclorelurpic.CapabilityLearningPromote,
		Name:          "Learning Promote",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "learning", "promote"},
		Annotations: map[string]any{
			"supported_profiles": []string{"chat_ask_respond", "review_suggest_implement", "plan_stage_execute"},
		},
	}
}

func (c *learningPromoteCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindLearningPromotion,
		},
	}
}

func (c *learningPromoteCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read tools required for learning promote"}
	}
	if !artifacts.Has(euclotypes.ArtifactKindIntake) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "learning promote requires intake", MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake}}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "intake and read tools available"}
}

func (c *learningPromoteCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	result, err := promoteLearningInteraction(ctx, env.State, archaeolearning.Service{}, c.promoteWorkflowResolver())
	if err != nil {
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "failed to promote learning",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "learning_promote_failed",
				Message:      err.Error(),
				Recoverable:  true,
				FailedPhase:  "learning_promote",
				ParadigmUsed: "planner",
			},
		}
	}
	return result
}

func (c *learningPromoteCapability) promoteWorkflowResolver() func(state *core.Context) (workflowID, explorationID string) {
	return func(state *core.Context) (string, string) {
		if state == nil {
			return "", ""
		}
		workflowID := strings.TrimSpace(state.GetString("euclo.workflow_id"))
		if workflowID == "" {
			workflowID = strings.TrimSpace(state.GetString("workflow_id"))
		}
		explorationID := strings.TrimSpace(state.GetString("euclo.active_exploration_id"))
		if explorationID == "" {
			explorationID = strings.TrimSpace(state.GetString("euclo.exploration_id"))
		}
		if explorationID == "" {
			explorationID = strings.TrimSpace(state.GetString("exploration_id"))
		}
		return workflowID, explorationID
	}
}

func (r LearningPromoteRoutine) ID() string {
	return euclorelurpic.CapabilityLearningPromote
}

func (r LearningPromoteRoutine) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	result, err := promoteLearningInteraction(ctx, in.State, r.LearningService, r.WorkflowResolver)
	if err != nil {
		return nil, err
	}
	artifacts := result.Artifacts
	if len(artifacts) == 0 {
		return &core.Result{Success: true}, nil
	}
	if in.State != nil {
		mergeStateArtifactsToContext(in.State, artifacts)
		if len(artifacts) > 0 {
			in.State.Set("euclo.promoted_learning_interaction", artifacts[0].Payload)
		}
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (r LearningPromoteRoutine) IsPrimary() bool { return false }

func (r LearningPromoteRoutine) Execute(ctx context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	result, err := promoteLearningInteraction(ctx, in.State, r.LearningService, r.WorkflowResolver)
	if err != nil {
		return nil, err
	}
	artifacts := result.Artifacts
	if len(artifacts) == 0 {
		return nil, nil
	}
	return artifacts, nil
}

func promoteLearningInteraction(ctx context.Context, state *core.Context, service archaeolearning.Service, resolver func(state *core.Context) (workflowID, explorationID string)) (euclotypes.ExecutionResult, error) {
	if state == nil {
		return euclotypes.ExecutionResult{}, fmt.Errorf("learning promotion requires state")
	}
	input, ok := learningPromoteInputFromState(state)
	if !ok {
		return euclotypes.ExecutionResult{}, fmt.Errorf("euclo.learning_promote_input missing or invalid")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Kind = strings.TrimSpace(input.Kind)
	if input.Title == "" {
		return euclotypes.ExecutionResult{}, fmt.Errorf("learning promotion requires title")
	}
	if input.Description == "" {
		return euclotypes.ExecutionResult{}, fmt.Errorf("learning promotion requires description")
	}
	if input.Kind == "" {
		return euclotypes.ExecutionResult{}, fmt.Errorf("learning promotion requires kind")
	}

	workflowID, explorationID := "", ""
	if resolver != nil {
		workflowID, explorationID = resolver(state)
	}
	if strings.TrimSpace(workflowID) == "" {
		return euclotypes.ExecutionResult{}, fmt.Errorf("cannot promote learning: no active workflow (start a planning session first)")
	}

	kind, subjectType, err := learningPromoteKindAndSubject(input.Kind, input.SubjectType)
	if err != nil {
		return euclotypes.ExecutionResult{}, err
	}
	evidence := extractEvidenceFromState(state, input)
	if strings.TrimSpace(input.SubjectID) != "" {
		evidence = append([]archaeolearning.EvidenceRef{{
			Kind:    string(subjectType),
			RefID:   strings.TrimSpace(input.SubjectID),
			Title:   input.Title,
			Summary: input.Description,
		}}, evidence...)
	}

	interaction, err := service.Create(ctx, archaeolearning.CreateInput{
		WorkflowID:      workflowID,
		ExplorationID:   explorationID,
		Kind:            kind,
		SubjectType:     subjectType,
		SubjectID:       strings.TrimSpace(input.SubjectID),
		Title:           input.Title,
		Description:     input.Description,
		Evidence:        evidence,
		Blocking:        input.Blocking,
		BasedOnRevision: strings.TrimSpace(firstNonEmpty(state.GetString("euclo.code_revision"), state.GetString("code_revision"))),
	})
	if err != nil {
		return euclotypes.ExecutionResult{}, err
	}
	if interaction == nil {
		return euclotypes.ExecutionResult{}, fmt.Errorf("learning promotion returned nil interaction")
	}

	artifact := euclotypes.Artifact{
		ID:      "learning_promotion_" + sanitizePromotedLearningFilename(interaction.ID),
		Kind:    euclotypes.ArtifactKindLearningPromotion,
		Summary: fmt.Sprintf("promoted learning %s", interaction.Title),
		Payload: map[string]any{
			"interaction_id": interaction.ID,
			"title":          interaction.Title,
			"kind":           string(interaction.Kind),
			"workflow_id":    interaction.WorkflowID,
			"exploration_id": interaction.ExplorationID,
		},
		ProducerID: euclorelurpic.CapabilityLearningPromote,
		Status:     "produced",
	}
	mergeStateArtifactsToContext(state, []euclotypes.Artifact{artifact})
	state.Set("euclo.promoted_learning_interaction", *interaction)
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   fmt.Sprintf("promoted learning %s", interaction.Title),
		Artifacts: []euclotypes.Artifact{artifact},
	}, nil
}

func learningPromoteInputFromState(state *core.Context) (eucloruntime.LearningPromoteInput, bool) {
	if state == nil {
		return eucloruntime.LearningPromoteInput{}, false
	}
	raw, ok := state.Get("euclo.learning_promote_input")
	if !ok || raw == nil {
		return eucloruntime.LearningPromoteInput{}, false
	}
	switch typed := raw.(type) {
	case eucloruntime.LearningPromoteInput:
		return typed, true
	case *eucloruntime.LearningPromoteInput:
		if typed == nil {
			return eucloruntime.LearningPromoteInput{}, false
		}
		return *typed, true
	case map[string]any:
		return eucloruntime.LearningPromoteInput{
			Title:       stringValue(typed["title"]),
			Description: stringValue(typed["description"]),
			Kind:        stringValue(typed["kind"]),
			SubjectID:   stringValue(typed["subject_id"]),
			SubjectType: stringValue(typed["subject_type"]),
			Blocking:    promoteBoolValue(typed["blocking"]),
		}, true
	default:
		return eucloruntime.LearningPromoteInput{}, false
	}
}

func learningPromoteKindAndSubject(kind, subjectType string) (archaeolearning.InteractionKind, archaeolearning.SubjectType, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case string(archaeolearning.InteractionPatternProposal):
		return archaeolearning.InteractionPatternProposal, learningPromoteSubjectType(subjectType, archaeolearning.SubjectPattern, archaeolearning.SubjectExploration), nil
	case string(archaeolearning.InteractionKnowledgeProposal):
		return archaeolearning.InteractionKnowledgeProposal, learningPromoteSubjectType(subjectType, archaeolearning.SubjectExploration, archaeolearning.SubjectPattern), nil
	case string(archaeolearning.InteractionTensionReview):
		return archaeolearning.InteractionTensionReview, learningPromoteSubjectType(subjectType, archaeolearning.SubjectTension, archaeolearning.SubjectExploration), nil
	default:
		return "", "", fmt.Errorf("unknown learning promotion kind %q", kind)
	}
}

func learningPromoteSubjectType(value string, defaultType archaeolearning.SubjectType, fallback archaeolearning.SubjectType) archaeolearning.SubjectType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(archaeolearning.SubjectPattern):
		return archaeolearning.SubjectPattern
	case string(archaeolearning.SubjectTension):
		return archaeolearning.SubjectTension
	case string(archaeolearning.SubjectExploration):
		return archaeolearning.SubjectExploration
	case "":
		if defaultType != "" {
			return defaultType
		}
		return fallback
	default:
		return defaultType
	}
}

func extractEvidenceFromState(state *core.Context, input eucloruntime.LearningPromoteInput) []archaeolearning.EvidenceRef {
	if state == nil {
		return nil
	}
	trimmedSubjectID := strings.TrimSpace(input.SubjectID)
	if trimmedSubjectID != "" {
		return nil
	}
	traceRaw, ok := state.Get("euclo.relurpic_behavior_trace")
	if !ok || traceRaw == nil {
		return nil
	}
	trace, ok := traceRaw.(eucloexec.Trace)
	if !ok || strings.TrimSpace(trace.PrimaryCapabilityID) == "" {
		return nil
	}
	bundle := semanticInputBundleFromState(state)
	evidence := make([]archaeolearning.EvidenceRef, 0, len(bundle.PatternRefs)+len(bundle.RequestProvenanceRefs)+1)
	for _, ref := range bundle.PatternRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		evidence = append(evidence, archaeolearning.EvidenceRef{
			Kind:  "pattern_ref",
			RefID: ref,
			Title: "Pattern reference",
			Metadata: map[string]any{
				"primary_capability_id": trace.PrimaryCapabilityID,
				"executor_family":       trace.ExecutorFamily,
				"source":                "behavior_trace",
			},
		})
	}
	for _, ref := range bundle.RequestProvenanceRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		evidence = append(evidence, archaeolearning.EvidenceRef{
			Kind:  "touched_symbol",
			RefID: ref,
			Title: "Touched symbol",
			Metadata: map[string]any{
				"primary_capability_id": trace.PrimaryCapabilityID,
				"executor_family":       trace.ExecutorFamily,
				"source":                "behavior_trace",
			},
		})
	}
	if len(evidence) == 0 {
		evidence = append(evidence, archaeolearning.EvidenceRef{
			Kind:    "behavior_trace",
			RefID:   trace.PrimaryCapabilityID,
			Title:   trace.PrimaryCapabilityID,
			Summary: strings.TrimSpace(trace.Path),
			Metadata: map[string]any{
				"executor_family": trace.ExecutorFamily,
				"source":          "behavior_trace",
			},
		})
	}
	return evidence
}

func semanticInputBundleFromState(state *core.Context) eucloruntime.SemanticInputBundle {
	if state == nil {
		return eucloruntime.SemanticInputBundle{}
	}
	raw, ok := state.Get("euclo.semantic_inputs")
	if !ok || raw == nil {
		return eucloruntime.SemanticInputBundle{}
	}
	switch typed := raw.(type) {
	case eucloruntime.SemanticInputBundle:
		return typed
	case *eucloruntime.SemanticInputBundle:
		if typed == nil {
			return eucloruntime.SemanticInputBundle{}
		}
		return *typed
	case map[string]any:
		return eucloruntime.SemanticInputBundle{
			PatternRefs:           stringSliceValue(typed["pattern_refs"]),
			RequestProvenanceRefs: stringSliceValue(typed["request_provenance_refs"]),
		}
	default:
		return eucloruntime.SemanticInputBundle{}
	}
}

func stringSliceValue(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(strings.TrimSpace(stringValue(item)))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) != "" {
			return []string{strings.TrimSpace(typed)}
		}
	}
	return nil
}

func promoteBoolValue(raw any) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func sanitizePromotedLearningFilename(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "learning-promotion"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-", "\n", "-")
	value = replacer.Replace(value)
	return strings.Trim(value, "-")
}
