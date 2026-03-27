package learning_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/archaeo/phases"
	"github.com/lexcodex/relurpify/archaeo/plans"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	"github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
	eucloplan "github.com/lexcodex/relurpify/named/euclo/plan"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

type learningPlanStore struct {
	plan    *frameworkplan.LivingPlan
	updates map[string]*frameworkplan.PlanStep
}

func (s *learningPlanStore) SavePlan(context.Context, *frameworkplan.LivingPlan) error { return nil }
func (s *learningPlanStore) LoadPlan(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return s.plan, nil
}
func (s *learningPlanStore) LoadPlanByWorkflow(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return s.plan, nil
}
func (s *learningPlanStore) UpdateStep(_ context.Context, _ string, stepID string, step *frameworkplan.PlanStep) error {
	if s.updates == nil {
		s.updates = map[string]*frameworkplan.PlanStep{}
	}
	copy := *step
	s.updates[stepID] = &copy
	return nil
}
func (s *learningPlanStore) InvalidateStep(context.Context, string, string, frameworkplan.InvalidationRule) error {
	return nil
}
func (s *learningPlanStore) DeletePlan(context.Context, string) error { return nil }
func (s *learningPlanStore) ListPlans(context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

func TestServiceCreatePersistsInteractionAndUpdatesPhaseQueue(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-learning",
		TaskID:      "task-learning",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	phaseSvc := phases.Service{Store: store}
	now := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	svc := learning.Service{
		Store:  store,
		Phases: &phaseSvc,
		Now:    func() time.Time { return now },
		NewID:  newSequenceID(),
	}

	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-learning",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionPatternProposal,
		SubjectType:   learning.SubjectPattern,
		SubjectID:     "pattern-1",
		Title:         "Confirm inferred pattern",
	})
	require.NoError(t, err)
	require.Equal(t, learning.StatusPending, interaction.Status)
	require.Len(t, interaction.Choices, 4)

	pending, err := svc.Pending(ctx, "wf-learning")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, interaction.ID, pending[0].ID)

	phaseState, ok, err := phaseSvc.Load(ctx, "wf-learning")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []string{interaction.ID}, phaseState.PendingLearning)

	events, err := store.ListEvents(ctx, "wf-learning", 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	var foundRequested bool
	for _, event := range events {
		if event.EventType == "archaeo.learning_interaction_requested" {
			foundRequested = true
		}
	}
	require.True(t, foundRequested)
}

func TestServicePublishesBrokerEvents(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-broker",
		TaskID:      "task-broker",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	broker := learning.NewBroker(time.Minute)
	events, cancel := broker.Subscribe(4)
	defer cancel()

	svc := learning.Service{
		Store:  store,
		Broker: broker,
		Now:    func() time.Time { return time.Date(2026, 3, 26, 10, 5, 0, 0, time.UTC) },
		NewID:  newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-broker",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionTensionReview,
		SubjectType:   learning.SubjectTension,
		SubjectID:     "tension-1",
		Title:         "Review tension",
	})
	require.NoError(t, err)
	requested := <-events
	require.Equal(t, learning.EventRequested, requested.Type)
	require.Equal(t, interaction.ID, requested.Interaction.ID)

	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-broker",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionConfirm,
	})
	require.NoError(t, err)
	resolved := <-events
	require.Equal(t, learning.EventResolved, resolved.Type)
	require.Equal(t, learning.StatusResolved, resolved.Interaction.Status)
}

func TestServiceSyncPatternProposalsCreatesOnceAndHydratesEvidence(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-sync",
		TaskID:      "task-sync",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	patternStore, _ := openPatternStores(t)
	require.NoError(t, patternStore.Save(ctx, patterns.PatternRecord{
		ID:          "pattern-sync",
		Kind:        patterns.PatternKindStructural,
		Title:       "Adapter",
		Description: "Use adapters at the boundary",
		Status:      patterns.PatternStatusProposed,
		Instances: []patterns.PatternInstance{{
			FilePath:  "adapter.go",
			StartLine: 10,
			EndLine:   18,
			Excerpt:   "type Adapter struct{}",
			SymbolID:  "pkg.Adapter",
		}},
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))
	phaseSvc := phases.Service{Store: store}
	svc := learning.Service{
		Store:        store,
		PatternStore: patternStore,
		Phases:       &phaseSvc,
		Now:          func() time.Time { return time.Date(2026, 3, 26, 10, 30, 0, 0, time.UTC) },
		NewID:        newSequenceID(),
	}

	created, err := svc.SyncPatternProposals(ctx, "wf-sync", "explore-1", "workspace", "rev-sync")
	require.NoError(t, err)
	require.Len(t, created, 1)
	require.Equal(t, learning.InteractionPatternProposal, created[0].Kind)
	require.True(t, created[0].Blocking)
	require.Len(t, created[0].Evidence, 1)
	require.Equal(t, "pkg.Adapter", created[0].Evidence[0].RefID)

	created, err = svc.SyncPatternProposals(ctx, "wf-sync", "explore-1", "workspace", "rev-sync")
	require.NoError(t, err)
	require.Empty(t, created)

	pending, err := svc.Pending(ctx, "wf-sync")
	require.NoError(t, err)
	require.Len(t, pending, 1)
}

func TestServiceSyncAnchorDriftsCreatesAnchorLearningInteractions(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-drift",
		TaskID:      "task-drift",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	retrievalDB := openRetrievalDB(t)
	anchor, err := retrieval.DeclareAnchor(ctx, retrievalDB, retrieval.AnchorDeclaration{
		Term:       "compatibility",
		Definition: "preserve wire format",
		Class:      "policy",
	}, "workspace", "workspace_trusted")
	require.NoError(t, err)
	require.NoError(t, retrieval.RecordAnchorDrift(ctx, retrievalDB, anchor.AnchorID, "significant", "implementation diverged"))

	phaseSvc := phases.Service{Store: store}
	svc := learning.Service{
		Store:     store,
		Retrieval: archaeoretrieval.NewSQLStore(retrievalDB),
		Phases:      &phaseSvc,
		Now:         func() time.Time { return time.Date(2026, 3, 26, 10, 45, 0, 0, time.UTC) },
		NewID:       newSequenceID(),
	}

	created, err := svc.SyncAnchorDrifts(ctx, "wf-drift", "explore-1", "workspace", "rev-drift")
	require.NoError(t, err)
	require.Len(t, created, 1)
	require.Equal(t, learning.InteractionAnchorProposal, created[0].Kind)
	require.True(t, created[0].Blocking)
	require.Equal(t, anchor.AnchorID, created[0].SubjectID)
	require.Len(t, created[0].Evidence, 1)
	require.Equal(t, "anchor_drift", created[0].Evidence[0].Kind)
	require.Contains(t, created[0].Description, "implementation diverged")

	created, err = svc.SyncAnchorDrifts(ctx, "wf-drift", "explore-1", "workspace", "rev-drift")
	require.NoError(t, err)
	require.Empty(t, created)

	pending, err := svc.Pending(ctx, "wf-drift")
	require.NoError(t, err)
	require.Len(t, pending, 1)
}

func TestServiceSyncTensionsCreatesLearningInteractions(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-tension-sync",
		TaskID:      "task-tension-sync",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	_, err := tensions.Service{Store: store, NewID: newSequenceID()}.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:         "wf-tension-sync",
		ExplorationID:      "explore-1",
		SnapshotID:         "snapshot-1",
		SourceRef:          "gap-1",
		Kind:               "intent_gap",
		Description:        "Boundary behavior contradicts declared intent",
		Severity:           "significant",
		Status:             archaeodomain.TensionUnresolved,
		RelatedPlanStepIDs: []string{"step-1"},
		BasedOnRevision:    "rev-tension",
	})
	require.NoError(t, err)
	phaseSvc := phases.Service{Store: store}
	svc := learning.Service{
		Store:  store,
		Phases: &phaseSvc,
		Now:    func() time.Time { return time.Date(2026, 3, 26, 10, 45, 0, 0, time.UTC) },
		NewID:  newSequenceID(),
	}

	created, err := svc.SyncTensions(ctx, "wf-tension-sync", "explore-1", "snapshot-1", "rev-tension")
	require.NoError(t, err)
	require.Len(t, created, 1)
	require.Equal(t, learning.InteractionTensionReview, created[0].Kind)
	require.True(t, created[0].Blocking)
	require.Equal(t, "unresolved", created[0].DefaultChoice)
	require.Len(t, created[0].Evidence, 1)
	require.Equal(t, "tension", created[0].Evidence[0].Kind)

	created, err = svc.SyncTensions(ctx, "wf-tension-sync", "explore-1", "snapshot-1", "rev-tension")
	require.NoError(t, err)
	require.Empty(t, created)
}

func TestServiceResolveConfirmPatternUpdatesPatternAndPersistsComment(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-pattern",
		TaskID:      "task-pattern",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	patternStore, commentStore := openPatternStores(t)
	require.NoError(t, patternStore.Save(ctx, patterns.PatternRecord{
		ID:           "pattern-1",
		Kind:         patterns.PatternKindStructural,
		Title:        "Wrap errors",
		Description:  "wrap them",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))
	phaseSvc := phases.Service{Store: store}
	svc := learning.Service{
		Store:        store,
		PatternStore: patternStore,
		CommentStore: commentStore,
		Phases:       &phaseSvc,
		Now:          func() time.Time { return time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC) },
		NewID:        newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-pattern",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionPatternProposal,
		SubjectType:   learning.SubjectPattern,
		SubjectID:     "pattern-1",
		Title:         "Confirm pattern",
	})
	require.NoError(t, err)

	resolved, err := svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:      "wf-pattern",
		InteractionID:   interaction.ID,
		Kind:            learning.ResolutionConfirm,
		ChoiceID:        "confirm",
		ResolvedBy:      "human",
		BasedOnRevision: "rev-1",
		Comment: &learning.CommentInput{
			IntentType: "intentional",
			AuthorKind: "human",
			Body:       "This is a real project rule.",
		},
	})
	require.NoError(t, err)
	require.Equal(t, learning.StatusResolved, resolved.Status)
	require.NotNil(t, resolved.Resolution)
	require.NotEmpty(t, resolved.Resolution.CommentRef)

	record, err := patternStore.Load(ctx, "pattern-1")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, patterns.PatternStatusConfirmed, record.Status)
	require.Equal(t, "human", record.ConfirmedBy)
	require.Contains(t, record.CommentIDs, resolved.Resolution.CommentRef)

	comment, err := commentStore.Load(ctx, resolved.Resolution.CommentRef)
	require.NoError(t, err)
	require.NotNil(t, comment)
	require.Equal(t, "pattern-1", comment.PatternID)

	phaseState, ok, err := phaseSvc.Load(ctx, "wf-pattern")
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, phaseState.PendingLearning)
}

func TestServiceResolvePatternAdjustsPlanConfidence(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-confidence",
		TaskID:      "task-confidence",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	patternStore, _ := openPatternStores(t)
	require.NoError(t, patternStore.Save(ctx, patterns.PatternRecord{
		ID:          "pattern-1",
		Kind:        patterns.PatternKindStructural,
		Title:       "Adapter",
		Description: "Use adapters",
		Status:      patterns.PatternStatusProposed,
		Instances: []patterns.PatternInstance{{
			FilePath: "adapter.go", SymbolID: "pkg.Adapter",
		}},
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))
	planStore := &learningPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-confidence",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", Scope: []string{"pkg.Adapter"}, ConfidenceScore: 0.40, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
				"step-2": {ID: "step-2", Scope: []string{"pkg.Other"}, ConfidenceScore: 0.40, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
			},
		},
	}
	svc := learning.Service{
		Store:        store,
		PatternStore: patternStore,
		PlanStore:    planStore,
		Now:          func() time.Time { return time.Date(2026, 3, 26, 11, 15, 0, 0, time.UTC) },
		NewID:        newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-confidence",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionPatternProposal,
		SubjectType:   learning.SubjectPattern,
		SubjectID:     "pattern-1",
		Title:         "Confirm pattern",
	})
	require.NoError(t, err)

	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-confidence",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionConfirm,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.52, planStore.plan.Steps["step-1"].ConfidenceScore, 0.0001)
	require.InDelta(t, 0.40, planStore.plan.Steps["step-2"].ConfidenceScore, 0.0001)

	interaction, err = svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-confidence",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionPatternProposal,
		SubjectType:   learning.SubjectPattern,
		SubjectID:     "pattern-1",
		Title:         "Reject pattern",
	})
	require.NoError(t, err)
	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-confidence",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionReject,
	})
	require.NoError(t, err)
	require.InDelta(t, 0.37, planStore.plan.Steps["step-1"].ConfidenceScore, 0.0001)
}

func TestServiceResolveAnchorAdjustsPlanConfidence(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-anchor-confidence",
		TaskID:      "task-anchor-confidence",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	retrievalDB := openRetrievalDB(t)
	anchor, err := retrieval.DeclareAnchor(ctx, retrievalDB, retrieval.AnchorDeclaration{
		Term:       "compatibility",
		Definition: "preserve wire format",
		Class:      "policy",
	}, "workspace", "workspace_trusted")
	require.NoError(t, err)
	planStore := &learningPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-anchor-confidence",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", AnchorDependencies: []string{anchor.AnchorID}, ConfidenceScore: 0.50, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
			},
		},
	}
	svc := learning.Service{
		Store:     store,
		Retrieval: archaeoretrieval.NewSQLStore(retrievalDB),
		PlanStore: planStore,
		Now:         func() time.Time { return time.Date(2026, 3, 26, 11, 20, 0, 0, time.UTC) },
		NewID:       newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-anchor-confidence",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionAnchorProposal,
		SubjectType:   learning.SubjectAnchor,
		SubjectID:     anchor.AnchorID,
		Title:         "Refine anchor",
	})
	require.NoError(t, err)
	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-anchor-confidence",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionRefine,
		RefinedPayload: map[string]any{
			"definition": "preserve protocol and wire format",
		},
	})
	require.NoError(t, err)
	require.InDelta(t, 0.55, planStore.plan.Steps["step-1"].ConfidenceScore, 0.0001)
}

func TestServiceResolveRefinePatternSupersedesPattern(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-refine",
		TaskID:      "task-refine",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	patternStore, _ := openPatternStores(t)
	require.NoError(t, patternStore.Save(ctx, patterns.PatternRecord{
		ID:           "pattern-old",
		Kind:         patterns.PatternKindStructural,
		Title:        "Old title",
		Description:  "old desc",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))
	svc := learning.Service{
		Store:        store,
		PatternStore: patternStore,
		Now:          func() time.Time { return time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC) },
		NewID:        newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-refine",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionIntentRefinement,
		SubjectType:   learning.SubjectPattern,
		SubjectID:     "pattern-old",
		Title:         "Refine pattern",
	})
	require.NoError(t, err)

	resolved, err := svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-refine",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionRefine,
		ResolvedBy:    "reviewer",
		RefinedPayload: map[string]any{
			"title":       "New title",
			"description": "new desc",
			"confidence":  0.95,
		},
	})
	require.NoError(t, err)
	require.Equal(t, learning.StatusResolved, resolved.Status)

	oldRecord, err := patternStore.Load(ctx, "pattern-old")
	require.NoError(t, err)
	require.Equal(t, patterns.PatternStatusSuperseded, oldRecord.Status)
	require.NotEmpty(t, oldRecord.SupersededBy)

	newRecord, err := patternStore.Load(ctx, oldRecord.SupersededBy)
	require.NoError(t, err)
	require.Equal(t, "New title", newRecord.Title)
	require.Equal(t, "new desc", newRecord.Description)
	require.Equal(t, 0.95, newRecord.Confidence)
	require.Equal(t, patterns.PatternStatusConfirmed, newRecord.Status)
}

func TestServiceResolveAnchorInteractionsMutateRetrieval(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-anchor",
		TaskID:      "task-anchor",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	retrievalDB := openRetrievalDB(t)
	svc := learning.Service{
		Store:     store,
		Retrieval: archaeoretrieval.NewSQLStore(retrievalDB),
		Now:         func() time.Time { return time.Date(2026, 3, 26, 13, 0, 0, 0, time.UTC) },
		NewID:       newSequenceID(),
	}

	create, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-anchor",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionAnchorProposal,
		SubjectType:   learning.SubjectAnchor,
		Title:         "Confirm anchor",
	})
	require.NoError(t, err)
	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-anchor",
		InteractionID: create.ID,
		Kind:          learning.ResolutionConfirm,
		RefinedPayload: map[string]any{
			"term":         "compatibility",
			"definition":   "must preserve existing wire format",
			"class":        "policy",
			"corpus_scope": "workspace",
			"trust_class":  "workspace_trusted",
		},
	})
	require.NoError(t, err)
	anchors, err := retrieval.ActiveAnchors(ctx, retrievalDB, "workspace")
	require.NoError(t, err)
	require.Len(t, anchors, 1)

	refine, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-anchor",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionAnchorProposal,
		SubjectType:   learning.SubjectAnchor,
		SubjectID:     anchors[0].AnchorID,
		Title:         "Refine anchor",
	})
	require.NoError(t, err)
	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-anchor",
		InteractionID: refine.ID,
		Kind:          learning.ResolutionRefine,
		RefinedPayload: map[string]any{
			"definition": "must preserve protocol and wire format",
			"context": map[string]any{
				"source": "review",
			},
		},
	})
	require.NoError(t, err)
	history, err := retrieval.AnchorHistory(ctx, retrievalDB, "compatibility", "workspace")
	require.NoError(t, err)
	require.Len(t, history, 2)
	activeAfterRefine, err := retrieval.ActiveAnchors(ctx, retrievalDB, "workspace")
	require.NoError(t, err)
	require.Len(t, activeAfterRefine, 2)
	activeAnchorID := ""
	for _, record := range activeAfterRefine {
		if record.SupersededBy == nil {
			activeAnchorID = record.AnchorID
			break
		}
	}
	require.NotEmpty(t, activeAnchorID)

	reject, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-anchor",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionAnchorProposal,
		SubjectType:   learning.SubjectAnchor,
		SubjectID:     activeAnchorID,
		Title:         "Reject anchor",
	})
	require.NoError(t, err)
	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-anchor",
		InteractionID: reject.ID,
		Kind:          learning.ResolutionReject,
		Comment: &learning.CommentInput{
			Body: "No longer applicable",
		},
	})
	require.NoError(t, err)
	active, err := retrieval.ActiveAnchors(ctx, retrievalDB, "workspace")
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.NotEqual(t, activeAnchorID, active[0].AnchorID)
}

func TestServiceResolveDeferMarksDeferredWithoutSemanticMutation(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-defer",
		TaskID:      "task-defer",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	patternStore, _ := openPatternStores(t)
	require.NoError(t, patternStore.Save(ctx, patterns.PatternRecord{
		ID:           "pattern-defer",
		Kind:         patterns.PatternKindStructural,
		Title:        "Maybe",
		Description:  "maybe",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))
	phaseSvc := phases.Service{Store: store}
	svc := learning.Service{
		Store:        store,
		PatternStore: patternStore,
		Phases:       &phaseSvc,
		Now:          func() time.Time { return time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC) },
		NewID:        newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-defer",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionPatternProposal,
		SubjectType:   learning.SubjectPattern,
		SubjectID:     "pattern-defer",
		Title:         "Defer pattern",
	})
	require.NoError(t, err)

	resolved, err := svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-defer",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionDefer,
	})
	require.NoError(t, err)
	require.Equal(t, learning.StatusDeferred, resolved.Status)

	record, err := patternStore.Load(ctx, "pattern-defer")
	require.NoError(t, err)
	require.Equal(t, patterns.PatternStatusProposed, record.Status)

	phaseState, ok, err := phaseSvc.Load(ctx, "wf-defer")
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, phaseState.PendingLearning)
}

func TestServiceBlockingPendingFiltersQueue(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-blocking",
		TaskID:      "task-blocking",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := learning.Service{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC) },
		NewID: newSequenceID(),
	}
	_, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-blocking",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionTensionReview,
		SubjectType:   learning.SubjectTension,
		SubjectID:     "tension-1",
		Title:         "Blocking",
		Blocking:      true,
	})
	require.NoError(t, err)
	_, err = svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-blocking",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionTensionReview,
		SubjectType:   learning.SubjectTension,
		SubjectID:     "tension-2",
		Title:         "Nonblocking",
		Blocking:      false,
	})
	require.NoError(t, err)

	pending, err := svc.Pending(ctx, "wf-blocking")
	require.NoError(t, err)
	require.Len(t, pending, 2)
	blocking, err := svc.BlockingPending(ctx, "wf-blocking")
	require.NoError(t, err)
	require.Len(t, blocking, 1)
	require.Equal(t, "tension-1", blocking[0].SubjectID)
}

func TestServiceResolveMarksActivePlanVersionStaleForReferencedPattern(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-version-stale",
		TaskID:      "task-version-stale",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	planStore, err := eucloplan.NewSQLitePlanStore(store.DB())
	require.NoError(t, err)
	patternStore, _ := openPatternStores(t)
	require.NoError(t, patternStore.Save(ctx, patterns.PatternRecord{
		ID:           "pattern-version",
		Kind:         patterns.PatternKindStructural,
		Title:        "Adapter",
		Description:  "Use adapters",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))
	versionSvc := plans.Service{
		Store:         planStore,
		WorkflowStore: store,
		Now:           func() time.Time { return time.Date(2026, 3, 27, 4, 0, 0, 0, time.UTC) },
	}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-version",
		WorkflowID: "wf-version-stale",
		Title:      "Versioned",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	version, err := versionSvc.DraftVersion(ctx, plan, plans.DraftVersionInput{
		WorkflowID:  "wf-version-stale",
		PatternRefs: []string{"pattern-version"},
	})
	require.NoError(t, err)
	_, err = versionSvc.ActivateVersion(ctx, "wf-version-stale", version.Version)
	require.NoError(t, err)

	svc := learning.Service{
		Store:        store,
		PatternStore: patternStore,
		PlanStore:    planStore,
		Now:          func() time.Time { return time.Date(2026, 3, 27, 4, 5, 0, 0, time.UTC) },
		NewID:        newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-version-stale",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionPatternProposal,
		SubjectType:   learning.SubjectPattern,
		SubjectID:     "pattern-version",
		Title:         "Confirm pattern",
	})
	require.NoError(t, err)

	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-version-stale",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionConfirm,
		ResolvedBy:    "reviewer",
	})
	require.NoError(t, err)

	active, err := versionSvc.LoadActiveVersion(ctx, "wf-version-stale")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.True(t, active.RecomputeRequired)
	require.Contains(t, active.StaleReason, "pattern:pattern-version")

	versions, err := versionSvc.ListVersions(ctx, "wf-version-stale")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, archaeodomain.LivingPlanVersionActive, versions[0].Status)
	require.True(t, versions[0].RecomputeRequired)
	require.Equal(t, archaeodomain.LivingPlanVersionDraft, versions[1].Status)
	require.NotNil(t, versions[1].ParentVersion)
	require.Equal(t, 1, *versions[1].ParentVersion)
	require.Contains(t, versions[1].StaleReason, "pattern:pattern-version")

	mutations, err := archaeoevents.ReadMutationEvents(ctx, store, "wf-version-stale")
	require.NoError(t, err)
	require.Len(t, mutations, 3)
	require.Contains(t, mutationCategories(mutations), archaeodomain.MutationConfidenceChange)
	require.Equal(t, 2, countMutationCategory(mutations, archaeodomain.MutationPlanStaleness))
}

func TestServiceResolveTensionAcceptsWithoutStalingVersion(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-tension-accept",
		TaskID:      "task-tension-accept",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	planStore, err := eucloplan.NewSQLitePlanStore(store.DB())
	require.NoError(t, err)
	versionSvc := plans.Service{
		Store:         planStore,
		WorkflowStore: store,
		Now:           func() time.Time { return time.Date(2026, 3, 27, 5, 0, 0, 0, time.UTC) },
	}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-tension-accept",
		WorkflowID: "wf-tension-accept",
		Title:      "Versioned",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", ConfidenceScore: 0.50, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	version, err := versionSvc.DraftVersion(ctx, plan, plans.DraftVersionInput{
		WorkflowID:  "wf-tension-accept",
		TensionRefs: []string{"tension-1"},
	})
	require.NoError(t, err)
	_, err = versionSvc.ActivateVersion(ctx, "wf-tension-accept", version.Version)
	require.NoError(t, err)

	tensionSvc := tensions.Service{Store: store, NewID: newSequenceID()}
	record, err := tensionSvc.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:         "wf-tension-accept",
		ExplorationID:      "explore-1",
		SourceRef:          "gap-accept",
		Kind:               "intent_gap",
		Description:        "Known tradeoff",
		Severity:           "significant",
		Status:             archaeodomain.TensionUnresolved,
		RelatedPlanStepIDs: []string{"step-1"},
	})
	require.NoError(t, err)

	_, commentStore := openPatternStores(t)
	svc := learning.Service{
		Store:        store,
		CommentStore: commentStore,
		PlanStore:    planStore,
		Now:          func() time.Time { return time.Date(2026, 3, 27, 5, 5, 0, 0, time.UTC) },
		NewID:        newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-tension-accept",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionTensionReview,
		SubjectType:   learning.SubjectTension,
		SubjectID:     record.ID,
		Title:         "Review tension",
	})
	require.NoError(t, err)

	resolved, err := svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-tension-accept",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionConfirm,
		ChoiceID:      "accept",
		Comment: &learning.CommentInput{
			IntentType: "intentional",
			AuthorKind: "human",
			Body:       "Accepted as intentional debt.",
		},
	})
	require.NoError(t, err)

	updated, err := tensionSvc.Load(ctx, "wf-tension-accept", record.ID)
	require.NoError(t, err)
	require.Equal(t, archaeodomain.TensionAccepted, updated.Status)
	require.NotEmpty(t, updated.CommentRefs)

	active, err := versionSvc.LoadActiveVersion(ctx, "wf-tension-accept")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.False(t, active.RecomputeRequired)

	versions, err := versionSvc.ListVersions(ctx, "wf-tension-accept")
	require.NoError(t, err)
	require.Len(t, versions, 1)

	comment, err := commentStore.Load(ctx, resolved.Resolution.CommentRef)
	require.NoError(t, err)
	require.NotNil(t, comment)
	require.Equal(t, record.ID, comment.TensionID)
}

func TestServiceResolveTensionUnresolvedStalesVersionAndAdjustsConfidence(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-tension-stale",
		TaskID:      "task-tension-stale",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	planStore, err := eucloplan.NewSQLitePlanStore(store.DB())
	require.NoError(t, err)
	versionSvc := plans.Service{
		Store:         planStore,
		WorkflowStore: store,
		Now:           func() time.Time { return time.Date(2026, 3, 27, 6, 0, 0, 0, time.UTC) },
	}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-tension-stale",
		WorkflowID: "wf-tension-stale",
		Title:      "Versioned",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", ConfidenceScore: 0.60, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	version, err := versionSvc.DraftVersion(ctx, plan, plans.DraftVersionInput{
		WorkflowID:  "wf-tension-stale",
		TensionRefs: []string{"tension-1"},
	})
	require.NoError(t, err)
	_, err = versionSvc.ActivateVersion(ctx, "wf-tension-stale", version.Version)
	require.NoError(t, err)

	tensionSvc := tensions.Service{Store: store, NewID: newSequenceID()}
	record, err := tensionSvc.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:         "wf-tension-stale",
		ExplorationID:      "explore-1",
		SourceRef:          "gap-unresolved",
		Kind:               "intent_gap",
		Description:        "Real unresolved contradiction",
		Severity:           "critical",
		Status:             archaeodomain.TensionInferred,
		RelatedPlanStepIDs: []string{"step-1"},
	})
	require.NoError(t, err)

	svc := learning.Service{
		Store:     store,
		PlanStore: planStore,
		Now:       func() time.Time { return time.Date(2026, 3, 27, 6, 5, 0, 0, time.UTC) },
		NewID:     newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-tension-stale",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionTensionReview,
		SubjectType:   learning.SubjectTension,
		SubjectID:     record.ID,
		Title:         "Review tension",
	})
	require.NoError(t, err)

	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-tension-stale",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionConfirm,
		ChoiceID:      "unresolved",
	})
	require.NoError(t, err)

	updated, err := tensionSvc.Load(ctx, "wf-tension-stale", record.ID)
	require.NoError(t, err)
	require.Equal(t, archaeodomain.TensionUnresolved, updated.Status)

	active, err := versionSvc.LoadActiveVersion(ctx, "wf-tension-stale")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.True(t, active.RecomputeRequired)
	require.Contains(t, active.StaleReason, "tension:")

	versions, err := versionSvc.ListVersions(ctx, "wf-tension-stale")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, archaeodomain.LivingPlanVersionDraft, versions[1].Status)

	mutations, err := archaeoevents.ReadMutationEvents(ctx, store, "wf-tension-stale")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(mutations), 4)
	require.Contains(t, mutationCategories(mutations), archaeodomain.MutationStepInvalidation)
	require.Contains(t, mutationCategories(mutations), archaeodomain.MutationBlockingSemantic)
}

func mutationCategories(events []archaeodomain.MutationEvent) []archaeodomain.MutationCategory {
	out := make([]archaeodomain.MutationCategory, 0, len(events))
	for _, event := range events {
		out = append(out, event.Category)
	}
	return out
}

func countMutationCategory(events []archaeodomain.MutationEvent, category archaeodomain.MutationCategory) int {
	count := 0
	for _, event := range events {
		if event.Category == category {
			count++
		}
	}
	return count
}

func TestServiceResolveTensionAdjustsPlanConfidence(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-tension-confidence",
		TaskID:      "task-tension-confidence",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	planStore := &learningPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-tension-confidence",
			WorkflowID: "wf-tension-confidence",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", ConfidenceScore: 0.60, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
			},
		},
	}
	tensionSvc := tensions.Service{Store: store, NewID: newSequenceID()}
	record, err := tensionSvc.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:         "wf-tension-confidence",
		ExplorationID:      "explore-1",
		SourceRef:          "gap-confidence",
		Kind:               "intent_gap",
		Description:        "Real unresolved contradiction",
		Severity:           "critical",
		Status:             archaeodomain.TensionInferred,
		RelatedPlanStepIDs: []string{"step-1"},
	})
	require.NoError(t, err)

	svc := learning.Service{
		Store:     store,
		PlanStore: planStore,
		Now:       func() time.Time { return time.Date(2026, 3, 27, 6, 30, 0, 0, time.UTC) },
		NewID:     newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-tension-confidence",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionTensionReview,
		SubjectType:   learning.SubjectTension,
		SubjectID:     record.ID,
		Title:         "Review tension",
	})
	require.NoError(t, err)

	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-tension-confidence",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionConfirm,
		ChoiceID:      "unresolved",
	})
	require.NoError(t, err)
	require.InDelta(t, 0.52, planStore.plan.Steps["step-1"].ConfidenceScore, 0.0001)
}

func TestServiceExpireAndRejectResolvedTransition(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-expire",
		TaskID:      "task-expire",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	phaseSvc := phases.Service{Store: store}
	svc := learning.Service{
		Store:  store,
		Phases: &phaseSvc,
		Now:    func() time.Time { return time.Date(2026, 3, 26, 15, 0, 0, 0, time.UTC) },
		NewID:  newSequenceID(),
	}
	interaction, err := svc.Create(ctx, learning.CreateInput{
		WorkflowID:    "wf-expire",
		ExplorationID: "explore-1",
		Kind:          learning.InteractionTensionReview,
		SubjectType:   learning.SubjectTension,
		SubjectID:     "tension-1",
		Title:         "Review tension",
	})
	require.NoError(t, err)

	expired, err := svc.Expire(ctx, "wf-expire", interaction.ID, "timed out")
	require.NoError(t, err)
	require.Equal(t, learning.StatusExpired, expired.Status)

	phaseState, ok, err := phaseSvc.Load(ctx, "wf-expire")
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, phaseState.PendingLearning)

	_, err = svc.Resolve(ctx, learning.ResolveInput{
		WorkflowID:    "wf-expire",
		InteractionID: interaction.ID,
		Kind:          learning.ResolutionConfirm,
	})
	require.Error(t, err)
}

func newWorkflowStore(t *testing.T) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func openPatternStores(t *testing.T) (*patterns.SQLitePatternStore, *patterns.SQLiteCommentStore) {
	t.Helper()
	db, err := patterns.OpenSQLite(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	patternStore, err := patterns.NewSQLitePatternStore(db)
	require.NoError(t, err)
	commentStore, err := patterns.NewSQLiteCommentStore(db)
	require.NoError(t, err)
	return patternStore, commentStore
}

func openRetrievalDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, retrieval.EnsureSchema(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func newSequenceID() func(prefix string) string {
	seq := 0
	return func(prefix string) string {
		seq++
		return prefix + "-" + fmt.Sprint(seq)
	}
}
