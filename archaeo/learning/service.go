package learning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	"codeburg.org/lexbit/relurpify/archaeo/internal/storeutil"
	"codeburg.org/lexbit/relurpify/archaeo/phases"
	"codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeoretrieval "codeburg.org/lexbit/relurpify/archaeo/retrieval"
	"codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	frameworkretrieval "codeburg.org/lexbit/relurpify/framework/retrieval"
)

const (
	interactionArtifactKind = "archaeo_learning_interaction"
	eventRequested          = archaeoevents.EventLearningInteractionRequested
	eventResolved           = archaeoevents.EventLearningInteractionResolved
	eventExpired            = archaeoevents.EventLearningInteractionExpired
)

type Service struct {
	Store        memory.WorkflowStateStore
	PatternStore patterns.PatternStore
	CommentStore patterns.CommentStore
	PlanStore    frameworkplan.PlanStore
	Retrieval    archaeoretrieval.Store
	Phases       *phases.Service
	Broker       *Broker
	Now          func() time.Time
	NewID        func(prefix string) string
}

func (s Service) Create(ctx context.Context, input CreateInput) (*Interaction, error) {
	return s.create(ctx, input, true)
}

func (s Service) create(ctx context.Context, input CreateInput, syncPending bool) (*Interaction, error) {
	if s.Store == nil {
		return nil, errors.New("workflow state store required")
	}
	if strings.TrimSpace(input.WorkflowID) == "" {
		return nil, errors.New("workflow id required")
	}
	if strings.TrimSpace(input.ExplorationID) == "" {
		return nil, errors.New("exploration id required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return nil, errors.New("title required")
	}
	if input.Kind == "" {
		return nil, errors.New("interaction kind required")
	}
	if input.SubjectType == "" {
		return nil, errors.New("subject type required")
	}
	if _, ok, err := s.Store.GetWorkflow(ctx, input.WorkflowID); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("workflow %s not found", input.WorkflowID)
	}
	choices := normalizeChoices(input.Choices)
	if len(choices) == 0 {
		choices = defaultChoices()
	}
	defaultChoice := strings.TrimSpace(input.DefaultChoice)
	if defaultChoice == "" {
		defaultChoice = defaultChoiceID(choices)
	}
	now := s.now()
	interaction := &Interaction{
		ID:              s.newID("learn"),
		WorkflowID:      input.WorkflowID,
		ExplorationID:   input.ExplorationID,
		SnapshotID:      strings.TrimSpace(input.SnapshotID),
		Kind:            input.Kind,
		SubjectType:     input.SubjectType,
		SubjectID:       strings.TrimSpace(input.SubjectID),
		Title:           strings.TrimSpace(input.Title),
		Description:     strings.TrimSpace(input.Description),
		Evidence:        append([]EvidenceRef(nil), input.Evidence...),
		Choices:         choices,
		DefaultChoice:   defaultChoice,
		TimeoutBehavior: normalizeTimeoutBehavior(input.TimeoutBehavior),
		Blocking:        input.Blocking,
		Status:          StatusPending,
		BasedOnRevision: strings.TrimSpace(input.BasedOnRevision),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.save(ctx, interaction); err != nil {
		return nil, err
	}
	if err := s.appendEvent(ctx, interaction.WorkflowID, eventRequested, interaction.Title, map[string]any{
		"interaction_id": interaction.ID,
		"kind":           interaction.Kind,
		"subject_type":   interaction.SubjectType,
		"subject_id":     interaction.SubjectID,
		"exploration_id": interaction.ExplorationID,
	}); err != nil {
		return nil, err
	}
	if syncPending {
		if err := s.syncPendingLearning(ctx, interaction.WorkflowID); err != nil {
			return nil, err
		}
	}
	if s.Broker != nil {
		if err := s.Broker.SubmitAsync(*interaction); err != nil && !strings.Contains(err.Error(), "already registered") {
			return nil, err
		}
	}
	return interaction, nil
}

func (s Service) Load(ctx context.Context, workflowID, interactionID string) (*Interaction, bool, error) {
	if s.Store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(interactionID) == "" {
		return nil, false, nil
	}
	if artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, s.Store, fmt.Sprintf("archaeo_learning_interaction:%s", strings.TrimSpace(interactionID))); err != nil {
		return nil, false, err
	} else if ok && artifact != nil {
		var interaction Interaction
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &interaction); err != nil {
			return nil, false, err
		}
		return &interaction, true, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKind(ctx, s.Store, workflowID, "", interactionArtifactKind)
	if err != nil {
		return nil, false, err
	}
	for _, artifact := range artifacts {
		var interaction Interaction
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &interaction); err != nil {
			return nil, false, err
		}
		if interaction.ID == interactionID {
			return &interaction, true, nil
		}
	}
	return nil, false, nil
}

func (s Service) ListByWorkflow(ctx context.Context, workflowID string) ([]Interaction, error) {
	if s.Store == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKind(ctx, s.Store, workflowID, "", interactionArtifactKind)
	if err != nil {
		return nil, err
	}
	out := make([]Interaction, 0)
	for _, artifact := range artifacts {
		var interaction Interaction
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &interaction); err != nil {
			return nil, err
		}
		out = append(out, interaction)
	}
	return out, nil
}

func (s Service) Pending(ctx context.Context, workflowID string) ([]Interaction, error) {
	all, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	out := make([]Interaction, 0)
	for _, interaction := range all {
		if interaction.Status == StatusPending {
			out = append(out, interaction)
		}
	}
	return out, nil
}

func (s Service) BlockingPending(ctx context.Context, workflowID string) ([]Interaction, error) {
	pending, err := s.Pending(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	out := make([]Interaction, 0)
	for _, interaction := range pending {
		if interaction.Blocking {
			out = append(out, interaction)
		}
	}
	return out, nil
}

func (s Service) SyncAll(ctx context.Context, workflowID, explorationID, snapshotID, corpusScope, basedOnRevision string) ([]Interaction, []Interaction, error) {
	workflowID = strings.TrimSpace(workflowID)
	explorationID = strings.TrimSpace(explorationID)
	corpusScope = strings.TrimSpace(corpusScope)
	if s.Store == nil || workflowID == "" || explorationID == "" {
		return nil, nil, nil
	}
	existing, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, nil, err
	}
	idx := buildInteractionIndex(existing)
	createdAny := false
	if s.PatternStore != nil && corpusScope != "" {
		proposed, err := s.PatternStore.ListByStatus(ctx, patterns.PatternStatusProposed, corpusScope)
		if err != nil {
			return nil, nil, err
		}
		created, err := s.syncPatternProposalsWithIndex(ctx, workflowID, explorationID, basedOnRevision, proposed, &idx)
		if err != nil {
			return nil, nil, err
		}
		createdAny = createdAny || len(created) > 0
	}
	if s.Retrieval != nil && corpusScope != "" {
		driftedAnchors, err := s.Retrieval.DriftedAnchors(ctx, corpusScope)
		if err != nil {
			return nil, nil, err
		}
		if len(driftedAnchors) > 0 {
			driftEvents, err := s.Retrieval.UnresolvedDrifts(ctx, corpusScope)
			if err != nil {
				return nil, nil, err
			}
			latestByAnchor := make(map[string]frameworkretrieval.AnchorEventRecord, len(driftEvents))
			for _, event := range driftEvents {
				current, ok := latestByAnchor[event.AnchorID]
				if !ok || current.CreatedAt.Before(event.CreatedAt) {
					latestByAnchor[event.AnchorID] = event
				}
			}
			created, err := s.syncAnchorDriftsWithIndex(ctx, workflowID, explorationID, basedOnRevision, driftedAnchors, latestByAnchor, &idx)
			if err != nil {
				return nil, nil, err
			}
			createdAny = createdAny || len(created) > 0
		}
	}
	active, err := (tensions.Service{Store: s.Store, Now: s.Now, NewID: s.NewID}).ActiveByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, nil, err
	}
	if len(active) > 0 {
		created, err := s.syncTensionsWithIndex(ctx, workflowID, explorationID, snapshotID, basedOnRevision, active, &idx)
		if err != nil {
			return nil, nil, err
		}
		createdAny = createdAny || len(created) > 0
	}
	if createdAny {
		if err := s.syncPendingLearningFromInteractions(ctx, workflowID, idx.pending); err != nil {
			return nil, nil, err
		}
	}
	return append([]Interaction(nil), idx.pending...), append([]Interaction(nil), idx.blocking...), nil
}

type interactionIndex struct {
	all              []Interaction
	pending          []Interaction
	blocking         []Interaction
	subjectsByTypeID map[SubjectType]map[string]struct{}
	pendingByTypeID  map[SubjectType]map[string]struct{}
}

func buildInteractionIndex(all []Interaction) interactionIndex {
	idx := interactionIndex{
		all:              append([]Interaction(nil), all...),
		subjectsByTypeID: make(map[SubjectType]map[string]struct{}),
		pendingByTypeID:  make(map[SubjectType]map[string]struct{}),
	}
	for _, interaction := range all {
		subjectID := strings.TrimSpace(interaction.SubjectID)
		if subjectID == "" {
			continue
		}
		if idx.subjectsByTypeID[interaction.SubjectType] == nil {
			idx.subjectsByTypeID[interaction.SubjectType] = make(map[string]struct{})
		}
		idx.subjectsByTypeID[interaction.SubjectType][subjectID] = struct{}{}
		if interaction.Status == StatusPending {
			idx.pending = append(idx.pending, interaction)
			if interaction.Blocking {
				idx.blocking = append(idx.blocking, interaction)
			}
			if idx.pendingByTypeID[interaction.SubjectType] == nil {
				idx.pendingByTypeID[interaction.SubjectType] = make(map[string]struct{})
			}
			idx.pendingByTypeID[interaction.SubjectType][subjectID] = struct{}{}
		}
	}
	return idx
}

func (i interactionIndex) has(subjectType SubjectType, subjectID string) bool {
	if subjectID = strings.TrimSpace(subjectID); subjectID == "" {
		return false
	}
	if subjects := i.subjectsByTypeID[subjectType]; subjects != nil {
		_, ok := subjects[subjectID]
		return ok
	}
	return false
}

func (i interactionIndex) hasPending(subjectType SubjectType, subjectID string) bool {
	if subjectID = strings.TrimSpace(subjectID); subjectID == "" {
		return false
	}
	if subjects := i.pendingByTypeID[subjectType]; subjects != nil {
		_, ok := subjects[subjectID]
		return ok
	}
	return false
}

func (i *interactionIndex) add(interaction Interaction) {
	i.all = append(i.all, interaction)
	subjectID := strings.TrimSpace(interaction.SubjectID)
	if subjectID == "" {
		return
	}
	if i.subjectsByTypeID[interaction.SubjectType] == nil {
		i.subjectsByTypeID[interaction.SubjectType] = make(map[string]struct{})
	}
	i.subjectsByTypeID[interaction.SubjectType][subjectID] = struct{}{}
	if interaction.Status == StatusPending {
		i.pending = append(i.pending, interaction)
		if interaction.Blocking {
			i.blocking = append(i.blocking, interaction)
		}
		if i.pendingByTypeID[interaction.SubjectType] == nil {
			i.pendingByTypeID[interaction.SubjectType] = make(map[string]struct{})
		}
		i.pendingByTypeID[interaction.SubjectType][subjectID] = struct{}{}
	}
}

func (s Service) SyncPatternProposals(ctx context.Context, workflowID, explorationID, corpusScope, basedOnRevision string) ([]Interaction, error) {
	if s.PatternStore == nil {
		return nil, nil
	}
	workflowID = strings.TrimSpace(workflowID)
	explorationID = strings.TrimSpace(explorationID)
	corpusScope = strings.TrimSpace(corpusScope)
	if workflowID == "" || explorationID == "" || corpusScope == "" {
		return nil, nil
	}
	proposed, err := s.PatternStore.ListByStatus(ctx, patterns.PatternStatusProposed, corpusScope)
	if err != nil {
		return nil, err
	}
	existing, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	idx := buildInteractionIndex(existing)
	created, err := s.syncPatternProposalsWithIndex(ctx, workflowID, explorationID, basedOnRevision, proposed, &idx)
	if err != nil {
		return nil, err
	}
	if len(created) > 0 {
		if err := s.syncPendingLearning(ctx, workflowID); err != nil {
			return nil, err
		}
	}
	return created, nil
}

func (s Service) syncPatternProposalsWithIndex(ctx context.Context, workflowID, explorationID, basedOnRevision string, proposed []patterns.PatternRecord, idx *interactionIndex) ([]Interaction, error) {
	created := make([]Interaction, 0)
	for _, record := range proposed {
		if idx != nil && idx.has(SubjectPattern, record.ID) {
			continue
		}
		interaction, err := s.create(ctx, CreateInput{
			WorkflowID:      workflowID,
			ExplorationID:   explorationID,
			Kind:            InteractionPatternProposal,
			SubjectType:     SubjectPattern,
			SubjectID:       record.ID,
			Title:           fmt.Sprintf("Confirm pattern: %s", record.Title),
			Description:     record.Description,
			Evidence:        patternEvidence(record),
			DefaultChoice:   string(ResolutionConfirm),
			TimeoutBehavior: TimeoutDefer,
			Blocking:        true,
			BasedOnRevision: basedOnRevision,
		}, false)
		if err != nil {
			return nil, err
		}
		created = append(created, *interaction)
		if idx != nil {
			idx.add(*interaction)
		}
	}
	return created, nil
}

func (s Service) SyncAnchorDrifts(ctx context.Context, workflowID, explorationID, corpusScope, basedOnRevision string) ([]Interaction, error) {
	if s.Retrieval == nil {
		return nil, nil
	}
	workflowID = strings.TrimSpace(workflowID)
	explorationID = strings.TrimSpace(explorationID)
	corpusScope = strings.TrimSpace(corpusScope)
	if workflowID == "" || explorationID == "" || corpusScope == "" {
		return nil, nil
	}
	driftedAnchors, err := s.Retrieval.DriftedAnchors(ctx, corpusScope)
	if err != nil {
		return nil, err
	}
	if len(driftedAnchors) == 0 {
		return nil, nil
	}
	driftEvents, err := s.Retrieval.UnresolvedDrifts(ctx, corpusScope)
	if err != nil {
		return nil, err
	}
	latestByAnchor := make(map[string]frameworkretrieval.AnchorEventRecord, len(driftEvents))
	for _, event := range driftEvents {
		current, ok := latestByAnchor[event.AnchorID]
		if !ok || current.CreatedAt.Before(event.CreatedAt) {
			latestByAnchor[event.AnchorID] = event
		}
	}
	existing, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	idx := buildInteractionIndex(existing)
	created, err := s.syncAnchorDriftsWithIndex(ctx, workflowID, explorationID, basedOnRevision, driftedAnchors, latestByAnchor, &idx)
	if err != nil {
		return nil, err
	}
	if len(created) > 0 {
		if err := s.syncPendingLearning(ctx, workflowID); err != nil {
			return nil, err
		}
	}
	return created, nil
}

func (s Service) syncAnchorDriftsWithIndex(ctx context.Context, workflowID, explorationID, basedOnRevision string, driftedAnchors []frameworkretrieval.AnchorRecord, latestByAnchor map[string]frameworkretrieval.AnchorEventRecord, idx *interactionIndex) ([]Interaction, error) {
	created := make([]Interaction, 0)
	for _, anchor := range driftedAnchors {
		if idx != nil && idx.has(SubjectAnchor, anchor.AnchorID) {
			continue
		}
		detail := "implementation drift detected"
		if event, ok := latestByAnchor[anchor.AnchorID]; ok && strings.TrimSpace(event.Detail) != "" {
			detail = strings.TrimSpace(event.Detail)
		}
		interaction, err := s.create(ctx, CreateInput{
			WorkflowID:      workflowID,
			ExplorationID:   explorationID,
			Kind:            InteractionAnchorProposal,
			SubjectType:     SubjectAnchor,
			SubjectID:       anchor.AnchorID,
			Title:           fmt.Sprintf("Review drifted anchor: %s", anchor.Term),
			Description:     detail,
			Evidence:        anchorDriftEvidence(anchor, latestByAnchor[anchor.AnchorID]),
			DefaultChoice:   string(ResolutionRefine),
			TimeoutBehavior: TimeoutDefer,
			Blocking:        true,
			BasedOnRevision: basedOnRevision,
		}, false)
		if err != nil {
			return nil, err
		}
		created = append(created, *interaction)
		if idx != nil {
			idx.add(*interaction)
		}
	}
	return created, nil
}

func (s Service) SyncTensions(ctx context.Context, workflowID, explorationID, snapshotID, basedOnRevision string) ([]Interaction, error) {
	if s.Store == nil {
		return nil, nil
	}
	workflowID = strings.TrimSpace(workflowID)
	explorationID = strings.TrimSpace(explorationID)
	if workflowID == "" || explorationID == "" {
		return nil, nil
	}
	active, err := (tensions.Service{Store: s.Store, Now: s.Now, NewID: s.NewID}).ActiveByWorkflow(ctx, workflowID)
	if err != nil || len(active) == 0 {
		return nil, err
	}
	existing, err := s.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	idx := buildInteractionIndex(existing)
	created, err := s.syncTensionsWithIndex(ctx, workflowID, explorationID, snapshotID, basedOnRevision, active, &idx)
	if err != nil {
		return nil, err
	}
	if len(created) > 0 {
		if err := s.syncPendingLearning(ctx, workflowID); err != nil {
			return nil, err
		}
	}
	return created, nil
}

func (s Service) syncTensionsWithIndex(ctx context.Context, workflowID, explorationID, snapshotID, basedOnRevision string, active []archaeodomain.Tension, idx *interactionIndex) ([]Interaction, error) {
	created := make([]Interaction, 0)
	for _, tensionRecord := range active {
		if idx != nil && idx.hasPending(SubjectTension, tensionRecord.ID) {
			continue
		}
		interaction, err := s.create(ctx, CreateInput{
			WorkflowID:    workflowID,
			ExplorationID: firstNonEmpty(strings.TrimSpace(tensionRecord.ExplorationID), explorationID),
			SnapshotID:    firstNonEmpty(strings.TrimSpace(tensionRecord.SnapshotID), snapshotID),
			Kind:          InteractionTensionReview,
			SubjectType:   SubjectTension,
			SubjectID:     tensionRecord.ID,
			Title:         fmt.Sprintf("Review tension: %s", tensionRecord.Kind),
			Description:   tensionRecord.Description,
			Evidence:      tensionEvidence(tensionRecord),
			Choices: []Choice{
				{ID: "unresolved", Label: "Keep Unresolved", Description: "Treat this as a real unresolved tension."},
				{ID: "accept", Label: "Accept Debt", Description: "Accept the tension as intentional debt or known tradeoff."},
				{ID: "resolve", Label: "Resolve", Description: "Mark the tension as resolved or dismissed."},
				{ID: string(ResolutionDefer), Label: "Defer", Description: "Defer the decision."},
			},
			DefaultChoice:   "unresolved",
			TimeoutBehavior: TimeoutDefer,
			Blocking:        tensionBlocking(tensionRecord),
			BasedOnRevision: firstNonEmpty(strings.TrimSpace(tensionRecord.BasedOnRevision), basedOnRevision),
		}, false)
		if err != nil {
			return nil, err
		}
		created = append(created, *interaction)
		if idx != nil {
			idx.add(*interaction)
		}
	}
	return created, nil
}

func (s Service) Resolve(ctx context.Context, input ResolveInput) (*Interaction, error) {
	if strings.TrimSpace(input.WorkflowID) == "" {
		return nil, errors.New("workflow id required")
	}
	if strings.TrimSpace(input.InteractionID) == "" {
		return nil, errors.New("interaction id required")
	}
	if input.Kind == "" {
		return nil, errors.New("resolution kind required")
	}
	interaction, ok, err := s.Load(ctx, input.WorkflowID, input.InteractionID)
	if err != nil {
		return nil, err
	}
	if !ok || interaction == nil {
		return nil, fmt.Errorf("learning interaction %s not found", input.InteractionID)
	}
	expected := input.ExpectedStatus
	if expected == "" {
		expected = StatusPending
	}
	if interaction.Status != expected {
		return nil, fmt.Errorf("learning interaction %s expected status %s, got %s", interaction.ID, expected, interaction.Status)
	}

	commentID, err := s.saveResolutionComment(ctx, interaction, input)
	if err != nil {
		return nil, err
	}
	if err := s.applyResolution(ctx, interaction, input, commentID); err != nil {
		return nil, err
	}
	if err := s.applyPlanConfidence(ctx, interaction, input); err != nil {
		return nil, err
	}
	if err := s.applyPlanVersionStaleness(ctx, interaction, input); err != nil {
		return nil, err
	}
	now := s.now()
	interaction.Status = StatusResolved
	if input.Kind == ResolutionDefer {
		interaction.Status = StatusDeferred
	}
	interaction.Resolution = &Resolution{
		Kind:           input.Kind,
		ChoiceID:       strings.TrimSpace(input.ChoiceID),
		RefinedPayload: cloneMap(input.RefinedPayload),
		CommentRef:     commentID,
		ResolvedBy:     strings.TrimSpace(input.ResolvedBy),
		ResolvedAt:     now,
	}
	if revision := strings.TrimSpace(input.BasedOnRevision); revision != "" {
		interaction.BasedOnRevision = revision
	}
	interaction.UpdatedAt = now
	if err := s.save(ctx, interaction); err != nil {
		return nil, err
	}
	if err := s.appendEvent(ctx, interaction.WorkflowID, eventResolved, interaction.Title, map[string]any{
		"interaction_id": interaction.ID,
		"resolution":     input.Kind,
		"subject_type":   interaction.SubjectType,
		"subject_id":     interaction.SubjectID,
		"comment_ref":    commentID,
	}); err != nil {
		return nil, err
	}
	if err := s.appendMutation(ctx, interaction, input); err != nil {
		return nil, err
	}
	if err := s.syncPendingLearning(ctx, interaction.WorkflowID); err != nil {
		return nil, err
	}
	if s.Broker != nil {
		_ = s.Broker.Resolve(*interaction)
	}
	return interaction, nil
}

func (s Service) applyPlanConfidence(ctx context.Context, interaction *Interaction, input ResolveInput) error {
	if s.PlanStore == nil || interaction == nil || strings.TrimSpace(interaction.WorkflowID) == "" {
		return nil
	}
	plan, err := s.PlanStore.LoadPlanByWorkflow(ctx, interaction.WorkflowID)
	if err != nil || plan == nil {
		return err
	}
	updated := false
	switch interaction.SubjectType {
	case SubjectPattern:
		record, err := s.PatternStore.Load(ctx, interaction.SubjectID)
		if err != nil {
			return err
		}
		updated = adjustPatternConfidence(plan, record, input.Kind, s.now())
	case SubjectAnchor:
		updated = adjustAnchorConfidence(plan, interaction.SubjectID, input.Kind, s.now())
	case SubjectTension:
		updated, err = s.adjustTensionConfidence(ctx, plan, interaction)
		if err != nil {
			return err
		}
	}
	if !updated {
		return nil
	}
	for stepID, step := range plan.Steps {
		if step == nil {
			continue
		}
		if err := s.PlanStore.UpdateStep(ctx, plan.ID, stepID, step); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) applyPlanVersionStaleness(ctx context.Context, interaction *Interaction, input ResolveInput) error {
	if s.Store == nil || s.PlanStore == nil || interaction == nil || strings.TrimSpace(interaction.WorkflowID) == "" {
		return nil
	}
	if input.Kind == ResolutionDefer {
		return nil
	}
	versionSvc := plans.Service{
		Store:         s.PlanStore,
		WorkflowStore: s.Store,
		Now:           s.Now,
	}
	active, err := versionSvc.LoadActiveVersion(ctx, interaction.WorkflowID)
	if err != nil || active == nil {
		return err
	}
	reason := fmt.Sprintf("learning resolution %s for %s:%s", input.Kind, interaction.SubjectType, interaction.SubjectID)
	switch interaction.SubjectType {
	case SubjectPattern:
		if !containsString(active.PatternRefs, interaction.SubjectID) {
			return nil
		}
	case SubjectAnchor:
		if !containsString(active.AnchorRefs, interaction.SubjectID) {
			return nil
		}
	case SubjectTension:
		if !containsString(active.TensionRefs, interaction.SubjectID) {
			return nil
		}
		switch strings.TrimSpace(input.ChoiceID) {
		case "accept", "resolve":
			return nil
		}
		if input.Kind == ResolutionReject {
			return nil
		}
	default:
		return nil
	}
	_, err = versionSvc.MarkVersionStale(ctx, interaction.WorkflowID, active.Version, reason)
	if err != nil {
		return err
	}
	_, err = versionSvc.EnsureDraftSuccessor(ctx, interaction.WorkflowID, active.Version, reason)
	return err
}

func (s Service) Expire(ctx context.Context, workflowID, interactionID, reason string) (*Interaction, error) {
	interaction, ok, err := s.Load(ctx, workflowID, interactionID)
	if err != nil {
		return nil, err
	}
	if !ok || interaction == nil {
		return nil, fmt.Errorf("learning interaction %s not found", interactionID)
	}
	if interaction.Status != StatusPending {
		return nil, fmt.Errorf("learning interaction %s expected status %s, got %s", interaction.ID, StatusPending, interaction.Status)
	}
	interaction.Status = StatusExpired
	interaction.UpdatedAt = s.now()
	if err := s.save(ctx, interaction); err != nil {
		return nil, err
	}
	if err := s.appendEvent(ctx, interaction.WorkflowID, eventExpired, interaction.Title, map[string]any{
		"interaction_id": interaction.ID,
		"reason":         strings.TrimSpace(reason),
	}); err != nil {
		return nil, err
	}
	if err := s.syncPendingLearning(ctx, interaction.WorkflowID); err != nil {
		return nil, err
	}
	if s.Broker != nil {
		_ = s.Broker.Resolve(*interaction)
	}
	return interaction, nil
}

func (s Service) applyResolution(ctx context.Context, interaction *Interaction, input ResolveInput, commentID string) error {
	switch interaction.SubjectType {
	case SubjectPattern:
		return s.applyPatternResolution(ctx, interaction, input, commentID)
	case SubjectAnchor:
		return s.applyAnchorResolution(ctx, interaction, input)
	case SubjectTension:
		return s.applyTensionResolution(ctx, interaction, input, commentID)
	case SubjectExploration:
		return nil
	default:
		return fmt.Errorf("unsupported learning subject type %q", interaction.SubjectType)
	}
}

func (s Service) applyTensionResolution(ctx context.Context, interaction *Interaction, input ResolveInput, commentID string) error {
	if s.Store == nil {
		return errors.New("workflow state store required for tension learning interactions")
	}
	tensionID := strings.TrimSpace(interaction.SubjectID)
	if tensionID == "" {
		return errors.New("tension subject id required")
	}
	tensionSvc := tensions.Service{Store: s.Store, Now: s.Now, NewID: s.NewID}
	record, err := tensionSvc.Load(ctx, interaction.WorkflowID, tensionID)
	if err != nil || record == nil {
		return err
	}
	switch input.Kind {
	case ResolutionConfirm:
		switch strings.TrimSpace(input.ChoiceID) {
		case "accept":
			record.Status = archaeodomain.TensionAccepted
		case "unresolved", "":
			record.Status = archaeodomain.TensionUnresolved
		default:
			record.Status = archaeodomain.TensionConfirmed
		}
	case ResolutionReject:
		record.Status = archaeodomain.TensionResolved
	case ResolutionRefine:
		if kind := strings.TrimSpace(stringFromPayload(input.RefinedPayload, "kind")); kind != "" {
			record.Kind = kind
		}
		if description := strings.TrimSpace(stringFromPayload(input.RefinedPayload, "description")); description != "" {
			record.Description = description
		}
		if severity := strings.TrimSpace(stringFromPayload(input.RefinedPayload, "severity")); severity != "" {
			record.Severity = severity
		}
		if patternIDs := stringSlicePayload(input.RefinedPayload, "pattern_ids"); len(patternIDs) > 0 {
			record.PatternIDs = patternIDs
		}
		if anchorRefs := stringSlicePayload(input.RefinedPayload, "anchor_refs"); len(anchorRefs) > 0 {
			record.AnchorRefs = anchorRefs
		}
		if symbolScope := stringSlicePayload(input.RefinedPayload, "symbol_scope"); len(symbolScope) > 0 {
			record.SymbolScope = symbolScope
		}
		if stepIDs := stringSlicePayload(input.RefinedPayload, "related_plan_step_ids"); len(stepIDs) > 0 {
			record.RelatedPlanStepIDs = stepIDs
		}
		record.Status = archaeodomain.TensionConfirmed
	case ResolutionDefer:
		return nil
	default:
		return fmt.Errorf("unsupported tension resolution %q", input.Kind)
	}
	if commentID != "" {
		record.CommentRefs = appendUnique(record.CommentRefs, commentID)
	}
	if revision := strings.TrimSpace(input.BasedOnRevision); revision != "" {
		record.BasedOnRevision = revision
	}
	_, err = tensionSvc.Update(ctx, record)
	return err
}

func (s Service) applyPatternResolution(ctx context.Context, interaction *Interaction, input ResolveInput, commentID string) error {
	if s.PatternStore == nil {
		return errors.New("pattern store required for pattern learning interactions")
	}
	patternID := strings.TrimSpace(interaction.SubjectID)
	if patternID == "" {
		return errors.New("pattern subject id required")
	}
	switch input.Kind {
	case ResolutionConfirm:
		if err := s.PatternStore.UpdateStatus(ctx, patternID, patterns.PatternStatusConfirmed, strings.TrimSpace(input.ResolvedBy)); err != nil {
			return err
		}
		return s.attachCommentToPattern(ctx, patternID, commentID)
	case ResolutionReject:
		if err := s.PatternStore.UpdateStatus(ctx, patternID, patterns.PatternStatusRejected, strings.TrimSpace(input.ResolvedBy)); err != nil {
			return err
		}
		return s.attachCommentToPattern(ctx, patternID, commentID)
	case ResolutionRefine:
		current, err := s.PatternStore.Load(ctx, patternID)
		if err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("pattern %s not found", patternID)
		}
		replacement := *current
		replacement.ID = s.newID("pattern")
		replacement.Status = patterns.PatternStatusConfirmed
		replacement.SupersededBy = ""
		replacement.ConfirmedBy = strings.TrimSpace(input.ResolvedBy)
		now := s.now()
		replacement.ConfirmedAt = &now
		replacement.CreatedAt = now
		replacement.UpdatedAt = now
		applyPatternRefinement(&replacement, input.RefinedPayload)
		if commentID != "" {
			replacement.CommentIDs = appendUnique(replacement.CommentIDs, commentID)
		}
		return s.PatternStore.Supersede(ctx, patternID, replacement)
	case ResolutionDefer:
		return s.attachCommentToPattern(ctx, patternID, commentID)
	default:
		return fmt.Errorf("unsupported pattern resolution %q", input.Kind)
	}
}

func (s Service) applyAnchorResolution(ctx context.Context, interaction *Interaction, input ResolveInput) error {
	if s.Retrieval == nil {
		return errors.New("retrieval db required for anchor learning interactions")
	}
	anchorID := strings.TrimSpace(interaction.SubjectID)
	switch input.Kind {
	case ResolutionConfirm:
		if anchorID != "" {
			return nil
		}
		_, err := s.Retrieval.DeclareAnchor(ctx, anchorDeclarationFromPayload(input.RefinedPayload), corpusScopeFromPayload(input.RefinedPayload), trustClassFromPayload(input.RefinedPayload))
		return err
	case ResolutionReject:
		if anchorID == "" {
			return nil
		}
		reason := "rejected via learning interaction"
		if input.Comment != nil && strings.TrimSpace(input.Comment.Body) != "" {
			reason = strings.TrimSpace(input.Comment.Body)
		}
		return s.Retrieval.InvalidateAnchor(ctx, anchorID, reason)
	case ResolutionRefine:
		if anchorID == "" {
			_, err := s.Retrieval.DeclareAnchor(ctx, anchorDeclarationFromPayload(input.RefinedPayload), corpusScopeFromPayload(input.RefinedPayload), trustClassFromPayload(input.RefinedPayload))
			return err
		}
		definition := strings.TrimSpace(stringFromPayload(input.RefinedPayload, "definition"))
		if definition == "" {
			return errors.New("anchor refinement definition required")
		}
		_, err := s.Retrieval.SupersedeAnchor(ctx, anchorID, definition, mapStringPayload(input.RefinedPayload, "context"))
		return err
	case ResolutionDefer:
		return nil
	default:
		return fmt.Errorf("unsupported anchor resolution %q", input.Kind)
	}
}

func (s Service) attachCommentToPattern(ctx context.Context, patternID, commentID string) error {
	if commentID == "" || s.PatternStore == nil {
		return nil
	}
	record, err := s.PatternStore.Load(ctx, patternID)
	if err != nil || record == nil {
		return err
	}
	record.CommentIDs = appendUnique(record.CommentIDs, commentID)
	record.UpdatedAt = s.now()
	return s.PatternStore.Save(ctx, *record)
}

func (s Service) saveResolutionComment(ctx context.Context, interaction *Interaction, input ResolveInput) (string, error) {
	if input.Comment == nil || s.CommentStore == nil {
		return "", nil
	}
	body := strings.TrimSpace(input.Comment.Body)
	if body == "" {
		return "", nil
	}
	intentType := patterns.CommentIntentType(strings.TrimSpace(input.Comment.IntentType))
	if intentType == "" {
		intentType = patterns.CommentIntentional
	}
	authorKind := patterns.AuthorKind(strings.TrimSpace(input.Comment.AuthorKind))
	if authorKind == "" {
		authorKind = patterns.AuthorKindHuman
	}
	trustClass := patterns.TrustClass(strings.TrimSpace(input.Comment.TrustClass))
	if trustClass == "" {
		if authorKind == patterns.AuthorKindAgent {
			trustClass = patterns.TrustClassBuiltinTrusted
		} else {
			trustClass = patterns.TrustClassWorkspaceTrusted
		}
	}
	commentID := s.newID("comment")
	record := patterns.CommentRecord{
		CommentID:   commentID,
		IntentType:  intentType,
		Body:        body,
		AuthorKind:  authorKind,
		TrustClass:  trustClass,
		CorpusScope: strings.TrimSpace(input.Comment.CorpusScope),
		CreatedAt:   s.now(),
		UpdatedAt:   s.now(),
	}
	switch interaction.SubjectType {
	case SubjectPattern:
		record.PatternID = interaction.SubjectID
	case SubjectAnchor:
		record.AnchorID = interaction.SubjectID
	case SubjectTension:
		record.TensionID = interaction.SubjectID
	}
	if err := s.CommentStore.Save(ctx, record); err != nil {
		return "", err
	}
	return commentID, nil
}

func (s Service) syncPendingLearning(ctx context.Context, workflowID string) error {
	if s.Phases == nil || strings.TrimSpace(workflowID) == "" {
		return nil
	}
	pending, err := s.Pending(ctx, workflowID)
	if err != nil {
		return err
	}
	return s.syncPendingLearningFromInteractions(ctx, workflowID, pending)
}

func (s Service) syncPendingLearningFromInteractions(ctx context.Context, workflowID string, pending []Interaction) error {
	if s.Phases == nil || strings.TrimSpace(workflowID) == "" {
		return nil
	}
	ids := make([]string, 0, len(pending))
	for _, interaction := range pending {
		ids = append(ids, interaction.ID)
	}
	_, err := s.Phases.SyncPendingLearning(ctx, workflowID, ids)
	return err
}

func (s Service) save(ctx context.Context, interaction *Interaction) error {
	if s.Store == nil || interaction == nil {
		return nil
	}
	raw, err := json.Marshal(interaction)
	if err != nil {
		return err
	}
	return s.Store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      fmt.Sprintf("archaeo_learning_interaction:%s", interaction.ID),
		WorkflowID:      interaction.WorkflowID,
		Kind:            interactionArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     fmt.Sprintf("learning interaction: %s (%s)", interaction.Title, interaction.Status),
		SummaryMetadata: map[string]any{"interaction_id": interaction.ID, "status": interaction.Status, "kind": interaction.Kind},
		InlineRawText:   string(raw),
		CreatedAt:       s.now(),
	})
}

func (s Service) appendEvent(ctx context.Context, workflowID, eventType, message string, metadata map[string]any) error {
	if s.Store == nil {
		return nil
	}
	return archaeoevents.AppendWorkflowEvent(ctx, s.Store, workflowID, eventType, message, metadata, s.now())
}

func (s Service) appendMutation(ctx context.Context, interaction *Interaction, input ResolveInput) error {
	if s.Store == nil || interaction == nil || input.Kind == ResolutionDefer {
		return nil
	}
	mutation := archaeodomain.MutationEvent{
		WorkflowID:      interaction.WorkflowID,
		ExplorationID:   interaction.ExplorationID,
		StepID:          "",
		Category:        archaeodomain.MutationConfidenceChange,
		SourceKind:      "learning_interaction",
		SourceRef:       interaction.ID,
		Description:     fmt.Sprintf("learning resolution %s for %s:%s", input.Kind, interaction.SubjectType, interaction.SubjectID),
		Impact:          archaeodomain.ImpactAdvisory,
		Disposition:     archaeodomain.DispositionContinue,
		Blocking:        interaction.Blocking,
		BasedOnRevision: firstNonEmpty(strings.TrimSpace(input.BasedOnRevision), interaction.BasedOnRevision),
		Metadata: map[string]any{
			"interaction_id": interaction.ID,
			"resolution":     string(input.Kind),
			"subject_type":   string(interaction.SubjectType),
			"subject_id":     interaction.SubjectID,
			"choice_id":      strings.TrimSpace(input.ChoiceID),
		},
		CreatedAt: s.now(),
	}
	switch interaction.SubjectType {
	case SubjectPattern:
		mutation.BlastRadius = archaeodomain.BlastRadius{
			Scope:              archaeodomain.BlastRadiusPlan,
			AffectedPatternIDs: []string{interaction.SubjectID},
			EstimatedCount:     1,
		}
		mutation.Metadata["pattern_id"] = interaction.SubjectID
		mutation.Metadata["kind"] = interaction.Kind
	case SubjectAnchor:
		mutation.BlastRadius = archaeodomain.BlastRadius{
			Scope:              archaeodomain.BlastRadiusStep,
			AffectedAnchorRefs: []string{interaction.SubjectID},
			EstimatedCount:     1,
		}
		mutation.Metadata["anchor_id"] = interaction.SubjectID
		mutation.Impact = archaeodomain.ImpactCaution
	case SubjectTension:
		mutation.BlastRadius = archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: stringSlicePayload(input.RefinedPayload, "related_plan_step_ids"),
			EstimatedCount:  1,
		}
		if mutation.BlastRadius.AffectedStepIDs == nil {
			mutation.BlastRadius.AffectedStepIDs = nil
		}
		mutation.Metadata["tension_id"] = interaction.SubjectID
		if strings.TrimSpace(input.ChoiceID) == "unresolved" || input.Kind == ResolutionConfirm && strings.TrimSpace(input.ChoiceID) == "" {
			mutation.Category = archaeodomain.MutationBlockingSemantic
			mutation.Impact = archaeodomain.ImpactLocalBlocking
			mutation.Disposition = archaeodomain.DispositionPauseForGuidance
			mutation.Blocking = true
		}
	default:
		mutation.BlastRadius = archaeodomain.BlastRadius{
			Scope:          archaeodomain.BlastRadiusLocal,
			EstimatedCount: 1,
		}
	}
	return archaeoevents.AppendMutationEvent(ctx, s.Store, mutation)
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
	return fmt.Sprintf("%s-%d", prefix, s.now().UnixNano())
}

func normalizeChoices(choices []Choice) []Choice {
	if len(choices) == 0 {
		return nil
	}
	out := make([]Choice, 0, len(choices))
	for _, choice := range choices {
		id := strings.TrimSpace(choice.ID)
		label := strings.TrimSpace(choice.Label)
		if id == "" || label == "" {
			continue
		}
		out = append(out, Choice{ID: id, Label: label, Description: strings.TrimSpace(choice.Description)})
	}
	return out
}

func defaultChoices() []Choice {
	return []Choice{
		{ID: string(ResolutionConfirm), Label: "Confirm", Description: "Accept this semantic finding."},
		{ID: string(ResolutionReject), Label: "Reject", Description: "Reject this semantic finding."},
		{ID: string(ResolutionRefine), Label: "Refine", Description: "Refine and supersede the finding."},
		{ID: string(ResolutionDefer), Label: "Defer", Description: "Defer this decision without applying semantics."},
	}
}

func tensionEvidence(record archaeodomain.Tension) []EvidenceRef {
	return []EvidenceRef{{
		Kind:    "tension",
		RefID:   record.ID,
		Title:   record.Kind,
		Summary: record.Description,
		Metadata: map[string]any{
			"severity":              record.Severity,
			"status":                record.Status,
			"related_plan_step_ids": append([]string(nil), record.RelatedPlanStepIDs...),
			"blast_radius_node_ids": append([]string(nil), record.BlastRadiusNodeIDs...),
		},
	}}
}

func tensionBlocking(record archaeodomain.Tension) bool {
	switch record.Status {
	case archaeodomain.TensionAccepted, archaeodomain.TensionResolved:
		return false
	case archaeodomain.TensionUnresolved, archaeodomain.TensionConfirmed:
		return true
	}
	switch strings.ToLower(strings.TrimSpace(record.Severity)) {
	case "critical", "significant", "high":
		return true
	default:
		return false
	}
}

func (s Service) adjustTensionConfidence(ctx context.Context, plan *frameworkplan.LivingPlan, interaction *Interaction) (bool, error) {
	if s.Store == nil || plan == nil || interaction == nil || strings.TrimSpace(interaction.SubjectID) == "" {
		return false, nil
	}
	record, err := (tensions.Service{Store: s.Store, Now: s.Now, NewID: s.NewID}).Load(ctx, interaction.WorkflowID, interaction.SubjectID)
	if err != nil || record == nil {
		return false, err
	}
	if len(record.RelatedPlanStepIDs) == 0 {
		return false, nil
	}
	updated := false
	for _, stepID := range record.RelatedPlanStepIDs {
		step := plan.Steps[strings.TrimSpace(stepID)]
		if step == nil {
			continue
		}
		before := step.ConfidenceScore
		switch record.Status {
		case archaeodomain.TensionResolved:
			step.ConfidenceScore = clampConfidence(step.ConfidenceScore + 0.06)
		case archaeodomain.TensionAccepted:
			step.ConfidenceScore = clampConfidence(step.ConfidenceScore)
		default:
			step.ConfidenceScore = clampConfidence(step.ConfidenceScore - 0.08)
		}
		if step.ConfidenceScore != before {
			step.UpdatedAt = s.now()
			updated = true
		}
	}
	return updated, nil
}

func defaultChoiceID(choices []Choice) string {
	if len(choices) == 0 {
		return ""
	}
	return choices[0].ID
}

func normalizeTimeoutBehavior(value TimeoutBehavior) TimeoutBehavior {
	switch value {
	case TimeoutDefer, TimeoutExpire:
		return value
	default:
		return TimeoutUseDefault
	}
}

func applyPatternRefinement(record *patterns.PatternRecord, payload map[string]any) {
	if record == nil || len(payload) == 0 {
		return
	}
	if title := strings.TrimSpace(stringFromPayload(payload, "title")); title != "" {
		record.Title = title
	}
	if description := strings.TrimSpace(stringFromPayload(payload, "description")); description != "" {
		record.Description = description
	}
	if confidence, ok := floatFromPayload(payload, "confidence"); ok {
		record.Confidence = confidence
	}
	if anchorRefs := stringSlicePayload(payload, "anchor_refs"); len(anchorRefs) > 0 {
		record.AnchorRefs = anchorRefs
	}
	if instances := patternInstancesPayload(payload, "instances"); len(instances) > 0 {
		record.Instances = instances
	}
	if corpusScope := strings.TrimSpace(stringFromPayload(payload, "corpus_scope")); corpusScope != "" {
		record.CorpusScope = corpusScope
	}
	if corpusSource := strings.TrimSpace(stringFromPayload(payload, "corpus_source")); corpusSource != "" {
		record.CorpusSource = corpusSource
	}
}

func anchorDeclarationFromPayload(payload map[string]any) frameworkretrieval.AnchorDeclaration {
	return frameworkretrieval.AnchorDeclaration{
		Term:       strings.TrimSpace(stringFromPayload(payload, "term")),
		Definition: strings.TrimSpace(stringFromPayload(payload, "definition")),
		Class:      strings.TrimSpace(stringFromPayload(payload, "class")),
		Context:    mapStringPayload(payload, "context"),
	}
}

func corpusScopeFromPayload(payload map[string]any) string {
	if scope := strings.TrimSpace(stringFromPayload(payload, "corpus_scope")); scope != "" {
		return scope
	}
	return "workspace"
}

func trustClassFromPayload(payload map[string]any) string {
	if trustClass := strings.TrimSpace(stringFromPayload(payload, "trust_class")); trustClass != "" {
		return trustClass
	}
	return string(patterns.TrustClassWorkspaceTrusted)
}

func appendUnique(values []string, next string) []string {
	next = strings.TrimSpace(next)
	if next == "" {
		return values
	}
	for _, value := range values {
		if value == next {
			return values
		}
	}
	return append(values, next)
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func stringFromPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return fmt.Sprint(value)
}

func stringSlicePayload(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func mapStringPayload(payload map[string]any, key string) map[string]string {
	if payload == nil {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]string:
		return typed
	case map[string]any:
		out := make(map[string]string, len(typed))
		for k, v := range typed {
			out[k] = fmt.Sprint(v)
		}
		return out
	default:
		return nil
	}
}

func floatFromPayload(payload map[string]any, key string) (float64, bool) {
	if payload == nil {
		return 0, false
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func patternInstancesPayload(payload map[string]any, key string) []patterns.PatternInstance {
	if payload == nil {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var instances []patterns.PatternInstance
	if err := json.Unmarshal(raw, &instances); err != nil {
		return nil
	}
	return instances
}

func adjustPatternConfidence(plan *frameworkplan.LivingPlan, record *patterns.PatternRecord, resolution ResolutionKind, now time.Time) bool {
	if plan == nil || record == nil {
		return false
	}
	targets := make(map[string]struct{})
	for _, instance := range record.Instances {
		if strings.TrimSpace(instance.SymbolID) != "" {
			targets[strings.TrimSpace(instance.SymbolID)] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return false
	}
	delta := confidenceDelta(resolution, 0.12, 0.15)
	if delta == 0 {
		return false
	}
	updated := false
	for _, step := range plan.Steps {
		if step == nil || !stepScopeIntersects(step.Scope, targets) {
			continue
		}
		step.ConfidenceScore = clampConfidenceScore(step.ConfidenceScore + delta)
		step.UpdatedAt = now
		updated = true
	}
	if updated {
		plan.UpdatedAt = now
	}
	return updated
}

func adjustAnchorConfidence(plan *frameworkplan.LivingPlan, anchorID string, resolution ResolutionKind, now time.Time) bool {
	if plan == nil || strings.TrimSpace(anchorID) == "" {
		return false
	}
	anchorID = strings.TrimSpace(anchorID)
	delta := confidenceDelta(resolution, 0.10, 0.18)
	if delta == 0 {
		return false
	}
	updated := false
	for _, step := range plan.Steps {
		if step == nil || !stepDependsOnAnchor(step, anchorID) {
			continue
		}
		step.ConfidenceScore = clampConfidenceScore(step.ConfidenceScore + delta)
		step.UpdatedAt = now
		updated = true
	}
	if updated {
		plan.UpdatedAt = now
	}
	return updated
}

func confidenceDelta(resolution ResolutionKind, positive, negative float64) float64 {
	switch resolution {
	case ResolutionConfirm:
		return positive
	case ResolutionReject:
		return -negative
	case ResolutionRefine:
		return positive / 2
	default:
		return 0
	}
}

func stepScopeIntersects(scope []string, targets map[string]struct{}) bool {
	for _, symbolID := range scope {
		if _, ok := targets[strings.TrimSpace(symbolID)]; ok {
			return true
		}
	}
	return false
}

func stepDependsOnAnchor(step *frameworkplan.PlanStep, anchorID string) bool {
	if step == nil {
		return false
	}
	for _, dependency := range step.AnchorDependencies {
		if strings.TrimSpace(dependency) == anchorID {
			return true
		}
	}
	if step.EvidenceGate == nil {
		return false
	}
	for _, required := range step.EvidenceGate.RequiredAnchors {
		if strings.TrimSpace(required) == anchorID {
			return true
		}
	}
	return false
}

func clampConfidenceScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func patternEvidence(record patterns.PatternRecord) []EvidenceRef {
	if len(record.Instances) == 0 {
		return nil
	}
	out := make([]EvidenceRef, 0, len(record.Instances))
	for _, instance := range record.Instances {
		metadata := map[string]any{
			"file_path":  instance.FilePath,
			"start_line": instance.StartLine,
			"end_line":   instance.EndLine,
		}
		if strings.TrimSpace(instance.SymbolID) != "" {
			metadata["symbol_id"] = instance.SymbolID
		}
		out = append(out, EvidenceRef{
			Kind:     "pattern_instance",
			RefID:    strings.TrimSpace(instance.SymbolID),
			Title:    record.Title,
			Summary:  strings.TrimSpace(instance.Excerpt),
			Metadata: metadata,
		})
	}
	return out
}

func anchorDriftEvidence(anchor frameworkretrieval.AnchorRecord, event frameworkretrieval.AnchorEventRecord) []EvidenceRef {
	metadata := map[string]any{
		"anchor_id":      anchor.AnchorID,
		"term":           anchor.Term,
		"definition":     anchor.Definition,
		"anchor_class":   anchor.AnchorClass,
		"corpus_scope":   anchor.CorpusScope,
		"context":        anchor.ContextSummary,
		"drift_detail":   event.Detail,
		"drift_event_id": event.EventID,
	}
	return []EvidenceRef{{
		Kind:     "anchor_drift",
		RefID:    anchor.AnchorID,
		Title:    anchor.Term,
		Summary:  strings.TrimSpace(event.Detail),
		Metadata: metadata,
	}}
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
