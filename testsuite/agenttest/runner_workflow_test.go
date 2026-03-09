package agenttest

import (
	"testing"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/capability"
)

func TestInstantiateAgentByNameConfiguresWorkflowPaths(t *testing.T) {
	workspace := t.TempDir()

	agent := instantiateAgentByName(workspace, "coding", nil, capability.NewRegistry(), nil, nil)
	coding, ok := agent.(*agents.CodingAgent)
	if !ok {
		t.Fatalf("expected coding agent, got %T", agent)
	}
	if coding.WorkflowStatePath == "" {
		t.Fatal("expected workflow state path to be configured")
	}
	if coding.CheckpointPath == "" {
		t.Fatal("expected checkpoint path to be configured")
	}
}
