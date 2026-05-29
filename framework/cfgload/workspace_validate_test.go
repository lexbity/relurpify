// Phase 1 — Backend vocabulary: single source of truth.
//
// Asserts that cfgload validation defers to sandbox.IsSupportedSandboxBackend
// for the sandbox-backend vocabulary, rejects "local" at load time, and stays
// in sync with authorization.SelectSandboxRuntime.
//
// See devdocs/plans/unified-boot-contract.md for the full plan.

package cfgload

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

// TestSandboxBackendLocalRejected asserts that sandbox.backend: "local" is
// rejected at config-validation time with a clear message naming the supported
// backends. Before Phase 1, "local" was silently accepted here and failed
// later at runtime in SelectSandboxRuntime — a fail-late condition.
func TestSandboxBackendLocalRejected(t *testing.T) {
	tests := []struct {
		name    string
		backend *string
		wantOK  bool
	}{
		{name: "empty defaults to gvisor", backend: nil, wantOK: true},
		{name: "gvisor accepted", backend: strPtr("gvisor"), wantOK: true},
		{name: "docker accepted", backend: strPtr("docker"), wantOK: true},
		{name: "local rejected", backend: strPtr("local"), wantOK: false},
		{name: "none rejected", backend: strPtr("none"), wantOK: false},
		{name: "bogus rejected", backend: strPtr("bogus"), wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &WorkspaceConfig{
				WorkspaceAbs: "/tmp/test-workspace",
				Paths: WorkspacePaths{
					StateDir: strPtr(".relurpify_state"),
				},
				Sandbox: WorkspaceSandbox{
					Backend: tc.backend,
				},
				Logging: WorkspaceLogging{
					Level:  strPtr("info"),
					Format: strPtr("json"),
				},
				Audit: WorkspaceAudit{
					RetentionDays: intPtr(30),
				},
			}
			err := cfg.Validate()
			if tc.wantOK && err != nil {
				t.Errorf("expected OK, got error: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("expected error for backend %v, got nil", strVal(tc.backend))
			}
			if !tc.wantOK && err != nil {
				if !containsSupportedBackend(err.Error()) {
					t.Errorf("error %q should mention supported backends", err.Error())
				}
			}
		})
	}
}

// TestSandboxBackendVocabularyMatchesRuntime asserts that the set of backends
// accepted by cfgload.Validate exactly matches sandbox.SupportedSandboxBackends.
// This is the drift guard: if a new backend is added to sandbox, both
// SelectSandboxRuntime and cfgload validation must accept it.
func TestSandboxBackendVocabularyMatchesRuntime(t *testing.T) {
	supported := sandbox.SupportedSandboxBackends()

	// Every supported backend must pass validation.
	for _, backend := range supported {
		t.Run("backend_"+backend, func(t *testing.T) {
			cfg := validConfig()
			cfg.Sandbox.Backend = &backend
			if err := cfg.Validate(); err != nil {
				t.Errorf("supported backend %q should pass validation, got: %v", backend, err)
			}
		})
	}

	// Empty string is also accepted (defaults to gvisor).
	t.Run("empty", func(t *testing.T) {
		cfg := validConfig()
		cfg.Sandbox.Backend = nil
		if err := cfg.Validate(); err != nil {
			t.Errorf("empty backend (defaults to gvisor) should pass validation, got: %v", err)
		}
	})

	// Any backend NOT in SupportedSandboxBackends must fail (except empty).
	unsupported := []string{"local", "none", "bogus", "something"}
	for _, bad := range unsupported {
		t.Run("rejected_"+bad, func(t *testing.T) {
			cfg := validConfig()
			cfg.Sandbox.Backend = &bad
			if err := cfg.Validate(); err == nil {
				t.Errorf("unsupported backend %q should be rejected, got nil", bad)
			}
		})
	}
}

func validConfig() *WorkspaceConfig {
	return &WorkspaceConfig{
		WorkspaceAbs: "/tmp/test-workspace",
		Paths: WorkspacePaths{
			StateDir: strPtr(".relurpify_state"),
		},
		Logging: WorkspaceLogging{
			Level:  strPtr("info"),
			Format: strPtr("json"),
		},
		Audit: WorkspaceAudit{
			RetentionDays: intPtr(30),
		},
	}
}

func containsSupportedBackend(msg string) bool {
	for _, b := range sandbox.SupportedSandboxBackends() {
		if strings.Contains(msg, b) {
			return true
		}
	}
	return false
}

func strVal(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
