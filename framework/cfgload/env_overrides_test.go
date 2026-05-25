package cfgload

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadEnvOverridesAndSecrets(t *testing.T) {
	env := []string{
		"RELURPIFY_WORKSPACE=/tmp/ws",
		"RELURPIFY_MODEL=gemma4:e4b",
		"RELURPIFY_SANDBOX_BACKEND=docker",
		"RELURPIFY_OLLAMA_HOST=http://ollama:11434",
		"RELURPIFY_LOG_LEVEL=debug",
		"RELURPIFY_STRICT=true",
		"RELURPIFY_LLM_API_KEY=llm-secret",
		"RELURPIFY_NEXUS_TOKEN=nexus-secret",
		"RELURPIFY_NEXUS_ADMIN_TOKEN=admin-secret",
	}

	overrides := LoadEnvOverrides(env)
	require.Equal(t, "/tmp/ws", overrides.WorkspaceRoot)
	require.Equal(t, "gemma4:e4b", overrides.Model)
	require.Equal(t, "docker", overrides.SandboxBackend)
	require.Equal(t, "http://ollama:11434", overrides.OllamaHost)
	require.Equal(t, "debug", overrides.LogLevel)
	require.True(t, overrides.Strict)

	secrets := LoadSecrets(env)
	require.Equal(t, "llm-secret", secrets.LLMAPIKey)
	require.Equal(t, "nexus-secret", secrets.NexusToken)
	require.Equal(t, "admin-secret", secrets.NexusAdminToken)
}
