package provenance

import (
	"context"
	"strings"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/memory"
)

type Service struct {
	Store memory.WorkflowStateStore
}

func (s Service) Build(ctx context.Context, workflowID string) (*archaeodomain.ProvenanceRecord, error) {
	workflowID = strings.TrimSpace(workflowID)
	if s.Store == nil || workflowID == "" {
		return nil, nil
	}
	learningItems, err := (archaeolearning.Service{Store: s.Store}).ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	tensions, err := (archaeotensions.Service{Store: s.Store}).ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	lineage, err := (archaeoplans.Service{WorkflowStore: s.Store}).LoadLineage(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	requests, err := (archaeorequests.Service{Store: s.Store}).ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	mutations, err := archaeoevents.ReadMutationEvents(ctx, s.Store, workflowID)
	if err != nil {
		return nil, err
	}
	record := &archaeodomain.ProvenanceRecord{WorkflowID: workflowID}
	for _, interaction := range learningItems {
		record.Learning = append(record.Learning, archaeodomain.LearningOutcomeProvenance{
			InteractionID:   interaction.ID,
			SubjectType:     string(interaction.SubjectType),
			SubjectID:       interaction.SubjectID,
			Status:          string(interaction.Status),
			Blocking:        interaction.Blocking,
			BasedOnRevision: interaction.BasedOnRevision,
			CommentRef:      learningCommentRef(interaction),
			EvidenceRefs:    evidenceRefs(interaction.Evidence),
			MutationIDs:     relatedMutationIDsForLearning(mutations, interaction),
		})
	}
	for _, tension := range tensions {
		record.Tensions = append(record.Tensions, archaeodomain.TensionProvenance{
			TensionID:          tension.ID,
			Status:             string(tension.Status),
			Description:        tension.Description,
			CommentRefs:        append([]string(nil), tension.CommentRefs...),
			PatternIDs:         append([]string(nil), tension.PatternIDs...),
			AnchorRefs:         append([]string(nil), tension.AnchorRefs...),
			RelatedPlanStepIDs: append([]string(nil), tension.RelatedPlanStepIDs...),
			BasedOnRevision:    tension.BasedOnRevision,
			MutationIDs:        relatedMutationIDsForTension(mutations, tension),
		})
	}
	if lineage != nil {
		for _, version := range lineage.Versions {
			record.PlanVersions = append(record.PlanVersions, archaeodomain.PlanVersionProvenance{
				PlanID:                  version.Plan.ID,
				Version:                 version.Version,
				ParentVersion:           intPointerClone(version.ParentVersion),
				DerivedFromExploration:  version.DerivedFromExploration,
				BasedOnRevision:         version.BasedOnRevision,
				SemanticSnapshotRef:     version.SemanticSnapshotRef,
				CommentRefs:             append([]string(nil), version.CommentRefs...),
				PatternRefs:             append([]string(nil), version.PatternRefs...),
				AnchorRefs:              append([]string(nil), version.AnchorRefs...),
				TensionRefs:             append([]string(nil), version.TensionRefs...),
				MutationIDs:             relatedMutationIDsForVersion(mutations, version),
				FormationResultRef:      formationResultRef(version),
				FormationProvenanceRefs: formationProvenanceRefs(version),
			})
		}
	}
	for _, request := range requests {
		entry := archaeodomain.RequestProvenance{
			RequestID:           request.ID,
			Kind:                request.Kind,
			Status:              request.Status,
			CorrelationID:       request.CorrelationID,
			IdempotencyKey:      request.IdempotencyKey,
			RequestedBy:         request.RequestedBy,
			ExplorationID:       request.ExplorationID,
			SnapshotID:          request.SnapshotID,
			PlanID:              request.PlanID,
			PlanVersion:         intPointerClone(request.PlanVersion),
			BasedOnRevision:     request.BasedOnRevision,
			SubjectRefs:         append([]string(nil), request.SubjectRefs...),
			FulfillmentRef:      request.FulfillmentRef,
			SupersedesRequestID: request.SupersedesRequestID,
			InvalidationReason:  request.InvalidationReason,
			RequestedAt:         request.RequestedAt,
			CompletedAt:         request.CompletedAt,
		}
		if request.Fulfillment != nil {
			entry.FulfillmentValidity = request.Fulfillment.Validity
		}
		record.Requests = append(record.Requests, entry)
		if !request.RequestedAt.IsZero() && (record.LastRequestAt == nil || record.LastRequestAt.Before(request.RequestedAt)) {
			value := request.RequestedAt
			record.LastRequestAt = &value
		}
	}
	for _, mutation := range mutations {
		record.Mutations = append(record.Mutations, archaeodomain.MutationProvenance{
			MutationID:          mutation.ID,
			Category:            mutation.Category,
			Impact:              mutation.Impact,
			Disposition:         mutation.Disposition,
			Blocking:            mutation.Blocking,
			SourceKind:          mutation.SourceKind,
			SourceRef:           mutation.SourceRef,
			BasedOnRevision:     mutation.BasedOnRevision,
			SemanticSnapshotRef: mutation.SemanticSnapshotRef,
			Description:         mutation.Description,
			CreatedAt:           mutation.CreatedAt,
		})
		if !mutation.CreatedAt.IsZero() && (record.LastMutationAt == nil || record.LastMutationAt.Before(mutation.CreatedAt)) {
			value := mutation.CreatedAt
			record.LastMutationAt = &value
		}
	}
	return record, nil
}

func learningCommentRef(interaction archaeolearning.Interaction) string {
	if interaction.Resolution == nil {
		return ""
	}
	return strings.TrimSpace(interaction.Resolution.CommentRef)
}

func evidenceRefs(values []archaeolearning.EvidenceRef) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if ref := strings.TrimSpace(value.RefID); ref != "" {
			out = append(out, ref)
		}
	}
	return out
}

func relatedMutationIDsForLearning(mutations []archaeodomain.MutationEvent, interaction archaeolearning.Interaction) []string {
	out := make([]string, 0)
	for _, mutation := range mutations {
		if strings.TrimSpace(mutation.SourceRef) == strings.TrimSpace(interaction.ID) || stringValue(mutation.Metadata["interaction_id"]) == strings.TrimSpace(interaction.ID) {
			out = append(out, mutation.ID)
		}
	}
	return out
}

func relatedMutationIDsForTension(mutations []archaeodomain.MutationEvent, tension archaeodomain.Tension) []string {
	out := make([]string, 0)
	for _, mutation := range mutations {
		if strings.TrimSpace(mutation.SourceRef) == strings.TrimSpace(tension.ID) || stringValue(mutation.Metadata["tension_id"]) == strings.TrimSpace(tension.ID) {
			out = append(out, mutation.ID)
		}
	}
	return out
}

func relatedMutationIDsForVersion(mutations []archaeodomain.MutationEvent, version archaeodomain.VersionedLivingPlan) []string {
	out := make([]string, 0)
	for _, mutation := range mutations {
		if strings.TrimSpace(mutation.PlanID) != strings.TrimSpace(version.Plan.ID) {
			continue
		}
		if mutation.PlanVersion != nil && *mutation.PlanVersion == version.Version {
			out = append(out, mutation.ID)
			continue
		}
		if intValue(mutation.Metadata["plan_version"]) == version.Version {
			out = append(out, mutation.ID)
		}
	}
	return out
}

func formationResultRef(version archaeodomain.VersionedLivingPlan) string {
	return strings.TrimSpace(version.FormationResultRef)
}

func formationProvenanceRefs(version archaeodomain.VersionedLivingPlan) []string {
	return append([]string(nil), version.FormationProvenanceRefs...)
}

func stringValue(raw any) string {
	if typed, ok := raw.(string); ok {
		return typed
	}
	return ""
}

func stringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func intValue(raw any) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func intPointerClone(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
