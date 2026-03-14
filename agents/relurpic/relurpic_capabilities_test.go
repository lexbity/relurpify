package relurpic

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

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
	require.NoError(t, registry.Register(architectStubTool{}))

	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"goal":"Plan work","steps":[{"id":"step-1","description":"Inspect the repository","tool":"echo","params":{"value":"README.md"},"expected":"repository summary","verification":"","files":["README.md"]}],"dependencies":{},"files":["README.md"]}`},
		},
	}
	require.NoError(t, RegisterBuiltinRelurpicCapabilities(registry, model, &core.Config{
		Name:              "coding",
		Model:             "stub",
		MaxIterations:     3,
		OllamaToolCalling: true,
	}))

	targets := registry.CoordinationTargets()
	require.Len(t, targets, 5)

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

func TestPlannerCapabilityReturnsStructuredPlan(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(architectStubTool{}))

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
	require.NoError(t, registry.Register(architectStubTool{}))

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
		OllamaToolCalling: true,
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
	require.NoError(t, registry.Register(architectStubTool{}))
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
	require.NoError(t, registry.Register(architectStubTool{}))
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
