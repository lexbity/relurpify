package cfgload

// LoadEnvOverrides resolves RELURPIFY_* config overrides from the supplied env list.
// An unrecognized RELURPIFY_STRICT value is returned as an error — a typo like
// "flase" or "enabled" fails the boot loudly rather than being silently ignored.
func LoadEnvOverrides(env []string) (EnvOverrides, error) {
	strict, err := parseBoolEnv(lookupEnv(env, "RELURPIFY_STRICT"))
	if err != nil {
		return EnvOverrides{}, err
	}
	return EnvOverrides{
		WorkspaceRoot:  lookupEnv(env, "RELURPIFY_WORKSPACE"),
		ModelProvider:  lookupEnv(env, "RELURPIFY_MODEL_PROVIDER"),
		ModelName:      lookupEnv(env, "RELURPIFY_MODEL_NAME"),
		SandboxBackend: lookupEnv(env, "RELURPIFY_SANDBOX_BACKEND"),
		OllamaHost:     lookupEnv(env, "RELURPIFY_OLLAMA_HOST"),
		LogLevel:       lookupEnv(env, "RELURPIFY_LOG_LEVEL"),
		Editor:         lookupEnv(env, "EDITOR"),
		XDGDataHome:    lookupEnv(env, "XDG_DATA_HOME"),
		Strict:         strict,
	}, nil
}

// LoadSecrets resolves env-only secrets from the supplied env list.
func LoadSecrets(env []string) Secrets {
	return Secrets{
		LLMAPIKey:       lookupEnv(env, "RELURPIFY_LLM_API_KEY"),
		NexusToken:      lookupEnv(env, "RELURPIFY_NEXUS_TOKEN"),
		NexusAdminToken: lookupEnv(env, "RELURPIFY_NEXUS_ADMIN_TOKEN"),
	}
}
