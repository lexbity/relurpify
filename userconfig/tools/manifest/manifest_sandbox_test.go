package manifest

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestToolManifestSandboxLimitsDecode verifies the manifest-level sandbox
// limits (memory/pids/cpus/output_ceiling/grace_period) survive YAML strict
// decode — the declarative input for FR-16 plumbing.
func TestToolManifestSandboxLimitsDecode(t *testing.T) {
	decoded := `
execution:
  backend: subprocess
  command:
    base: [worker]
  sandbox:
    memory_mb: 1024
    pids_limit: 128
    cpus: 2.5
    output_ceiling: 1048576
    grace_period: 2s
`
	var m ToolManifest
	if err := yaml.Unmarshal([]byte(decoded), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	exec := m.Execution
	if exec.Sandbox == nil {
		t.Fatal("sandbox nil")
	}
	if exec.Sandbox.MemoryMB != 1024 {
		t.Errorf("memory_mb = %d, want 1024", exec.Sandbox.MemoryMB)
	}
	if exec.Sandbox.PidsLimit != 128 {
		t.Errorf("pids_limit = %d, want 128", exec.Sandbox.PidsLimit)
	}
	if exec.Sandbox.CPUs != 2.5 {
		t.Errorf("cpus = %v, want 2.5", exec.Sandbox.CPUs)
	}
	if exec.Sandbox.OutputCeiling != 1048576 {
		t.Errorf("output_ceiling = %d, want 1048576", exec.Sandbox.OutputCeiling)
	}
	if exec.Sandbox.GracePeriod != 2*time.Second {
		t.Errorf("grace_period = %v, want 2s", exec.Sandbox.GracePeriod)
	}
}

// TestToolManifestSandboxZeroValuesOmit verifies the new fields are additive
// and decode absent values to zero (no behavioral change for existing tools).
func TestToolManifestSandboxZeroValuesOmit(t *testing.T) {
	decoded := `
execution:
  backend: subprocess
  command:
    base: [worker]
`
	var m ToolManifest
	if err := yaml.Unmarshal([]byte(decoded), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Execution.Sandbox != nil {
		t.Fatalf("sandbox should be nil when absent, got %+v", m.Execution.Sandbox)
	}
}
