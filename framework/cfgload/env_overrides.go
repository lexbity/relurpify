package cfgload

// LoadEnvOverrides resolves RELURPIFY_* config overrides from the supplied env list.
func LoadEnvOverrides(env []string) EnvOverrides {
	return EnvOverrides{
		WorkspaceRoot:  lookupEnv(env, "RELURPIFY_WORKSPACE"),
		ModelProvider:  lookupEnv(env, "RELURPIFY_MODEL_PROVIDER"),
		ModelName:      lookupEnv(env, "RELURPIFY_MODEL_NAME"),
		SandboxBackend: lookupEnv(env, "RELURPIFY_SANDBOX_BACKEND"),
		OllamaHost:     lookupEnv(env, "RELURPIFY_OLLAMA_HOST"),
		LogLevel:       lookupEnv(env, "RELURPIFY_LOG_LEVEL"),
		Editor:         lookupEnv(env, "EDITOR"),
		XDGDataHome:    lookupEnv(env, "XDG_DATA_HOME"),
		Strict:         parseBoolEnv(lookupEnv(env, "RELURPIFY_STRICT")),
	}
}

// LoadSecrets resolves env-only secrets from the supplied env list.
func LoadSecrets(env []string) Secrets {
	return Secrets{
		LLMAPIKey:       lookupEnv(env, "RELURPIFY_LLM_API_KEY"),
		NexusToken:      lookupEnv(env, "RELURPIFY_NEXUS_TOKEN"),
		NexusAdminToken: lookupEnv(env, "RELURPIFY_NEXUS_ADMIN_TOKEN"),
	}
}
