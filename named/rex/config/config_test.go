package config

import "testing"

func TestDefaultUsesNexusManagedHosting(t *testing.T) {
	cfg := Default()
	if cfg.RuntimeMode != RuntimeModeNexusManaged {
		t.Fatalf("RuntimeMode = %q", cfg.RuntimeMode)
	}
	if cfg.QueueCapacity <= 0 {
		t.Fatalf("QueueCapacity = %d", cfg.QueueCapacity)
	}
	if cfg.WorkerCount != 4 {
		t.Fatalf("WorkerCount = %d", cfg.WorkerCount)
	}
	if !cfg.RequireProof {
		t.Fatalf("RequireProof = false")
	}
}
