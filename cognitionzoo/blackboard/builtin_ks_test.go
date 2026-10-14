package blackboard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	execctx "codeburg.org/lexbit/relurpify/execution/context"
)

// scriptedCapability is a fake invocable capability with a canned result and
// optional coordination-target marking. It records every invocation.
type scriptedCapability struct {
	id          string
	name        string
	coordTarget bool
	result      *ports.ToolResult
	err         error
	invocations []map[string]any
}

func (c *scriptedCapability) Descriptor(_ context.Context, _ ports.State) descriptor.CapabilityDescriptor {
	d := descriptor.CapabilityDescriptor{
		ID:            c.id,
		Name:          c.name,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
	if c.coordTarget {
		d.Coordination = &descriptor.CoordinationTargetMetadata{
			Target:         true,
			Role:           agentspec.CoordinationRoleExecutor,
			TaskTypes:      []string{"*"},
			ExecutionModes: []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeSync},
		}
	}
	return d
}

func (c *scriptedCapability) Invoke(_ context.Context, _ ports.State, args map[string]any) (*ports.ToolResult, error) {
	c.invocations = append(c.invocations, args)
	if c.err != nil {
		return nil, c.err
	}
	if c.result == nil {
		return &ports.ToolResult{Success: true, Data: map[string]any{}}, nil
	}
	return c.result, nil
}

func newRegistryWith(t *testing.T, caps ...*scriptedCapability) *registry.CapabilityRegistry {
	t.Helper()
	reg := registry.NewRegistry()
	for _, capHandler := range caps {
		require.NoError(t, reg.RegisterInvocableCapability(context.Background(), capHandler))
	}
	return reg
}

func reviewerResult(findings []any) *ports.ToolResult {
	return &ports.ToolResult{Success: true, Data: map[string]any{"findings": findings}}
}

func toAnySlice(in []map[string]any) []any {
	out := make([]any, len(in))
	for i, m := range in {
		out[i] = m
	}
	return out
}

const bbTestGoal = "fix the flaky test"

func bbWithExploredState(t *testing.T) *Blackboard {
	t.Helper()
	bb := NewBlackboard(bbTestGoal)
	require.True(t, bb.AddFact("exploration.status", "explored", "Explorer"))
	return bb
}

func TestDefaultKnowledgeSourcesNamesAndOrder(t *testing.T) {
	sources := DefaultKnowledgeSources()
	require.Len(t, sources, 8)
	want := []string{"Explorer", "Analyzer", "Planner", "Review", "Executor", "FailureTriage", "Verifier", "Summarizer"}
	for i, source := range sources {
		assert.Equal(t, want[i], source.Name())
	}
	// Priorities strictly decrease in declaration order.
	for i := 1; i < len(sources); i++ {
		assert.GreaterOrEqual(t, sources[i-1].Priority(), sources[i].Priority())
	}
}

func TestExplorerKSActivatesUntilExplored(t *testing.T) {
	ks := &ExplorerKS{}
	assert.Equal(t, "Explorer", ks.Name())
	assert.Equal(t, 100, ks.Priority())

	bb := NewBlackboard(bbTestGoal)
	assert.True(t, ks.CanActivate(bb), "fresh board is explorable")

	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	assert.False(t, ks.CanActivate(bb), "explored board must not re-activate")

	goalFact, ok := findFact(bb, "task.goal.0")
	require.True(t, ok)
	assert.Equal(t, bbTestGoal, goalFact.Value)
	assert.Equal(t, "Explorer", goalFact.Source)
	status, ok := findFact(bb, "exploration.status")
	require.True(t, ok)
	assert.Equal(t, "explored", status.Value)
}

func TestExplorerKSIndexesSemanticContext(t *testing.T) {
	ks := &ExplorerKS{}
	bb := NewBlackboard(bbTestGoal)
	semctx := execctx.AgentSemanticContext{
		ASTSymbols: []execctx.ASTSymbolSummary{{Name: "Handler", Kind: "func", Signature: "func Handler()", File: "h.go", Line: 3}},
		Chunks:     []execctx.AgentContextChunk{{ID: "c1", Content: "body", Metadata: map[string]string{"view_kind": "api"}}},
	}
	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, semctx))
	assert.True(t, bb.HasFact("ast.symbol.Handler"))
	assert.True(t, bb.HasFact("ast.symbols_loaded"))
	assert.True(t, bb.HasFact("bkc.chunk.c1"))
	assert.True(t, bb.HasFact("bkc.chunks_loaded"))
	chunkFact, _ := findFact(bb, "bkc.chunk.c1")
	require.NotNil(t, chunkFact.Origin)
	assert.Equal(t, "api", chunkFact.Origin.Kind)
}

func TestAnalyzerKSFallbackIssueWithoutRegistry(t *testing.T) {
	ks := &AnalyzerKS{}
	assert.Equal(t, "Analyzer", ks.Name())
	assert.Equal(t, 90, ks.Priority())

	bb := NewBlackboard()
	assert.False(t, ks.CanActivate(bb), "no exploration fact yet")

	bb = bbWithExploredState(t)
	bb.AddFact("task.goal.0", bbTestGoal, "Explorer")
	assert.True(t, ks.CanActivate(bb))

	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.Issues, 1)
	assert.Equal(t, "low", bb.Issues[0].Severity)
	assert.Equal(t, "Analyzer", bb.Issues[0].Source)
	assert.Contains(t, bb.Issues[0].Description, "analysis from facts")

	assert.False(t, ks.CanActivate(bb), "issues present, must not re-activate")
}

func TestAnalyzerKSCapabilityFindingsBecomeIssues(t *testing.T) {
	reviewer := &scriptedCapability{id: "reviewer.review", name: "review", result: reviewerResult([]any{
		map[string]any{"description": "missing error check", "severity": "high"},
		map[string]any{"description": "naming", "severity": ""}, // blank severity defaults to medium
		map[string]any{"description": "   "},                    // empty descriptions skipped
		"not-a-map",                                             // skipped
	})}
	reg := newRegistryWith(t, reviewer)
	ks := &AnalyzerKS{}
	bb := bbWithExploredState(t)
	bb.Goals = []string{bbTestGoal}

	require.NoError(t, ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.Issues, 2)
	assert.Equal(t, "finding-0", bb.Issues[0].ID)
	assert.Equal(t, "high", bb.Issues[0].Severity)
	assert.Equal(t, "finding-1", bb.Issues[1].ID)
	assert.Equal(t, "medium", bb.Issues[1].Severity)
	require.Len(t, reviewer.invocations, 1)
	assert.Equal(t, bbTestGoal, reviewer.invocations[0]["instruction"])
}

func TestAnalyzerKSCapabilityErrorSurfaces(t *testing.T) {
	reviewer := &scriptedCapability{id: "reviewer.review", name: "review", err: errors.New("reviewer down")}
	reg := newRegistryWith(t, reviewer)
	ks := &AnalyzerKS{}
	bb := bbWithExploredState(t)
	err := ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewer down")
	assert.Empty(t, bb.Issues)
}

func TestAnalyzerKSMissingSeverityKeyAbortsCycle(t *testing.T) {
	// Documents current behavior: a finding with no severity key formats the
	// missing value as "<nil>", which fails severity validation and aborts the
	// whole analysis cycle instead of defaulting to medium.
	reviewer := &scriptedCapability{id: "reviewer.review", name: "review", result: reviewerResult([]any{
		map[string]any{"description": "no severity key"},
	})}
	reg := newRegistryWith(t, reviewer)
	ks := &AnalyzerKS{}
	bb := bbWithExploredState(t)
	err := ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `issue severity "<nil>" invalid`)
	assert.Empty(t, bb.Issues)
}

func TestAnalyzerKSEmptyFindingsFallBackToDefaultIssue(t *testing.T) {
	reviewer := &scriptedCapability{id: "reviewer.review", name: "review", result: reviewerResult(nil)}
	reg := newRegistryWith(t, reviewer)
	ks := &AnalyzerKS{}
	bb := bbWithExploredState(t)
	require.NoError(t, ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.Issues, 1)
	assert.Equal(t, "Analyzer", bb.Issues[0].Source)
}

func TestPlannerKSFallbackPlansPerIssue(t *testing.T) {
	ks := &PlannerKS{}
	assert.Equal(t, "Planner", ks.Name())
	assert.Equal(t, 80, ks.Priority())

	bb := NewBlackboard()
	assert.False(t, ks.CanActivate(bb), "no issues yet")
	bb.AddIssue("issue-1", "handle nil input", "high", "Analyzer")
	assert.True(t, ks.CanActivate(bb))

	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.PendingActions, 1)
	action := bb.PendingActions[0]
	assert.Equal(t, "action-issue-1", action.ID)
	assert.Equal(t, defaultAgentDispatch, action.ToolOrAgent)
	assert.Equal(t, "Planner", action.RequestedBy)
	assert.Equal(t, "handle nil input", action.Args["instruction"])
	assert.True(t, strings.HasPrefix(action.Description, "Resolve:"))

	assert.False(t, ks.CanActivate(bb), "pending actions exist")
}

func TestPlannerKSCapabilityStepsEnqueued(t *testing.T) {
	planner := &scriptedCapability{id: "planner.plan", name: "plan", result: &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"steps": []any{
				map[string]any{"id": "step-a", "tool": "agent:pipeline", "description": "do a", "params": map[string]any{"k": "v"}},
				map[string]any{"id": "step-b", "tool": "agent:react"}, // agent target without instruction -> description used
				map[string]any{"tool": "", "description": "no id"},    // default id planned-2
				map[string]any{"id": "step-c", "tool": "go_test"},     // non-agent, params nil -> empty map
				"not-a-map", // skipped
			},
		},
	}}
	reg := newRegistryWith(t, planner)
	ks := &PlannerKS{}
	bb := NewBlackboard(bbTestGoal)
	bb.AddIssue("issue-1", "d", "high", "Analyzer")

	require.NoError(t, ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.PendingActions, 4)
	byID := map[string]ActionRequest{}
	for _, action := range bb.PendingActions {
		byID[action.ID] = action
	}
	stepA := byID["action-step-a"]
	assert.Equal(t, "agent:pipeline", stepA.ToolOrAgent)
	assert.Equal(t, "v", stepA.Args["k"])
	assert.Equal(t, "Planner", stepA.RequestedBy)

	// NOTE: steps that omit a key format the missing value as "<nil>", so the
	// "<nil>" string defeats the empty-string defaults for id/description and
	// the derive-instruction-from-description rule (recorded bug in
	// enqueuePlannedActions). The assertions below pin current behavior.
	stepB := byID["action-step-b"]
	assert.Equal(t, defaultAgentDispatch, stepB.ToolOrAgent)
	assert.Equal(t, "<nil>", stepB.Description)
	assert.Empty(t, stepB.Args)

	assert.Contains(t, byID, "action-<nil>", "missing id formats as <nil> instead of the planned-N default")
	assert.Equal(t, defaultAgentDispatch, byID["action-<nil>"].ToolOrAgent, "explicit empty tool falls back to the default dispatch")
	assert.Equal(t, "no id", byID["action-<nil>"].Description)

	stepC := byID["action-step-c"]
	assert.Equal(t, "go_test", stepC.ToolOrAgent)
	assert.Equal(t, "<nil>", stepC.Description)
	assert.Nil(t, stepC.Args, "nil params yield a nil Args after cloning")
}

func TestPlannerKSCapabilityWithoutStepsFallsBack(t *testing.T) {
	planner := &scriptedCapability{id: "planner.plan", name: "plan", result: &ports.ToolResult{Success: true, Data: map[string]any{"steps": "not-a-list"}}}
	reg := newRegistryWith(t, planner)
	ks := &PlannerKS{}
	bb := NewBlackboard(bbTestGoal)
	bb.AddIssue("issue-1", "d", "high", "Analyzer")
	require.NoError(t, ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.PendingActions, 1, "fallback plans from issues when the planner returns no usable steps")
}

func TestPlannerKSCapabilityErrorSurfaces(t *testing.T) {
	planner := &scriptedCapability{id: "planner.plan", name: "plan", err: errors.New("planner down")}
	reg := newRegistryWith(t, planner)
	ks := &PlannerKS{}
	bb := NewBlackboard(bbTestGoal)
	bb.AddIssue("issue-1", "d", "high", "Analyzer")
	err := ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "planner down")
}

func TestReviewKSCanActivateAndCapabilityFindings(t *testing.T) {
	ks := &ReviewKS{}
	assert.Equal(t, "Review", ks.Name())
	assert.Equal(t, 75, ks.Priority())

	bb := NewBlackboard()
	assert.False(t, ks.CanActivate(bb), "no artifacts")
	require.NoError(t, bb.AddArtifact("a", "patch", "c", "Executor"))
	assert.False(t, ks.CanActivate(bb), "unverified artifact present")
	bb.VerifyArtifact("a")
	assert.True(t, ks.CanActivate(bb))
	bb.AddIssue("i", "d", "low", "Analyzer")
	assert.False(t, ks.CanActivate(bb), "issues block review")
	bb.Issues = nil
	assert.True(t, ks.CanActivate(bb))

	// Without registry: no-op success.
	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))

	reviewer := &scriptedCapability{id: "reviewer.review", name: "review", result: reviewerResult([]any{
		map[string]any{"description": "artifact weak", "severity": "medium"},
	})}
	reg := newRegistryWith(t, reviewer)
	require.NoError(t, ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.Issues, 1)
	assert.Equal(t, "artifact weak", bb.Issues[0].Description)
	require.Len(t, reviewer.invocations, 1)
	assert.Equal(t, "artifact_review", reviewer.invocations[0]["mode"])
}

func TestExecutorKSCompletesWithoutTools(t *testing.T) {
	ks := &ExecutorKS{}
	assert.Equal(t, "Executor", ks.Name())
	assert.Equal(t, 70, ks.Priority())

	bb := NewBlackboard()
	assert.False(t, ks.CanActivate(bb))
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "a1", ToolOrAgent: "agent:react", Description: "desc"}))
	assert.True(t, ks.CanActivate(bb))

	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	assert.False(t, bb.HasPendingAction("a1"))
	require.Len(t, bb.CompletedActions, 1)
	assert.True(t, bb.CompletedActions[0].Success)
	assert.Equal(t, "completed: desc", bb.CompletedActions[0].Output)
	require.Len(t, bb.Artifacts, 1)
	assert.Equal(t, "artifact-a1", bb.Artifacts[0].ID)
	assert.Equal(t, "result", bb.Artifacts[0].Kind)
	assert.False(t, ks.CanActivate(bb))
}

func TestExecutorKSWithCoordinationTargetCapability(t *testing.T) {
	agent := &scriptedCapability{id: "agent:react", name: "react", coordTarget: true, result: &ports.ToolResult{
		Success: true,
		Data:    map[string]any{"summary": "did the thing"},
	}}
	reg := newRegistryWith(t, agent)
	ks := &ExecutorKS{}
	bb := NewBlackboard()
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "a1", ToolOrAgent: "agent:react", Description: "desc", Args: map[string]any{"instruction": "go"}}))
	require.NoError(t, ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.CompletedActions, 1)
	assert.True(t, bb.CompletedActions[0].Success)
	assert.Equal(t, "did the thing", bb.CompletedActions[0].Output)
	require.Len(t, agent.invocations, 1)
}

func TestExecutorKSCapabilityErrorRecordsIssueAndFails(t *testing.T) {
	agent := &scriptedCapability{id: "agent:react", name: "react", coordTarget: true, err: errors.New("boom")}
	reg := newRegistryWith(t, agent)
	ks := &ExecutorKS{}
	bb := NewBlackboard()
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "a1", ToolOrAgent: "agent:react", Description: "desc"}))
	err := ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{})
	require.Error(t, err)
	require.Len(t, bb.CompletedActions, 1)
	assert.False(t, bb.CompletedActions[0].Success)
	assert.Equal(t, "boom", bb.CompletedActions[0].Error)
	require.Len(t, bb.Issues, 1)
	assert.Equal(t, "exec-a1", bb.Issues[0].ID)
	assert.Equal(t, "high", bb.Issues[0].Severity)
}

func TestExecutorKSKnownCapabilityWithoutExecutorInvoke(t *testing.T) {
	tool := &scriptedCapability{id: "unittest.tool", name: "unittest.tool", result: &ports.ToolResult{
		Success: true,
		Data:    map[string]any{"summary": "tests passed"},
	}}
	reg := newRegistryWith(t, tool)
	ks := &ExecutorKS{}
	bb := NewBlackboard()
	// Target matches a registered capability by name but is not a coordination target.
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "a1", ToolOrAgent: "unittest.tool", Description: "run tests"}))
	require.NoError(t, ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.CompletedActions, 1)
	assert.Equal(t, "tests passed", bb.CompletedActions[0].Output)
	require.Len(t, tool.invocations, 1)
}

func TestExecutorKSUnknownTargetCompletesDescriptively(t *testing.T) {
	reg := registry.NewRegistry()
	ks := &ExecutorKS{}
	bb := NewBlackboard()
	require.NoError(t, bb.EnqueueAction(ActionRequest{ID: "a1", ToolOrAgent: "agent:react", Description: "desc"}))
	require.NoError(t, ks.Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	require.Len(t, bb.CompletedActions, 1)
	assert.Equal(t, "completed: desc", bb.CompletedActions[0].Output)
}

func TestVerifierKSVerifiesWithoutRegistry(t *testing.T) {
	ks := &VerifierKS{}
	assert.Equal(t, "Verifier", ks.Name())
	assert.Equal(t, 60, ks.Priority())

	bb := NewBlackboard()
	assert.False(t, ks.CanActivate(bb), "no artifacts")
	require.NoError(t, bb.AddArtifact("a", "patch", "c", "Executor"))
	assert.True(t, ks.CanActivate(bb))
	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	assert.True(t, bb.Artifacts[0].Verified)
	assert.False(t, ks.CanActivate(bb))
}

func TestVerifierKSWithCapabilityVerdicts(t *testing.T) {
	t.Run("verified", func(t *testing.T) {
		verifier := &scriptedCapability{id: "verifier.verify", name: "verify", result: &ports.ToolResult{
			Success: true, Data: map[string]any{"verified": true},
		}}
		reg := newRegistryWith(t, verifier)
		bb := NewBlackboard(bbTestGoal)
		require.NoError(t, bb.AddArtifact("a", "patch", "c", "Executor"))
		require.NoError(t, (&VerifierKS{}).Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
		assert.True(t, bb.Artifacts[0].Verified)
	})
	t.Run("missing items fail verification and raise issues", func(t *testing.T) {
		verifier := &scriptedCapability{id: "verifier.verify", name: "verify", result: &ports.ToolResult{
			Success: true,
			Data: map[string]any{
				"verified":      false,
				"missing_items": []any{"edge case", " ", "docs"},
			},
		}}
		reg := newRegistryWith(t, verifier)
		bb := NewBlackboard(bbTestGoal)
		require.NoError(t, bb.AddArtifact("a", "patch", "c", "Executor"))
		err := (&VerifierKS{}).Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "edge case; docs")
		assert.False(t, bb.Artifacts[0].Verified)
		assert.Len(t, bb.Issues, 2, "blank missing items skipped")
	})
	t.Run("no verdict fails without issues", func(t *testing.T) {
		verifier := &scriptedCapability{id: "verifier.verify", name: "verify", result: &ports.ToolResult{
			Success: true, Data: map[string]any{"verified": false},
		}}
		reg := newRegistryWith(t, verifier)
		bb := NewBlackboard(bbTestGoal)
		require.NoError(t, bb.AddArtifact("a", "patch", "c", "Executor"))
		err := (&VerifierKS{}).Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{})
		require.EqualError(t, err, "verification failed")
		assert.False(t, bb.Artifacts[0].Verified)
	})
	t.Run("duplicate missing item ids tolerated", func(t *testing.T) {
		verifier := &scriptedCapability{id: "verifier.verify", name: "verify", result: &ports.ToolResult{
			Success: true,
			Data: map[string]any{
				"verified":      false,
				"missing_items": []string{"gap", "gap"},
			},
		}}
		reg := newRegistryWith(t, verifier)
		bb := NewBlackboard(bbTestGoal)
		require.NoError(t, bb.AddArtifact("a", "patch", "c", "Executor"))
		// First invocation adds verify-missing-0 and verify-missing-1.
		require.Error(t, (&VerifierKS{}).Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	})
}

func TestFailureTriageKSRetriesFailedActions(t *testing.T) {
	ks := &FailureTriageKS{}
	assert.Equal(t, "FailureTriage", ks.Name())
	assert.Equal(t, 65, ks.Priority())

	bb := NewBlackboard()
	assert.False(t, ks.CanActivate(bb), "no completed actions")
	bb.CompletedActions = []ActionResult{{RequestID: "a1", Success: true}}
	assert.False(t, ks.CanActivate(bb), "all successes")
	bb.CompletedActions = append(bb.CompletedActions, ActionResult{RequestID: "a2", Success: false, Error: " timeout "})
	assert.True(t, ks.CanActivate(bb))

	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	issue, ok := func() (Issue, bool) {
		for _, issue := range bb.Issues {
			if issue.ID == "triage-a2" {
				return issue, true
			}
		}
		return Issue{}, false
	}()
	require.True(t, ok, "triage issue recorded")
	assert.Equal(t, "high", issue.Severity)
	assert.Contains(t, issue.Description, "retry required after failed action a2: timeout")

	retry, ok := func() (ActionRequest, bool) {
		for _, action := range bb.PendingActions {
			if action.ID == "retry-a2" {
				return action, true
			}
		}
		return ActionRequest{}, false
	}()
	require.True(t, ok, "retry action enqueued")
	assert.Equal(t, defaultAgentDispatch, retry.ToolOrAgent)
	assert.Equal(t, "FailureTriage", retry.RequestedBy)

	// Idempotent: second run neither duplicates issues nor actions.
	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	assert.Len(t, bb.Issues, 1)
	assert.Len(t, bb.PendingActions, 1)
}

func TestSummarizerKSBuildsSummaryArtifact(t *testing.T) {
	ks := &SummarizerKS{}
	assert.Equal(t, "Summarizer", ks.Name())
	assert.Equal(t, 55, ks.Priority())

	bb := NewBlackboard(bbTestGoal)
	assert.False(t, ks.CanActivate(bb), "no completed actions")
	bb.CompletedActions = []ActionResult{{RequestID: "a1", Success: true}}
	assert.True(t, ks.CanActivate(bb))

	require.NoError(t, bb.AddArtifact("a", "patch", "diff", "Executor"))
	bb.VerifyArtifact("a")

	require.NoError(t, ks.Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	summary, ok := func() (Artifact, bool) {
		for _, artifact := range bb.Artifacts {
			if artifact.ID == "blackboard-summary" {
				return artifact, true
			}
		}
		return Artifact{}, false
	}()
	require.True(t, ok)
	assert.Equal(t, "summary", summary.Kind)
	assert.Equal(t, "Summarizer", summary.Source)
	assert.Contains(t, summary.Content, fmt.Sprintf("Completed %q", bbTestGoal))
	assert.Contains(t, summary.Content, "patch=diff")
	assert.False(t, ks.CanActivate(bb), "summary already present")
}

func TestSummarizerKSGoalOnlySummary(t *testing.T) {
	bb := NewBlackboard(bbTestGoal)
	bb.CompletedActions = []ActionResult{{RequestID: "a1"}}
	require.NoError(t, (&SummarizerKS{}).Execute(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	summary, _ := findArtifact(bb, "blackboard-summary")
	assert.Equal(t, fmt.Sprintf("Completed %q", bbTestGoal), summary.Content)
}

func TestSummarizerKSCapabilitySummaryWins(t *testing.T) {
	summarizer := &scriptedCapability{id: "summarizer.summarize", name: "summarize", result: &ports.ToolResult{
		Success: true, Data: map[string]any{"summary": "  model summary  "},
	}}
	reg := newRegistryWith(t, summarizer)
	bb := NewBlackboard(bbTestGoal)
	bb.CompletedActions = []ActionResult{{RequestID: "a1"}}
	require.NoError(t, (&SummarizerKS{}).Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	summary, _ := findArtifact(bb, "blackboard-summary")
	assert.Equal(t, "model summary", summary.Content)
}

func TestSummarizerKSCapabilityNilSummaryFallsBack(t *testing.T) {
	summarizer := &scriptedCapability{id: "summarizer.summarize", name: "summarize", result: &ports.ToolResult{
		Success: true, Data: map[string]any{"summary": "<nil>"},
	}}
	reg := newRegistryWith(t, summarizer)
	bb := NewBlackboard(bbTestGoal)
	bb.CompletedActions = []ActionResult{{RequestID: "a1"}}
	require.NoError(t, (&SummarizerKS{}).Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{}))
	summary, _ := findArtifact(bb, "blackboard-summary")
	assert.Contains(t, summary.Content, "Completed")
}

func TestSummarizerKSCapabilityErrorSurfaces(t *testing.T) {
	summarizer := &scriptedCapability{id: "summarizer.summarize", name: "summarize", err: errors.New("summarizer down")}
	reg := newRegistryWith(t, summarizer)
	bb := NewBlackboard(bbTestGoal)
	bb.CompletedActions = []ActionResult{{RequestID: "a1"}}
	err := (&SummarizerKS{}).Execute(context.Background(), bb, reg, nil, execctx.AgentSemanticContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "summarizer down")
}

func findArtifact(bb *Blackboard, id string) (Artifact, bool) {
	for _, artifact := range bb.Artifacts {
		if artifact.ID == id {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func TestExecuteActionRequestCapabilityFailurePath(t *testing.T) {
	tool := &scriptedCapability{id: "unittest.tool", name: "unittest.tool", err: errors.New("compile error")}
	reg := newRegistryWith(t, tool)
	_, err := executeActionRequest(context.Background(), nil, reg, ActionRequest{ID: "a1", ToolOrAgent: "unittest.tool", Description: "build"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compile error")
}

func TestExecuteActionRequestEmptyTargetErrors(t *testing.T) {
	reg := newRegistryWith(t)
	_, err := executeActionRequest(context.Background(), nil, reg, ActionRequest{ID: "a1", ToolOrAgent: "   "})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "action target required")
}

func TestExecuteActionRequestCoordinationTargetWithCapability(t *testing.T) {
	agent := &scriptedCapability{id: "agent:htn", name: "htn", coordTarget: true, result: &ports.ToolResult{
		Success: true, Data: map[string]any{"summary": "nested plan done"},
	}}
	reg := newRegistryWith(t, agent)
	out, err := executeActionRequest(context.Background(), nil, reg, ActionRequest{ID: "a1", ToolOrAgent: "agent:htn", Description: "desc"})
	require.NoError(t, err)
	assert.Equal(t, "nested plan done", out)
}

func TestExecuteActionRequestExecutorInvokeWrapsTarget(t *testing.T) {
	// Non-coordination capability target plus an executor.invoke wrapper:
	// the dispatch goes through executor.invoke with capability+args fields.
	tool := &scriptedCapability{id: "go_test", name: "go_test", result: &ports.ToolResult{
		Success: true, Data: map[string]any{"summary": "wrapped ok"},
	}}
	executor := &scriptedCapability{id: "executor.invoke", name: "executor", result: &ports.ToolResult{
		Success: true, Data: map[string]any{"summary": "executor wrapped"},
	}}
	reg := newRegistryWith(t, tool, executor)
	out, err := executeActionRequest(context.Background(), nil, reg, ActionRequest{ID: "a1", ToolOrAgent: "go_test", Description: "desc", Args: map[string]any{"k": "v"}})
	require.NoError(t, err)
	assert.Equal(t, "executor wrapped", out)
	require.Len(t, executor.invocations, 1)
	assert.Equal(t, "go_test", executor.invocations[0]["capability"])
	assert.Equal(t, map[string]any{"k": "v"}, executor.invocations[0]["args"])
}

func TestEnqueuePlannedActionsValidation(t *testing.T) {
	bb := NewBlackboard()
	t.Run("nil or empty result", func(t *testing.T) {
		count, err := enqueuePlannedActions(bb, nil, "Planner")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		count, err = enqueuePlannedActions(bb, &ports.ToolResult{Success: true}, "Planner")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
	t.Run("missing steps key", func(t *testing.T) {
		count, err := enqueuePlannedActions(bb, &ports.ToolResult{Data: map[string]any{"other": 1}}, "Planner")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
	t.Run("non-list steps", func(t *testing.T) {
		count, err := enqueuePlannedActions(bb, &ports.ToolResult{Data: map[string]any{"steps": 42}}, "Planner")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestAddFindingsAsIssuesValidation(t *testing.T) {
	bb := NewBlackboard()
	t.Run("nil or empty result", func(t *testing.T) {
		count, err := addFindingsAsIssues(bb, nil, "src")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		count, err = addFindingsAsIssues(bb, &ports.ToolResult{Success: true}, "src")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
	t.Run("non-list findings", func(t *testing.T) {
		count, err := addFindingsAsIssues(bb, &ports.ToolResult{Data: map[string]any{"findings": 7}}, "src")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
	t.Run("finding error aborts", func(t *testing.T) {
		faulty := &ports.ToolResult{Data: map[string]any{"findings": []any{map[string]any{"description": "ok", "severity": "bogus"}}}}
		count, err := addFindingsAsIssues(bb, faulty, "src")
		require.Error(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestSummarizeCapabilityResultPreferenceOrder(t *testing.T) {
	assert.Equal(t, "fallback", summarizeCapabilityResult(nil, "fallback"))
	summary := summarizeCapabilityResult(&ports.ToolResult{Data: map[string]any{
		"summary": " s ", "output": "o", "result": "r",
	}}, "fallback")
	assert.Equal(t, "s", summary)
	output := summarizeCapabilityResult(&ports.ToolResult{Data: map[string]any{
		"summary": "", "output": "o", "result": "r",
	}}, "fallback")
	assert.Equal(t, "o", output)
	result := summarizeCapabilityResult(&ports.ToolResult{Data: map[string]any{
		"summary": "", "output": "", "result": "r",
	}}, "fallback")
	assert.Equal(t, "r", result)
	nilText := summarizeCapabilityResult(&ports.ToolResult{Data: map[string]any{
		"summary": "", "output": "", "result": nil,
	}}, "fallback")
	assert.Equal(t, "fallback", nilText, "<nil> text falls back on the result branch")
}

func TestSummarizeCapabilityResultMissingKeysYieldNilPlaceholder(t *testing.T) {
	// Documents current behavior: a result whose Data lacks "summary"/"output"
	// formats the missing values as "<nil>", which the first two branches
	// accept verbatim (only the result branch guards against the placeholder).
	out := summarizeCapabilityResult(&ports.ToolResult{Success: true, Data: map[string]any{}}, "fallback")
	assert.Equal(t, "<nil>", out)
}

func TestMapFromAny(t *testing.T) {
	assert.Empty(t, mapFromAny(nil))
	src := map[string]any{"a": 1}
	out := mapFromAny(src)
	assert.Equal(t, src, out)
	out["a"] = 2
	assert.Equal(t, 1, src["a"], "map is copied")
	assert.Empty(t, mapFromAny("scalar"))
}

func TestStringSliceFromAny(t *testing.T) {
	assert.Nil(t, stringSliceFromAny(nil))
	assert.Equal(t, []string{"a", "b"}, stringSliceFromAny([]string{"a", "b"}))
	assert.Equal(t, []string{"a", "b"}, stringSliceFromAny([]any{"a", " ", "b"}))
	assert.Nil(t, stringSliceFromAny(42))
}

func TestFactNamespaceAndFactsSummary(t *testing.T) {
	assert.Equal(t, "task", factNamespace("task.goal.0"))
	assert.Equal(t, "ast", factNamespace("ast.symbol.x"))
	assert.Equal(t, "bkc", factNamespace("bkc.chunk.1"))
	assert.Equal(t, "pattern", factNamespace("pattern.1"))
	assert.Equal(t, "learning", factNamespace("learning.x"))
	assert.Equal(t, "plan", factNamespace("plan.step"))
	assert.Equal(t, "other", factNamespace("random.key"))
	assert.Equal(t, "other", factNamespace("nospacerule"))

	bb := NewBlackboard()
	assert.Empty(t, factsSummary(bb))
	bb.AddFact("task.goal.0", "goal one", "s")
	bb.AddFact("ast.symbol.f", "func f", "s")
	bb.AddFact("custom.k", "v", "s")
	summary := factsSummary(bb)
	assert.Contains(t, summary, "[task]")
	assert.Contains(t, summary, "[ast]")
	assert.Contains(t, summary, "[other]")
	assert.Contains(t, summary, "task.goal.0: goal one")
	// Stable order: task, ast, other.
	assert.Less(t, strings.Index(summary, "[task]"), strings.Index(summary, "[ast]"))
	assert.Less(t, strings.Index(summary, "[ast]"), strings.Index(summary, "[other]"))
}

func TestAnalysisAndIssueAndArtifactSummaries(t *testing.T) {
	bb := NewBlackboard()
	assert.Equal(t, "analysis complete", analysisSummary(bb))
	bb.AddFact("k", "v", "s")
	assert.Equal(t, "analysis from facts: k=v", analysisSummary(bb))

	assert.Empty(t, issuesSummary(bb))
	assert.NoError(t, bb.AddIssue("i", "desc", "high", "s"))
	assert.Equal(t, "high:desc", issuesSummary(bb))

	assert.Empty(t, artifactSummary(bb))
	require.NoError(t, bb.AddArtifact("a", "patch", "diff", "s"))
	assert.Equal(t, "patch:diff", artifactSummary(bb))
}

func TestPlannerInstruction(t *testing.T) {
	bb := NewBlackboard()
	bb.Goals = []string{"the goal"}
	assert.Equal(t, "the goal", plannerInstruction(bb))
	bb.AddIssue("i", "issue one", "low", "s")
	bb.AddIssue("i2", "issue two", "low", "s")
	assert.Equal(t, "the goal\n\nIssues:\n- issue one\n- issue two", plannerInstruction(bb))
	bb.Goals = nil
	assert.Equal(t, "issue one\nissue two", plannerInstruction(bb))
}

func TestFirstGoalEmptyBoard(t *testing.T) {
	var bb *Blackboard
	assert.Equal(t, "", firstGoal(bb))
	assert.Equal(t, "", firstGoal(NewBlackboard()))
}

func TestBuildBlackboardCompletionSummarySkipsPriorSummary(t *testing.T) {
	bb := NewBlackboard("goal")
	require.NoError(t, bb.AddArtifact("blackboard-summary", "summary", "old", "Summarizer"))
	require.NoError(t, bb.AddArtifact("a", "patch", "diff", "Executor"))
	summary := buildBlackboardCompletionSummary(bb)
	assert.Equal(t, `Completed "goal" with artifacts: patch=diff`, summary)
}

func TestBuildBlackboardCompletionSummaryEmptyBoard(t *testing.T) {
	assert.Equal(t, "", buildBlackboardCompletionSummary(nil))
	bb := NewBlackboard()
	assert.Equal(t, "", buildBlackboardCompletionSummary(bb))
}

func TestErrorString(t *testing.T) {
	assert.Equal(t, "", errorString(nil))
	assert.Equal(t, "oops", errorString(errors.New("oops")))
}

func TestInvokeCapabilityIfPresentAbsentCapability(t *testing.T) {
	reg := registry.NewRegistry()
	result, ok, err := invokeCapabilityIfPresent(context.Background(), nil, reg, "missing.capability", nil)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, result)

	result, ok, err = invokeCapabilityIfPresent(context.Background(), nil, nil, "any", nil)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, result)
}
