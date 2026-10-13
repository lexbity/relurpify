package config

import (
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/userconfig/securefile"
	"github.com/stretchr/testify/require"
)

func TestTelemetrySettingsResolvedDefaults(t *testing.T) {
	got, err := TelemetrySettings{}.Resolved()
	require.NoError(t, err)
	require.True(t, got.Enabled)
	require.Empty(t, got.Endpoint)
	require.Equal(t, 10*time.Second, got.Timeout)
	require.InDelta(t, 1.0, got.SamplingRatio, 0.0001)
	require.Equal(t, 60*time.Second, got.MetricsInterval)
	require.True(t, got.FileEnabled)
	require.Equal(t, 168*time.Hour, got.FileRetention)
	require.Equal(t, 256, got.FileMaxSizeMB)
	require.Equal(t, 15*time.Minute, got.OrphanTimeout)
}

func TestTelemetrySettingsResolvedAuthored(t *testing.T) {
	ratio := 0.25
	enabled := true
	fileEnabled := false
	got, err := TelemetrySettings{
		Enabled:         &enabled,
		Endpoint:        " https://collector.example:4318 ",
		Headers:         map[string]string{"authorization": "Bearer x"},
		Timeout:         "5s",
		TLS:             TelemetryTLSConfig{Insecure: true, CAFile: "  "},
		Sampling:        TelemetrySamplingConfig{Ratio: &ratio},
		MetricsInterval: "30s",
		File:            TelemetryFileConfig{Enabled: &fileEnabled, Retention: "24h", MaxSizeMB: 32},
		OrphanTimeout:   "20m",
	}.Resolved()
	require.NoError(t, err)
	require.True(t, got.Enabled)
	require.Equal(t, "https://collector.example:4318", got.Endpoint)
	require.True(t, got.TLSInsecure)
	require.Empty(t, got.CAFile)
	require.Equal(t, 5*time.Second, got.Timeout)
	require.InDelta(t, 0.25, got.SamplingRatio, 0.0001)
	require.Equal(t, 30*time.Second, got.MetricsInterval)
	require.False(t, got.FileEnabled)
	require.Equal(t, 24*time.Hour, got.FileRetention)
	require.Equal(t, 32, got.FileMaxSizeMB)
	require.Equal(t, 20*time.Minute, got.OrphanTimeout)
}

func TestTelemetrySettingsValidationErrors(t *testing.T) {
	caDir := t.TempDir()
	tests := []struct {
		name string
		in   TelemetrySettings
		want string
	}{
		{name: "bad endpoint", in: TelemetrySettings{Endpoint: "ftp://x"}, want: "telemetry.endpoint"},
		{name: "missing ca", in: TelemetrySettings{Endpoint: "https://x:4318", TLS: TelemetryTLSConfig{CAFile: filepath.Join(caDir, "nope.pem")}}, want: "telemetry.tls.ca_file"},
		{name: "ca directory", in: TelemetrySettings{Endpoint: "https://x:4318", TLS: TelemetryTLSConfig{CAFile: caDir}}, want: "directory"},
		{name: "ratio high", in: TelemetrySettings{Sampling: TelemetrySamplingConfig{Ratio: floatPtr(1.2)}}, want: "telemetry.sampling.ratio"},
		{name: "ratio negative", in: TelemetrySettings{Sampling: TelemetrySamplingConfig{Ratio: floatPtr(-0.1)}}, want: "telemetry.sampling.ratio"},
		{name: "zero timeout", in: TelemetrySettings{Timeout: "0s"}, want: "telemetry.timeout"},
		{name: "negative retention", in: TelemetrySettings{File: TelemetryFileConfig{Retention: "-1h"}}, want: "telemetry.file.retention"},
		{name: "negative max size", in: TelemetrySettings{File: TelemetryFileConfig{MaxSizeMB: -1}}, want: "telemetry.file.max_size_mb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestWorkspaceConfigDecodesTelemetrySection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify_cfg", "workspace.yaml")
	require.NoError(t, securefile.MkdirAllSecure(filepath.Dir(path)))
	require.NoError(t, securefile.WriteFileSecure(path, []byte(`schema: relurpify/workspace/v1
model:
  provider: ollama
  name: gemma4:e4b
sandbox:
  backend: gvisor
telemetry:
  enabled: true
  endpoint: "http://localhost:4318"
  timeout: 5s
  sampling:
    ratio: 0.5
  file:
    retention: 24h
    max_size_mb: 32
`)))

	cfg, err := LoadWorkspaceConfig(path, dir, WorkspaceLoadOptions{})
	require.NoError(t, err)
	resolved, err := cfg.Telemetry.Resolved()
	require.NoError(t, err)
	require.True(t, resolved.Enabled)
	require.Equal(t, "http://localhost:4318", resolved.Endpoint)
	require.Equal(t, 5*time.Second, resolved.Timeout)
	require.InDelta(t, 0.5, resolved.SamplingRatio, 0.0001)
	require.Equal(t, 24*time.Hour, resolved.FileRetention)
	require.Equal(t, 32, resolved.FileMaxSizeMB)
	require.True(t, resolved.FileEnabled)
}

func TestRuntimeWorkspaceConfigV1DecodesTelemetrySection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.yaml")
	require.NoError(t, securefile.WriteFileSecure(path, []byte(`schema: relurpify/workspace/v1
model:
  provider: ollama
  name: gemma4:e4b
sandbox:
  backend: gvisor
telemetry:
  enabled: true
  endpoint: "https://collector.example:4318"
  metrics_interval: 15s
`)))

	cfg, err := LoadRuntimeWorkspaceConfigV1(path)
	require.NoError(t, err)
	resolved, err := cfg.Telemetry.Resolved()
	require.NoError(t, err)
	require.True(t, resolved.Enabled)
	require.Equal(t, "https://collector.example:4318", resolved.Endpoint)
	require.Equal(t, 15*time.Second, resolved.MetricsInterval)
}

func TestRuntimeWorkspaceConfigV1RejectsUnknownTelemetryKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.yaml")
	require.NoError(t, securefile.WriteFileSecure(path, []byte(`schema: relurpify/workspace/v1
model:
  provider: ollama
  name: gemma4:e4b
sandbox:
  backend: gvisor
telemetry:
  nope: true
`)))
	_, err := LoadRuntimeWorkspaceConfigV1(path)
	require.Error(t, err)
}

func floatPtr(v float64) *float64 { return &v }
