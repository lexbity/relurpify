package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const WorkspaceSchemaV1 = "relurpify/workspace/v1"

type WorkspaceConfigV1 struct {
	Schema    string          `yaml:"schema"`
	Paths     PathsConfigV1   `yaml:"paths"`
	Model     ModelConfigV1   `yaml:"model"`
	Sandbox   SandboxConfigV1 `yaml:"sandbox"`
	Logging   LoggingConfigV1 `yaml:"logging"`
	Audit     AuditConfigV1   `yaml:"audit"`
	Telemetry TelemetryConfigV1 `yaml:"telemetry"`
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
	RetentionDays int `yaml:"retention_days"`
}

type TelemetryConfigV1 struct {
	Enabled bool `yaml:"enabled"`
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
