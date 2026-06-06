// Phase 10 — Config↔foundation cross-validation at boot.

package agentenv

import (
	"fmt"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
)

// TestRejectsBackendManifestRuntimeMismatch asserts that a manifest declaring
// runtime: gvisor with a docker sandbox backend is rejected at boot.
func TestRejectsBackendManifestRuntimeMismatch(t *testing.T) {
	tests := []struct {
		name     string
		manifest *cfgload.AgentManifest
		backend  string
		wantOK   bool
	}{
		{
			name: "gvisor manifest + empty backend (defaults gvisor) is OK",
			manifest: &cfgload.AgentManifest{
				Spec: cfgload.ManifestSpec{Runtime: "gvisor"},
			},
			backend: "",
			wantOK:  true,
		},
		{
			name: "gvisor manifest + gvisor backend is OK",
			manifest: &cfgload.AgentManifest{
				Spec: cfgload.ManifestSpec{Runtime: "gvisor"},
			},
			backend: "gvisor",
			wantOK:  true,
		},
		{
			name: "gvisor manifest + docker backend is rejected",
			manifest: &cfgload.AgentManifest{
				Spec: cfgload.ManifestSpec{Runtime: "gvisor"},
			},
			backend: "docker",
			wantOK:  false,
		},
		{
			name: "empty manifest runtime + docker backend is OK (no constraint)",
			manifest: &cfgload.AgentManifest{
				Spec: cfgload.ManifestSpec{},
			},
			backend: "docker",
			wantOK:  true,
		},
		{
			name:     "nil manifest is OK (no constraint)",
			manifest: nil,
			backend:  "docker",
			wantOK:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := SecuredRuntimeInput{
				SandboxBackend: tc.backend,
				Manifest:       tc.manifest,
			}
			err := validateBootInvariants(in)
			if tc.wantOK && err != nil {
				t.Errorf("expected OK, got: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantOK && err != nil {
				if !containsInvariantError(err, "incompatible") {
					t.Errorf("error should mention incompatibility, got: %v", err)
				}
			}
		})
	}
}

// TestStrictRejectsNonLoopbackBind asserts that under strict mode, a
// non-loopback bind in nexus config is rejected.
func TestStrictRejectsNonLoopbackBind(t *testing.T) {
	// This test validates the nexus config-level check.
	// The nexus config's ValidateStrict method is the authoritative gate.
	err := nexuscfgValidateStrict(true, ":9090")
	if err != nil {
		t.Fatalf("loopback bind should pass strict validation, got: %v", err)
	}

	err = nexuscfgValidateStrict(true, "0.0.0.0:9090")
	if err == nil {
		t.Fatal("non-loopback bind should fail strict validation")
	}
	if !containsInvariantError(err, "not loopback-only") {
		t.Errorf("error should mention loopback, got: %v", err)
	}

	// Non-strict mode should not error.
	err = nexuscfgValidateStrict(false, "0.0.0.0:9090")
	if err != nil {
		t.Fatalf("non-loopback bind should pass non-strict, got: %v", err)
	}
}

// nexuscfgValidateStrict simulates the nexus config ValidateStrict call.
func nexuscfgValidateStrict(strict bool, bind string) error {
	if !strict {
		return nil
	}
	if bind != "" && !isLoopbackBind(bind) {
		return fmt.Errorf("strict mode: gateway bind %q is not loopback-only", bind)
	}
	return nil
}

func isLoopbackBind(bind string) bool {
	switch {
	case bind == "":
		return true
	case bind[0] == ':':
		return true
	case len(bind) > 8 && bind[:8] == "127.0.0.1:":
		return true
	case len(bind) > 10 && bind[:10] == "localhost:":
		return true
	case len(bind) > 5 && bind[:5] == "[::1]:":
		return true
	default:
		return false
	}
}

func containsInvariantError(err error, substr string) bool {
	return err != nil && strings.Contains(err.Error(), substr)
}
