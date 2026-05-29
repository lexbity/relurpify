package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"gopkg.in/yaml.v3"
)

// Config is the nexus gateway configuration, loaded via DecodeWithSchema
// with a nexus-local schema registry (relurpify/nexus/v1).
type Config struct {
	Gateway  GatewayConfig `yaml:"gateway"`
	Channels map[string]any `yaml:"channels,omitempty"`
	Nodes    NodesConfig   `yaml:"nodes,omitempty"`

	// DefaultsUsed records every default that was applied at load time.
	// Exposed so that RELURPIFY_STRICT can reject defaulted fields.
	DefaultsUsed []cfgload.WorkspaceDefaultUsage `yaml:"-"`
}

type GatewayConfig struct {
	Bind       string                  `yaml:"bind"`
	Path       string                  `yaml:"path"`
	Auth       GatewayAuthConfig       `yaml:"auth,omitempty"`
	Log        GatewayLogConfig        `yaml:"log,omitempty"`
	Federation GatewayFederationConfig `yaml:"federation,omitempty"`
}

type GatewayFederationConfig struct {
	Endpoints map[string]string `yaml:"endpoints,omitempty"`
}

type GatewayAuthConfig struct {
	Enabled bool               `yaml:"enabled,omitempty"`
	Tokens  []GatewayTokenAuth `yaml:"tokens,omitempty"`
}

type GatewayTokenAuth struct {
	TokenHash   string   `yaml:"token_hash"`
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

// nexusSchemaRegistry returns a registry that knows relurpify/nexus/v1.
func nexusSchemaRegistry() *cfgload.SchemaRegistry {
	reg := cfgload.NewSchemaRegistry()
	_ = reg.Register("nexus", 1)
	return reg
}

// Load reads, validates, and decodes a nexus config file.
//
// The file must begin with schema: relurpify/nexus/v1. During a one-release
// deprecation window, files without a schema declaration are accepted with
// a warning; after the window they will be rejected.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	reg := nexusSchemaRegistry()

	var cfg Config
	_, err = cfgload.DecodeWithSchema(path, data, reg, &cfg)
	if err != nil {
		if isMissingSchema(err) {
			// Deprecation window: accept missing schema line with warning.
			log.Printf("WARNING: nexus config %q is missing schema declaration; add 'schema: relurpify/nexus/v1' as the first line", path)
			if decodeErr := decodeLegacy(path, data, &cfg); decodeErr != nil {
				return Config{}, decodeErr
			}
		} else {
			return Config{}, err
		}
	}

	applyDefaults(&cfg)
	return cfg, nil
}

func isMissingSchema(err error) bool {
	return errors.Is(err, cfgload.ErrMissingSchemaDeclaration)
}

func decodeLegacy(path string, data []byte, cfg *Config) error {
	if err := cfgload.RejectForbiddenSecretFields(path, data); err != nil {
		return err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	return dec.Decode(cfg)
}

func applyDefaults(cfg *Config) {
	if cfg.Gateway.Bind == "" {
		cfg.Gateway.Bind = ":8090"
		cfg.DefaultsUsed = append(cfg.DefaultsUsed, cfgload.WorkspaceDefaultUsage{Key: "gateway.bind", Value: ":8090"})
	}
	if cfg.Gateway.Path == "" {
		cfg.Gateway.Path = "/gateway"
		cfg.DefaultsUsed = append(cfg.DefaultsUsed, cfgload.WorkspaceDefaultUsage{Key: "gateway.path", Value: "/gateway"})
	}
	if cfg.Gateway.Log.RetentionDays <= 0 {
		cfg.Gateway.Log.RetentionDays = 30
		cfg.DefaultsUsed = append(cfg.DefaultsUsed, cfgload.WorkspaceDefaultUsage{Key: "gateway.log.retention_days", Value: 30})
	}
	if cfg.Gateway.Log.SnapshotInterval <= 0 {
		cfg.Gateway.Log.SnapshotInterval = 10000
		cfg.DefaultsUsed = append(cfg.DefaultsUsed, cfgload.WorkspaceDefaultUsage{Key: "gateway.log.snapshot_interval_events", Value: 10000})
	}
	if cfg.Nodes.PairingCodeTTL <= 0 {
		cfg.Nodes.PairingCodeTTL = time.Hour
		cfg.DefaultsUsed = append(cfg.DefaultsUsed, cfgload.WorkspaceDefaultUsage{Key: "nodes.pairing_code_ttl", Value: "1h0m0s"})
	}
}

// ValidateStrict returns an error if the config violates strict-mode policies.
// Non-loopback gateway binds are rejected under strict mode (promoted from
// SecurityWarnings to a hard gate).
func (cfg Config) ValidateStrict(strict bool) error {
	if !strict {
		return nil
	}
	if bind := strings.TrimSpace(cfg.Gateway.Bind); bind != "" && !IsLoopbackBind(bind) {
		return fmt.Errorf("strict mode: gateway bind %q is not loopback-only", bind)
	}
	return nil
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
