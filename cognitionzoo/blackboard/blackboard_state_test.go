package blackboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	execctx "codeburg.org/lexbit/relurpify/execution/context"
)

func TestNewBlackboardSetsSchemaVersionAndCopiesGoals(t *testing.T) {
	goals := []string{"goal-a", "goal-b"}
	bb := NewBlackboard(goals...)
	require.Equal(t, BlackboardStateSchemaVersion, bb.SchemaVersion)
	require.Equal(t, goals, bb.Goals)
	goals[0] = "mutated"
	assert.Equal(t, "goal-a", bb.Goals[0], "goals must be a detached copy")

	empty := NewBlackboard()
	assert.Empty(t, empty.Goals)
	assert.Equal(t, Metrics{}, empty.Metrics())
}

func TestBlackboardCloneIsDetached(t *testing.T) {
	bb := NewBlackboard("g")
	bb.AddFact("k", "v", "s")
	bb.AddHypothesis("h1", "desc", 0.5, "s")
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "pending-1", ToolOrAgent: "agent:react", Args: map[string]any{"k": "v"}}))
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "done-1", ToolOrAgent: "agent:react"}))
	require.NoError(t, bb.CompleteAction(ActionResult{RequestID: "done-1"}))
	require.NoError(t, bb.AddIssue("i1", "desc", "high", "s"))
	require.NoError(t, bb.AddArtifact("art1", "patch", "content", "s"))

	clone := bb.Clone()
	require.NotNil(t, clone)
	assert.Equal(t, bb.SchemaVersion, clone.SchemaVersion)
	assert.Equal(t, bb.Metrics(), clone.Metrics())
	require.Len(t, clone.PendingActions, 1)

	// Mutating clone must not affect the original.
	clone.Goals[0] = "changed"
	clone.Facts[0].Value = "changed"
	clone.PendingActions[0].Args["k"] = "changed"
	clone.PendingActions = append(clone.PendingActions, ActionRequest{ID: "extra"})
	assert.Equal(t, "g", bb.Goals[0])
	assert.Equal(t, "v", bb.Facts[0].Value)
	assert.Equal(t, "v", bb.PendingActions[0].Args["k"], "Args map must be cloned")
	assert.Len(t, bb.PendingActions, 1)
	assert.Equal(t, "done-1", bb.CompletedActions[0].RequestID)
}

func TestBlackboardCloneEmptyBoard(t *testing.T) {
	clone := NewBlackboard().Clone()
	require.NotNil(t, clone)
	assert.Empty(t, clone.PendingActions, "empty pending actions must not allocate")
	assert.Empty(t, clone.Facts)
}

func TestBlackboardNilReceiverMethodsAreSafe(t *testing.T) {
	var bb *Blackboard
	assert.Nil(t, bb.Clone())
	// IsGoalSatisfied and HasUnverifiedArtifacts are the only Blackboard
	// methods without a nil-receiver guard (inconsistent with the rest of the
	// API); they dereference the receiver directly.
	assert.Panics(t, func() { bb.IsGoalSatisfied() })
	assert.Panics(t, func() { bb.HasUnverifiedArtifacts() })
	assert.False(t, bb.AddFact("k", "v", "s"))
	assert.Error(t, bb.AddHypothesis("h", "d", 0.5, "s"))
	assert.Error(t, bb.AddIssue("i", "d", "low", "s"))
	assert.Error(t, bb.EnqueueAction(ActionRequest{ID: "a"}))
	assert.Error(t, bb.CompleteAction(ActionResult{RequestID: "r"}))
	assert.Error(t, bb.AddArtifact("art", "kind", "c", "s"))
	assert.False(t, bb.VerifyArtifact("art"))
	assert.Equal(t, 0, bb.VerifyAllArtifacts())
	assert.Error(t, bb.Validate())
	assert.False(t, bb.HasIssue("i"))
	assert.False(t, bb.HasHypothesis("h"))
	assert.False(t, bb.HasPendingAction("a"))
	assert.False(t, bb.HasCompletedAction("r"))
	assert.False(t, bb.HasArtifact("art"))
	assert.False(t, bb.HasFact("k"))
	assert.Equal(t, Metrics{}, bb.Metrics())
	bb.SetGoals("x") // no-op
	bb.Normalize()   // no-op
	bb.AddFactWithOrigin("k", "v", "s", nil)
	bb.AddHypothesisWithOrigin("h", "d", 0.5, "s", nil)
	SeedBlackboardFromSemanticContext(bb, execctx.AgentSemanticContext{})
}

func TestBlackboardNormalizeFillsSchemaVersion(t *testing.T) {
	bb := &Blackboard{}
	bb.Normalize()
	assert.Equal(t, BlackboardStateSchemaVersion, bb.SchemaVersion)
}

func TestBlackboardGoalSatisfactionRequiresVerifiedArtifact(t *testing.T) {
	bb := NewBlackboard("goal")
	assert.False(t, bb.IsGoalSatisfied())
	assert.False(t, bb.HasUnverifiedArtifacts())

	require.NoError(t, bb.AddArtifact("art", "report", "body", "ks"))
	assert.False(t, bb.IsGoalSatisfied())
	assert.True(t, bb.HasUnverifiedArtifacts())

	assert.True(t, bb.VerifyArtifact("art"))
	assert.True(t, bb.IsGoalSatisfied())
	assert.False(t, bb.HasUnverifiedArtifacts())
	assert.False(t, bb.VerifyArtifact("art"), "double-verify must report no change")
	assert.False(t, bb.VerifyArtifact("missing"))
	assert.False(t, bb.VerifyArtifact("  "))
}

func TestBlackboardVerifyAllArtifactsCountsOnlyNewlyVerified(t *testing.T) {
	bb := NewBlackboard()
	require.NoError(t, bb.AddArtifact("a", "k", "c", "s"))
	require.NoError(t, bb.AddArtifact("b", "k", "c", "s"))
	assert.Equal(t, 2, bb.VerifyAllArtifacts())
	assert.Equal(t, 0, bb.VerifyAllArtifacts())
	assert.Equal(t, 2, bb.Metrics().VerifiedCount)
}

func TestBlackboardSetGoalsReplacesDetachedCopy(t *testing.T) {
	bb := NewBlackboard("old")
	goals := []string{"new"}
	bb.SetGoals(goals...)
	goals[0] = "mutated"
	assert.Equal(t, []string{"new"}, bb.Goals)
}

func TestBlackboardAddFactDeduplicatesAndTrims(t *testing.T) {
	bb := NewBlackboard()
	assert.True(t, bb.AddFact("  key ", "value", " src "))
	assert.False(t, bb.AddFact("key", "value", "src"), "exact duplicate triple must be rejected")
	assert.False(t, bb.AddFact("", "value", "src"))
	assert.False(t, bb.AddFact("   ", "value", "src"))
	assert.Len(t, bb.Facts, 1)
	assert.True(t, bb.HasFact("key"))
	assert.False(t, bb.HasFact("missing"))
	// Same key different value is a distinct fact (dedupe is on the full triple).
	assert.True(t, bb.AddFact("key", "value2", "src"))
	assert.Len(t, bb.Facts, 2)
}

func TestBlackboardAddHypothesisValidation(t *testing.T) {
	bb := NewBlackboard()
	require.NoError(t, bb.AddHypothesis(" h1 ", " desc ", 0.5, " s "))
	assert.Equal(t, "h1", bb.Hypotheses[0].ID)
	assert.Equal(t, "desc", bb.Hypotheses[0].Description)
	assert.Equal(t, "s", bb.Hypotheses[0].Source)

	assert.Error(t, bb.AddHypothesis("", "d", 0.5, "s"))
	assert.Error(t, bb.AddHypothesis("  ", "d", 0.5, "s"))
	assert.Error(t, bb.AddHypothesis("h2", "", 0.5, "s"))
	assert.Error(t, bb.AddHypothesis("h2", "d", -0.1, "s"))
	assert.Error(t, bb.AddHypothesis("h2", "d", 1.1, "s"))
	assert.Error(t, bb.AddHypothesis("h1", "d", 0.5, "s"), "duplicate id rejected")
	assert.Error(t, bb.AddHypothesis(" h1 ", "d", 0.5, "s"), "trimmed duplicate id rejected")
	assert.Len(t, bb.Hypotheses, 1)
	assert.True(t, bb.HasHypothesis("h1"))
}

func TestBlackboardAddIssueNormalizesSeverity(t *testing.T) {
	bb := NewBlackboard()
	require.NoError(t, bb.AddIssue("i1", "d", " HIGH ", "s"))
	assert.Equal(t, "high", bb.Issues[0].Severity)

	require.NoError(t, bb.AddIssue("i-low", "d", "LOW", "s"), "trimmed severity valid, distinct id")
	assert.Equal(t, "low", bb.Issues[1].Severity)

	assert.Error(t, bb.AddIssue("i-bad", "d", "critical", "s"))
	assert.Error(t, bb.AddIssue("i-empty", "d", "", "s"))
	assert.Error(t, bb.AddIssue("", "d", "low", "s"))
	assert.Error(t, bb.AddIssue("i2", "", "low", "s"))
	assert.Error(t, bb.AddIssue("i1", "d", "low", "s"), "duplicate id rejected")
	assert.True(t, bb.HasIssue("i1"))
	assert.False(t, bb.HasIssue("nope"))
}

func TestBlackboardActionLifecycle(t *testing.T) {
	bb := NewBlackboard()
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: " a1 ", ToolOrAgent: " agent:react ", Args: map[string]any{"k": "v"}}))
	assert.True(t, bb.HasPendingAction("a1"))
	assert.Equal(t, "a1", bb.PendingActions[0].ID)
	assert.Equal(t, "agent:react", bb.PendingActions[0].ToolOrAgent)
	assert.False(t, bb.PendingActions[0].CreatedAt.IsZero(), "CreatedAt defaulted")

	assert.Error(t, bb.EnqueueAction(ActionRequest{ID: "", ToolOrAgent: "x"}))
	assert.Error(t, bb.EnqueueAction(ActionRequest{ID: "a2", ToolOrAgent: ""}))
	assert.Error(t, bb.EnqueueAction(ActionRequest{ID: "a1", ToolOrAgent: "x"}))

	require.NoError(t, bb.CompleteAction(ActionResult{RequestID: "a1", Success: true}))
	assert.False(t, bb.HasPendingAction("a1"), "completed request removed from pending")
	assert.True(t, bb.HasCompletedAction("a1"))
	assert.Error(t, bb.CompleteAction(ActionResult{RequestID: "a1"}), "duplicate result rejected")
	assert.Error(t, bb.CompleteAction(ActionResult{RequestID: " "}))
}

func TestBlackboardCompleteActionKeepsOtherPending(t *testing.T) {
	bb := NewBlackboard()
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "a1", ToolOrAgent: "agent:react"}))
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "a2", ToolOrAgent: "agent:react"}))
	require.NoError(t, bb.CompleteAction(ActionResult{RequestID: "a1"}))
	assert.False(t, bb.HasPendingAction("a1"))
	assert.True(t, bb.HasPendingAction("a2"))
	require.NoError(t, bb.CompleteAction(ActionResult{RequestID: "ghost"}), "result without pending request still records")
}

func TestBlackboardAddArtifactAndMetrics(t *testing.T) {
	bb := NewBlackboard("g1", "g2")
	require.NoError(t, bb.AddArtifact(" art ", " patch ", "content", " src "))
	assert.Equal(t, "art", bb.Artifacts[0].ID)
	assert.Equal(t, "patch", bb.Artifacts[0].Kind)
	assert.Error(t, bb.AddArtifact("", "k", "c", "s"))
	assert.Error(t, bb.AddArtifact("a2", "", "c", "s"))
	assert.Error(t, bb.AddArtifact("art", "k", "c", "s"), "duplicate id rejected")
	assert.True(t, bb.HasArtifact("art"))
	assert.False(t, bb.HasArtifact("other"))

	bb.AddFact("f", "v", "s")
	bb.AddHypothesis("h", "d", 0.5, "s")
	bb.AddIssue("i", "d", "low", "s")
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "a", ToolOrAgent: "agent:react"}))
	require.NoError(t, bb.CompleteAction(ActionResult{RequestID: "a"}))
	bb.VerifyArtifact("art")

	m := bb.Metrics()
	assert.Equal(t, Metrics{
		GoalCount:       2,
		FactCount:       1,
		HypothesisCount: 1,
		IssueCount:      1,
		PendingCount:    0,
		CompletedCount:  1,
		ArtifactCount:   1,
		VerifiedCount:   1,
	}, m)
}

func TestBlackboardValidateErrorBranches(t *testing.T) {
	t.Run("missing schema version", func(t *testing.T) {
		bb := &Blackboard{SchemaVersion: 0}
		require.EqualError(t, bb.Validate(), "schema version required")
	})
	t.Run("issue branches", func(t *testing.T) {
		bb := NewBlackboard()
		bb.Issues = []Issue{{ID: "", Severity: "low"}}
		require.EqualError(t, bb.Validate(), "issue id required")

		bb.Issues = []Issue{{ID: "i", Severity: "bogus"}}
		require.ErrorContains(t, bb.Validate(), "invalid")

		bb.Issues = []Issue{{ID: "i", Severity: "low"}, {ID: "i", Severity: "low"}}
		require.ErrorContains(t, bb.Validate(), "duplicate issue id")
	})
	t.Run("pending action branches", func(t *testing.T) {
		bb := NewBlackboard()
		bb.PendingActions = []ActionRequest{{ID: "", ToolOrAgent: "t"}}
		require.EqualError(t, bb.Validate(), "pending action id required")

		bb.PendingActions = []ActionRequest{{ID: "a", ToolOrAgent: ""}}
		require.EqualError(t, bb.Validate(), `pending action "a" tool or agent required`)

		bb.PendingActions = []ActionRequest{{ID: "a", ToolOrAgent: "t"}, {ID: "a", ToolOrAgent: "t"}}
		require.ErrorContains(t, bb.Validate(), "duplicate pending action id")
	})
	t.Run("completed action branches", func(t *testing.T) {
		bb := NewBlackboard()
		bb.CompletedActions = []ActionResult{{RequestID: ""}}
		require.EqualError(t, bb.Validate(), "completed action request id required")

		bb.CompletedActions = []ActionResult{{RequestID: "r"}, {RequestID: "r"}}
		require.ErrorContains(t, bb.Validate(), "duplicate completed action id")
	})
	t.Run("artifact branches", func(t *testing.T) {
		bb := NewBlackboard()
		bb.Artifacts = []Artifact{{ID: "", Kind: "k"}}
		require.EqualError(t, bb.Validate(), "artifact id required")

		bb.Artifacts = []Artifact{{ID: "a", Kind: ""}}
		require.EqualError(t, bb.Validate(), `artifact "a" kind required`)

		bb.Artifacts = []Artifact{{ID: "a", Kind: "k"}, {ID: "a", Kind: "k"}}
		require.ErrorContains(t, bb.Validate(), "duplicate artifact id")
	})
	t.Run("valid board passes", func(t *testing.T) {
		bb := NewBlackboard("g")
		require.NoError(t, bb.AddIssue("i", "d", "low", "s"))
		require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "a", ToolOrAgent: "t"}))
		require.NoError(t, bb.CompleteAction(ActionResult{RequestID: "a"}))
		require.NoError(t, bb.AddArtifact("art", "kind", "c", "s"))
		require.NoError(t, bb.Validate())
	})
}

func TestBlackboardFactAndHypothesisWithOrigin(t *testing.T) {
	bb := NewBlackboard()
	origin := &FactOrigin{
		SourceSystem: "ast_index",
		RecordID:     "file.go:10",
		CapturedAt:   time.Unix(1700000000, 0).UTC(),
	}
	bb.AddFactWithOrigin("k", "v", "src", origin)
	require.Len(t, bb.Facts, 1)
	assert.Equal(t, origin, bb.Facts[0].Origin)
	assert.Equal(t, "ast_index", bb.Facts[0].Origin.SourceSystem)

	bb.AddHypothesisWithOrigin("h", "d", 0.5, "src", origin)
	require.Len(t, bb.Hypotheses, 1)
	assert.Equal(t, origin, bb.Hypotheses[0].Origin)
}

func TestSanitizeKey(t *testing.T) {
	assert.Equal(t, "my_func_2", sanitizeKey("my-func.2"))
	assert.Equal(t, "_", sanitizeKey(""))
	assert.Equal(t, "___", sanitizeKey("!!!"))
	assert.Equal(t, "abcXYZ019", sanitizeKey("abcXYZ019"))
	assert.Equal(t, "a_b", sanitizeKey("a b"))
}

func TestSeedBlackboardFromSemanticContext(t *testing.T) {
	bb := NewBlackboard("goal")
	semctx := execctx.AgentSemanticContext{
		ASTSymbols: []execctx.ASTSymbolSummary{
			{Name: "MyFunc", Kind: "func", Signature: "func MyFunc()", File: "a.go", Line: 10},
			{Name: "documented", Kind: "method", Signature: "func (r R) documented()", File: "b.go", Line: 20, DocSummary: "does things"},
		},
		Chunks: []execctx.AgentContextChunk{
			{ID: "chunk-1", Content: "chunk body", Metadata: map[string]string{"view_kind": "summary"}},
			{ID: "chunk-2", Content: "chunk body 2"},
		},
	}
	SeedBlackboardFromSemanticContext(bb, semctx)

	symFact, ok := findFact(bb, "ast.symbol.MyFunc")
	require.True(t, ok, "AST symbol fact seeded")
	assert.Equal(t, "func func MyFunc() [a.go:10]", symFact.Value)
	require.NotNil(t, symFact.Origin)
	assert.Equal(t, "ast_index", symFact.Origin.SourceSystem)
	assert.Equal(t, "a.go:10", symFact.Origin.RecordID)
	assert.Equal(t, "func", symFact.Origin.Kind)

	docFact, ok := findFact(bb, "ast.symbol.documented")
	require.True(t, ok)
	assert.Equal(t, "method func (r R) documented() // does things [b.go:20]", docFact.Value)

	loaded, ok := findFact(bb, "ast.symbols_loaded")
	require.True(t, ok)
	assert.Equal(t, "2", loaded.Value)

	chunkFact, ok := findFact(bb, "bkc.chunk.chunk-1")
	require.True(t, ok)
	assert.Equal(t, "chunk body", chunkFact.Value)
	require.NotNil(t, chunkFact.Origin)
	assert.Equal(t, "bkc", chunkFact.Origin.SourceSystem)
	assert.Equal(t, "summary", chunkFact.Origin.Kind, "view_kind metadata promoted to origin kind")

	_, ok = findFact(bb, "bkc.chunks_loaded")
	require.True(t, ok)
}

func TestSeedBlackboardFromSemanticContextEmpty(t *testing.T) {
	bb := NewBlackboard()
	SeedBlackboardFromSemanticContext(bb, execctx.AgentSemanticContext{})
	assert.Empty(t, bb.Facts)
}

func TestFactOriginDerivationStamping(t *testing.T) {
	origin := relurpctx.OriginDerivation("blackboard")
	derived := origin.Derive("blackboard_inject", "blackboard", 0.0, "")
	require.NotEmpty(t, derived.Steps)
	last := derived.Steps[len(derived.Steps)-1]
	assert.Equal(t, "blackboard_inject", last.Transform)
	assert.Equal(t, "blackboard", last.SourceSystem)
	assert.InDelta(t, 0.0, last.LossMagnitude, 0.0001)
}

func findFact(bb *Blackboard, key string) (Fact, bool) {
	for _, fact := range bb.Facts {
		if fact.Key == key {
			return fact, true
		}
	}
	return Fact{}, false
}
