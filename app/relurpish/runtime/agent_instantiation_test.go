package runtime

import (
	"testing"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestInstantiateAgentUsesCodingAgentForDefinitionBackedCodingManifest(t *testing.T) {
	cfg := Config{Workspace: t.TempDir(), AgentName: "coding-go"}
	defs := map[string]*core.AgentDefinition{
		"coding-go": {
			Name: "coding-go",
			Spec: core.AgentRuntimeSpec{
				Implementation: "coding",
				Mode:           core.AgentModePrimary,
			},
		},
	}

	agent := instantiateAgent(cfg, nil, capability.NewRegistry(), nil, defs, &core.Config{}, nil)

	_, ok := agent.(*agents.CodingAgent)
	require.True(t, ok, "expected CodingAgent for implementation=coding")
}

func TestInstantiateAgentUsesReflectionAgentForDefinitionBackedReflectionManifest(t *testing.T) {
	cfg := Config{Workspace: t.TempDir(), AgentName: "reflection"}
	defs := map[string]*core.AgentDefinition{
		"reflection": {
			Name: "reflection",
			Spec: core.AgentRuntimeSpec{
				Implementation: "reflection",
				Mode:           core.AgentModePrimary,
			},
		},
	}

	agent := instantiateAgent(cfg, nil, capability.NewRegistry(), nil, defs, &core.Config{}, nil)

	reflection, ok := agent.(*agents.ReflectionAgent)
	require.True(t, ok, "expected ReflectionAgent for implementation=reflection")
	_, ok = reflection.Delegate.(*agents.CodingAgent)
	require.True(t, ok, "expected reflection delegate to remain CodingAgent-backed")
}

func TestInstantiateAgentTreatsExpertDefinitionAsCodingAgent(t *testing.T) {
	cfg := Config{Workspace: t.TempDir(), AgentName: "expert"}
	defs := map[string]*core.AgentDefinition{
		"expert": {
			Name: "expert",
			Spec: core.AgentRuntimeSpec{
				Implementation: "expert",
				Mode:           core.AgentModePrimary,
			},
		},
	}

	agent := instantiateAgent(cfg, nil, capability.NewRegistry(), nil, defs, &core.Config{}, nil)

	_, ok := agent.(*agents.CodingAgent)
	require.True(t, ok, "expected CodingAgent fallback for implementation=expert")
}
