package session

import (
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
)

func TestResolveRuntimeModelSettingsPrefersRegisteredAgentSpec(t *testing.T) {
	reg := &Registration{
		AgentSpec: &agentspec.AgentRuntimeSpec{
			Model: agentspec.AgentModelConfig{Name: "registered-model"},
			Logging: &agentspec.AgentLoggingSpec{
				LLM: boolPtr(false),
			},
		},
	}

	modelName, logLLM := resolveRuntimeModelSettings("", true, reg)
	if modelName != "registered-model" {
		t.Fatalf("model name = %q, want registered-model", modelName)
	}
	if logLLM {
		t.Fatal("expected registered logging flag to override debug logging")
	}
}

func TestResolveRuntimeModelSettingsKeepsConfiguredValuesWhenAgentSpecMissing(t *testing.T) {
	modelName, logLLM := resolveRuntimeModelSettings("configured-model", true, nil)
	if modelName != "configured-model" {
		t.Fatalf("model name = %q, want configured-model", modelName)
	}
	if !logLLM {
		t.Fatal("expected debug logging to remain enabled")
	}
}

func boolPtr(v bool) *bool {
	return &v
}
