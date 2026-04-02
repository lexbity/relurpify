package testutil

import (
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

func RegistryWith(tools ...core.Tool) *capability.Registry {
	registry := capability.NewRegistry()
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		if err := registry.Register(tool); err != nil {
			panic(err)
		}
	}
	return registry
}

func EnvWithScenarioModel(t interface {
	Helper()
	Fatalf(string, ...interface{})
	TempDir() string
}, turns ...ScenarioModelTurn) (agentenv.AgentEnvironment, *ScenarioStubModel) {
	t.Helper()

	model := NewScenarioStubModel(turns...)
	env := Env(t)
	env.Model = model
	return env, model
}
