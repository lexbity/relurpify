package sandbox

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

func TestDockerSandboxBackendDefaults(t *testing.T) {
	d := NewDockerSandboxBackend(SandboxConfig{})
	if d.Name() != "docker" {
		t.Fatalf("Name() = %q, want docker", d.Name())
	}
	cfg := d.RunConfig()
	if cfg.ContainerRuntime != "docker" {
		t.Fatalf("ContainerRuntime = %q, want docker", cfg.ContainerRuntime)
	}
	if !cfg.NetworkIsolation {
		t.Fatal("NetworkIsolation should default to true")
	}
	caps := d.Capabilities()
	if !caps.NetworkIsolation || !caps.ReadOnlyRoot || !caps.Seccomp || !caps.NoNewPrivileges || !caps.ProtectedPaths {
		t.Fatalf("docker capabilities missing required features: %+v", caps)
	}
}

func TestDockerSandboxBackendPolicyRoundTrip(t *testing.T) {
	d := NewDockerSandboxBackend(SandboxConfig{})
	policy := SandboxPolicy{ReadOnlyRoot: true, NoNewPrivileges: true, ProtectedPaths: []string{"/workspace/secret"}}
	if err := d.ApplyPolicy(context.Background(), policy); err != nil {
		t.Fatalf("ApplyPolicy: %v", err)
	}
	got := d.Policy()
	if !got.ReadOnlyRoot || !got.NoNewPrivileges || len(got.ProtectedPaths) != 1 {
		t.Fatalf("policy roundtrip mismatch: %+v", got)
	}
}

func TestDockerSandboxBackendRejectsEnvFiltering(t *testing.T) {
	d := NewDockerSandboxBackend(SandboxConfig{})
	err := d.ValidatePolicy(SandboxPolicy{AllowedEnvKeys: []string{"PATH"}})
	if err == nil || !strings.Contains(err.Error(), "environment filtering") {
		t.Fatalf("expected env-filtering rejection, got %v", err)
	}
}

func TestDockerSandboxBackendRejectsPrivateNetworkRule(t *testing.T) {
	d := NewDockerSandboxBackend(SandboxConfig{})
	err := d.ValidatePolicy(SandboxPolicy{NetworkRules: []NetworkRule{{Direction: "egress", Protocol: "tcp", Host: "127.0.0.1", Port: 80}}})
	if err == nil || !strings.Contains(err.Error(), "blocked host") {
		t.Fatalf("expected private-host rejection, got %v", err)
	}
}

func TestNewSandboxRuntimeForBackendDocker(t *testing.T) {
	rt, err := NewSandboxRuntimeForBackend("docker", SandboxConfig{}, "", t.TempDir())
	if err != nil {
		t.Fatalf("NewSandboxRuntimeForBackend(docker): %v", err)
	}
	if _, ok := rt.(*DockerSandboxBackend); !ok {
		t.Fatalf("runtime type = %T, want *DockerSandboxBackend", rt)
	}
}

func TestNewSandboxRuntimeForBackendGvisor(t *testing.T) {
	rt, err := NewSandboxRuntimeForBackend("gvisor", SandboxConfig{}, "", t.TempDir())
	if err != nil {
		t.Fatalf("NewSandboxRuntimeForBackend(gvisor): %v", err)
	}
	if _, ok := rt.(*SandboxRuntimeImpl); !ok {
		t.Fatalf("runtime type = %T, want *SandboxRuntimeImpl", rt)
	}
}

func TestDockerSandboxBackendCommandRunnerArgs(t *testing.T) {
	d := NewDockerSandboxBackend(SandboxConfig{NetworkIsolation: true, SeccompProfile: "/etc/seccomp.json"})
	runner, err := d.NewCommandRunner(&CommandRunnerConfig{
		Workspace:    t.TempDir(),
		Image:        "example/runtime:1",
		ReadOnlyRoot: true,
	})
	if err != nil {
		t.Fatalf("NewCommandRunner: %v", err)
	}
	sr, ok := runner.(*SandboxCommandRunner)
	if !ok {
		t.Fatalf("runner type = %T, want *SandboxCommandRunner", runner)
	}

	args := sr.runArgs("container", containerLabels("ws"), "/workspace", CommandRequest{Args: []string{"echo", "hi"}})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--cap-drop ALL",
		"--network none",
		"--read-only",
		"--tmpfs /tmp",
		"--security-opt seccomp=/etc/seccomp.json",
		"example/runtime:1",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("docker args missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "--runtime") {
		t.Errorf("docker args must not set --runtime: %s", joined)
	}
}

func TestGVisorCommandRunnerArgsUseRunsc(t *testing.T) {
	rt := NewSandboxRuntime(SandboxConfig{NetworkIsolation: true})
	runner, err := NewSandboxCommandRunner(&CommandRunnerConfig{Workspace: t.TempDir()}, rt)
	if err != nil {
		t.Fatalf("NewSandboxCommandRunner: %v", err)
	}
	args := runner.runArgs("container", containerLabels("ws"), "/workspace", CommandRequest{Args: []string{"echo"}})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--runtime runsc") {
		t.Errorf("gvisor args must set --runtime runsc: %s", joined)
	}
	if strings.Contains(joined, "--cap-drop") {
		t.Errorf("gvisor args must not add docker-only --cap-drop: %s", joined)
	}
}

func TestMarkOOMClassifiesSIGKILL(t *testing.T) {
	oom := &ports.CommandResult{ExitCode: 137}
	markOOM(oom)
	if !oom.OOMKilled || !oom.Signaled {
		t.Fatalf("exit 137 should be classified as OOM: %+v", oom)
	}

	torn := &ports.CommandResult{ExitCode: 137, TornDown: true}
	markOOM(torn)
	if torn.OOMKilled {
		t.Fatal("runner-issued teardown must not be classified as OOM")
	}

	normal := &ports.CommandResult{ExitCode: 1}
	markOOM(normal)
	if normal.OOMKilled {
		t.Fatal("exit 1 must not be classified as OOM")
	}
}
