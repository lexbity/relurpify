package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Telemetry defaults (D6 of the OpenTelemetry telemetry cutover spec). A fresh
// workspace is fully functional offline: enabled with an empty endpoint means
// the local OTLP-JSON file leg is the only sink.
const (
	telemetryDefaultTimeout         = 10 * time.Second
	telemetryDefaultSamplingRatio   = 1.0
	telemetryDefaultMetricsInterval = 60 * time.Second
	telemetryDefaultFileRetention   = 168 * time.Hour
	telemetryDefaultFileMaxSizeMB   = 256
	telemetryDefaultOrphanTimeout   = 15 * time.Minute
)

// TelemetrySettings is the shared telemetry: section. Both boot schemas embed
// it (the v1 runtime workspace and the v0 workspace), so the two spellings
// cannot drift. Zero-value fields take their documented defaults in Resolved;
// the section itself may be absent.
//
// Durations are authored as strings (e.g. "10s", "168h") because yaml.v3 does
// not decode time.Duration; they are parsed and validated in Resolved, the
// same contract RunnerConfigV1 uses.
type TelemetrySettings struct {
	// Enabled is a pointer so an absent key decodes as the default-true
	// contract while an authored enabled: false is honored exactly.
	Enabled         *bool                   `yaml:"enabled,omitempty"`
	Endpoint        string                  `yaml:"endpoint,omitempty"`
	Headers         map[string]string       `yaml:"headers,omitempty"`
	Timeout         string                  `yaml:"timeout,omitempty"`
	TLS             TelemetryTLSConfig      `yaml:"tls,omitempty"`
	Sampling        TelemetrySamplingConfig `yaml:"sampling,omitempty"`
	MetricsInterval string                  `yaml:"metrics_interval,omitempty"`
	File            TelemetryFileConfig     `yaml:"file,omitempty"`
	OrphanTimeout   string                  `yaml:"orphan_timeout,omitempty"`
}

// TelemetryTLSConfig configures transport security for the OTLP endpoint.
type TelemetryTLSConfig struct {
	// Insecure allows an http:// endpoint on loopback without repeating the
	// scheme in the URL.
	Insecure bool `yaml:"insecure,omitempty"`
	// CAFile is an optional PEM bundle path for a collector with a private CA.
	CAFile string `yaml:"ca_file,omitempty"`
}

// TelemetrySamplingConfig configures span sampling. Log records and metrics
// are never sampled.
type TelemetrySamplingConfig struct {
	// Ratio is a pointer so an absent key decodes as the default-1.0 contract
	// while an authored 0 (drop all spans) is honored exactly.
	Ratio *float64 `yaml:"ratio,omitempty"`
}

// TelemetryFileConfig configures the always-available local OTLP-JSON leg.
type TelemetryFileConfig struct {
	Enabled   *bool  `yaml:"enabled,omitempty"`
	Retention string `yaml:"retention,omitempty"`
	MaxSizeMB int    `yaml:"max_size_mb,omitempty"`
}

// ResolvedTelemetrySettings is the fully-resolved telemetry configuration: every
// field carries a real value, never a zero sentinel.
type ResolvedTelemetrySettings struct {
	Enabled         bool
	Endpoint        string
	Headers         map[string]string
	Timeout         time.Duration
	TLSInsecure     bool
	CAFile          string
	SamplingRatio   float64
	MetricsInterval time.Duration
	FileEnabled     bool
	FileRetention   time.Duration
	FileMaxSizeMB   int
	OrphanTimeout   time.Duration
}

// Resolved applies the documented defaults and validates the result. Every
// error is actionable: it names the offending key so an operator can fix the
// config without reading source.
func (t TelemetrySettings) Resolved() (ResolvedTelemetrySettings, error) {
	out := ResolvedTelemetrySettings{
		Enabled:     t.Enabled == nil || *t.Enabled,
		Endpoint:    strings.TrimSpace(t.Endpoint),
		Headers:     copyTelemetryHeaders(t.Headers),
		TLSInsecure: t.TLS.Insecure,
		CAFile:      strings.TrimSpace(t.TLS.CAFile),
		FileEnabled: t.File.Enabled == nil || *t.File.Enabled,
	}
	if t.Sampling.Ratio != nil {
		out.SamplingRatio = *t.Sampling.Ratio
	} else {
		out.SamplingRatio = telemetryDefaultSamplingRatio
	}
	if t.File.MaxSizeMB < 0 {
		return ResolvedTelemetrySettings{}, fmt.Errorf("telemetry.file.max_size_mb: must be >= 0, got %d", t.File.MaxSizeMB)
	}
	if t.File.MaxSizeMB == 0 {
		out.FileMaxSizeMB = telemetryDefaultFileMaxSizeMB
	} else {
		out.FileMaxSizeMB = t.File.MaxSizeMB
	}

	var err error
	if out.Timeout, err = telemetryDuration("timeout", t.Timeout, telemetryDefaultTimeout); err != nil {
		return ResolvedTelemetrySettings{}, err
	}
	if out.MetricsInterval, err = telemetryDuration("metrics_interval", t.MetricsInterval, telemetryDefaultMetricsInterval); err != nil {
		return ResolvedTelemetrySettings{}, err
	}
	if out.FileRetention, err = telemetryDuration("file.retention", t.File.Retention, telemetryDefaultFileRetention); err != nil {
		return ResolvedTelemetrySettings{}, err
	}
	if out.OrphanTimeout, err = telemetryDuration("orphan_timeout", t.OrphanTimeout, telemetryDefaultOrphanTimeout); err != nil {
		return ResolvedTelemetrySettings{}, err
	}

	if err := out.validate(); err != nil {
		return ResolvedTelemetrySettings{}, err
	}
	return out, nil
}

// Validate resolves the section and reports any configuration error. It is the
// fail-closed gate D9 requires: a misconfigured endpoint or CA path aborts
// workspace open rather than degrading silently into file-only mode.
func (t TelemetrySettings) Validate() error {
	_, err := t.Resolved()
	return err
}

func (r ResolvedTelemetrySettings) validate() error {
	if r.SamplingRatio < 0 || r.SamplingRatio > 1 {
		return fmt.Errorf("telemetry.sampling.ratio: must be within [0,1], got %v", r.SamplingRatio)
	}
	if r.Endpoint != "" {
		if err := validateTelemetryEndpoint(r.Endpoint); err != nil {
			return err
		}
	}
	if r.CAFile != "" {
		info, err := os.Stat(filepath.Clean(r.CAFile))
		if err != nil {
			return fmt.Errorf("telemetry.tls.ca_file: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("telemetry.tls.ca_file: %s is a directory", r.CAFile)
		}
	}
	return nil
}

// validateTelemetryEndpoint accepts either a full OTLP/HTTP URL or a bare
// host:port. An empty endpoint is the file-only offline default and is valid.
func validateTelemetryEndpoint(endpoint string) error {
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return fmt.Errorf("telemetry.endpoint: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("telemetry.endpoint: scheme %q must be http or https", u.Scheme)
		}
		if u.Host == "" {
			return fmt.Errorf("telemetry.endpoint: %q has no host", endpoint)
		}
		return nil
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("telemetry.endpoint: %q is neither a URL nor host:port: %w", endpoint, err)
	}
	if host == "" || port == "" {
		return fmt.Errorf("telemetry.endpoint: %q must name a host and port", endpoint)
	}
	return nil
}

// telemetryDuration parses an authored duration string. An absent value takes
// the default; an authored non-positive duration is a configuration error (a
// zero interval would silently disable the reader or retention it configures).
func telemetryDuration(name, raw string, def time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("telemetry.%s: %w", name, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("telemetry.%s: must be > 0, got %s", name, raw)
	}
	return d, nil
}

func copyTelemetryHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
