package otelleg

import (
	"os"
	"path/filepath"
	"testing"
)

// testTLSEndpoint is a syntactically valid https endpoint reused across the
// settings and chain validation tables.
const testTLSEndpoint = "https://collector.example:4318"

func TestSettingsResolveDefaults(t *testing.T) {
	got, err := Settings{Enabled: true, FileEnabled: true, FileDir: t.TempDir()}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Timeout != defaultTimeout {
		t.Errorf("Timeout = %s, want %s", got.Timeout, defaultTimeout)
	}
	if got.MetricsInterval != defaultMetricsInterval {
		t.Errorf("MetricsInterval = %s, want %s", got.MetricsInterval, defaultMetricsInterval)
	}
	if got.FileRetention != defaultFileRetention {
		t.Errorf("FileRetention = %s, want %s", got.FileRetention, defaultFileRetention)
	}
	if got.FileMaxSizeMB != defaultFileMaxSizeMB {
		t.Errorf("FileMaxSizeMB = %d, want %d", got.FileMaxSizeMB, defaultFileMaxSizeMB)
	}
	if got.OrphanTimeout != defaultOrphanTimeout {
		t.Errorf("OrphanTimeout = %s, want %s", got.OrphanTimeout, defaultOrphanTimeout)
	}
}

func TestSettingsValidate(t *testing.T) {
	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caFile, []byte("not a real pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		settings Settings
		wantErr  bool
	}{
		{name: "disabled", settings: Settings{}},
		{name: "offline file only", settings: Settings{Enabled: true, FileEnabled: true, FileDir: dir}},
		{name: "url endpoint", settings: Settings{Enabled: true, Endpoint: testTLSEndpoint}},
		{name: "bare host port", settings: Settings{Enabled: true, Endpoint: "localhost:4318", TLSInsecure: true}},
		{name: "bad url", settings: Settings{Enabled: true, Endpoint: "http://[::1"}, wantErr: true},
		{name: "bad scheme", settings: Settings{Enabled: true, Endpoint: "ftp://collector"}, wantErr: true},
		{name: "bare missing port", settings: Settings{Enabled: true, Endpoint: "localhost"}, wantErr: true},
		{name: "missing ca", settings: Settings{Enabled: true, Endpoint: testTLSEndpoint, CAFile: filepath.Join(dir, "nope.pem")}, wantErr: true},
		{name: "ca is directory", settings: Settings{Enabled: true, Endpoint: testTLSEndpoint, CAFile: dir}, wantErr: true},
		{name: "ratio too high", settings: Settings{Enabled: true, SamplingRatio: 1.5}, wantErr: true},
		{name: "ratio negative", settings: Settings{Enabled: true, SamplingRatio: -0.1}, wantErr: true},
		{name: "file enabled without dir", settings: Settings{Enabled: true, FileEnabled: true}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.settings.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSettingsSamplingDefaultsToOne(t *testing.T) {
	got, err := Settings{Enabled: true}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.SamplingRatio != defaultSamplingRatio {
		t.Fatalf("SamplingRatio = %v, want %v", got.SamplingRatio, defaultSamplingRatio)
	}
}

func TestSettingsHeadersAreCopied(t *testing.T) {
	in := map[string]string{"authorization": "Bearer x"}
	got, err := Settings{Enabled: true, Headers: in}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	in["authorization"] = "mutated"
	if got.Headers["authorization"] != "Bearer x" {
		t.Fatalf("headers were not copied: %v", got.Headers)
	}
}
