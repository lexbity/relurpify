package authorization

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/platform/sandbox/dockersandbox"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
)

func TestSelectSandboxRuntime_DefaultsToGVisor(t *testing.T) {
	t.Parallel()

	t.Run("empty backend defaults to gVisor", func(t *testing.T) {
		rt, err := SelectSandboxRuntime("", sandbox.SandboxConfig{}, "", "")
		if err != nil {
			t.Fatalf("SelectSandboxRuntime('') failed: %v", err)
		}
		if _, ok := rt.(*sandbox.SandboxRuntimeImpl); !ok {
			t.Errorf("expected *sandbox.SandboxRuntimeImpl, got %T", rt)
		}
	})

	t.Run("gvisor backend", func(t *testing.T) {
		rt, err := SelectSandboxRuntime("gvisor", sandbox.SandboxConfig{}, "", "")
		if err != nil {
			t.Fatalf("SelectSandboxRuntime('gvisor') failed: %v", err)
		}
		if _, ok := rt.(*sandbox.SandboxRuntimeImpl); !ok {
			t.Errorf("expected *sandbox.SandboxRuntimeImpl, got %T", rt)
		}
	})
}

func TestSelectSandboxRuntime_Docker(t *testing.T) {
	t.Parallel()

	rt, err := SelectSandboxRuntime("docker", sandbox.SandboxConfig{}, "test-image:latest", "/tmp/ws")
	if err != nil {
		t.Fatalf("SelectSandboxRuntime('docker') failed: %v", err)
	}
	if _, ok := rt.(*dockersandbox.Backend); !ok {
		t.Errorf("expected *dockersandbox.Backend, got %T", rt)
	}
}

func TestSelectSandboxRuntime_RejectsUnsupported(t *testing.T) {
	t.Parallel()

	unsupported := []struct {
		name string
		want string
	}{
		{"local", "unsupported sandbox backend"},
		{"none", "unsupported sandbox backend"},
		{"bogus", "unsupported sandbox backend"},
	}
	for _, tc := range unsupported {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := SelectSandboxRuntime(tc.name, sandbox.SandboxConfig{}, "", "")
			if err == nil {
				t.Fatalf("SelectSandboxRuntime(%q) should fail, got runtime %T", tc.name, rt)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

// TestRegisterAgent_VerifyFailure proves that RegisterAgent fails closed when
// the sandbox runtime cannot be verified (runsc binary missing).
func TestRegisterAgent_VerifyFailure(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()

	minimalManifest := &config.AgentManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata:   config.ManifestMetadata{Name: "test-agent"},
		Spec: config.ManifestSpec{
			Image:   "test-image:latest",
			Runtime: "gvisor",
		},
	}

	manifestSnapshot := &config.AgentManifestSnapshot{
		Manifest: minimalManifest,
	}

	securityBundle := &cfgsecurity.Bundle{
		Sandbox: &sandbox.SandboxPolicy{},
	}

	cfg := RuntimeConfig{
		ManifestSnapshot: manifestSnapshot,
		SecurityBundle:   securityBundle,
		BaseFS:           ws,
		Sandbox: sandbox.SandboxConfig{
			RunscPath: "/nonexistent/runsc-binary",
		},
	}

	_, err := RegisterAgent(ctx, cfg)
	if err == nil {
		t.Fatal("RegisterAgent should fail when sandbox verification fails (runsc not found)")
	}
	if !strings.Contains(err.Error(), "sandbox verification failed") {
		t.Errorf("expected error about sandbox verification, got: %v", err)
	}
	if !strings.Contains(err.Error(), "runsc binary not found") {
		t.Errorf("expected error about runsc binary not found, got: %v", err)
	}
}
