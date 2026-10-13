package otelleg

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Documented defaults (spec D6/§6.2). A zero-value field that cannot be told
// apart from an explicit choice is resolved here; New always calls resolve so
// callers never depend on hidden zero-value semantics.
const (
	defaultTimeout         = 10 * time.Second
	defaultSamplingRatio   = 1.0
	defaultMetricsInterval = 60 * time.Second
	defaultFileRetention   = 168 * time.Hour
	defaultFileMaxSizeMB   = 256
	defaultOrphanTimeout   = 15 * time.Minute
)

// Settings is the resolved configuration for the OTel export leg. It mirrors the
// userconfig telemetry section field-for-field; wiring sites convert the
// userconfig value into this struct.
//
// Enabled and FileEnabled are authoritative booleans: a Go zero value cannot
// distinguish "unset" from "explicitly disabled", so the default-true posture
// lives in userconfig's TelemetrySettings.Resolved (the actual config entry
// point). A zero Settings is therefore a disabled leg — the safe default for
// telemetry. resolve fills the numeric and duration defaults.
type Settings struct {
	Enabled         bool
	Endpoint        string
	Headers         map[string]string
	Timeout         time.Duration
	TLSInsecure     bool
	CAFile          string
	SamplingRatio   float64
	MetricsInterval time.Duration
	FileEnabled     bool
	// FileDir is the directory the OTLP-JSON segments are written to. It is
	// required when FileEnabled is set; wiring derives it from the workspace
	// state directory (<state>/telemetry/otlp).
	FileDir       string
	FileRetention time.Duration
	FileMaxSizeMB int
	OrphanTimeout time.Duration
}

// resolve returns a copy with every numeric and duration default applied, then
// validates the result. Booleans are left exactly as the caller set them.
func (s Settings) resolve() (Settings, error) {
	out := s
	out.Endpoint = strings.TrimSpace(out.Endpoint)
	out.CAFile = strings.TrimSpace(out.CAFile)
	out.FileDir = strings.TrimSpace(out.FileDir)
	out.Headers = copyHeaders(out.Headers)

	if out.Timeout <= 0 {
		out.Timeout = defaultTimeout
	}
	if out.MetricsInterval <= 0 {
		out.MetricsInterval = defaultMetricsInterval
	}
	// Per spec §6.2 a zero sampling ratio resolves to the keep-everything
	// default. A negative ratio is a configuration error, not a sentinel.
	if out.SamplingRatio < 0 {
		return Settings{}, fmt.Errorf("telemetry sampling ratio: must be >= 0, got %v", out.SamplingRatio)
	}
	if out.SamplingRatio == 0 {
		out.SamplingRatio = defaultSamplingRatio
	}
	if out.FileRetention <= 0 {
		out.FileRetention = defaultFileRetention
	}
	if out.FileMaxSizeMB <= 0 {
		out.FileMaxSizeMB = defaultFileMaxSizeMB
	}
	if out.OrphanTimeout <= 0 {
		out.OrphanTimeout = defaultOrphanTimeout
	}

	if err := out.validate(); err != nil {
		return Settings{}, err
	}
	return out, nil
}

// Validate reports a configuration error for the settings. It is the fail-closed
// gate D9 requires: a bad endpoint, a missing CA file, an out-of-range sampling
// ratio, or a file leg without a directory aborts boot instead of degrading
// silently.
func (s Settings) Validate() error {
	_, err := s.resolve()
	return err
}

func (s Settings) validate() error {
	if s.SamplingRatio < 0 || s.SamplingRatio > 1 {
		return fmt.Errorf("telemetry sampling ratio: must be within [0,1], got %v", s.SamplingRatio)
	}
	if s.Endpoint != "" {
		if err := validateEndpoint(s.Endpoint); err != nil {
			return err
		}
	}
	if s.CAFile != "" {
		info, err := os.Stat(filepath.Clean(s.CAFile))
		if err != nil {
			return fmt.Errorf("telemetry CA file: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("telemetry CA file: %s is a directory", s.CAFile)
		}
	}
	if s.FileEnabled && s.FileDir == "" {
		return fmt.Errorf("telemetry file leg: a directory is required when the local leg is enabled")
	}
	return nil
}

// validateEndpoint accepts a full OTLP/HTTP URL or a bare host:port.
func validateEndpoint(endpoint string) error {
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return fmt.Errorf("telemetry endpoint: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("telemetry endpoint: scheme %q must be http or https", u.Scheme)
		}
		if u.Host == "" {
			return fmt.Errorf("telemetry endpoint: %q has no host", endpoint)
		}
		return nil
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("telemetry endpoint: %q is neither a URL nor host:port: %w", endpoint, err)
	}
	if host == "" || port == "" {
		return fmt.Errorf("telemetry endpoint: %q must name a host and port", endpoint)
	}
	return nil
}

func copyHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
