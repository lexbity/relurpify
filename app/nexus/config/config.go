package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Gateway  GatewayConfig             `yaml:"gateway"`
	Channels map[string]map[string]any `yaml:"channels,omitempty"`
	Nodes    NodesConfig               `yaml:"nodes,omitempty"`
}

type GatewayConfig struct {
	Bind string            `yaml:"bind"`
	Path string            `yaml:"path"`
	Auth GatewayAuthConfig `yaml:"auth,omitempty"`
	Log  GatewayLogConfig  `yaml:"log,omitempty"`
}

type GatewayAuthConfig struct {
	Enabled bool               `yaml:"enabled,omitempty"`
	Tokens  []GatewayTokenAuth `yaml:"tokens,omitempty"`
}

type GatewayTokenAuth struct {
	Token       string   `yaml:"token"`
	TenantID    string   `yaml:"tenant_id,omitempty"`
	Role        string   `yaml:"role"`
	SubjectKind string   `yaml:"subject_kind,omitempty"`
	SubjectID   string   `yaml:"subject_id"`
	Scopes      []string `yaml:"scopes,omitempty"`
}

type GatewayLogConfig struct {
	Path             string `yaml:"path,omitempty"`
	RetentionDays    int    `yaml:"retention_days,omitempty"`
	SnapshotInterval int    `yaml:"snapshot_interval_events,omitempty"`
}

type NodesConfig struct {
	AutoApproveLocal bool          `yaml:"auto_approve_local,omitempty"`
	PairingCodeTTL   time.Duration `yaml:"pairing_code_ttl,omitempty"`
}

// SecurityWarnings returns operator-visible warnings about the current config.
func (cfg Config) SecurityWarnings(pendingPairings int) []string {
	var warnings []string
	if bind := strings.TrimSpace(cfg.Gateway.Bind); bind != "" && !IsLoopbackBind(bind) {
		warnings = append(warnings, fmt.Sprintf("Gateway bind %q is not loopback-only.", bind))
	}
	if cfg.Nodes.AutoApproveLocal {
		warnings = append(warnings, "Local node auto-approval is enabled.")
	}
	if pendingPairings > 0 {
		warnings = append(warnings, fmt.Sprintf("%d node pairing request(s) are pending approval.", pendingPairings))
	}
	if len(cfg.Channels) == 0 {
		warnings = append(warnings, "No channels are configured; gateway surface may be incomplete.")
	}
	return warnings
}

// IsLoopbackBind reports whether bind address is loopback-only (safe for local dev).
func IsLoopbackBind(bind string) bool {
	switch {
	case bind == "":
		return true
	case strings.HasPrefix(bind, ":"):
		return true
	case strings.HasPrefix(bind, "127.0.0.1:"):
		return true
	case strings.HasPrefix(bind, "localhost:"):
		return true
	case strings.HasPrefix(bind, "[::1]:"):
		return true
	default:
		return false
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Gateway.Bind == "" {
		cfg.Gateway.Bind = ":8090"
	}
	if cfg.Gateway.Path == "" {
		cfg.Gateway.Path = "/gateway"
	}
	if cfg.Gateway.Log.RetentionDays <= 0 {
		cfg.Gateway.Log.RetentionDays = 30
	}
	if cfg.Gateway.Log.SnapshotInterval <= 0 {
		cfg.Gateway.Log.SnapshotInterval = 10000
	}
	if cfg.Nodes.PairingCodeTTL <= 0 {
		cfg.Nodes.PairingCodeTTL = time.Hour
	}
	return cfg, nil
}
