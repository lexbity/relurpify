package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/execution/compiler"
)

func TestKnowledgeProductCloseStopsEveryOwnedLifecycle(t *testing.T) {
	// Every lifecycle handle tolerates a zero value: Close must not panic and
	// must call each stopper exactly through its nil-guard.
	product := &KnowledgeProduct{
		Invalidation:   &knowledge.InvalidationPass{},
		Compiler:       &compiler.Compiler{},
		Health:         &knowledge.KnowledgeHealth{},
		CloseRetriever: func() {},
	}
	assert.NotPanics(t, product.Close)
	// Close is safe to repeat.
	assert.NotPanics(t, product.Close)
}

func TestKnowledgeProductCloseNilSafe(t *testing.T) {
	var product *KnowledgeProduct
	assert.NotPanics(t, product.Close)
	assert.NotPanics(t, (&KnowledgeProduct{}).Close)
}

func TestWorkspaceConfigInferenceValueGetters(t *testing.T) {
	cfg := WorkspaceConfig{
		InferenceProvider:          "ollama",
		InferenceEndpoint:          "http://localhost:11434",
		InferenceModel:             "gemma4:e4b",
		InferenceNativeToolCalling: true,
	}
	assert.Equal(t, "ollama", cfg.InferenceProviderValue())
	assert.Equal(t, "http://localhost:11434", cfg.InferenceEndpointValue())
	assert.Equal(t, "gemma4:e4b", cfg.InferenceModelValue())
	assert.True(t, cfg.InferenceNativeToolCallingValue())

	var zero WorkspaceConfig
	assert.Empty(t, zero.InferenceProviderValue())
	assert.Empty(t, zero.InferenceEndpointValue())
	assert.Empty(t, zero.InferenceModelValue())
	assert.False(t, zero.InferenceNativeToolCallingValue())
}

func TestReadinessReadyRequiresBothLegsAndNoDegradation(t *testing.T) {
	tests := []struct {
		name      string
		readiness Readiness
		want      bool
	}{
		{"fully ready", Readiness{SandboxReady: true, ModelReady: true}, true},
		{"sandbox missing", Readiness{ModelReady: true}, false},
		{"model missing", Readiness{SandboxReady: true}, false},
		{"degraded", Readiness{SandboxReady: true, ModelReady: true, Degraded: true}, false},
		{"nothing ready", Readiness{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.readiness.Ready())
		})
	}
}

func TestDegradedWorkspaceCarriesReasonAndNoBackend(t *testing.T) {
	ws := DegradedWorkspace("sandbox unavailable")
	require.NotNil(t, ws)
	assert.True(t, ws.Readiness.Degraded)
	assert.Equal(t, "sandbox unavailable", ws.Readiness.Reason)
	assert.False(t, ws.Readiness.Ready())
	assert.Nil(t, ws.Backend, "degraded workspaces must not carry a backend")
}

func TestSynthesizeDocumentSnapshotDefaultsAgentName(t *testing.T) {
	spec := &agentspec.AgentRuntimeSpec{Model: agentspec.AgentModelConfig{Name: "m"}}
	snap := synthesizeDocumentSnapshot("", spec)
	require.NotNil(t, snap)
	require.NotNil(t, snap.Document)
	assert.Equal(t, "agent", snap.Document.Metadata.Name)
	assert.Equal(t, "relurpify.io/v1", snap.Document.APIVersion)
	assert.Equal(t, "AgentManifest", snap.Document.Kind)
	_, ok := snap.Document.Section("agent")
	assert.True(t, ok, "synthesized snapshot must carry the agent section")

	named := synthesizeDocumentSnapshot("custom-name", spec)
	assert.Equal(t, "custom-name", named.Document.Metadata.Name)
}
