package archaeographqlserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	graphql "github.com/graph-gophers/graphql-go"
	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	relurpishbindings "github.com/lexcodex/relurpify/archaeo/bindings/relurpish"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeophases "github.com/lexcodex/relurpify/archaeo/phases"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	eucloplan "github.com/lexcodex/relurpify/named/euclo/plan"
	"github.com/stretchr/testify/require"
)

func TestHandlerQueryWorkflowProjectionUsesGraphQLEngine(t *testing.T) {
	handler := NewHandler(newGraphQLRuntimeFixture(t))

	body := requestBody(t, map[string]any{
		"query": `query($workflowId: String!) {
			workflowProjection(workflowId: $workflowId)
		}`,
		"variables": map[string]any{"workflowId": "wf-graphql"},
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	data := decodeGraphQLResponse(t, rec.Body.Bytes())
	projection := mustMap(t, mustMap(t, data["data"])["workflowProjection"])
	require.Equal(t, "wf-graphql", projection["workflow_id"])
	require.NotEmpty(t, projection["last_event_seq"])
	active := mustMap(t, projection["active_plan_version"])
	require.Equal(t, "active", active["status"])
}

func TestHandlerMutationResolveLearningInteractionUsesRuntime(t *testing.T) {
	ctx := context.Background()
	fixture := newGraphQLFixture(t)
	require.NoError(t, fixture.patternStore.Save(ctx, patterns.PatternRecord{
		ID:           "pattern-1",
		Kind:         patterns.PatternKindStructural,
		Title:        "Adapter",
		Description:  "Use adapters",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))
	learnSvc := archaeolearning.Service{
		Store:        fixture.workflowStore,
		PatternStore: fixture.patternStore,
		CommentStore: fixture.commentStore,
	}
	interaction, err := learnSvc.Create(ctx, archaeolearning.CreateInput{
		WorkflowID:    "wf-graphql",
		ExplorationID: "explore-1",
		Kind:          archaeolearning.InteractionPatternProposal,
		SubjectType:   archaeolearning.SubjectPattern,
		SubjectID:     "pattern-1",
		Title:         "Confirm pattern",
	})
	require.NoError(t, err)

	handler := NewHandler(fixture.runtime())
	body := requestBody(t, map[string]any{
		"query": `mutation($input: ResolveLearningInteractionInput!) { resolveLearningInteraction(input: $input) }`,
		"variables": map[string]any{
			"input": map[string]any{
				"workflowId":    "wf-graphql",
				"interactionId": interaction.ID,
				"kind":          string(archaeolearning.ResolutionConfirm),
				"resolvedBy":    "graphql-test",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	data := decodeGraphQLResponse(t, rec.Body.Bytes())
	resolved := mustMap(t, mustMap(t, data["data"])["resolveLearningInteraction"])
	require.Equal(t, interaction.ID, resolved["id"])
	require.Equal(t, "resolved", resolved["status"])

	record, err := fixture.patternStore.Load(ctx, "pattern-1")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, patterns.PatternStatusConfirmed, record.Status)
}

func TestHandlerWorkspaceDecisionTrailQueryUsesCurrentArchaeoRecords(t *testing.T) {
	ctx := context.Background()
	fixture := newGraphQLFixture(t)
	_, err := (archaeodecisions.Service{Store: fixture.workflowStore}).Create(ctx, archaeodecisions.CreateInput{
		WorkspaceID: "/workspace/graphql",
		WorkflowID:  "wf-graphql",
		Kind:        archaeodomain.DecisionKindConvergence,
		Title:       "Need convergence answer",
		Summary:     "Choose whether to defer.",
	})
	require.NoError(t, err)

	handler := NewHandler(fixture.runtime())
	body := requestBody(t, map[string]any{
		"query": `query($workspaceId: String!) {
			decisionTrail(workspaceId: $workspaceId)
		}`,
		"variables": map[string]any{"workspaceId": "/workspace/graphql"},
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	data := decodeGraphQLResponse(t, rec.Body.Bytes())
	trail := mustMap(t, mustMap(t, data["data"])["decisionTrail"])
	require.Equal(t, "/workspace/graphql", trail["workspace_id"])
	require.EqualValues(t, 1, trail["open_count"])
	records := trail["records"].([]any)
	require.Len(t, records, 1)
}

func TestHandlerMutationClaimRequestUsesRequestLifecycle(t *testing.T) {
	ctx := context.Background()
	fixture := newGraphQLFixture(t)
	request, err := (archaeorequests.Service{Store: fixture.workflowStore}).Create(ctx, archaeorequests.CreateInput{
		WorkflowID:  "wf-graphql",
		Kind:        archaeodomain.RequestPatternSurfacing,
		Title:       "Surface patterns",
		RequestedBy: "test",
	})
	require.NoError(t, err)

	handler := NewHandler(fixture.runtime())
	body := requestBody(t, map[string]any{
		"query": `mutation($input: ClaimRequestInput!) { claimRequest(input: $input) }`,
		"variables": map[string]any{
			"input": map[string]any{
				"workflowId":   "wf-graphql",
				"requestId":    request.ID,
				"claimedBy":    "graphql-worker",
				"leaseSeconds": 30,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	data := decodeGraphQLResponse(t, rec.Body.Bytes())
	record := mustMap(t, mustMap(t, data["data"])["claimRequest"])
	require.Equal(t, request.ID, record["id"])
	require.Equal(t, "running", record["status"])
	require.Equal(t, "graphql-worker", record["claimed_by"])
}

func TestHandlerSubscriptionWorkflowProjectionUsesSchemaSubscribe(t *testing.T) {
	ctx := context.Background()
	fixture := newGraphQLFixture(t)
	runtime := fixture.runtime()
	runtime.PollInterval = 10 * time.Millisecond
	handler := NewHandler(runtime)

	ch, err := handler.Subscribe(ctx, `subscription($workflowId: String!) {
		workflowProjectionUpdated(workflowId: $workflowId)
	}`, "", map[string]any{"workflowId": "wf-graphql"})
	require.NoError(t, err)

	first := <-ch
	firstResp, ok := first.(*graphql.Response)
	require.True(t, ok)
	firstData := decodeGraphQLResponse(t, mustMarshal(t, firstResp))
	projection := mustMap(t, mustMap(t, firstData["data"])["workflowProjectionUpdated"])
	require.Equal(t, "wf-graphql", projection["workflow_id"])

	require.NoError(t, archaeoevents.AppendWorkflowEvent(ctx, fixture.workflowStore, "wf-graphql", archaeoevents.EventWorkflowPhaseTransitioned, "graph update", map[string]any{"phase": "execution"}, time.Now().UTC()))

	select {
	case second := <-ch:
		secondResp := second.(*graphql.Response)
		secondData := decodeGraphQLResponse(t, mustMarshal(t, secondResp))
		updated := mustMap(t, mustMap(t, secondData["data"])["workflowProjectionUpdated"])
		require.Equal(t, "wf-graphql", updated["workflow_id"])
	case <-time.After(time.Second):
		t.Fatal("expected workflow subscription update")
	}
}

type graphQLFixture struct {
	workflowStore *memorydb.SQLiteWorkflowStateStore
	planStore     frameworkplan.PlanStore
	patternStore  patterns.PatternStore
	commentStore  patterns.CommentStore
}

func newGraphQLRuntimeFixture(t *testing.T) Runtime {
	return newGraphQLFixture(t).runtime()
}

func newGraphQLFixture(t *testing.T) graphQLFixture {
	ctx := context.Background()
	dir := t.TempDir()
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(dir, "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, workflowStore.Close()) })
	patternDB, err := patterns.OpenSQLite(filepath.Join(dir, "patterns.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, patternDB.Close()) })
	patternStore, err := patterns.NewSQLitePatternStore(patternDB)
	require.NoError(t, err)
	commentStore, err := patterns.NewSQLiteCommentStore(patternDB)
	require.NoError(t, err)
	planStore, err := eucloplan.NewSQLitePlanStore(workflowStore.DB())
	require.NoError(t, err)

	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-graphql",
		TaskID:      "task-graphql",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "graphql fixture",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	phaseSvc := archaeophases.Service{Store: workflowStore}
	_, err = phaseSvc.Transition(ctx, "wf-graphql", archaeodomain.PhaseArchaeology, archaeodomain.PhaseTransition{To: archaeodomain.PhasePlanFormation})
	require.NoError(t, err)
	_, err = phaseSvc.Transition(ctx, "wf-graphql", archaeodomain.PhasePlanFormation, archaeodomain.PhaseTransition{To: archaeodomain.PhaseExecution})
	require.NoError(t, err)
	archSvc := archaeoarch.Service{Store: workflowStore}
	session, err := archSvc.EnsureExplorationSession(ctx, "wf-graphql", "/workspace/graphql", "rev-graphql")
	require.NoError(t, err)
	_, err = archSvc.CreateExplorationSnapshot(ctx, session, archaeoarch.SnapshotInput{
		WorkflowID:          "wf-graphql",
		WorkspaceID:         "/workspace/graphql",
		BasedOnRevision:     "rev-graphql",
		SemanticSnapshotRef: "semantic-graphql",
		Summary:             "graphql exploration",
	})
	require.NoError(t, err)
	planSvc := archaeoplans.Service{Store: planStore, WorkflowStore: workflowStore}
	now := time.Now().UTC()
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-graphql",
		WorkflowID: "wf-graphql",
		Title:      "GraphQL Plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Description: "run", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	version, err := planSvc.DraftVersion(ctx, plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-graphql",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-graphql",
		SemanticSnapshotRef:    "semantic-graphql",
	})
	require.NoError(t, err)
	_, err = planSvc.ActivateVersion(ctx, "wf-graphql", version.Version)
	require.NoError(t, err)
	_, err = (archaeotensions.Service{Store: workflowStore}).CreateOrUpdate(ctx, archaeotensions.CreateInput{
		WorkflowID:    "wf-graphql",
		ExplorationID: session.ID,
		SourceRef:     "gap-graphql",
		Kind:          "intent_gap",
		Description:   "Intent gap remains unresolved.",
	})
	require.NoError(t, err)

	return graphQLFixture{
		workflowStore: workflowStore,
		planStore:     planStore,
		patternStore:  patternStore,
		commentStore:  commentStore,
	}
}

func (f graphQLFixture) runtime() Runtime {
	return Runtime{
		Bindings: relurpishbindings.Runtime{
			WorkflowStore: f.workflowStore,
			PlanStore:     f.planStore,
			PatternStore:  f.patternStore,
			CommentStore:  f.commentStore,
		},
		PollInterval: 25 * time.Millisecond,
	}
}

func requestBody(t *testing.T, req map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(req)
	require.NoError(t, err)
	return body
}

func decodeGraphQLResponse(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	if errs, ok := payload["errors"]; ok && errs != nil {
		t.Fatalf("graphql returned errors: %v", errs)
	}
	return payload
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	require.True(t, ok)
	return out
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}
