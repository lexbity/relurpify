package tensions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	"codeburg.org/lexbit/relurpify/archaeo/internal/storeutil"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

const tensionArtifactKind = "archaeo_tension"

type CreateInput struct {
	WorkflowID         string
	ExplorationID      string
	SnapshotID         string
	SourceRef          string
	PatternIDs         []string
	AnchorRefs         []string
	SymbolScope        []string
	Kind               string
	Description        string
	Severity           string
	Status             archaeodomain.TensionStatus
	BlastRadiusNodeIDs []string
	RelatedPlanStepIDs []string
	CommentRefs        []string
	BasedOnRevision    string
}

type Service struct {
	Store memory.WorkflowStateStore
	Now   func() time.Time
	NewID func(prefix string) string
}

func (s Service) CreateOrUpdate(ctx context.Context, input CreateInput) (*archaeodomain.Tension, error) {
	if s.Store == nil || strings.TrimSpace(input.WorkflowID) == "" || strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Description) == "" {
		return nil, nil
	}
	if existing, err := s.findBySource(ctx, input.WorkflowID, input.SourceRef); err != nil {
		return nil, err
	} else if existing != nil {
		previous := *existing
		existing.PatternIDs = cloneStrings(input.PatternIDs)
		existing.AnchorRefs = cloneStrings(input.AnchorRefs)
		existing.SymbolScope = cloneStrings(input.SymbolScope)
		existing.Description = strings.TrimSpace(input.Description)
		existing.Severity = strings.TrimSpace(input.Severity)
		existing.BlastRadiusNodeIDs = cloneStrings(input.BlastRadiusNodeIDs)
		existing.RelatedPlanStepIDs = cloneStrings(input.RelatedPlanStepIDs)
		existing.CommentRefs = cloneStrings(input.CommentRefs)
		existing.BasedOnRevision = strings.TrimSpace(input.BasedOnRevision)
		existing.ExplorationID = firstNonEmpty(input.ExplorationID, existing.ExplorationID)
		existing.SnapshotID = firstNonEmpty(input.SnapshotID, existing.SnapshotID)
		if input.Status != "" {
			existing.Status = input.Status
		}
		existing.UpdatedAt = s.now()
		if err := s.save(ctx, existing); err != nil {
			return nil, err
		}
		if err := s.appendMutation(ctx, existing, &previous, "tension updated"); err != nil {
			return nil, err
		}
		return existing, nil
	}
	record := &archaeodomain.Tension{
		ID:                 s.newID("tension"),
		WorkflowID:         strings.TrimSpace(input.WorkflowID),
		ExplorationID:      strings.TrimSpace(input.ExplorationID),
		SnapshotID:         strings.TrimSpace(input.SnapshotID),
		SourceRef:          strings.TrimSpace(input.SourceRef),
		PatternIDs:         cloneStrings(input.PatternIDs),
		AnchorRefs:         cloneStrings(input.AnchorRefs),
		SymbolScope:        cloneStrings(input.SymbolScope),
		Kind:               strings.TrimSpace(input.Kind),
		Description:        strings.TrimSpace(input.Description),
		Severity:           strings.TrimSpace(input.Severity),
		Status:             normalizeStatus(input.Status),
		BlastRadiusNodeIDs: cloneStrings(input.BlastRadiusNodeIDs),
		RelatedPlanStepIDs: cloneStrings(input.RelatedPlanStepIDs),
		CommentRefs:        cloneStrings(input.CommentRefs),
		BasedOnRevision:    strings.TrimSpace(input.BasedOnRevision),
		CreatedAt:          s.now(),
		UpdatedAt:          s.now(),
	}
	if err := s.save(ctx, record); err != nil {
		return nil, err
	}
	if err := s.appendMutation(ctx, record, nil, "tension created"); err != nil {
		return nil, err
	}
	return record, nil
}

func (s Service) UpdateStatus(ctx context.Context, workflowID, tensionID string, status archaeodomain.TensionStatus, commentRefs []string) (*archaeodomain.Tension, error) {
	record, err := s.Load(ctx, workflowID, tensionID)
	if err != nil || record == nil {
		return record, err
	}
	previous := *record
	record.Status = normalizeStatus(status)
	if len(commentRefs) > 0 {
		record.CommentRefs = cloneStrings(commentRefs)
	}
	record.UpdatedAt = s.now()
	if err := s.save(ctx, record); err != nil {
		return nil, err
	}
	if err := s.appendMutation(ctx, record, &previous, "tension status updated"); err != nil {
		return nil, err
	}
	return record, nil
}

func (s Service) Load(ctx context.Context, workflowID, tensionID string) (*archaeodomain.Tension, error) {
	if s.Store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(tensionID) == "" {
		return nil, nil
	}
	if artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, s.Store, fmt.Sprintf("archaeo-tension:%s", strings.TrimSpace(tensionID))); err != nil {
		return nil, err
	} else if ok && artifact != nil {
		var record archaeodomain.Tension
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		return &record, nil
	}
	return nil, nil
}

func (s Service) ListByWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.Tension, error) {
	if s.Store == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKind(ctx, s.Store, workflowID, "", tensionArtifactKind)
	if err != nil {
		return nil, err
	}
	var out []archaeodomain.Tension
	for _, artifact := range artifacts {
		var record archaeodomain.Tension
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, nil
}

func (s Service) ActiveTensions(ctx context.Context) ([]string, error) {
	if s.Store == nil {
		return nil, nil
	}
	workflows, err := s.Store.ListWorkflows(ctx, 1024)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, workflow := range workflows {
		tensions, err := s.ListByWorkflow(ctx, workflow.WorkflowID)
		if err != nil {
			return nil, err
		}
		for _, tension := range tensions {
			if isActiveStatus(tension.Status) {
				out = append(out, tension.ID)
			}
		}
	}
	return out, nil
}

func (s Service) ActiveByWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.Tension, error) {
	records, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	out := make([]archaeodomain.Tension, 0, len(records))
	for _, record := range records {
		if isActiveStatus(record.Status) {
			out = append(out, record)
		}
	}
	return out, nil
}

func (s Service) ListByExploration(ctx context.Context, explorationID string) ([]archaeodomain.Tension, error) {
	if s.Store == nil || strings.TrimSpace(explorationID) == "" {
		return nil, nil
	}
	workflows, err := s.Store.ListWorkflows(ctx, 1024)
	if err != nil {
		return nil, err
	}
	var out []archaeodomain.Tension
	for _, workflow := range workflows {
		records, err := s.ListByWorkflow(ctx, workflow.WorkflowID)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			if strings.TrimSpace(record.ExplorationID) == strings.TrimSpace(explorationID) {
				out = append(out, record)
			}
		}
	}
	return out, nil
}

func (s Service) SummaryByWorkflow(ctx context.Context, workflowID string) (*archaeodomain.TensionSummary, error) {
	records, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	summary := summarizeTensions(records)
	if summary == nil {
		return nil, nil
	}
	summary.WorkflowID = strings.TrimSpace(workflowID)
	return summary, nil
}

func (s Service) SummaryByExploration(ctx context.Context, explorationID string) (*archaeodomain.TensionSummary, error) {
	records, err := s.ListByExploration(ctx, explorationID)
	if err != nil {
		return nil, err
	}
	summary := summarizeTensions(records)
	if summary == nil {
		return nil, nil
	}
	summary.ExplorationID = strings.TrimSpace(explorationID)
	return summary, nil
}

func (s Service) Update(ctx context.Context, record *archaeodomain.Tension) (*archaeodomain.Tension, error) {
	if record == nil {
		return nil, nil
	}
	var previous *archaeodomain.Tension
	if loaded, err := s.Load(ctx, record.WorkflowID, record.ID); err != nil {
		return nil, err
	} else if loaded != nil {
		copy := *loaded
		previous = &copy
	}
	record.UpdatedAt = s.now()
	if err := s.save(ctx, record); err != nil {
		return nil, err
	}
	if err := s.appendMutation(ctx, record, previous, "tension updated"); err != nil {
		return nil, err
	}
	return record, nil
}

func (s Service) findBySource(ctx context.Context, workflowID, sourceRef string) (*archaeodomain.Tension, error) {
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceRef == "" {
		return nil, nil
	}
	records, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].SourceRef == sourceRef {
			record := records[i]
			return &record, nil
		}
	}
	return nil, nil
}

func (s Service) save(ctx context.Context, record *archaeodomain.Tension) error {
	if s.Store == nil || record == nil {
		return nil
	}
	now := s.now()
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := s.Store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      fmt.Sprintf("archaeo-tension:%s", record.ID),
		WorkflowID:      record.WorkflowID,
		Kind:            tensionArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     fmt.Sprintf("tension %s (%s)", record.ID, record.Status),
		SummaryMetadata: map[string]any{"tension_id": record.ID, "status": record.Status, "severity": record.Severity},
		InlineRawText:   string(raw),
		CreatedAt:       now,
	}); err != nil {
		return err
	}
	return archaeoevents.AppendWorkflowEvent(ctx, s.Store, record.WorkflowID, archaeoevents.EventTensionUpserted, record.Description, map[string]any{
		"tension_id":       record.ID,
		"status":           record.Status,
		"severity":         record.Severity,
		"source_ref":       record.SourceRef,
		"exploration_id":   record.ExplorationID,
		"snapshot_id":      record.SnapshotID,
		"kind":             record.Kind,
		"related_step_ids": record.RelatedPlanStepIDs,
	}, now)
}

func (s Service) appendMutation(ctx context.Context, record *archaeodomain.Tension, previous *archaeodomain.Tension, description string) error {
	if s.Store == nil || record == nil {
		return nil
	}
	if previous != nil &&
		previous.Status == record.Status &&
		strings.TrimSpace(previous.Description) == strings.TrimSpace(record.Description) &&
		stringSlicesEqual(previous.RelatedPlanStepIDs, record.RelatedPlanStepIDs) &&
		stringSlicesEqual(previous.BlastRadiusNodeIDs, record.BlastRadiusNodeIDs) {
		return nil
	}
	scope := archaeodomain.BlastRadiusLocal
	if len(record.RelatedPlanStepIDs) > 0 {
		scope = archaeodomain.BlastRadiusStep
	} else if len(record.BlastRadiusNodeIDs) > 0 {
		scope = archaeodomain.BlastRadiusWorkflow
	}
	mutation := archaeodomain.MutationEvent{
		WorkflowID:    record.WorkflowID,
		ExplorationID: record.ExplorationID,
		Category:      archaeodomain.MutationConfidenceChange,
		SourceKind:    "tension",
		SourceRef:     record.ID,
		Description:   strings.TrimSpace(description),
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           scope,
			AffectedStepIDs: append([]string(nil), record.RelatedPlanStepIDs...),
			AffectedNodeIDs: append([]string(nil), record.BlastRadiusNodeIDs...),
			EstimatedCount:  len(record.RelatedPlanStepIDs) + len(record.BlastRadiusNodeIDs),
		},
		Impact:          archaeodomain.ImpactAdvisory,
		Disposition:     archaeodomain.DispositionContinue,
		BasedOnRevision: record.BasedOnRevision,
		Metadata: map[string]any{
			"tension_id": record.ID,
			"status":     string(record.Status),
			"previous_status": func() string {
				if previous == nil {
					return ""
				}
				return string(previous.Status)
			}(),
			"kind":     record.Kind,
			"severity": record.Severity,
		},
		CreatedAt: s.now(),
	}
	switch record.Status {
	case archaeodomain.TensionUnresolved, archaeodomain.TensionConfirmed:
		if len(record.RelatedPlanStepIDs) > 0 {
			mutation.Category = archaeodomain.MutationStepInvalidation
			mutation.Impact = archaeodomain.ImpactLocalBlocking
			mutation.Disposition = archaeodomain.DispositionInvalidateStep
			mutation.Blocking = true
		} else {
			mutation.Category = archaeodomain.MutationBlockingSemantic
			mutation.Impact = archaeodomain.ImpactCaution
			mutation.Disposition = archaeodomain.DispositionPauseForGuidance
			mutation.Blocking = true
		}
	case archaeodomain.TensionAccepted:
		mutation.Category = archaeodomain.MutationConfidenceChange
		mutation.Impact = archaeodomain.ImpactAdvisory
		mutation.Disposition = archaeodomain.DispositionContinueOnStalePlan
	case archaeodomain.TensionResolved:
		mutation.Category = archaeodomain.MutationObservation
		mutation.Impact = archaeodomain.ImpactInformational
		mutation.Disposition = archaeodomain.DispositionContinue
	}
	return archaeoevents.AppendMutationEvent(ctx, s.Store, mutation)
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeStatus(status archaeodomain.TensionStatus) archaeodomain.TensionStatus {
	switch status {
	case archaeodomain.TensionConfirmed, archaeodomain.TensionAccepted, archaeodomain.TensionUnresolved, archaeodomain.TensionResolved:
		return status
	default:
		return archaeodomain.TensionInferred
	}
}

func isActiveStatus(status archaeodomain.TensionStatus) bool {
	switch status {
	case archaeodomain.TensionAccepted, archaeodomain.TensionResolved:
		return false
	default:
		return true
	}
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID(prefix string) string {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	return fmt.Sprintf("%s-%d", strings.TrimSpace(prefix), s.now().UnixNano())
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func summarizeTensions(records []archaeodomain.Tension) *archaeodomain.TensionSummary {
	if len(records) == 0 {
		return &archaeodomain.TensionSummary{
			BySeverity: map[string]int{},
			ByKind:     map[string]int{},
		}
	}
	summary := &archaeodomain.TensionSummary{
		Total:      len(records),
		BySeverity: map[string]int{},
		ByKind:     map[string]int{},
	}
	var latest *time.Time
	for _, record := range records {
		if isActiveStatus(record.Status) {
			summary.Active++
		}
		switch record.Status {
		case archaeodomain.TensionAccepted:
			summary.Accepted++
			summary.AcceptedDebt++
		case archaeodomain.TensionResolved:
			summary.Resolved++
		case archaeodomain.TensionUnresolved:
			summary.Unresolved++
			summary.BlockingCount++
		case archaeodomain.TensionConfirmed:
			summary.BlockingCount++
		}
		if severity := strings.TrimSpace(record.Severity); severity != "" {
			summary.BySeverity[severity]++
		}
		if kind := strings.TrimSpace(record.Kind); kind != "" {
			summary.ByKind[kind]++
		}
		if latest == nil || record.UpdatedAt.After(*latest) {
			value := record.UpdatedAt
			latest = &value
		}
	}
	summary.LatestUpdatedAt = latest
	return summary
}
