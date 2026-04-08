package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAppliesDefaultsAndPreservesExplicitValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nexus.yaml")
	contents := []byte(strings.TrimSpace(`
gateway:
  bind: 127.0.0.1:9123
nodes:
  auto_approve_local: true
channels:
  control:
    transport: stdio
`))
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Gateway.Bind; got != "127.0.0.1:9123" {
		t.Fatalf("Gateway.Bind = %q", got)
	}
	if got := cfg.Gateway.Path; got != "/gateway" {
		t.Fatalf("Gateway.Path = %q", got)
	}
	if got := cfg.Gateway.Log.RetentionDays; got != 30 {
		t.Fatalf("Gateway.Log.RetentionDays = %d", got)
	}
	if got := cfg.Gateway.Log.SnapshotInterval; got != 10000 {
		t.Fatalf("Gateway.Log.SnapshotInterval = %d", got)
	}
	if got := cfg.Nodes.PairingCodeTTL; got != time.Hour {
		t.Fatalf("Nodes.PairingCodeTTL = %s", got)
	}
	if _, ok := cfg.Channels["control"]; !ok {
		t.Fatalf("expected control channel to be preserved, got %#v", cfg.Channels)
	}
}

func TestSecurityWarningsReflectBindAndApprovalState(t *testing.T) {
	cfg := Config{
		Gateway: GatewayConfig{Bind: "0.0.0.0:8090"},
		Nodes:   NodesConfig{AutoApproveLocal: true},
	}

	warnings := cfg.SecurityWarnings(2)
	want := []string{
		`Gateway bind "0.0.0.0:8090" is not loopback-only.`,
		"Local node auto-approval is enabled.",
		"2 node pairing request(s) are pending approval.",
		"No channels are configured; gateway surface may be incomplete.",
	}
	if len(warnings) != len(want) {
		t.Fatalf("warnings len = %d, want %d: %#v", len(warnings), len(want), warnings)
	}
	for i, msg := range want {
		if warnings[i] != msg {
			t.Fatalf("warnings[%d] = %q, want %q", i, warnings[i], msg)
		}
	}
}

func TestIsLoopbackBind(t *testing.T) {
	tests := []struct {
		name string
		bind string
		want bool
	}{
		{name: "empty", bind: "", want: true},
		{name: "port only", bind: ":8090", want: true},
		{name: "ipv4 loopback", bind: "127.0.0.1:8090", want: true},
		{name: "localhost", bind: "localhost:8090", want: true},
		{name: "ipv6 loopback", bind: "[::1]:8090", want: true},
		{name: "external", bind: "0.0.0.0:8090", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsLoopbackBind(tc.bind); got != tc.want {
				t.Fatalf("IsLoopbackBind(%q) = %v, want %v", tc.bind, got, tc.want)
			}
		})
	}
}
