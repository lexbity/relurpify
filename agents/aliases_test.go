package agents

import (
	"context"
	"database/sql"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/chainer/checkpoint"
	"codeburg.org/lexbit/relurpify/agents/relurpic"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

type aliasHandler struct{}

func (h *aliasHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:   "custom.alias",
		Kind: core.CapabilityKindTool,
		Name: "custom.alias",
	}
}

func (h *aliasHandler) Invoke(context.Context, *core.Context, map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	return &core.CapabilityExecutionResult{}, nil
}

func TestAliasWrappers(t *testing.T) {
	if got := NewSQLitePipelineCheckpointStore(nil, "workflow", "run"); got == nil {
		t.Fatal("expected checkpoint store adapter")
	}
	if opt := WithIndexManager(&ast.IndexManager{}); opt == nil {
		t.Fatal("expected index manager option")
	}
	if opt := WithGraphDB(&graphdb.Engine{}); opt == nil {
		t.Fatal("expected graph db option")
	}
	if opt := WithPlanStore(frameworkplan.PlanStore(nil)); opt == nil {
		t.Fatal("expected plan store option")
	}
	if opt := WithRetrievalDB(&sql.DB{}); opt == nil {
		t.Fatal("expected retrieval db option")
	}
	if opt := WithGuidanceBroker(&guidance.GuidanceBroker{}); opt == nil {
		t.Fatal("expected guidance broker option")
	}
	if opt := WithWorkflowStore(memory.WorkflowStateStore(nil)); opt == nil {
		t.Fatal("expected workflow store option")
	}

	registry := capability.NewRegistry()
	if err := RegisterBuiltinRelurpicCapabilities(registry, nil, nil); err != nil {
		t.Fatalf("RegisterBuiltinRelurpicCapabilities: %v", err)
	}
	if err := RegisterBuiltinRelurpicCapabilitiesWithOptions(registry, nil, nil, nil); err != nil {
		t.Fatalf("RegisterBuiltinRelurpicCapabilitiesWithOptions: %v", err)
	}

	env := agentenv.AgentEnvironment{}
	if err := RegisterAgentCapabilities(registry, env); err != nil {
		t.Fatalf("RegisterAgentCapabilities: %v", err)
	}
	if err := RegisterCustomAgentHandler(registry, "custom.alias", &aliasHandler{}); err != nil {
		t.Fatalf("RegisterCustomAgentHandler: %v", err)
	}

	_ = relurpic.DefaultInvocationPolicies
	_ = checkpoint.NewStore
}
