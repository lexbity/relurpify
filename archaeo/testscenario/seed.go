package testscenario

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"time"

	archaeoarch "codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

func SeedWorkflow(t testing.TB, store *memorydb.SQLiteWorkflowStateStore, workflowID, instruction string) {
	t.Helper()
	fixture := &Fixture{T: t, WorkflowStore: store}
	record := fixture.WorkflowRecord(workflowID, instruction)
	if err := store.CreateWorkflow(context.Background(), record); err != nil {
		t.Fatalf("create workflow %s: %v", workflowID, err)
	}
}

func SeedExplorationSession(t testing.TB, fixture *Fixture, workflowID, workspaceID, revision string) *archaeodomain.ExplorationSession {
	t.Helper()
	if fixture == nil {
		t.Fatal("fixture required")
	}
	if workspaceID == "" {
		workspaceID = fixture.Workspace
	}
	session, err := fixture.ArchaeologyService().EnsureExplorationSession(fixture.Context(), workflowID, workspaceID, revision)
	if err != nil {
		t.Fatalf("ensure exploration session: %v", err)
	}
	return session
}

func SeedExplorationSnapshot(t testing.TB, fixture *Fixture, session *archaeodomain.ExplorationSession, patterns []patterns.PatternRecord, anchors []string, tensions []archaeodomain.Tension) *archaeodomain.ExplorationSnapshot {
	t.Helper()
	if fixture == nil || session == nil {
		t.Fatal("fixture and session required")
	}
	patternRefs := make([]string, 0, len(patterns))
	for _, record := range patterns {
		patternRefs = append(patternRefs, record.ID)
	}
	tensionIDs := make([]string, 0, len(tensions))
	for _, tension := range tensions {
		tensionIDs = append(tensionIDs, tension.ID)
	}
	snapshot, err := fixture.ArchaeologyService().CreateExplorationSnapshot(fixture.Context(), session, archaeoarch.SnapshotInput{
		WorkflowID:           sessionIDWorkflowID(t, fixture, session.ID),
		WorkspaceID:          session.WorkspaceID,
		BasedOnRevision:      session.BasedOnRevision,
		CandidatePatternRefs: patternRefs,
		CandidateAnchorRefs:  append([]string(nil), anchors...),
		TensionIDs:           tensionIDs,
		Summary:              "scenario snapshot",
	})
	if err != nil {
		t.Fatalf("create exploration snapshot: %v", err)
	}
	return snapshot
}

func SeedPatterns(t testing.TB, store *patterns.SQLitePatternStore, corpusScope string, count int) []patterns.PatternRecord {
	t.Helper()
	now := time.Now().UTC()
	out := make([]patterns.PatternRecord, 0, count)
	for i := 0; i < count; i++ {
		record := patterns.PatternRecord{
			ID:           fmt.Sprintf("pattern-%03d", i+1),
			Kind:         patterns.PatternKindStructural,
			Title:        fmt.Sprintf("Pattern %03d", i+1),
			Description:  "scenario pattern",
			Status:       patterns.PatternStatusProposed,
			CorpusScope:  corpusScope,
			CorpusSource: corpusScope,
			Confidence:   0.75,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := store.Save(context.Background(), record); err != nil {
			t.Fatalf("save pattern %s: %v", record.ID, err)
		}
		out = append(out, record)
	}
	return out
}

func SeedTensions(t testing.TB, fixture *Fixture, workflowID, explorationID, snapshotID string, count int) []archaeodomain.Tension {
	t.Helper()
	if fixture == nil {
		t.Fatal("fixture required")
	}
	out := make([]archaeodomain.Tension, 0, count)
	for i := 0; i < count; i++ {
		record, err := fixture.TensionService().CreateOrUpdate(fixture.Context(), archaeotensions.CreateInput{
			WorkflowID:      workflowID,
			ExplorationID:   explorationID,
			SnapshotID:      snapshotID,
			SourceRef:       fmt.Sprintf("tension-source-%03d", i+1),
			Kind:            "contradiction",
			Description:     fmt.Sprintf("scenario tension %03d", i+1),
			Severity:        "medium",
			Status:          archaeodomain.TensionInferred,
			BasedOnRevision: revisionForFixture(fixture, workflowID),
		})
		if err != nil {
			t.Fatalf("create tension: %v", err)
		}
		out = append(out, *record)
	}
	return out
}

func SeedLearningInteractions(t testing.TB, fixture *Fixture, workflowID, explorationID string, count int) []archaeolearning.Interaction {
	t.Helper()
	if fixture == nil {
		t.Fatal("fixture required")
	}
	out := make([]archaeolearning.Interaction, 0, count)
	for i := 0; i < count; i++ {
		record, err := fixture.LearningService().Create(fixture.Context(), archaeolearning.CreateInput{
			WorkflowID:      workflowID,
			ExplorationID:   explorationID,
			Kind:            archaeolearning.InteractionIntentRefinement,
			SubjectType:     archaeolearning.SubjectExploration,
			SubjectID:       fmt.Sprintf("exploration-subject-%03d", i+1),
			Title:           fmt.Sprintf("Learning %03d", i+1),
			Description:     "scenario learning interaction",
			Blocking:        i == 0,
			BasedOnRevision: revisionForFixture(fixture, workflowID),
		})
		if err != nil {
			t.Fatalf("create learning interaction: %v", err)
		}
		out = append(out, *record)
	}
	return out
}

func SeedPlanWithSteps(t testing.TB, fixture *Fixture, workflowID, explorationID string, stepCount int) *archaeodomain.VersionedLivingPlan {
	t.Helper()
	if fixture == nil {
		t.Fatal("fixture required")
	}
	now := fixture.Now()
	plan := &frameworkplan.LivingPlan{
		ID:         fmt.Sprintf("plan-%s", workflowID),
		WorkflowID: workflowID,
		Title:      "Scenario Plan",
		Steps:      make(map[string]*frameworkplan.PlanStep, stepCount),
		StepOrder:  make([]string, 0, stepCount),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	for i := 0; i < stepCount; i++ {
		stepID := fmt.Sprintf("step-%03d", i+1)
		plan.Steps[stepID] = &frameworkplan.PlanStep{
			ID:              stepID,
			Description:     fmt.Sprintf("Scenario step %03d", i+1),
			Status:          frameworkplan.PlanStepPending,
			ConfidenceScore: 0.6,
			CreatedAt:       fixture.Now(),
			UpdatedAt:       fixture.Now(),
		}
		plan.StepOrder = append(plan.StepOrder, stepID)
	}
	record, err := fixture.PlansService().DraftVersion(fixture.Context(), plan, archaeoplans.DraftVersionInput{
		WorkflowID:             workflowID,
		DerivedFromExploration: explorationID,
		BasedOnRevision:        revisionForFixture(fixture, workflowID),
	})
	if err != nil {
		t.Fatalf("draft plan version: %v", err)
	}
	return record
}

func SeedMutationEvent(t testing.TB, store *memorydb.SQLiteWorkflowStateStore, workflowID string, category archaeodomain.MutationCategory, impact archaeodomain.MutationImpact, disposition archaeodomain.ExecutionDisposition) archaeodomain.MutationEvent {
	t.Helper()
	event := archaeodomain.MutationEvent{
		ID:          fmt.Sprintf("mutation-%s-%s", workflowID, strings.ReplaceAll(string(category), "_", "-")),
		WorkflowID:  workflowID,
		Category:    category,
		Impact:      impact,
		Disposition: disposition,
		Description: fmt.Sprintf("%s -> %s", category, impact),
	}
	if err := fixtureWorkflowLogAppend(t, store, event); err != nil {
		t.Fatalf("append mutation event: %v", err)
	}
	return event
}

func SeedRevisionChange(t testing.TB, fixture *Fixture, workflowID, oldRev, newRev string) *archaeodomain.ExplorationSession {
	t.Helper()
	if fixture == nil {
		t.Fatal("fixture required")
	}
	session, err := fixture.ArchaeologyService().EnsureExplorationSession(fixture.Context(), workflowID, fixture.Workspace, oldRev)
	if err != nil {
		t.Fatalf("ensure baseline exploration session: %v", err)
	}
	session, err = fixture.ArchaeologyService().EnsureExplorationSession(fixture.Context(), workflowID, fixture.Workspace, newRev)
	if err != nil {
		t.Fatalf("ensure revised exploration session: %v", err)
	}
	return session
}

func fixtureWorkflowLogAppend(t testing.TB, store *memorydb.SQLiteWorkflowStateStore, event archaeodomain.MutationEvent) error {
	t.Helper()
	return archaeoevents.AppendMutationEvent(context.Background(), store, event)
}

func revisionForFixture(fixture *Fixture, workflowID string) string {
	if fixture == nil {
		return ""
	}
	session, err := fixture.ArchaeologyService().LoadExplorationByWorkflow(fixture.Context(), workflowID)
	if err == nil && session != nil && strings.TrimSpace(session.BasedOnRevision) != "" {
		return session.BasedOnRevision
	}
	return "rev-1"
}

func sessionIDWorkflowID(t testing.TB, fixture *Fixture, explorationID string) string {
	t.Helper()
	if fixture == nil {
		return ""
	}
	workflows, err := fixture.WorkflowStore.ListWorkflows(fixture.Context(), 128)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	for _, workflow := range workflows {
		session, err := fixture.ArchaeologyService().LoadExplorationByWorkflow(fixture.Context(), workflow.WorkflowID)
		if err != nil {
			t.Fatalf("load exploration by workflow: %v", err)
		}
		if session != nil && session.ID == explorationID {
			return workflow.WorkflowID
		}
	}
	return ""
}
