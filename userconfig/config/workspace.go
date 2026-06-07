package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config/model"
	"codeburg.org/lexbit/relurpify/userconfig/config/secretscan"
)

const defaultWorkspaceStateDir = secretscan.RuntimeStateDirName

// WorkspaceLoadOptions controls workspace loader behavior.
type WorkspaceLoadOptions struct {
	Strict bool
}

// WorkspaceDefaultUsage records a default that was applied while loading.
type WorkspaceDefaultUsage struct {
	Key   string
	Value any
}

// WorkspaceConfig models relurpify_cfg/workspace.yaml.
type WorkspaceConfig struct {
	Paths     WorkspacePaths     `yaml:"paths"`
	Model     model.ModelRef     `yaml:"model"`
	Sandbox   WorkspaceSandbox   `yaml:"sandbox"`
	Logging   WorkspaceLogging   `yaml:"logging"`
	Audit     WorkspaceAudit     `yaml:"audit"`
	Telemetry WorkspaceTelemetry `yaml:"telemetry"`

	SourcePath   string                  `yaml:"-"`
	Workspace    string                  `yaml:"-"`
	WorkspaceAbs string                  `yaml:"-"`
	StateDirAbs  string                  `yaml:"-"`
	DefaultsUsed []WorkspaceDefaultUsage `yaml:"-"`
}

// WorkspacePaths configures workspace-relative filesystem roots.
type WorkspacePaths struct {
	StateDir *string `yaml:"state_dir"`
}

// WorkspaceSandbox configures the default sandbox backend.
type WorkspaceSandbox struct {
	Backend *string `yaml:"backend"`
}

// WorkspaceLogging configures log formatting defaults.
type WorkspaceLogging struct {
	Level  *string `yaml:"level"`
	Format *string `yaml:"format"`
}

// WorkspaceAudit configures audit retention defaults.
type WorkspaceAudit struct {
	RetentionDays *int `yaml:"retention_days"`
}

// WorkspaceTelemetry configures state telemetry behavior.
type WorkspaceTelemetry struct {
	Enabled *bool `yaml:"enabled"`
}

// DefaultWorkspaceConfigPath returns the canonical workspace.yaml location.
func DefaultWorkspaceConfigPath(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "workspace.yaml")
}

// DefaultWorkspaceStateConfigPath returns the canonical workspace.yaml location under state.
func DefaultWorkspaceStateConfigPath(workspace string) string {
	return filepath.Join(DefaultWorkspaceStateDir(workspace), "workspace.yaml")
}

// DefaultWorkspaceStateDir returns the default runtime state directory.
func DefaultWorkspaceStateDir(workspace string) string {
	return filepath.Join(workspace, defaultWorkspaceStateDir)
}

// DefaultWorkspaceStateLogsDir returns the default log directory under state.
func DefaultWorkspaceStateLogsDir(workspace string) string {
	return filepath.Join(DefaultWorkspaceStateDir(workspace), "logs")
}

// DefaultWorkspaceStateTelemetryDir returns the default telemetry directory.
func DefaultWorkspaceStateTelemetryDir(workspace string) string {
	return filepath.Join(DefaultWorkspaceStateDir(workspace), "telemetry")
}

// DefaultWorkspaceStateMemoryDir returns the default memory directory.
func DefaultWorkspaceStateMemoryDir(workspace string) string {
	return filepath.Join(DefaultWorkspaceStateDir(workspace), "memory")
}

// DefaultWorkspaceStateEventsFile returns the default events database path.
func DefaultWorkspaceStateEventsFile(workspace string) string {
	return filepath.Join(DefaultWorkspaceStateDir(workspace), "events.db")
}

// LoadWorkspaceConfig loads, validates, and normalizes workspace.yaml.
func LoadWorkspaceConfig(path, workspace string, opts WorkspaceLoadOptions) (*WorkspaceConfig, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace path required")
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		path = DefaultWorkspaceConfigPath(absWorkspace)
	}
	data, err := ReadConfigFile(workspace, path)
	if err != nil {
		return nil, err
	}

	var cfg WorkspaceConfig
	if _, err := DecodeWithSchema(path, data, NewSchemaRegistry(), &cfg); err != nil {
		return nil, err
	}
	cfg.SourcePath = path
	cfg.Workspace = workspace
	cfg.WorkspaceAbs = absWorkspace

	if err := cfg.applyDefaults(opts.Strict); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	cfg.StateDirAbs = filepath.Join(absWorkspace, cfg.stateDirValue())
	return &cfg, nil
}

func (c *WorkspaceConfig) applyDefaults(strict bool) error {
	if c == nil {
		return fmt.Errorf("workspace config required")
	}
	c.DefaultsUsed = nil
	applyStringDefault := func(key string, target **string, value string) {
		if target == nil || *target != nil {
			return
		}
		*target = &value
		c.DefaultsUsed = append(c.DefaultsUsed, WorkspaceDefaultUsage{Key: key, Value: value})
	}
	applyIntDefault := func(key string, target **int, value int) {
		if target == nil || *target != nil {
			return
		}
		*target = &value
		c.DefaultsUsed = append(c.DefaultsUsed, WorkspaceDefaultUsage{Key: key, Value: value})
	}
	applyBoolDefault := func(key string, target **bool, value bool) {
		if target == nil || *target != nil {
			return
		}
		*target = &value
		c.DefaultsUsed = append(c.DefaultsUsed, WorkspaceDefaultUsage{Key: key, Value: value})
	}

	applyStringDefault("paths.state_dir", &c.Paths.StateDir, defaultWorkspaceStateDir)
	applyStringDefault("logging.level", &c.Logging.Level, "info")
	applyStringDefault("logging.format", &c.Logging.Format, "json")
	applyIntDefault("audit.retention_days", &c.Audit.RetentionDays, 7)
	applyBoolDefault("telemetry.enabled", &c.Telemetry.Enabled, false)

	if strict && len(c.DefaultsUsed) > 0 {
		keys := make([]string, 0, len(c.DefaultsUsed))
		for _, d := range c.DefaultsUsed {
			keys = append(keys, d.Key)
		}
		return fmt.Errorf("strict mode rejects defaulted workspace values: %s", strings.Join(keys, ", "))
	}
	return nil
}

func (c WorkspaceConfig) stateDirValue() string {
	if c.Paths.StateDir == nil {
		return defaultWorkspaceStateDir
	}
	return strings.TrimSpace(*c.Paths.StateDir)
}

func (c WorkspaceConfig) StateDir() string {
	if c.StateDirAbs != "" {
		return c.StateDirAbs
	}
	if c.WorkspaceAbs == "" {
		return ""
	}
	return filepath.Join(c.WorkspaceAbs, c.stateDirValue())
}

func (c WorkspaceConfig) LogsDir() string {
	stateDir := c.StateDir()
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, "logs")
}

func (c WorkspaceConfig) TelemetryDir() string {
	stateDir := c.StateDir()
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, "telemetry")
}

func (c WorkspaceConfig) EventsFile() string {
	stateDir := c.StateDir()
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, "events.db")
}
