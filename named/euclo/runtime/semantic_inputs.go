package runtime

import (
	"strings"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
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
			case archaeodomain.RequestTensionAnalysis:
				bundle.TensionRefs = uniqueStrings(append(bundle.TensionRefs, request.SubjectRefs...))
			case archaeodomain.RequestProspectiveAnalysis:
				bundle.ProspectiveRefs = uniqueStrings(append(bundle.ProspectiveRefs, ref.RequestID))
			case archaeodomain.RequestConvergenceReview:
				bundle.ConvergenceRefs = uniqueStrings(append(bundle.ConvergenceRefs, ref.RequestID))
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
			}
		}
		for _, tension := range provenance.Tensions {
			if strings.TrimSpace(tension.TensionID) != "" {
				bundle.TensionRefs = uniqueStrings(append(bundle.TensionRefs, tension.TensionID))
			}
			bundle.PatternRefs = uniqueStrings(append(bundle.PatternRefs, tension.PatternIDs...))
			bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, tension.AnchorRefs...))
		}
		for _, planVersion := range provenance.PlanVersions {
			bundle.PatternRefs = uniqueStrings(append(bundle.PatternRefs, planVersion.PatternRefs...))
			bundle.TensionRefs = uniqueStrings(append(bundle.TensionRefs, planVersion.TensionRefs...))
			bundle.ProvenanceRefs = uniqueStrings(append(bundle.ProvenanceRefs, planVersion.FormationProvenanceRefs...))
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
			}
		}
	}
	if convergence != nil {
		if convergence.Current != nil && strings.TrimSpace(convergence.Current.ID) != "" {
			bundle.ConvergenceRefs = uniqueStrings(append(bundle.ConvergenceRefs, convergence.Current.ID))
		}
		for _, record := range convergence.History {
			if strings.TrimSpace(record.ID) != "" {
				bundle.ConvergenceRefs = uniqueStrings(append(bundle.ConvergenceRefs, record.ID))
			}
		}
	}
	return bundle
}
