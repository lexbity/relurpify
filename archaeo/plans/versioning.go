package plans

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/domain"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/framework/memory"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

const versionArtifactKind = "archaeo_living_plan_version"

type DraftVersionInput struct {
	WorkflowID              string
	DerivedFromExploration  string
	BasedOnRevision         string
	SemanticSnapshotRef     string
	CommentRefs             []string
	TensionRefs             []string
	PatternRefs             []string
	AnchorRefs              []string
	FormationResultRef      string
	FormationProvenanceRefs []string
}

type ExplorationAlignmentInput struct {
	ExplorationID       string
	SnapshotID          string
	BasedOnRevision     string
	SemanticSnapshotRef string
	PatternRefs         []string
	AnchorRefs          []string
	TensionRefs         []string
}

func (s Service) ArchiveVersion(ctx context.Context, workflowID string, version int, reason string) (*archaeodomain.VersionedLivingPlan, error) {
	record, err := s.LoadVersion(ctx, workflowID, version)
	if err != nil || record == nil {
		return record, err
	}
	now := s.now()
	record.Status = archaeodomain.LivingPlanVersionArchived
	record.RecomputeRequired = true
	record.StaleReason = strings.TrimSpace(reason)
	record.UpdatedAt = now
	if err := s.saveVersion(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

func (s Service) MarkVersionStale(ctx context.Context, workflowID string, version int, reason string) (*archaeodomain.VersionedLivingPlan, error) {
	record, err := s.LoadVersion(ctx, workflowID, version)
	if err != nil || record == nil {
		return record, err
	}
	record.RecomputeRequired = true
	record.StaleReason = strings.TrimSpace(reason)
	record.UpdatedAt = s.now()
	if err := s.saveVersion(ctx, record); err != nil {
		return nil, err
	}
	if err := s.appendPlanStalenessMutation(ctx, record, strings.TrimSpace(reason)); err != nil {
		return nil, err
	}
	return record, nil
}

func (s Service) EnsureDraftSuccessor(ctx context.Context, workflowID string, baseVersion int, reason string) (*archaeodomain.VersionedLivingPlan, error) {
	versions, err := s.ListVersions(ctx, workflowID)
	if err != nil || len(versions) == 0 {
		return nil, err
	}
	latest := versions[len(versions)-1]
	if latest.Status == archaeodomain.LivingPlanVersionDraft && latest.ParentVersion != nil && *latest.ParentVersion == baseVersion {
		return &latest, nil
	}
	return s.ensureDraftSuccessorWithInput(ctx, workflowID, baseVersion, reason, DraftVersionInput{})
}

func (s Service) SyncActiveVersionWithExploration(ctx context.Context, workflowID string, snapshot *archaeoarch.ExplorationSnapshot) (*archaeodomain.VersionedLivingPlan, error) {
	if snapshot == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	active, err := s.LoadActiveVersion(ctx, workflowID)
	if err != nil || active == nil {
		return nil, err
	}
	var reasons []string
	if revisionChanged(active.BasedOnRevision, snapshot.BasedOnRevision) {
		reasons = append(reasons, fmt.Sprintf("revision changed: %s -> %s", active.BasedOnRevision, snapshot.BasedOnRevision))
	}
	if active.SemanticSnapshotRef != strings.TrimSpace(snapshot.ID) && active.SemanticSnapshotRef != strings.TrimSpace(snapshot.SemanticSnapshotRef) {
		reasons = append(reasons, "semantic snapshot changed")
	}
	if !stringSetEqual(active.PatternRefs, snapshot.CandidatePatternRefs) {
		reasons = append(reasons, "candidate patterns changed")
	}
	if !stringSetEqual(active.AnchorRefs, snapshot.CandidateAnchorRefs) {
		reasons = append(reasons, "candidate anchors changed")
	}
	if !stringSetEqual(active.TensionRefs, snapshot.TensionIDs) {
		reasons = append(reasons, "tensions changed")
	}
	if len(reasons) == 0 {
		return nil, nil
	}
	reason := strings.Join(reasons, "; ")
	if _, err := s.MarkVersionStale(ctx, workflowID, active.Version, reason); err != nil {
		return nil, err
	}
	return s.ensureDraftSuccessorWithInput(ctx, workflowID, active.Version, reason, DraftVersionInput{
		WorkflowID:             workflowID,
		DerivedFromExploration: snapshot.ExplorationID,
		BasedOnRevision:        snapshot.BasedOnRevision,
		SemanticSnapshotRef:    firstNonEmpty(snapshot.SemanticSnapshotRef, snapshot.ID),
		PatternRefs:            append([]string(nil), snapshot.CandidatePatternRefs...),
		AnchorRefs:             append([]string(nil), snapshot.CandidateAnchorRefs...),
		TensionRefs:            append([]string(nil), snapshot.TensionIDs...),
	})
}

func (s Service) DraftVersion(ctx context.Context, plan *frameworkplan.LivingPlan, input DraftVersionInput) (*archaeodomain.VersionedLivingPlan, error) {
	if s.Store == nil || s.workflowStore() == nil || plan == nil {
		return nil, nil
	}
	workflowID := firstNonEmpty(strings.TrimSpace(input.WorkflowID), plan.WorkflowID)
	if workflowID == "" {
		return nil, fmt.Errorf("workflow id required")
	}
	versions, err := s.ListVersions(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	nextVersion := 1
	var parentVersion *int
	if len(versions) > 0 {
		latest := versions[len(versions)-1]
		nextVersion = latest.Version + 1
		parentVersion = &latest.Version
	}
	plan.WorkflowID = workflowID
	plan.Version = nextVersion
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = now
	}
	plan.UpdatedAt = now
	record := archaeodomain.VersionedLivingPlan{
		ID:                      firstNonEmpty(strings.TrimSpace(plan.ID), fmt.Sprintf("plan-%s-v%d", workflowID, nextVersion)),
		WorkflowID:              workflowID,
		Version:                 nextVersion,
		ParentVersion:           parentVersion,
		DerivedFromExploration:  strings.TrimSpace(input.DerivedFromExploration),
		BasedOnRevision:         strings.TrimSpace(input.BasedOnRevision),
		SemanticSnapshotRef:     strings.TrimSpace(input.SemanticSnapshotRef),
		Status:                  archaeodomain.LivingPlanVersionDraft,
		ComputedAt:              now,
		Plan:                    *plan,
		CommentRefs:             append([]string(nil), input.CommentRefs...),
		TensionRefs:             append([]string(nil), input.TensionRefs...),
		PatternRefs:             append([]string(nil), input.PatternRefs...),
		AnchorRefs:              append([]string(nil), input.AnchorRefs...),
		FormationResultRef:      strings.TrimSpace(input.FormationResultRef),
		FormationProvenanceRefs: append([]string(nil), input.FormationProvenanceRefs...),
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if strings.TrimSpace(plan.ID) == "" {
		plan.ID = record.ID
		record.Plan.ID = record.ID
	}
	if err := s.Store.SavePlan(ctx, plan); err != nil {
		return nil, err
	}
	if err := s.saveVersion(ctx, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (s Service) ActivateVersion(ctx context.Context, workflowID string, version int) (*archaeodomain.VersionedLivingPlan, error) {
	if s.workflowStore() == nil || strings.TrimSpace(workflowID) == "" || version <= 0 {
		return nil, nil
	}
	versions, err := s.ListVersions(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var active *archaeodomain.VersionedLivingPlan
	for i := range versions {
		record := versions[i]
		if record.Version == version {
			record.Status = archaeodomain.LivingPlanVersionActive
			record.ActivatedAt = &now
			record.UpdatedAt = now
			active = &record
		} else if record.Status == archaeodomain.LivingPlanVersionActive {
			record.Status = archaeodomain.LivingPlanVersionSuperseded
			record.SupersededAt = &now
			record.UpdatedAt = now
		}
		if err := s.saveVersion(ctx, &record); err != nil {
			return nil, err
		}
	}
	if active == nil {
		return nil, fmt.Errorf("plan version %d not found", version)
	}
	return active, nil
}

func (s Service) EnsureActiveVersion(ctx context.Context, workflowID string, plan *frameworkplan.LivingPlan, input DraftVersionInput) (*archaeodomain.VersionedLivingPlan, error) {
	if plan == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	active, err := s.LoadActiveVersion(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if active != nil && active.Plan.ID == plan.ID && active.Version == plan.Version {
		return active, nil
	}
	if active == nil {
		record, err := s.DraftVersion(ctx, plan, input)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, nil
		}
		return s.ActivateVersion(ctx, workflowID, record.Version)
	}
	record, err := s.DraftVersion(ctx, plan, input)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return active, nil
	}
	return s.ActivateVersion(ctx, workflowID, record.Version)
}

func (s Service) saveVersion(ctx context.Context, version *archaeodomain.VersionedLivingPlan) error {
	store := s.workflowStore()
	if store == nil || version == nil {
		return nil
	}
	raw, err := json.Marshal(version)
	if err != nil {
		return err
	}
	if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      fmt.Sprintf("archaeo-plan-version:%s:%d", version.WorkflowID, version.Version),
		WorkflowID:      version.WorkflowID,
		Kind:            versionArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     fmt.Sprintf("living plan version %d (%s)", version.Version, version.Status),
		SummaryMetadata: map[string]any{"version": version.Version, "status": version.Status, "plan_id": version.Plan.ID},
		InlineRawText:   string(raw),
		CreatedAt:       s.now(),
	}); err != nil {
		return err
	}
	eventType := archaeoevents.EventPlanVersionUpserted
	switch version.Status {
	case archaeodomain.LivingPlanVersionActive:
		eventType = archaeoevents.EventPlanVersionActivated
	case archaeodomain.LivingPlanVersionArchived, archaeodomain.LivingPlanVersionSuperseded:
		eventType = archaeoevents.EventPlanVersionArchived
	}
	return archaeoevents.AppendWorkflowEvent(ctx, store, version.WorkflowID, eventType, fmt.Sprintf("plan version %d", version.Version), map[string]any{
		"plan_id":                  version.Plan.ID,
		"version":                  version.Version,
		"status":                   version.Status,
		"parent_version":           version.ParentVersion,
		"derived_from_exploration": version.DerivedFromExploration,
		"based_on_revision":        version.BasedOnRevision,
		"semantic_snapshot_ref":    version.SemanticSnapshotRef,
		"recompute_required":       version.RecomputeRequired,
	}, s.now())
}

func clonePlan(plan frameworkplan.LivingPlan) (frameworkplan.LivingPlan, error) {
	raw, err := json.Marshal(plan)
	if err != nil {
		return frameworkplan.LivingPlan{}, err
	}
	var clone frameworkplan.LivingPlan
	if err := json.Unmarshal(raw, &clone); err != nil {
		return frameworkplan.LivingPlan{}, err
	}
	return clone, nil
}

func (s Service) ensureDraftSuccessorWithInput(ctx context.Context, workflowID string, baseVersion int, reason string, override DraftVersionInput) (*archaeodomain.VersionedLivingPlan, error) {
	versions, err := s.ListVersions(ctx, workflowID)
	if err != nil || len(versions) == 0 {
		return nil, err
	}
	latest := versions[len(versions)-1]
	base, err := s.LoadVersion(ctx, workflowID, baseVersion)
	if err != nil || base == nil {
		return nil, err
	}
	if latest.Status == archaeodomain.LivingPlanVersionDraft && latest.ParentVersion != nil && *latest.ParentVersion == baseVersion {
		latest.DerivedFromExploration = firstNonEmpty(override.DerivedFromExploration, latest.DerivedFromExploration, base.DerivedFromExploration)
		latest.BasedOnRevision = firstNonEmpty(override.BasedOnRevision, latest.BasedOnRevision, base.BasedOnRevision)
		latest.SemanticSnapshotRef = firstNonEmpty(override.SemanticSnapshotRef, latest.SemanticSnapshotRef, base.SemanticSnapshotRef)
		if len(override.PatternRefs) > 0 {
			latest.PatternRefs = append([]string(nil), override.PatternRefs...)
		}
		if len(override.AnchorRefs) > 0 {
			latest.AnchorRefs = append([]string(nil), override.AnchorRefs...)
		}
		if len(override.TensionRefs) > 0 {
			latest.TensionRefs = append([]string(nil), override.TensionRefs...)
		}
		latest.StaleReason = strings.TrimSpace(reason)
		latest.UpdatedAt = s.now()
		if err := s.saveVersion(ctx, &latest); err != nil {
			return nil, err
		}
		if err := s.appendSuccessorMutation(ctx, &latest, strings.TrimSpace(reason)); err != nil {
			return nil, err
		}
		return &latest, nil
	}
	clone, err := clonePlan(base.Plan)
	if err != nil {
		return nil, err
	}
	nextVersion := latest.Version + 1
	if base.Plan.ID != "" {
		clone.ID = fmt.Sprintf("%s-v%d", strings.TrimSpace(base.Plan.ID), nextVersion)
	}
	record, err := s.DraftVersion(ctx, &clone, DraftVersionInput{
		WorkflowID:              workflowID,
		DerivedFromExploration:  firstNonEmpty(override.DerivedFromExploration, base.DerivedFromExploration),
		BasedOnRevision:         firstNonEmpty(override.BasedOnRevision, base.BasedOnRevision),
		SemanticSnapshotRef:     firstNonEmpty(override.SemanticSnapshotRef, base.SemanticSnapshotRef),
		CommentRefs:             append([]string(nil), base.CommentRefs...),
		TensionRefs:             firstNonEmptySlice(override.TensionRefs, base.TensionRefs),
		PatternRefs:             firstNonEmptySlice(override.PatternRefs, base.PatternRefs),
		AnchorRefs:              firstNonEmptySlice(override.AnchorRefs, base.AnchorRefs),
		FormationResultRef:      firstNonEmpty(override.FormationResultRef, base.FormationResultRef),
		FormationProvenanceRefs: firstNonEmptySlice(override.FormationProvenanceRefs, base.FormationProvenanceRefs),
	})
	if err != nil || record == nil {
		return record, err
	}
	record.StaleReason = strings.TrimSpace(reason)
	record.UpdatedAt = s.now()
	if err := s.saveVersion(ctx, record); err != nil {
		return nil, err
	}
	if err := s.appendSuccessorMutation(ctx, record, strings.TrimSpace(reason)); err != nil {
		return nil, err
	}
	return record, nil
}

func stringSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, value := range a {
		seen[strings.TrimSpace(value)]++
	}
	for _, value := range b {
		key := strings.TrimSpace(value)
		if seen[key] == 0 {
			return false
		}
		seen[key]--
	}
	for _, remaining := range seen {
		if remaining != 0 {
			return false
		}
	}
	return true
}

func firstNonEmptySlice(primary, fallback []string) []string {
	if len(primary) > 0 {
		return append([]string(nil), primary...)
	}
	return append([]string(nil), fallback...)
}

func revisionChanged(existing, updated string) bool {
	existing = strings.TrimSpace(existing)
	updated = strings.TrimSpace(updated)
	return existing != "" && updated != "" && existing != updated
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

func (s Service) appendPlanStalenessMutation(ctx context.Context, version *archaeodomain.VersionedLivingPlan, reason string) error {
	store := s.workflowStore()
	if store == nil || version == nil {
		return nil
	}
	return archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:          version.WorkflowID,
		PlanID:              version.Plan.ID,
		PlanVersion:         &version.Version,
		Category:            archaeodomain.MutationPlanStaleness,
		SourceKind:          "plan_version",
		SourceRef:           fmt.Sprintf("%s:%d", version.Plan.ID, version.Version),
		Description:         firstNonEmpty(reason, "active plan version marked stale"),
		BlastRadius:         archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan, AffectedStepIDs: append([]string(nil), version.Plan.StepOrder...), EstimatedCount: len(version.Plan.StepOrder)},
		Impact:              archaeodomain.ImpactPlanRecomputeRequired,
		Disposition:         archaeodomain.DispositionRequireReplan,
		Blocking:            true,
		BasedOnRevision:     version.BasedOnRevision,
		SemanticSnapshotRef: version.SemanticSnapshotRef,
		Metadata: map[string]any{
			"plan_id":             version.Plan.ID,
			"plan_version":        version.Version,
			"status":              string(version.Status),
			"recompute_required":  version.RecomputeRequired,
			"stale_reason":        version.StaleReason,
			"derived_exploration": version.DerivedFromExploration,
		},
		CreatedAt: s.now(),
	})
}

func (s Service) appendSuccessorMutation(ctx context.Context, version *archaeodomain.VersionedLivingPlan, reason string) error {
	store := s.workflowStore()
	if store == nil || version == nil {
		return nil
	}
	return archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:          version.WorkflowID,
		PlanID:              version.Plan.ID,
		PlanVersion:         &version.Version,
		Category:            archaeodomain.MutationPlanStaleness,
		SourceKind:          "plan_successor",
		SourceRef:           fmt.Sprintf("%s:%d", version.Plan.ID, version.Version),
		Description:         firstNonEmpty(reason, "successor draft version created"),
		BlastRadius:         archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan, AffectedStepIDs: append([]string(nil), version.Plan.StepOrder...), EstimatedCount: len(version.Plan.StepOrder)},
		Impact:              archaeodomain.ImpactPlanRecomputeRequired,
		Disposition:         archaeodomain.DispositionContinueOnStalePlan,
		BasedOnRevision:     version.BasedOnRevision,
		SemanticSnapshotRef: version.SemanticSnapshotRef,
		Metadata: map[string]any{
			"plan_id":               version.Plan.ID,
			"plan_version":          version.Version,
			"parent_version":        version.ParentVersion,
			"status":                string(version.Status),
			"derived_exploration":   version.DerivedFromExploration,
			"semantic_snapshot_ref": version.SemanticSnapshotRef,
		},
		CreatedAt: s.now(),
	})
}
