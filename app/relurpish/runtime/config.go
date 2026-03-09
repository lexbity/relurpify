package runtime

import (
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/workspacecfg"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"time"
)

// Config captures every knob shared across the relurpish CLI, TUI, and server
// entry points. Keeping it as a lightweight struct makes it trivial to reuse in
// tests or future headless workflows.
type Config struct {
	Workspace      string
	ManifestPath   string
	AgentsDir      string
	MemoryPath     string
	LogPath        string
	TelemetryPath  string
	ConfigPath     string
	OllamaEndpoint string
	OllamaModel    string
	AgentName      string
	ServerAddr     string
	RecordingMode  string
	Sandbox        fruntime.SandboxConfig
	AuditLimit     int
	HITLTimeout    time.Duration
}

// DefaultConfig infers sensible defaults based on the current working
// directory. Errors from os.Getwd are ignored so callers can override manually.
func DefaultConfig() Config {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	paths := workspacecfg.New(cwd)
	return Config{
		Workspace:     cwd,
		ManifestPath:  paths.ManifestFile(),
		AgentsDir:     paths.AgentsDir(),
		MemoryPath:    paths.MemoryDir(),
		LogPath:       paths.LogFile("relurpish.log"),
		TelemetryPath: paths.TelemetryFile(""),
		ConfigPath:    paths.ConfigFile(),
		ServerAddr:    ":8080",
		AuditLimit:    512,
		HITLTimeout:   45 * time.Second,
		Sandbox: fruntime.SandboxConfig{
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
	paths := workspacecfg.New(c.Workspace)
	if c.ManifestPath == "" {
		c.ManifestPath = paths.ManifestFile()
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
		c.MemoryPath = paths.MemoryDir()
	}
	if !filepath.IsAbs(c.MemoryPath) {
		c.MemoryPath = filepath.Join(c.Workspace, c.MemoryPath)
	}
	if c.LogPath == "" {
		c.LogPath = paths.LogFile("relurpish.log")
	}
	if !filepath.IsAbs(c.LogPath) {
		c.LogPath = filepath.Join(c.Workspace, c.LogPath)
	}
	if c.TelemetryPath == "" {
		c.TelemetryPath = paths.TelemetryFile("")
	}
	if !filepath.IsAbs(c.TelemetryPath) {
		c.TelemetryPath = filepath.Join(c.Workspace, c.TelemetryPath)
	}
	if c.ConfigPath == "" {
		c.ConfigPath = paths.ConfigFile()
	}
	if !filepath.IsAbs(c.ConfigPath) {
		c.ConfigPath = filepath.Join(c.Workspace, c.ConfigPath)
	}
	if c.AgentName == "" {
		c.AgentName = "coding"
	}
	if c.OllamaEndpoint == "" {
		c.OllamaEndpoint = "http://localhost:11434"
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

// WorkspaceConfig captures persisted workspace preferences under relurpify_cfg.
type WorkspaceConfig struct {
	Model               string                    `yaml:"model"`
	Agents              []string                  `yaml:"agents"`
	AllowedCapabilities []core.CapabilitySelector `yaml:"allowed_capabilities,omitempty"`
	LastUpdated         int64                     `yaml:"last_updated"`
}

// LoadWorkspaceConfig loads workspace preferences from disk.
func LoadWorkspaceConfig(path string) (WorkspaceConfig, error) {
	if path == "" {
		return WorkspaceConfig{}, fmt.Errorf("config path required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceConfig{}, err
	}
	var cfg WorkspaceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return WorkspaceConfig{}, err
	}
	return cfg, nil
}

// SaveWorkspaceConfig persists selections for future sessions.
func SaveWorkspaceConfig(path string, cfg WorkspaceConfig) error {
	if path == "" {
		return fmt.Errorf("config path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
