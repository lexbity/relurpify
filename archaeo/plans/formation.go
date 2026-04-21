package plans

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/archaeo/internal/storeutil"
	archaeorequests "codeburg.org/lexbit/relurpify/archaeo/requests"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

const formationArtifactKind = "archaeo_plan_formation_result"

type FormationInput struct {
	WorkflowID       string
	ExplorationID    string
	SnapshotID       string
	BasedOnRevision  string
	SemanticSnapshot string
	PatternRefs      []string
	AnchorRefs       []string
	TensionRefs      []string
	PendingLearning  []string
	RequestRefs      []string
	MutationRefs     []string
	ProvenanceRefs   []string
	AcceptedTensions []string
	ResolvedLearning []string
	ActiveVersion    *int
	DraftVersions    []int
	ConvergenceState string
}

func (s Service) EnsureDraftFromExploration(ctx context.Context, input FormationInput) (*archaeodomain.VersionedLivingPlan, error) {
	workflowID := strings.TrimSpace(input.WorkflowID)
	if s.Store == nil || s.workflowStore() == nil || workflowID == "" {
		return nil, nil
	}
	aggregate, err := s.buildFormationInput(ctx, input)
	if err != nil {
		return nil, err
	}
	draft, err := s.latestDraftVersion(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if draft != nil && draftMatchesFormationInput(draft, aggregate) {
		return draft, nil
	}
	active, err := s.LoadActiveVersion(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if active != nil && !active.RecomputeRequired && formationMatchesVersion(active, aggregate) {
		return nil, nil
	}
	if active != nil && !formationMatchesVersion(active, aggregate) {
		if _, err := s.MarkVersionStale(ctx, workflowID, active.Version, formationReason(aggregate)); err != nil {
			return nil, err
		}
	}
	plan := s.formPlan(aggregate)
	result := s.buildFormationResult(aggregate, plan)
	record, err := s.DraftVersion(ctx, plan, DraftVersionInput{
		WorkflowID:              workflowID,
		DerivedFromExploration:  strings.TrimSpace(aggregate.ExplorationID),
		BasedOnRevision:         strings.TrimSpace(aggregate.BasedOnRevision),
		SemanticSnapshotRef:     strings.TrimSpace(aggregate.SemanticSnapshot),
		PatternRefs:             uniqueSortedStrings(aggregate.PatternRefs),
		AnchorRefs:              uniqueSortedStrings(aggregate.AnchorRefs),
		TensionRefs:             uniqueSortedStrings(aggregate.TensionRefs),
		FormationProvenanceRefs: uniqueSortedStrings(result.ProvenanceRefs),
	})
	if err != nil || record == nil {
		return record, err
	}
	result.PlanID = record.Plan.ID
	result.PlanVersion = cloneInt(record.Version)
	formationRef, err := s.saveFormationResult(ctx, result)
	if err != nil {
		return nil, err
	}
	record.FormationResultRef = formationRef
	record.FormationProvenanceRefs = uniqueSortedStrings(result.ProvenanceRefs)
	record.UpdatedAt = s.now()
	if err := s.saveVersion(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

func (s Service) buildFormationInput(ctx context.Context, input FormationInput) (FormationInput, error) {
	aggregate := input
	aggregate.PatternRefs = uniqueSortedStrings(aggregate.PatternRefs)
	aggregate.AnchorRefs = uniqueSortedStrings(aggregate.AnchorRefs)
	aggregate.TensionRefs = uniqueSortedStrings(aggregate.TensionRefs)
	aggregate.PendingLearning = uniqueSortedStrings(aggregate.PendingLearning)
	aggregate.RequestRefs = uniqueSortedStrings(aggregate.RequestRefs)
	aggregate.MutationRefs = uniqueSortedStrings(aggregate.MutationRefs)
	aggregate.ProvenanceRefs = uniqueSortedStrings(aggregate.ProvenanceRefs)
	aggregate.AcceptedTensions = uniqueSortedStrings(aggregate.AcceptedTensions)
	aggregate.ResolvedLearning = uniqueSortedStrings(aggregate.ResolvedLearning)
	if s.workflowStore() == nil || strings.TrimSpace(aggregate.WorkflowID) == "" {
		return aggregate, nil
	}
	tensions, err := (archaeotensions.Service{Store: s.workflowStore()}).ListByWorkflow(ctx, aggregate.WorkflowID)
	if err != nil {
		return aggregate, err
	}
	requests, err := (archaeorequests.Service{Store: s.workflowStore()}).ListByWorkflow(ctx, aggregate.WorkflowID)
	if err != nil {
		return aggregate, err
	}
	lineage, err := s.LoadLineage(ctx, aggregate.WorkflowID)
	if err != nil {
		return aggregate, err
	}
	for _, item := range requests {
		aggregate.RequestRefs = append(aggregate.RequestRefs, item.ID)
		if strings.TrimSpace(item.FulfillmentRef) != "" {
			aggregate.ProvenanceRefs = append(aggregate.ProvenanceRefs, item.FulfillmentRef)
		}
	}
	for _, item := range tensions {
		if item.Status == archaeodomain.TensionAccepted {
			aggregate.AcceptedTensions = append(aggregate.AcceptedTensions, item.ID)
		}
		aggregate.ProvenanceRefs = append(aggregate.ProvenanceRefs, item.CommentRefs...)
	}
	if lineage != nil {
		if lineage.ActiveVersion != nil {
			aggregate.ActiveVersion = cloneInt(lineage.ActiveVersion.Version)
		}
		for _, draft := range lineage.DraftVersions {
			aggregate.DraftVersions = append(aggregate.DraftVersions, draft.Version)
		}
	}
	aggregate.RequestRefs = uniqueSortedStrings(aggregate.RequestRefs)
	aggregate.MutationRefs = uniqueSortedStrings(aggregate.MutationRefs)
	aggregate.ProvenanceRefs = uniqueSortedStrings(aggregate.ProvenanceRefs)
	aggregate.AcceptedTensions = uniqueSortedStrings(aggregate.AcceptedTensions)
	aggregate.ResolvedLearning = uniqueSortedStrings(aggregate.ResolvedLearning)
	aggregate.PendingLearning = uniqueSortedStrings(aggregate.PendingLearning)
	return aggregate, nil
}

func (s Service) formPlan(input FormationInput) *frameworkplan.LivingPlan {
	now := s.now()
	patternRefs := uniqueSortedStrings(input.PatternRefs)
	anchorRefs := uniqueSortedStrings(input.AnchorRefs)
	tensionRefs := uniqueSortedStrings(input.TensionRefs)
	learningRefs := uniqueSortedStrings(input.PendingLearning)
	steps := make(map[string]*frameworkplan.PlanStep)
	order := make([]string, 0, 5)
	var previous string

	addStep := func(id, description string, scope, anchors []string, dependsOn []string) {
		step := &frameworkplan.PlanStep{
			ID:                 id,
			Description:        description,
			Scope:              uniqueSortedStrings(scope),
			AnchorDependencies: uniqueSortedStrings(anchors),
			ConfidenceScore:    0.7,
			DependsOn:          append([]string(nil), dependsOn...),
			Status:             frameworkplan.PlanStepPending,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if len(step.AnchorDependencies) > 0 || len(step.Scope) > 0 {
			step.EvidenceGate = &frameworkplan.EvidenceGate{
				RequiredAnchors: append([]string(nil), step.AnchorDependencies...),
				RequiredSymbols: append([]string(nil), step.Scope...),
			}
		}
		for _, symbol := range step.Scope {
			step.InvalidatedBy = append(step.InvalidatedBy, frameworkplan.InvalidationRule{
				Kind:   frameworkplan.InvalidationSymbolChanged,
				Target: symbol,
			})
		}
		for _, anchor := range step.AnchorDependencies {
			step.InvalidatedBy = append(step.InvalidatedBy, frameworkplan.InvalidationRule{
				Kind:   frameworkplan.InvalidationAnchorDrifted,
				Target: anchor,
			})
		}
		steps[id] = step
		order = append(order, id)
		previous = id
	}

	if len(learningRefs) > 0 {
		addStep("resolve_learning", fmt.Sprintf("Resolve %d pending archaeology learning interactions", len(learningRefs)), nil, nil, nil)
	}
	if len(tensionRefs) > 0 {
		addStep("resolve_tensions", fmt.Sprintf("Review %d active tensions before continuing plan evolution", len(tensionRefs)), nil, nil, dependsOn(previous))
	}
	if len(patternRefs) > 0 || len(anchorRefs) > 0 {
		addStep("ground_findings", buildAnalysisDescription(patternRefs, anchorRefs), patternRefs, anchorRefs, dependsOn(previous))
	}
	if len(input.RequestRefs) > 0 || len(input.MutationRefs) > 0 {
		addStep("reconcile_state", buildReconciliationDescription(input.RequestRefs, input.MutationRefs), nil, nil, dependsOn(previous))
	}
	addStep("advance_execution", "Advance the living plan with archaeology-grounded execution work", patternRefs, anchorRefs, dependsOn(previous))

	return &frameworkplan.LivingPlan{
		ID:         fmt.Sprintf("formed-plan-%s", strings.TrimSpace(input.WorkflowID)),
		WorkflowID: strings.TrimSpace(input.WorkflowID),
		Title:      "Archaeology-Grounded Living Plan",
		Steps:      steps,
		StepOrder:  order,
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{
			PatternIDs: append([]string(nil), patternRefs...),
			TensionIDs: append([]string(nil), tensionRefs...),
			Commentary: formationCommentary(input),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s Service) buildFormationResult(input FormationInput, plan *frameworkplan.LivingPlan) *archaeodomain.FormationResult {
	if plan == nil {
		return nil
	}
	return &archaeodomain.FormationResult{
		ID:            fmt.Sprintf("formation:%s:%s", strings.TrimSpace(input.WorkflowID), firstNonEmpty(strings.TrimSpace(input.SnapshotID), "current")),
		WorkflowID:    strings.TrimSpace(input.WorkflowID),
		ExplorationID: strings.TrimSpace(input.ExplorationID),
		SnapshotID:    strings.TrimSpace(input.SnapshotID),
		InputSummary: map[string]any{
			"pattern_refs":      uniqueSortedStrings(input.PatternRefs),
			"anchor_refs":       uniqueSortedStrings(input.AnchorRefs),
			"tension_refs":      uniqueSortedStrings(input.TensionRefs),
			"pending_learning":  uniqueSortedStrings(input.PendingLearning),
			"resolved_learning": uniqueSortedStrings(input.ResolvedLearning),
			"request_refs":      uniqueSortedStrings(input.RequestRefs),
			"mutation_refs":     uniqueSortedStrings(input.MutationRefs),
			"accepted_tensions": uniqueSortedStrings(input.AcceptedTensions),
			"active_version":    derefInt(input.ActiveVersion),
			"draft_versions":    append([]int(nil), input.DraftVersions...),
			"convergence_state": strings.TrimSpace(input.ConvergenceState),
			"based_on_revision": strings.TrimSpace(input.BasedOnRevision),
			"semantic_snapshot": strings.TrimSpace(input.SemanticSnapshot),
		},
		ChosenCandidate: archaeodomain.FormationCandidate{
			ID:                 fmt.Sprintf("candidate:%s:%s", strings.TrimSpace(input.WorkflowID), firstNonEmpty(strings.TrimSpace(input.SnapshotID), "current")),
			Summary:            formationCommentary(input),
			StepIDs:            append([]string(nil), plan.StepOrder...),
			PatternRefs:        uniqueSortedStrings(input.PatternRefs),
			AnchorRefs:         uniqueSortedStrings(input.AnchorRefs),
			TensionRefs:        uniqueSortedStrings(input.TensionRefs),
			PendingLearningIDs: uniqueSortedStrings(input.PendingLearning),
			ProvenanceRefs:     uniqueSortedStrings(input.ProvenanceRefs),
		},
		Rationale:            formationRationale(input),
		UnresolvedTensionIDs: uniqueSortedStrings(input.TensionRefs),
		DeferredUncertainty:  deferredFormationUncertainty(input),
		ProvenanceRefs:       uniqueSortedStrings(input.ProvenanceRefs),
		CreatedAt:            s.now(),
		UpdatedAt:            s.now(),
	}
}

func formationMatchesVersion(record *archaeodomain.VersionedLivingPlan, input FormationInput) bool {
	if record == nil {
		return false
	}
	return strings.TrimSpace(record.DerivedFromExploration) == strings.TrimSpace(input.ExplorationID) &&
		strings.TrimSpace(record.BasedOnRevision) == strings.TrimSpace(input.BasedOnRevision) &&
		strings.TrimSpace(record.SemanticSnapshotRef) == strings.TrimSpace(input.SemanticSnapshot) &&
		stringSetEqual(record.PatternRefs, input.PatternRefs) &&
		stringSetEqual(record.AnchorRefs, input.AnchorRefs) &&
		stringSetEqual(record.TensionRefs, input.TensionRefs) &&
		stringSetEqual(record.FormationProvenanceRefs, input.ProvenanceRefs)
}

func draftMatchesFormationInput(record *archaeodomain.VersionedLivingPlan, input FormationInput) bool {
	if record == nil || record.Status != archaeodomain.LivingPlanVersionDraft {
		return false
	}
	return formationMatchesVersion(record, input)
}

func (s Service) latestDraftVersion(ctx context.Context, workflowID string) (*archaeodomain.VersionedLivingPlan, error) {
	versions, err := s.ListVersions(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	for i := len(versions) - 1; i >= 0; i-- {
		if versions[i].Status == archaeodomain.LivingPlanVersionDraft {
			record := versions[i]
			return &record, nil
		}
	}
	return nil, nil
}

func formationReason(input FormationInput) string {
	parts := []string{"archaeology formation inputs changed"}
	if len(input.PatternRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d pattern refs", len(uniqueSortedStrings(input.PatternRefs))))
	}
	if len(input.AnchorRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d anchor refs", len(uniqueSortedStrings(input.AnchorRefs))))
	}
	if len(input.TensionRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d tension refs", len(uniqueSortedStrings(input.TensionRefs))))
	}
	if len(input.PendingLearning) > 0 {
		parts = append(parts, fmt.Sprintf("%d pending learning interactions", len(uniqueSortedStrings(input.PendingLearning))))
	}
	if len(input.ProvenanceRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d provenance refs", len(uniqueSortedStrings(input.ProvenanceRefs))))
	}
	return strings.Join(parts, "; ")
}

func formationCommentary(input FormationInput) string {
	parts := make([]string, 0, 6)
	if len(input.PatternRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d pattern findings", len(uniqueSortedStrings(input.PatternRefs))))
	}
	if len(input.AnchorRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d drifted anchors", len(uniqueSortedStrings(input.AnchorRefs))))
	}
	if len(input.TensionRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d active tensions", len(uniqueSortedStrings(input.TensionRefs))))
	}
	if len(input.PendingLearning) > 0 {
		parts = append(parts, fmt.Sprintf("%d pending learning interactions", len(uniqueSortedStrings(input.PendingLearning))))
	}
	if len(input.RequestRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d request lineage refs", len(uniqueSortedStrings(input.RequestRefs))))
	}
	if len(input.MutationRefs) > 0 {
		parts = append(parts, fmt.Sprintf("%d mutation refs", len(uniqueSortedStrings(input.MutationRefs))))
	}
	if len(parts) == 0 {
		return "formed from current archaeology state"
	}
	sort.Strings(parts)
	return "formed from " + strings.Join(parts, ", ")
}

func buildAnalysisDescription(patternRefs, anchorRefs []string) string {
	switch {
	case len(patternRefs) > 0 && len(anchorRefs) > 0:
		return fmt.Sprintf("Ground %d surfaced patterns and %d anchor findings into executable work", len(patternRefs), len(anchorRefs))
	case len(patternRefs) > 0:
		return fmt.Sprintf("Ground %d surfaced patterns into executable work", len(patternRefs))
	case len(anchorRefs) > 0:
		return fmt.Sprintf("Ground %d anchor findings into executable work", len(anchorRefs))
	default:
		return "Ground current archaeology findings into executable work"
	}
}

func buildReconciliationDescription(requestRefs, mutationRefs []string) string {
	switch {
	case len(requestRefs) > 0 && len(mutationRefs) > 0:
		return fmt.Sprintf("Reconcile %d request lineage refs against %d mutation refs before continuing", len(uniqueSortedStrings(requestRefs)), len(uniqueSortedStrings(mutationRefs)))
	case len(requestRefs) > 0:
		return fmt.Sprintf("Reconcile %d request lineage refs before continuing", len(uniqueSortedStrings(requestRefs)))
	case len(mutationRefs) > 0:
		return fmt.Sprintf("Reconcile %d mutation refs before continuing", len(uniqueSortedStrings(mutationRefs)))
	default:
		return "Reconcile archaeology state before continuing"
	}
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	sort.Strings(out)
	return out
}

func dependsOn(previous string) []string {
	if strings.TrimSpace(previous) == "" {
		return nil
	}
	return []string{previous}
}

func deferredFormationUncertainty(input FormationInput) []string {
	out := make([]string, 0, 3)
	if len(input.PendingLearning) > 0 {
		out = append(out, fmt.Sprintf("%d learning interactions unresolved", len(uniqueSortedStrings(input.PendingLearning))))
	}
	if len(input.TensionRefs) > 0 {
		out = append(out, fmt.Sprintf("%d tensions remain active", len(uniqueSortedStrings(input.TensionRefs))))
	}
	if len(input.MutationRefs) > 0 {
		out = append(out, fmt.Sprintf("%d recent mutations may affect structure", len(uniqueSortedStrings(input.MutationRefs))))
	}
	return out
}

func formationRationale(input FormationInput) string {
	parts := []string{"formed from exploration, learning, tension, request, and provenance state"}
	if len(input.ResolvedLearning) > 0 {
		parts = append(parts, fmt.Sprintf("%d resolved learning outcomes reused", len(uniqueSortedStrings(input.ResolvedLearning))))
	}
	if len(input.AcceptedTensions) > 0 {
		parts = append(parts, fmt.Sprintf("%d accepted tensions carried as debt", len(uniqueSortedStrings(input.AcceptedTensions))))
	}
	return strings.Join(parts, "; ")
}

func (s Service) saveFormationResult(ctx context.Context, result *archaeodomain.FormationResult) (string, error) {
	store := s.workflowStore()
	if store == nil || result == nil || strings.TrimSpace(result.WorkflowID) == "" {
		return "", nil
	}
	artifactID := fmt.Sprintf("archaeo-formation-result:%s:%d", strings.TrimSpace(result.WorkflowID), derefInt(result.PlanVersion))
	raw, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      artifactID,
		WorkflowID:      result.WorkflowID,
		Kind:            formationArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     fmt.Sprintf("formation result for plan version %d", derefInt(result.PlanVersion)),
		SummaryMetadata: map[string]any{"plan_id": result.PlanID, "plan_version": derefInt(result.PlanVersion), "snapshot_id": result.SnapshotID},
		InlineRawText:   string(raw),
		CreatedAt:       s.now(),
	}); err != nil {
		return "", err
	}
	return artifactID, nil
}

func (s Service) LoadFormationResult(ctx context.Context, workflowID string, version int) (*archaeodomain.FormationResult, error) {
	store := s.workflowStore()
	if store == nil || strings.TrimSpace(workflowID) == "" || version <= 0 {
		return nil, nil
	}
	artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, store, fmt.Sprintf("archaeo-formation-result:%s:%d", strings.TrimSpace(workflowID), version))
	if err != nil || !ok || artifact == nil {
		return nil, err
	}
	var result archaeodomain.FormationResult
	if err := json.Unmarshal([]byte(artifact.InlineRawText), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func cloneInt(value int) *int {
	return &value
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
