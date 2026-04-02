package euclo

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

func TestInitializeEnvironment_WiresDefaultVerificationPlanner(t *testing.T) {
	agent := &Agent{}
	err := agent.InitializeEnvironment(agentenv.AgentEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.Environment.VerificationPlanner == nil {
		t.Fatal("expected default verification planner to be wired")
	}
}
