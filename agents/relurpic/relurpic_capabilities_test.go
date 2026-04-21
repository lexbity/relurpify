package relurpic

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

func registerRelurpicReadTool(t *testing.T, registry *capability.Registry, basePath string) {
	t.Helper()
	require.NoError(t, registry.Register(&platformfs.ReadFileTool{BasePath: basePath}))
}

type relurpicCapabilityQueueModel struct {
	responses []*core.LLMResponse
	index     int
}

func (m *relurpicCapabilityQueueModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	if m.index >= len(m.responses) {
		return nil, context.DeadlineExceeded
	}
	response := m.responses[m.index]
	m.index++
	return response, nil
}

func (m *relurpicCapabilityQueueModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *relurpicCapabilityQueueModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{}, nil
}

func (m *relurpicCapabilityQueueModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return m.Generate(context.Background(), "", nil)
}

func TestRegisterBuiltinRelurpicCapabilitiesRegistersCoordinationTargets(t *testing.T) {
	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	require.NoError(t, registry.Register(testutil.EchoTool{}))

	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"goal":"Plan work","steps":[{"id":"step-1","description":"Inspect the repository","tool":"echo","params":{"value":"README.md"},"expected":"repository summary","verification":"","files":["README.md"]}],"dependencies":{},"files":["README.md"]}`},
		},
	}
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:              "coding",
		Model:             "stub",
		MaxIterations:     3,
		NativeToolCalling: true,
	}))

	targets := registry.CoordinationTargets()
	require.Len(t, targets, 9)

	planner, ok := registry.GetCoordinationTarget("planner.plan")
	require.True(t, ok)
	require.Equal(t, core.CoordinationRolePlanner, planner.Coordination.Role)
	require.Equal(t, core.CapabilityRuntimeFamilyRelurpic, planner.RuntimeFamily)
	require.Equal(t, core.CapabilityExposureCallable, registry.EffectiveExposure(planner))

	reviewerTargets := registry.CoordinationTargets(core.CapabilitySelector{
		CoordinationRoles: []core.CoordinationRole{core.CoordinationRoleReviewer},
	})
	require.Len(t, reviewerTargets, 1)
	require.Equal(t, "reviewer.review", reviewerTargets[0].Name)

	executor, ok := registry.GetCoordinationTarget("executor.invoke")
	require.True(t, ok)
	require.Equal(t, core.CoordinationRoleExecutor, executor.Coordination.Role)
	require.Contains(t, executor.Coordination.TaskTypes, "execute")
}

func TestRegisterAgentCapabilitiesRegistersAgentNamespaceEntries(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, RegisterAgentCapabilities(registry, agentenv.AgentEnvironment{
		Model:    &relurpicCapabilityQueueModel{},
		Registry: registry,
		Config:   &core.Config{Name: "agent-capabilities", Model: "stub"},
	}))

	listed := registry.InvocableCapabilities()
	var found []string
	for _, desc := range listed {
		if len(desc.ID) >= len("agent:") && desc.ID[:len("agent:")] == "agent:" {
			found = append(found, desc.ID)
		}
	}
	require.Contains(t, found, "agent:react")
	require.Contains(t, found, "agent:architect")
	require.Contains(t, found, "agent:goalcon")
}

func TestPatternDetectorCapabilityReturnsAndPersistsProposals(t *testing.T) {
	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"proposals":[{"kind":"boundary","title":"Error wrapping at boundaries","description":"Boundary functions wrap returned errors consistently.","instances":[{"file_path":"","start_line":3,"end_line":7,"excerpt":"func Wrap() error {\n\treturn fmt.Errorf(\"wrap: %w\", err)\n}"}],"confidence":0.87}]}`},
		},
	}
	indexManager, graphEngine, patternStore, retrievalDB, sourcePath := newPatternDetectorFixtures(t)
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:  "coding",
		Model: "stub",
	},
		WithIndexManager(indexManager),
		WithGraphDB(graphEngine),
		WithPatternStore(patternStore),
		WithRetrievalDB(retrievalDB),
	))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "pattern-detector.detect", map[string]interface{}{
		"symbol_scope":  sourcePath,
		"corpus_scope":  "workspace",
		"max_proposals": 3,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, result.Data["count"])

	proposals, ok := result.Data["proposals"].([]any)
	require.True(t, ok)
	require.Len(t, proposals, 1)
	proposal, ok := proposals[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, patterns.PatternKindBoundary, proposal["kind"])
	instances, ok := proposal["instances"].([]any)
	require.True(t, ok)
	require.Len(t, instances, 1)
	instance, ok := instances[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, sourcePath, instance["file_path"])
	require.NotEmpty(t, instance["symbol_id"])

	saved, err := patternStore.Load(context.Background(), proposal["id"].(string))
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, patterns.PatternStatusProposed, saved.Status)
	require.Len(t, saved.Instances, 1)
	require.Equal(t, instance["symbol_id"], saved.Instances[0].SymbolID)
}

func TestPatternDetectorCapabilityResolvesNamedScopeWithoutGraphOrStore(t *testing.T) {
	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `[{"kind":"structural","title":"Utility helper","description":"A small helper function centralizes formatting.","instances":[{"file_path":"","start_line":3,"end_line":5,"excerpt":"func Wrap() error { return err }"}],"confidence":0.6}]`},
		},
	}
	indexManager, _, _, retrievalDB, _ := newPatternDetectorFixtures(t)
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:  "coding",
		Model: "stub",
	},
		WithIndexManager(indexManager),
		WithRetrievalDB(retrievalDB),
	))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "pattern-detector.detect", map[string]interface{}{
		"symbol_scope": "Wrap",
		"corpus_scope": "workspace",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, result.Data["count"])
}

func TestPatternDetectorCapabilityFiltersKindsAfterModelResponse(t *testing.T) {
	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"proposals":[{"kind":"behavioral","title":"Retry loop","description":"Retries on transient failure.","instances":[{"file_path":"","start_line":3,"end_line":5,"excerpt":"func Wrap() error { return err }"}],"confidence":0.8},{"kind":"structural","title":"Helper wrapper","description":"A helper wraps one operation.","instances":[{"file_path":"","start_line":3,"end_line":5,"excerpt":"func Wrap() error { return err }"}],"confidence":0.7}]}`},
		},
	}
	indexManager, _, _, retrievalDB, sourcePath := newPatternDetectorFixtures(t)
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:  "coding",
		Model: "stub",
	},
		WithIndexManager(indexManager),
		WithRetrievalDB(retrievalDB),
	))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "pattern-detector.detect", map[string]interface{}{
		"symbol_scope": sourcePath,
		"corpus_scope": "workspace",
		"kinds":        []any{"structural"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Data["count"])
	proposals := result.Data["proposals"].([]any)
	proposal := proposals[0].(map[string]any)
	require.Equal(t, patterns.PatternKindStructural, proposal["kind"])
}

func TestResolveSymbolScopeReturnsTypedErrorForAmbiguousSymbol(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	fileA := filepath.Join(tmpDir, "a.go")
	fileB := filepath.Join(tmpDir, "b.go")
	require.NoError(t, os.WriteFile(fileA, []byte("package sample\n\nfunc Wrap() error { return nil }\n"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("package sample\n\nfunc Wrap() error { return nil }\n"), 0o644))
	require.NoError(t, manager.IndexFile(fileA))
	require.NoError(t, manager.IndexFile(fileB))

	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	_, err := resolveSymbolScope(context.Background(), "Wrap", manager, registry)
	require.Error(t, err)
	var resolutionErr *ResolutionError
	require.ErrorAs(t, err, &resolutionErr)
	require.Len(t, resolutionErr.Candidates, 2)
}

func TestGapDetectorCapabilityRecordsDriftWritesEdgesAndInvalidatesPlan(t *testing.T) {
	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"results":[{"severity":"significant","description":"Wrap returns the raw error instead of wrapping it at the boundary.","evidence_lines":[5]}]}`},
		},
	}
	indexManager, graphEngine, _, retrievalDB, sourcePath := newPatternDetectorFixtures(t)
	anchor, err := retrieval.DeclareAnchor(context.Background(), retrievalDB, retrieval.AnchorDeclaration{
		Term:       "error wrapping",
		Definition: "Boundary functions wrap returned errors.",
		Class:      "commitment",
	}, "workspace", string(patterns.TrustClassBuiltinTrusted))
	require.NoError(t, err)

	planStore := &stubRelurpicPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Title:      "Gap test",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {
					ID:            "step-1",
					Description:   "Address wrapping drift",
					Status:        frameworkplan.PlanStepPending,
					InvalidatedBy: []frameworkplan.InvalidationRule{{Kind: frameworkplan.InvalidationAnchorDrifted, Target: anchor.AnchorID}},
				},
			},
			StepOrder: []string{"step-1"},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
	})
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-gap",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "detect gap",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:  "coding",
		Model: "stub",
	},
		WithIndexManager(indexManager),
		WithGraphDB(graphEngine),
		WithRetrievalDB(retrievalDB),
		WithPlanStore(planStore),
		WithWorkflowStore(workflowStore),
	))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "gap-detector.detect", map[string]interface{}{
		"file_path":    sourcePath,
		"corpus_scope": "workspace",
		"anchor_ids":   []any{anchor.AnchorID},
		"workflow_id":  "wf-1",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, result.Data["count"])
	require.Equal(t, 1, result.Data["anchor_count_checked"])

	gaps, ok := result.Data["gaps"].([]any)
	require.True(t, ok)
	require.Len(t, gaps, 1)

	drifted, err := retrieval.DriftedAnchors(context.Background(), retrievalDB, "workspace")
	require.NoError(t, err)
	require.Len(t, drifted, 1)
	require.Equal(t, anchor.AnchorID, drifted[0].AnchorID)

	nodes, err := indexManager.SearchNodes(ast.NodeQuery{NamePattern: "Wrap", Limit: 5})
	require.NoError(t, err)
	require.NotEmpty(t, nodes)
	edges := graphEngine.GetOutEdges(nodes[0].ID, ast.EdgeKindViolatesContract)
	require.Len(t, edges, 1)
	require.Equal(t, anchor.AnchorID, edges[0].TargetID)

	require.Len(t, planStore.invalidated, 1)
	require.Equal(t, "step-1", planStore.invalidated[0])

	tensions, err := (archaeotensions.Service{Store: workflowStore}).ListByWorkflow(context.Background(), "wf-1")
	require.NoError(t, err)
	require.Len(t, tensions, 1)
	require.Equal(t, archaeodomain.TensionUnresolved, tensions[0].Status)
	require.Equal(t, []string{anchor.AnchorID}, tensions[0].AnchorRefs)
	require.Equal(t, []string{"step-1"}, tensions[0].RelatedPlanStepIDs)
}

func TestGapDetectorCapabilityAddsDeferralObservationBelowEscalationThreshold(t *testing.T) {
	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"results":[{"severity":"minor","description":"Wrap returns raw errors in one branch.","evidence_lines":[5]}]}`},
		},
	}
	indexManager, graphEngine, _, retrievalDB, sourcePath := newPatternDetectorFixtures(t)
	anchor, err := retrieval.DeclareAnchor(context.Background(), retrievalDB, retrieval.AnchorDeclaration{
		Term:       "error wrapping",
		Definition: "Boundary functions wrap returned errors.",
		Class:      "commitment",
	}, "workspace", string(patterns.TrustClassBuiltinTrusted))
	require.NoError(t, err)
	broker := guidance.NewGuidanceBroker(time.Second)
	deferralPlan := &guidance.DeferralPlan{ID: "dp-1", WorkflowID: "wf-1"}
	broker.SetDeferralPlan(deferralPlan)

	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:  "coding",
		Model: "stub",
	},
		WithIndexManager(indexManager),
		WithGraphDB(graphEngine),
		WithRetrievalDB(retrievalDB),
		WithGuidanceBroker(broker),
	))

	_, err = registry.InvokeCapability(context.Background(), core.NewContext(), "gap-detector.detect", map[string]interface{}{
		"file_path":    sourcePath,
		"corpus_scope": "workspace",
		"anchor_ids":   []any{anchor.AnchorID},
	})
	require.NoError(t, err)
	require.Empty(t, broker.PendingRequests())
	observations := deferralPlan.PendingObservations()
	require.Len(t, observations, 1)
	require.Equal(t, guidance.GuidanceContradiction, observations[0].GuidanceKind)
}

func TestProspectiveMatcherReturnsRankedConfirmedPatterns(t *testing.T) {
	registry := capability.NewRegistry()
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `[{"pattern_id":"pattern-wrap","relevance":0.9},{"pattern_id":"pattern-cache","relevance":0.2}]`},
		},
	}
	_, _, patternStore, retrievalDB, _ := newPatternDetectorFixtures(t)
	now := time.Now().UTC()
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:           "pattern-wrap",
		Kind:         patterns.PatternKindBoundary,
		Title:        "Error wrapping boundary",
		Description:  "Boundary functions wrap errors before returning them to callers.",
		Status:       patterns.PatternStatusConfirmed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		Confidence:   0.9,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:           "pattern-cache",
		Kind:         patterns.PatternKindStructural,
		Title:        "Read-through cache",
		Description:  "Services use a read-through cache before querying the remote source.",
		Status:       patterns.PatternStatusConfirmed,
		CorpusScope:  "workspace",
		CorpusSource: "external:stdlib",
		Confidence:   0.8,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))
	anchor, err := retrieval.DeclareAnchor(context.Background(), retrievalDB, retrieval.AnchorDeclaration{
		Term:       "errors",
		Definition: "Failure values that should be wrapped at boundaries.",
		Class:      "technical",
	}, "workspace", string(patterns.TrustClassBuiltinTrusted))
	require.NoError(t, err)

	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:  "coding",
		Model: "stub",
	},
		WithPatternStore(patternStore),
		WithRetrievalDB(retrievalDB),
	))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "prospective-matcher.match", map[string]any{
		"description":  "Add boundary handling so errors are wrapped before returning",
		"corpus_scope": "workspace",
		"limit":        5,
		"min_score":    0.2,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, result.Data["count"])

	matches, ok := result.Data["matches"].([]any)
	require.True(t, ok)
	require.Len(t, matches, 1)
	match, ok := matches[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "pattern-wrap", match["pattern_id"])
	require.Equal(t, "workspace", match["corpus_source"])
	anchorRefs, ok := match["anchor_refs"].([]string)
	require.True(t, ok)
	require.Contains(t, anchorRefs, anchor.AnchorID)
}

func TestProspectiveMatcherReturnsEmptyResultWhenPatternStoreMissing(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, &relurpicCapabilityQueueModel{}, &core.Config{
		Name:  "coding",
		Model: "stub",
	}))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "prospective-matcher.match", map[string]any{
		"description":  "match something",
		"corpus_scope": "workspace",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 0, result.Data["count"])
	matches, ok := result.Data["matches"].([]any)
	require.True(t, ok)
	require.Empty(t, matches)
}

func TestCommenterAnnotatePersistsCommentAndPromotesAnchor(t *testing.T) {
	registry := capability.NewRegistry()
	_, _, patternStore, retrievalDB, _ := newPatternDetectorFixtures(t)
	now := time.Now().UTC()
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:           "pattern-comment",
		Kind:         patterns.PatternKindBoundary,
		Title:        "Boundary wrapper",
		Description:  "Errors are wrapped at boundaries.",
		Status:       patterns.PatternStatusConfirmed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		Confidence:   0.8,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))
	commentDB, err := patterns.OpenSQLite(filepath.Join(t.TempDir(), "comments.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, commentDB.Close())
	})
	commentStore, err := patterns.NewSQLiteCommentStore(commentDB)
	require.NoError(t, err)

	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, &relurpicCapabilityQueueModel{}, &core.Config{
		Name:  "coding",
		Model: "stub",
	},
		WithPatternStore(patternStore),
		WithCommentStore(commentStore),
		WithRetrievalDB(retrievalDB),
	))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "commenter.annotate", map[string]any{
		"pattern_id":   "pattern-comment",
		"intent_type":  "intentional",
		"body":         "ErrorWrap: all boundary errors are wrapped before return",
		"author_kind":  "human",
		"corpus_scope": "workspace",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.NotEmpty(t, result.Data["comment_id"])
	require.NotEmpty(t, result.Data["anchor_ref"])

	savedComment, err := commentStore.Load(context.Background(), result.Data["comment_id"].(string))
	require.NoError(t, err)
	require.NotNil(t, savedComment)
	require.Equal(t, patterns.AuthorKindHuman, savedComment.AuthorKind)
	require.Equal(t, patterns.TrustClassWorkspaceTrusted, savedComment.TrustClass)
	require.Equal(t, result.Data["anchor_ref"], savedComment.AnchorRef)

	patternRecord, err := patternStore.Load(context.Background(), "pattern-comment")
	require.NoError(t, err)
	require.NotNil(t, patternRecord)
	require.Contains(t, patternRecord.CommentIDs, savedComment.CommentID)
	require.Contains(t, patternRecord.AnchorRefs, savedComment.AnchorRef)

	anchors, err := retrieval.ActiveAnchors(context.Background(), retrievalDB, "workspace")
	require.NoError(t, err)
	var promoted *retrieval.AnchorRecord
	for i := range anchors {
		if anchors[i].AnchorID == savedComment.AnchorRef {
			promoted = &anchors[i]
			break
		}
	}
	require.NotNil(t, promoted)
	require.Equal(t, "commitment", promoted.AnchorClass)
	require.Equal(t, "workspace_trusted", promoted.TrustClass)
}

func TestCommenterAnnotateWithoutPromotionStillPersists(t *testing.T) {
	registry := capability.NewRegistry()
	commentDB, err := patterns.OpenSQLite(filepath.Join(t.TempDir(), "comments.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, commentDB.Close())
	})
	commentStore, err := patterns.NewSQLiteCommentStore(commentDB)
	require.NoError(t, err)

	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, &relurpicCapabilityQueueModel{}, &core.Config{
		Name:  "coding",
		Model: "stub",
	},
		WithCommentStore(commentStore),
	))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "commenter.annotate", map[string]any{
		"file_path":   "/tmp/example.go",
		"intent_type": "open-question",
		"body":        "Should this retry on transient failure?",
		"author_kind": "agent",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	_, ok := result.Data["anchor_ref"]
	require.False(t, ok)

	savedComment, err := commentStore.Load(context.Background(), result.Data["comment_id"].(string))
	require.NoError(t, err)
	require.NotNil(t, savedComment)
	require.Equal(t, patterns.TrustClassBuiltinTrusted, savedComment.TrustClass)
	require.Empty(t, savedComment.AnchorRef)
}

func TestCommenterAnnotateRequiresTarget(t *testing.T) {
	registry := capability.NewRegistry()
	commentDB, err := patterns.OpenSQLite(filepath.Join(t.TempDir(), "comments.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, commentDB.Close())
	})
	commentStore, err := patterns.NewSQLiteCommentStore(commentDB)
	require.NoError(t, err)

	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, &relurpicCapabilityQueueModel{}, &core.Config{
		Name:  "coding",
		Model: "stub",
	},
		WithCommentStore(commentStore),
	))

	_, err = registry.InvokeCapability(context.Background(), core.NewContext(), "commenter.annotate", map[string]any{
		"intent_type": "intentional",
		"body":        "Term: definition",
		"author_kind": "human",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one of")
}

func TestPlannerCapabilityReturnsStructuredPlan(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.EchoTool{}))

	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"goal":"Plan work","steps":[{"id":"step-1","description":"Inspect the repository","tool":"echo","params":{"value":"README.md"},"expected":"repository summary","verification":"","files":["README.md"]}],"dependencies":{},"files":["README.md"]}`},
		},
	}
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:  "coding",
		Model: "stub",
	}))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "planner.plan", map[string]interface{}{
		"instruction": "Plan the work",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "Plan work", result.Data["goal"])
	require.NotEmpty(t, result.Data["steps"])
}

func newPatternDetectorFixtures(t *testing.T) (*ast.IndexManager, *graphdb.Engine, *patterns.SQLitePatternStore, *sql.DB, string) {
	t.Helper()

	tmpDir := t.TempDir()
	store, err := ast.NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 1})

	graphEngine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(tmpDir, "graphdb")))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close())
	})
	indexManager.GraphDB = graphEngine

	sourcePath := filepath.Join(tmpDir, "sample.go")
	require.NoError(t, os.WriteFile(sourcePath, []byte(`package sample

func Wrap(err error) error {
	if err == nil {
		return nil
	}
	return err
}
`), 0o644))
	require.NoError(t, indexManager.IndexFile(sourcePath))

	patternDB, err := patterns.OpenSQLite(filepath.Join(tmpDir, "patterns.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, patternDB.Close())
	})
	patternStore, err := patterns.NewSQLitePatternStore(patternDB)
	require.NoError(t, err)

	retrievalDB, err := patterns.OpenSQLite(filepath.Join(tmpDir, "retrieval.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, retrievalDB.Close())
	})
	require.NoError(t, retrieval.EnsureSchema(context.Background(), retrievalDB))

	return indexManager, graphEngine, patternStore, retrievalDB, sourcePath
}

func newTestIndexManager(t *testing.T) (*ast.IndexManager, string) {
	t.Helper()
	tmpDir := t.TempDir()
	store, err := ast.NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 1})
	return manager, tmpDir
}

type stubRelurpicPlanStore struct {
	plan        *frameworkplan.LivingPlan
	invalidated []string
}

func (s *stubRelurpicPlanStore) SavePlan(context.Context, *frameworkplan.LivingPlan) error {
	return nil
}
func (s *stubRelurpicPlanStore) LoadPlan(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return s.plan, nil
}
func (s *stubRelurpicPlanStore) LoadPlanByWorkflow(_ context.Context, workflowID string) (*frameworkplan.LivingPlan, error) {
	if s.plan == nil || s.plan.WorkflowID != workflowID {
		return nil, nil
	}
	return s.plan, nil
}
func (s *stubRelurpicPlanStore) UpdateStep(context.Context, string, string, *frameworkplan.PlanStep) error {
	return nil
}
func (s *stubRelurpicPlanStore) InvalidateStep(_ context.Context, _ string, stepID string, _ frameworkplan.InvalidationRule) error {
	s.invalidated = append(s.invalidated, stepID)
	return nil
}
func (s *stubRelurpicPlanStore) DeletePlan(context.Context, string) error { return nil }
func (s *stubRelurpicPlanStore) ListPlans(context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

func TestReviewerAndVerifierCapabilitiesReturnStructuredOutputs(t *testing.T) {
	registry := capability.NewRegistry()
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"summary":"Review complete","approve":false,"findings":[{"severity":"high","description":"Missing tests","suggestion":"Add unit coverage"}]}`},
			{Text: `{"summary":"Verification complete","verified":true,"evidence":["unit tests passed"],"missing_items":[]}`},
		},
	}
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:  "coding",
		Model: "stub",
	}))

	review, err := registry.InvokeCapability(context.Background(), core.NewContext(), "reviewer.review", map[string]interface{}{
		"instruction":         "Review the change",
		"artifact_summary":    "Updated planner target registration",
		"acceptance_criteria": []any{"must identify missing tests"},
	})
	require.NoError(t, err)
	require.Equal(t, "Review complete", review.Data["summary"])
	require.Equal(t, false, review.Data["approve"])

	verify, err := registry.InvokeCapability(context.Background(), core.NewContext(), "verifier.verify", map[string]interface{}{
		"instruction":           "Verify the change",
		"artifact_summary":      "Added coordination target coverage",
		"verification_criteria": []any{"unit tests pass"},
	})
	require.NoError(t, err)
	require.Equal(t, "Verification complete", verify.Data["summary"])
	require.Equal(t, true, verify.Data["verified"])
}

func TestArchitectExecuteCapabilityUsesArchitectWorkflow(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.EchoTool{}))

	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"goal":"say hi","steps":[{"id":"step-1","description":"call echo","files":["README.md"]}],"dependencies":{},"files":["README.md"]}`},
			{ToolCalls: []core.ToolCall{{Name: "echo", Args: map[string]interface{}{"value": "hi"}}}},
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"finished"}`},
		},
	}
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:              "coding",
		Model:             "stub",
		MaxIterations:     3,
		NativeToolCalling: true,
	}))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "architect.execute", map[string]interface{}{
		"task_id":         "coord-1",
		"instruction":     "Implement a tiny change",
		"workflow_id":     "workflow-1",
		"context_summary": "existing plan context",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "workflow-1", result.Data["workflow_id"])
	require.Equal(t, "architect", result.Data["workflow_mode"])
	require.NotEmpty(t, result.Data["completed"])
}

func TestExecutorInvokeCapabilityExecutesNonCoordinationCapability(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.EchoTool{}))
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, &relurpicCapabilityQueueModel{}, &core.Config{
		Name:  "coding",
		Model: "stub",
	}))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "executor.invoke", map[string]interface{}{
		"capability": "echo",
		"args": map[string]any{
			"value": "delegated",
		},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "echo", result.Data["capability"])
	require.Equal(t, "delegated", result.Data["result"].(map[string]any)["echo"])
}

func TestExecutorInvokeCapabilityRejectsCoordinationTargets(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.EchoTool{}))
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, &relurpicCapabilityQueueModel{}, &core.Config{
		Name:  "coding",
		Model: "stub",
	}))

	_, err := registry.InvokeCapability(context.Background(), core.NewContext(), "executor.invoke", map[string]interface{}{
		"capability": "planner.plan",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "coordination target")
}

func TestBuildAgentFromEnvironmentLeavesGenericPipelineUnconfigured(t *testing.T) {
	agent, err := buildAgentFromEnvironment(agentenv.AgentEnvironment{
		Model:  &relurpicCapabilityQueueModel{},
		Config: &core.Config{Name: "generic-pipeline", Model: "stub"},
	}, "pipeline")
	require.NoError(t, err)

	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "pipeline-generic",
		Instruction: "run pipeline",
	}, core.NewContext())
	require.Error(t, err)
	require.Contains(t, err.Error(), "pipeline stages not configured")
}
