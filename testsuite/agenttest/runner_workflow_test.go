package agenttest

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex"
)

func TestInstantiateAgentByNameConfiguresWorkflowPaths(t *testing.T) {
	workspace := t.TempDir()

	agent := instantiateAgentByName(workspace, "coding", agentenv.WorkspaceEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{MaxIterations: 1},
	})
	rexAgent, ok := agent.(*rex.Agent)
	if !ok {
		t.Fatalf("expected rex.Agent, got %T", agent)
	}
	if rexAgent.Workspace == "" {
		t.Fatal("expected rex workspace path to be configured")
	}
}
