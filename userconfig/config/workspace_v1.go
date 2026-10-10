package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const WorkspaceSchemaV1 = "relurpify/workspace/v1"

type WorkspaceConfigV1 struct {
	Schema    string            `yaml:"schema"`
	Paths     PathsConfigV1     `yaml:"paths"`
	Model     ModelConfigV1     `yaml:"model"`
	Sandbox   SandboxConfigV1   `yaml:"sandbox"`
	Logging   LoggingConfigV1   `yaml:"logging"`
	Audit     AuditConfigV1     `yaml:"audit"`
	Telemetry TelemetryConfigV1 `yaml:"telemetry"`
	Runner    RunnerConfigV1    `yaml:"runner"`
}

type PathsConfigV1 struct {
	StateDir string `yaml:"state_dir"`
}

type ModelConfigV1 struct {
	Provider string `yaml:"provider"`
	Name     string `yaml:"name"`
	Endpoint string `yaml:"endpoint,omitempty"`
}

type SandboxConfigV1 struct {
	Backend string `yaml:"backend"`
}

type LoggingConfigV1 struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type AuditConfigV1 struct {
	RetentionDays int    `yaml:"retention_days"`
	Enforcement   string `yaml:"enforcement"`
}

type TelemetryConfigV1 struct {
	Enabled bool `yaml:"enabled"`
}

// RunnerConfigV1 is the service-runner section (S9). Zero-value fields take
// defaults in Resolved; the section itself may be absent (enabled defaults
// true — the runner is part of a normal boot).
type RunnerConfigV1 struct {
	// Enabled is a pointer so an absent key decodes as the default-true
	// contract while an authored enabled: false is honored exactly.
	Enabled         *bool    `yaml:"enabled,omitempty"`
	Workers         int      `yaml:"workers,omitempty"`
	Queues          []string `yaml:"queues,omitempty"`
	PollInterval    string   `yaml:"poll_interval,omitempty"`
	Heartbeat       string   `yaml:"heartbeat,omitempty"`
	RefreshInterval string   `yaml:"refresh_interval,omitempty"`
}

// Runner defaults (§5.4/S9).
const (
	runnerDefaultPollInterval    = 500 * time.Millisecond
	runnerDefaultHeartbeat       = 5 * time.Second
	runnerDefaultRefreshInterval = 0 // off: absent, not stubbed (FR-23)
)

// RunnerSettings is the resolved runner configuration: every field carries a
// real value, never a zero sentinel.
type RunnerSettings struct {
	Enabled         bool
	Workers         int
	Queues          []string
	PollInterval    time.Duration
	Heartbeat       time.Duration
	RefreshInterval time.Duration // 0 = scheduled refresh off
}

// Resolved applies the defaults and validates the duration fields.
func (r RunnerConfigV1) Resolved() (RunnerSettings, error) {
	out := RunnerSettings{
		// Default true: an absent key (nil pointer) means the runner runs.
		Enabled:      r.Enabled == nil || *r.Enabled,
		Workers:      r.Workers,
		Queues:       append([]string(nil), r.Queues...),
		PollInterval: runnerDefaultPollInterval,
		Heartbeat:    runnerDefaultHeartbeat,
	}
	if out.Workers <= 0 {
		out.Workers = 2
	}
	if len(out.Queues) == 0 {
		out.Queues = []string{"knowledge"}
	}
	parse := func(name, raw string, def time.Duration) (time.Duration, error) {
		if strings.TrimSpace(raw) == "" {
			return def, nil
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, fmt.Errorf("runner.%s: %w", name, err)
		}
		if d < 0 {
			return 0, fmt.Errorf("runner.%s: must be >= 0", name)
		}
		return d, nil
	}
	var err error
	if out.PollInterval, err = parse("poll_interval", r.PollInterval, runnerDefaultPollInterval); err != nil {
		return RunnerSettings{}, err
	}
	if out.Heartbeat, err = parse("heartbeat", r.Heartbeat, runnerDefaultHeartbeat); err != nil {
		return RunnerSettings{}, err
	}
	if out.RefreshInterval, err = parse("refresh_interval", r.RefreshInterval, runnerDefaultRefreshInterval); err != nil {
		return RunnerSettings{}, err
	}
	return out, nil
}

func LoadRuntimeWorkspaceConfigV1(path string) (WorkspaceConfigV1, error) {
	if path == "" {
		return WorkspaceConfigV1{}, fmt.Errorf("config path required")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return WorkspaceConfigV1{}, err
	}
	if err := RejectForbiddenSecretFields(path, data); err != nil {
		return WorkspaceConfigV1{}, err
	}
	var cfg WorkspaceConfigV1
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return WorkspaceConfigV1{}, fmt.Errorf("decode workspace config: %w", err)
	}
	if strings.TrimSpace(cfg.Schema) != WorkspaceSchemaV1 {
		return WorkspaceConfigV1{}, fmt.Errorf("unsupported schema %q (expected %q)", cfg.Schema, WorkspaceSchemaV1)
	}
	return cfg, nil
}
