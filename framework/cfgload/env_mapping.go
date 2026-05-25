package cfgload

import "strings"

// EnvOverrides captures deterministic RELURPIFY_* config overrides.
type EnvOverrides struct {
	WorkspaceRoot  string
	Model          string
	SandboxBackend string
	OllamaHost     string
	LogLevel       string
	Strict         bool
}

// Secrets captures env-only secret material.
type Secrets struct {
	LLMAPIKey       string
	NexusToken      string
	NexusAdminToken string
}

func lookupEnv(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(entry, prefix))
		}
	}
	return ""
}

func parseBoolEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
