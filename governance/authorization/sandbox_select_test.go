package authorization

import (
	"codeburg.org/lexbit/relurpify/governance/sandbox"
	"context"
	"errors"
	"testing"

	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

// fakeSandboxRuntime implements governanceports.SandboxRuntime for testing.
type fakeSandboxRuntime struct {
	name string
}

func (f *fakeSandboxRuntime) Verify(_ context.Context) error               { return nil }
func (f *fakeSandboxRuntime) ValidatePolicy(_ sandbox.SandboxPolicy) error { return nil }
func (f *fakeSandboxRuntime) ApplyPolicy(_ context.Context, _ sandbox.SandboxPolicy) error {
	return nil
}
func (f *fakeSandboxRuntime) Policy() sandbox.SandboxPolicy {
	return sandbox.SandboxPolicy{}
}
func (f *fakeSandboxRuntime) RunConfig() sandbox.SandboxConfig {
	return sandbox.SandboxConfig{}
}
func (f *fakeSandboxRuntime) Name() string { return f.name }

func TestSandboxBackendFactory_ReturnsRuntime(t *testing.T) {
	factory := SandboxBackendFactory(func(_ context.Context, backend string, _ sandbox.SandboxConfig, _, _ string) (governanceports.SandboxRuntime, error) {
		if backend == "test" {
			return &fakeSandboxRuntime{name: "test-backend"}, nil
		}
		return nil, errors.New("unknown backend")
	})

	rt, err := factory(context.Background(), "test", sandbox.SandboxConfig{}, "", "")
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}
	if rt.Name() != "test-backend" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "test-backend")
	}
}

func TestSandboxBackendFactory_RejectsUnknown(t *testing.T) {
	factory := SandboxBackendFactory(func(_ context.Context, backend string, _ sandbox.SandboxConfig, _, _ string) (governanceports.SandboxRuntime, error) {
		return nil, errors.New("unknown backend")
	})

	_, err := factory(context.Background(), "nonexistent", sandbox.SandboxConfig{}, "", "")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestSelectSandboxRuntime_WithFactory(t *testing.T) {
	factory := SandboxBackendFactory(func(_ context.Context, backend string, _ sandbox.SandboxConfig, _, _ string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{name: backend}, nil
	})

	rt, err := selectSandboxRuntime(context.Background(), "gvisor", sandbox.SandboxConfig{}, "", "", factory)
	if err != nil {
		t.Fatalf("selectSandboxRuntime failed: %v", err)
	}
	if rt.Name() != "gvisor" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "gvisor")
	}
}

func TestSelectSandboxRuntime_NoFactory(t *testing.T) {
	_, err := selectSandboxRuntime(context.Background(), "gvisor", sandbox.SandboxConfig{}, "", "", nil)
	if err == nil {
		t.Fatal("expected error when no factory is provided")
	}
}
