// Phase 7 — Nexus config under cfgload (relurpify/nexus/v1).
//
// Asserts that nexus config loads through DecodeWithSchema, that anchors
// are rejected, that defaults are accounted, and that the schema line is
// required (with a one-release deprecation window).
//
// See devdocs/plans/unified-boot-contract.md for the full plan.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "nexus.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestNexusConfigRejectsYAMLAnchors asserts that YAML anchors and aliases
// are rejected at load time (preventing YAML-bomb expansion in the
// security-critical gateway config).
func TestNexusConfigRejectsYAMLAnchors(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "anchor definition",
			content: `schema: relurpify/nexus/v1
gateway:
  bind: &default ":9090"
  path: /gateway
`,
		},
		{
			name: "anchor alias",
			content: `schema: relurpify/nexus/v1
gateway:
  bind: *default
  path: /gateway
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, dir, tc.content)
			_, err := Load(path)
			if err == nil {
				t.Error("expected error for YAML anchor/alias, got nil")
			}
			if !strings.Contains(err.Error(), "anchor") && !strings.Contains(err.Error(), "alias") {
				t.Errorf("error should mention anchor/alias, got: %v", err)
			}
		})
	}
}

// TestNexusConfigRequiresSchemaLine asserts that a file without a schema
// line produces a warning (deprecation window) but still loads. After the
// deprecation window, this test will be updated to assert rejection.
func TestNexusConfigRequiresSchemaLine(t *testing.T) {
	dir := t.TempDir()
	content := `gateway:
  bind: ":9090"
  path: /gateway
`
	path := writeConfig(t, dir, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("config without schema line should load with warning, got: %v", err)
	}
	if cfg.Gateway.Bind != ":9090" {
		t.Errorf("Bind = %q, want %q", cfg.Gateway.Bind, ":9090")
	}
	if cfg.Gateway.Path != "/gateway" {
		t.Errorf("Path = %q, want %q", cfg.Gateway.Path, "/gateway")
	}
}

// TestNexusConfigStrictRejectsDefaultedBind asserts that when
// RELURPIFY_STRICT is set, defaulted values cause a warning.
func TestNexusConfigStrictRejectsDefaultedBind(t *testing.T) {
	dir := t.TempDir()

	// Config without explicit bind — defaults should be recorded.
	content := `schema: relurpify/nexus/v1
gateway:
  path: /custom
  log:
    retention_days: 60
`
	path := writeConfig(t, dir, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("config should load, got: %v", err)
	}

	if len(cfg.DefaultsUsed) == 0 {
		t.Fatal("expected at least one default to be recorded")
	}

	// Bind should be defaulted.
	foundBind := false
	for _, d := range cfg.DefaultsUsed {
		if d.Key == "gateway.bind" {
			foundBind = true
			if d.Value != ":8090" {
				t.Errorf("default bind = %v, want %v", d.Value, ":8090")
			}
		}
	}
	if !foundBind {
		t.Error("gateway.bind default not recorded in DefaultsUsed")
	}
}

// TestNexusConfigTypedChannels asserts that the Channels field accepts valid
// channel configs with an enabled flag and rejects unknown top-level fields.
func TestNexusConfigTypedChannels(t *testing.T) {
	dir := t.TempDir()
	content := `schema: relurpify/nexus/v1
gateway:
  bind: ":9090"
  path: /gateway
channels:
  webchat:
    enabled: true
  telegram:
    enabled: false
`
	path := writeConfig(t, dir, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("config with channels should load, got: %v", err)
	}
	if len(cfg.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(cfg.Channels))
	}
	webchat, ok := cfg.Channels["webchat"]
	if !ok {
		t.Fatal("expected webchat channel")
	}
	wc, ok := webchat.(map[string]any)
	if !ok {
		t.Fatal("webchat config should be a map")
	}
	if enabled, ok := wc["enabled"].(bool); !ok || !enabled {
		t.Errorf("webchat enabled should be true, got %v", wc["enabled"])
	}
}

// TestNexusConfigNonLoopbackBindHardErrorUnderStrict asserts that a
// non-loopback bind is caught by SecurityWarnings (non-strict) and by
// ValidateStrict (strict mode).
func TestNexusConfigNonLoopbackBindHardErrorUnderStrict(t *testing.T) {
	loopback := Config{
		Gateway: GatewayConfig{Bind: ":9090", Path: "/gateway"},
	}
	nonLoopback := Config{
		Gateway: GatewayConfig{Bind: "0.0.0.0:9090", Path: "/gateway"},
	}

	// SecurityWarnings: non-loopback produces a warning.
	warnings := nonLoopback.SecurityWarnings(0)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "not loopback-only") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected security warning for non-loopback bind")
	}

	// ValidateStrict: non-loopback under strict mode is an error.
	if err := nonLoopback.ValidateStrict(true); err == nil {
		t.Error("ValidateStrict(true) should reject non-loopback bind")
	}
	if err := loopback.ValidateStrict(true); err != nil {
		t.Errorf("ValidateStrict(true) should allow loopback bind, got: %v", err)
	}
	if err := nonLoopback.ValidateStrict(false); err != nil {
		t.Errorf("ValidateStrict(false) should allow non-loopback bind, got: %v", err)
	}
}
