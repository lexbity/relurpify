package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// Config captures every knob shared across the relurpish CLI, TUI, and server
// entry points. Keeping it as a lightweight struct makes it trivial to reuse in
// tests or future headless workflows.
type Config struct {
	Workspace                  string
	ManifestPath               string
	AgentsDir                  string
	SharedRoot                 string
	MemoryPath                 string
	LogPath                    string
	TelemetryPath              string
	EventsPath                 string
	ConfigPath                 string
	InferenceProvider          string
	InferenceEndpoint          string
	InferenceModel             string
	InferenceNativeToolCalling bool
	EmbeddingProvider          string
	EmbeddingEndpoint          string
	EmbeddingModel             string
	AgentName                  string
	ServerAddr                 string
	RecordingMode              string
	SandboxBackend             string
	EnvOverrides               []string
	Sandbox                    fsandbox.SandboxConfig
	CommandPolicy              fsandbox.CommandPolicy
	AuditLimit                 int
	HITLTimeout                time.Duration
	Editor                     string
}

// DefaultConfig infers sensible defaults based on the current working
// directory. Errors from os.Getwd are ignored so callers can override manually.
func DefaultConfig() Config {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return Config{
		Workspace:      cwd,
		ManifestPath:   "relurpify_cfg/agents/coding.yaml",
		AgentsDir:      "relurpify_cfg/agents",
		MemoryPath:     ".relurpify_state/memory",
		LogPath:        ".relurpify_state/logs/relurpish.log",
		TelemetryPath:  ".relurpify_state/telemetry/telemetry.jsonl",
		EventsPath:     ".relurpify_state/events.db",
		ConfigPath:     ".relurpify_state/workspace.yaml",
		ServerAddr:     ":8080",
		AuditLimit:     512,
		HITLTimeout:    45 * time.Second,
		SandboxBackend: "",
		Sandbox: fsandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "",
			NetworkIsolation: true,
			ReadOnlyRoot:     true,
		},
	}
}

// Normalize ensures every filesystem path is absolute and fills missing
// defaults so runtime initialization never has to re-check the same invariants.
func (c *Config) Normalize() error {
	if c.Workspace == "" {
		return fmt.Errorf("workspace path required")
	}
	absWorkspace, err := filepath.Abs(c.Workspace)
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	c.Workspace = absWorkspace
	paths := config.New(c.Workspace)
	if c.AgentName == "" {
		c.AgentName = "coding"
	}
	if c.ManifestPath == "" {
		c.ManifestPath = filepath.Join(paths.AgentsDir(), c.AgentName+".yaml")
	}
	if !filepath.IsAbs(c.ManifestPath) {
		c.ManifestPath = filepath.Join(c.Workspace, c.ManifestPath)
	}
	if c.AgentsDir == "" {
		c.AgentsDir = paths.AgentsDir()
	}
	if !filepath.IsAbs(c.AgentsDir) {
		c.AgentsDir = filepath.Join(c.Workspace, c.AgentsDir)
	}
	if c.MemoryPath == "" {
		c.MemoryPath = config.DefaultWorkspaceStateMemoryDir(c.Workspace)
	}
	if !filepath.IsAbs(c.MemoryPath) {
		c.MemoryPath = filepath.Join(c.Workspace, c.MemoryPath)
	}
	if c.LogPath == "" {
		c.LogPath = filepath.Join(config.DefaultWorkspaceStateLogsDir(c.Workspace), "relurpish.log")
	}
	if !filepath.IsAbs(c.LogPath) {
		c.LogPath = filepath.Join(c.Workspace, c.LogPath)
	}
	if c.TelemetryPath == "" {
		c.TelemetryPath = filepath.Join(config.DefaultWorkspaceStateTelemetryDir(c.Workspace), "telemetry.jsonl")
	}
	if !filepath.IsAbs(c.TelemetryPath) {
		c.TelemetryPath = filepath.Join(c.Workspace, c.TelemetryPath)
	}
	if c.EventsPath == "" {
		c.EventsPath = config.DefaultWorkspaceStateEventsFile(c.Workspace)
	}
	if !filepath.IsAbs(c.EventsPath) {
		c.EventsPath = filepath.Join(c.Workspace, c.EventsPath)
	}
	if c.ConfigPath == "" {
		c.ConfigPath = config.DefaultWorkspaceStateConfigPath(c.Workspace)
	}
	if !filepath.IsAbs(c.ConfigPath) {
		c.ConfigPath = filepath.Join(c.Workspace, c.ConfigPath)
	}
	if c.AgentName == "" {
		c.AgentName = "coding"
	}
	if c.InferenceProvider == "" {
		c.InferenceProvider = "ollama"
	}
	if c.InferenceEndpoint == "" {
		c.InferenceEndpoint = "http://localhost:11434"
	}
	if c.ServerAddr == "" {
		c.ServerAddr = ":8080"
	}
	if c.AuditLimit <= 0 {
		c.AuditLimit = 256
	}
	if c.HITLTimeout <= 0 {
		c.HITLTimeout = 30 * time.Second
	}
	return nil
}

func (c Config) InferenceProviderValue() string {
	return c.InferenceProvider
}

func (c Config) InferenceEndpointValue() string {
	return c.InferenceEndpoint
}

func (c Config) InferenceModelValue() string {
	return c.InferenceModel
}

func (c Config) InferenceNativeToolCallingValue() bool {
	return c.InferenceNativeToolCalling
}

// AgentLabel returns the normalized agent identifier used across telemetry and
// UI views.
func (c Config) AgentLabel() string {
	switch c.AgentName {
	case "planner", "react", "reflection", "expert":
		return c.AgentName
	case "coding", "coder":
		return "coding"
	default:
		return "coding"
	}
}
