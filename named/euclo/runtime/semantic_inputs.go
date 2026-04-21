package runtime

import (
	"fmt"
	"strings"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
)

type SemanticLearningRef struct {
	InteractionID string
}

type SemanticTensionRef struct {
	TensionID  string
	PatternIDs []string
	AnchorRefs []string
}

type SemanticPlanVersionRef struct {
	PatternRefs             []string
	TensionRefs             []string
	FormationProvenanceRefs []string
	FormationResultRef      string
	SemanticSnapshotRef     string
}

type SemanticRequestProvenanceRef struct {
	RequestID string
}

type SemanticRequestHistory struct {
	Requests []archaeodomain.RequestRecord
}

type SemanticProvenance struct {
	Requests        []SemanticRequestProvenanceRef
	Learning        []SemanticLearningRef
	Tensions        []SemanticTensionRef
	PlanVersions    []SemanticPlanVersionRef
	ConvergenceRefs []string
	DecisionRefs    []string
}

type SemanticLearningQueue struct {
	PendingLearning []SemanticLearningRef
}

func SemanticInputBundleFromSources(
	workflowID string,
	activePlan *archaeodomain.VersionedLivingPlan,
	requests *SemanticRequestHistory,
	provenance *SemanticProvenance,
	learning *SemanticLearningQueue,
	convergence *archaeodomain.WorkspaceConvergenceProjection,
) SemanticInputBundle {
	bundle := SemanticInputBundle{
		WorkflowID: workflowID,
		Source:     "archaeo.projections",
	}
	if activePlan != nil {
		bundle.ExplorationID = strings.TrimSpace(activePlan.DerivedFromExploration)
		bundle.BasedOnRevision = strings.TrimSpace(activePlan.BasedOnRevision)
		bundle.PatternRefs = uniqueStrings(append(bundle.PatternRefs, activePlan.PatternRefs...))
		bundle.TensionRefs = uniqueStrings(append(bundle.TensionRefs, activePlan.TensionRefs...))
		bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, activePlan.FormationProvenanceRefs...))
		bundle.PatternFindings = append(bundle.PatternFindings, buildPlanPatternFindings(activePlan)...)
		bundle.TensionFindings = append(bundle.TensionFindings, buildPlanTensionFindings(activePlan)...)
		if activePlan.Version > 0 || strings.TrimSpace(activePlan.ID) != "" {
			bundle.ConvergenceFindings = append(bundle.ConvergenceFindings, SemanticFindingSummary{
				RefID:   strings.TrimSpace(activePlan.ID),
				Kind:    "active_plan_version",
				Status:  string(activePlan.Status),
				Title:   fmt.Sprintf("Active plan version %d", activePlan.Version),
				Summary: strings.TrimSpace(activePlan.Plan.Title),
			})
		}
		if ref := strings.TrimSpace(activePlan.SemanticSnapshotRef); ref != "" {
			bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, ref))
		}
		if bundle.ExplorationID != "" {
			bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, bundle.ExplorationID))
		}
	}
	if requests != nil {
		for _, request := range requests.Requests {
			ref := SemanticRequestRef{
				RequestID: strings.TrimSpace(request.ID),
				Kind:      string(request.Kind),
				Status:    string(request.Status),
				Title:     strings.TrimSpace(request.Title),
			}
			if ref.RequestID == "" {
				continue
			}
			bundle.RequestProvenanceRefs = uniqueStrings(append(bundle.RequestProvenanceRefs, ref.RequestID))
			switch request.Kind {
			case archaeodomain.RequestPatternSurfacing:
				bundle.PatternRefs = uniqueStrings(append(bundle.PatternRefs, request.SubjectRefs...))
				bundle.PatternFindings = append(bundle.PatternFindings, SemanticFindingSummary{
					RefID:       ref.RequestID,
					Kind:        "pattern_request",
					Status:      ref.Status,
					Title:       nonEmpty(ref.Title, "Pattern surfacing request"),
					Summary:     nonEmpty(strings.TrimSpace(request.Description), "Pattern surfacing available for execution context."),
					RelatedRefs: append([]string(nil), request.SubjectRefs...),
				})
			case archaeodomain.RequestTensionAnalysis:
				bundle.TensionRefs = uniqueStrings(append(bundle.TensionRefs, request.SubjectRefs...))
				bundle.TensionFindings = append(bundle.TensionFindings, SemanticFindingSummary{
					RefID:       ref.RequestID,
					Kind:        "tension_request",
					Status:      ref.Status,
					Title:       nonEmpty(ref.Title, "Tension analysis request"),
					Summary:     nonEmpty(strings.TrimSpace(request.Description), "Tension analysis available for execution context."),
					RelatedRefs: append([]string(nil), request.SubjectRefs...),
				})
			case archaeodomain.RequestProspectiveAnalysis:
				bundle.ProspectiveRefs = uniqueStrings(append(bundle.ProspectiveRefs, ref.RequestID))
				bundle.ProspectiveFindings = append(bundle.ProspectiveFindings, SemanticFindingSummary{
					RefID:   ref.RequestID,
					Kind:    "prospective_request",
					Status:  ref.Status,
					Title:   nonEmpty(ref.Title, "Prospective analysis request"),
					Summary: nonEmpty(strings.TrimSpace(request.Description), "Prospective structure analysis available."),
				})
			case archaeodomain.RequestConvergenceReview:
				bundle.ConvergenceRefs = uniqueStrings(append(bundle.ConvergenceRefs, ref.RequestID))
				bundle.ConvergenceFindings = append(bundle.ConvergenceFindings, SemanticFindingSummary{
					RefID:   ref.RequestID,
					Kind:    "convergence_request",
					Status:  ref.Status,
					Title:   nonEmpty(ref.Title, "Convergence review request"),
					Summary: nonEmpty(strings.TrimSpace(request.Description), "Convergence review available."),
				})
			}
			switch request.Status {
			case archaeodomain.RequestStatusCompleted:
				bundle.CompletedRequests = append(bundle.CompletedRequests, ref)
			default:
				bundle.PendingRequests = append(bundle.PendingRequests, ref)
			}
		}
	}
	if provenance != nil {
		for _, request := range provenance.Requests {
			if strings.TrimSpace(request.RequestID) != "" {
				bundle.RequestProvenanceRefs = uniqueStrings(append(bundle.RequestProvenanceRefs, request.RequestID))
			}
		}
		for _, learningRef := range provenance.Learning {
			if strings.TrimSpace(learningRef.InteractionID) != "" {
				bundle.LearningInteractionRefs = uniqueStrings(append(bundle.LearningInteractionRefs, learningRef.InteractionID))
				bundle.LearningFindings = append(bundle.LearningFindings, SemanticFindingSummary{
					RefID:   strings.TrimSpace(learningRef.InteractionID),
					Kind:    "learning_interaction",
					Status:  "recorded",
					Title:   "Learning interaction",
					Summary: "Archaeology learning interaction available for review.",
				})
			}
		}
		for _, tension := range provenance.Tensions {
			if strings.TrimSpace(tension.TensionID) != "" {
				bundle.TensionRefs = uniqueStrings(append(bundle.TensionRefs, tension.TensionID))
			}
			bundle.PatternRefs = uniqueStrings(append(bundle.PatternRefs, tension.PatternIDs...))
			bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, tension.AnchorRefs...))
			bundle.TensionFindings = append(bundle.TensionFindings, SemanticFindingSummary{
				RefID:       strings.TrimSpace(tension.TensionID),
				Kind:        "tension_provenance",
				Status:      "recorded",
				Title:       "Tension provenance",
				Summary:     "Previously observed tension remains relevant to this execution context.",
				RelatedRefs: append(append([]string(nil), tension.PatternIDs...), tension.AnchorRefs...),
			})
		}
		for _, planVersion := range provenance.PlanVersions {
			bundle.PatternRefs = uniqueStrings(append(bundle.PatternRefs, planVersion.PatternRefs...))
			bundle.TensionRefs = uniqueStrings(append(bundle.TensionRefs, planVersion.TensionRefs...))
			bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, planVersion.FormationProvenanceRefs...))
			bundle.PatternFindings = append(bundle.PatternFindings, SemanticFindingSummary{
				Kind:        "plan_version_patterns",
				Status:      "recorded",
				Title:       "Pattern provenance from plan formation",
				Summary:     "Pattern evidence from prior plan formation remains available.",
				RelatedRefs: append([]string(nil), planVersion.PatternRefs...),
			})
			if ref := strings.TrimSpace(planVersion.FormationResultRef); ref != "" {
				bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, ref))
			}
			if ref := strings.TrimSpace(planVersion.SemanticSnapshotRef); ref != "" {
				bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, ref))
			}
		}
		bundle.ConvergenceRefs = uniqueStrings(append(bundle.ConvergenceRefs, provenance.ConvergenceRefs...))
		bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, provenance.DecisionRefs...))
	}
	if learning != nil {
		for _, interaction := range learning.PendingLearning {
			if strings.TrimSpace(interaction.InteractionID) != "" {
				bundle.LearningInteractionRefs = uniqueStrings(append(bundle.LearningInteractionRefs, interaction.InteractionID))
				bundle.LearningFindings = append(bundle.LearningFindings, SemanticFindingSummary{
					RefID:   strings.TrimSpace(interaction.InteractionID),
					Kind:    "pending_learning",
					Status:  "pending",
					Title:   "Pending learning review",
					Summary: "Learning interaction still needs confirmation or follow-up.",
				})
			}
		}
	}
	if convergence != nil {
		if convergence.Current != nil && strings.TrimSpace(convergence.Current.ID) != "" {
			bundle.ConvergenceRefs = uniqueStrings(append(bundle.ConvergenceRefs, convergence.Current.ID))
			bundle.ConvergenceFindings = append(bundle.ConvergenceFindings, SemanticFindingSummary{
				RefID:   strings.TrimSpace(convergence.Current.ID),
				Kind:    "current_convergence",
				Status:  string(convergence.Current.Status),
				Title:   "Current convergence state",
				Summary: nonEmpty(strings.TrimSpace(convergence.Current.Title), strings.TrimSpace(convergence.Current.Question), "Current convergence state is available."),
			})
		}
		for _, record := range convergence.History {
			if strings.TrimSpace(record.ID) != "" {
				bundle.ConvergenceRefs = uniqueStrings(append(bundle.ConvergenceRefs, record.ID))
				bundle.ConvergenceFindings = append(bundle.ConvergenceFindings, SemanticFindingSummary{
					RefID:   strings.TrimSpace(record.ID),
					Kind:    "convergence_history",
					Status:  string(record.Status),
					Title:   "Convergence history",
					Summary: nonEmpty(strings.TrimSpace(record.Title), strings.TrimSpace(record.Question), "Prior convergence state is available."),
				})
			}
		}
	}
	bundle.PatternFindings = dedupeSemanticFindings(bundle.PatternFindings)
	bundle.TensionFindings = dedupeSemanticFindings(bundle.TensionFindings)
	bundle.ProspectiveFindings = dedupeSemanticFindings(bundle.ProspectiveFindings)
	bundle.ConvergenceFindings = dedupeSemanticFindings(bundle.ConvergenceFindings)
	bundle.LearningFindings = dedupeSemanticFindings(bundle.LearningFindings)
	return bundle
}

func buildPlanPatternFindings(activePlan *archaeodomain.VersionedLivingPlan) []SemanticFindingSummary {
	if activePlan == nil || len(activePlan.PatternRefs) == 0 {
		return nil
	}
	return []SemanticFindingSummary{{
		RefID:       strings.TrimSpace(activePlan.ID),
		Kind:        "active_plan_patterns",
		Status:      string(activePlan.Status),
		Title:       "Patterns attached to active plan",
		Summary:     "Pattern evidence carried forward from the active living plan.",
		RelatedRefs: append([]string(nil), activePlan.PatternRefs...),
	}}
}

func buildPlanTensionFindings(activePlan *archaeodomain.VersionedLivingPlan) []SemanticFindingSummary {
	if activePlan == nil || len(activePlan.TensionRefs) == 0 {
		return nil
	}
	return []SemanticFindingSummary{{
		RefID:       strings.TrimSpace(activePlan.ID),
		Kind:        "active_plan_tensions",
		Status:      string(activePlan.Status),
		Title:       "Tensions attached to active plan",
		Summary:     "Tension evidence carried forward from the active living plan.",
		RelatedRefs: append([]string(nil), activePlan.TensionRefs...),
	}}
}

func dedupeSemanticFindings(input []SemanticFindingSummary) []SemanticFindingSummary {
	if len(input) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(input))
	out := make([]SemanticFindingSummary, 0, len(input))
	for _, finding := range input {
		key := strings.Join([]string{
			strings.TrimSpace(finding.RefID),
			strings.TrimSpace(finding.Kind),
			strings.TrimSpace(finding.Status),
			strings.TrimSpace(finding.Title),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		finding.RelatedRefs = uniqueStrings(finding.RelatedRefs)
		out = append(out, finding)
	}
	return out
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
