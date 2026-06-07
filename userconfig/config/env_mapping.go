package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvOverrides captures deterministic RELURPIFY_* config overrides.
type EnvOverrides struct {
	WorkspaceRoot  string
	ModelProvider  string
	ModelName      string
	SandboxBackend string
	OllamaHost     string
	LogLevel       string
	Editor         string
	XDGDataHome    string
	Strict         bool
}

// Secrets captures env-only secret material.
type Secrets struct {
	LLMAPIKey       string
	NexusToken      string
	NexusAdminToken string
}

// RecognizedEnvVars lists every environment variable consumed by config.
func RecognizedEnvVars() []string {
	return []string{
		"RELURPIFY_WORKSPACE",
		"RELURPIFY_MODEL_PROVIDER",
		"RELURPIFY_MODEL_NAME",
		"RELURPIFY_SANDBOX_BACKEND",
		"RELURPIFY_OLLAMA_HOST",
		"RELURPIFY_LOG_LEVEL",
		"RELURPIFY_STRICT",
		"EDITOR",
		"XDG_DATA_HOME",
		"RELURPIFY_LLM_API_KEY",
		"RELURPIFY_NEXUS_TOKEN",
		"RELURPIFY_NEXUS_ADMIN_TOKEN",
	}
}

// ResolveSharedRoot derives the machine-local shared template root.
func ResolveSharedRoot(xdgDataHome string) string {
	xdgDataHome = strings.TrimSpace(xdgDataHome)
	if xdgDataHome != "" {
		return filepath.Join(xdgDataHome, "relurpify")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".local", "share", "relurpify")
	}
	return filepath.Join(home, ".local", "share", "relurpify")
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

// parseBoolEnv parses a boolean environment variable and returns an error
// for unrecognized input. Accepts: 1, true, yes, on (true) and 0, false,
// no, off (false). Any other value is an error — a typo like "flase" or
// "enabled" is caught immediately rather than silently treated as false.
func parseBoolEnv(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	case "":
		return false, nil
	default:
		return false, fmt.Errorf("unrecognized boolean value %q (expected 1/true/yes/on or 0/false/no/off)", value)
	}
}
