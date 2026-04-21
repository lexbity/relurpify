package dockersandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

func TestRunnerExecutesDockerRun(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(protected), 0o755); err != nil {
		t.Fatalf("mkdir protected path: %v", err)
	}
	if err := os.WriteFile(protected, []byte("manifest"), 0o644); err != nil {
		t.Fatalf("write protected path: %v", err)
	}

	docker := writeDockerScript(t, dir, "docker", "#!/bin/sh\nprintf '%s' \"$*\" > \"$TMPDIR/docker-args\"\ncase \"$1\" in\nversion) printf 'Docker version 25.0';;\nrun) printf 'hello from docker';;\n*) printf 'unexpected';;\nesac\n")
	t.Setenv("TMPDIR", dir)

	backend := NewBackend(Config{DockerPath: docker, Workspace: dir, Image: "example/runtime:latest"})
	policy := sandbox.SandboxPolicy{
		ReadOnlyRoot:    true,
		ProtectedPaths:  []string{protected},
		NoNewPrivileges: true,
	}
	if err := backend.ApplyPolicy(context.Background(), policy); err != nil {
		t.Fatalf("ApplyPolicy: %v", err)
	}

	runner, err := NewRunner(backend)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, stderr, err := runner.Run(ctx, sandbox.CommandRequest{
		Workdir: "src",
		Args:    []string{"sh", "-c", "echo hi"},
		Env:     []string{"HELLO=WORLD"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout != "hello from docker" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	argsData, err := os.ReadFile(filepath.Join(dir, "docker-args"))
	if err != nil {
		t.Fatalf("read docker args: %v", err)
	}
	args := string(argsData)
	for _, want := range []string{"run", "--read-only", "--security-opt no-new-privileges", "--network none", "-e HELLO=WORLD", "example/runtime:latest", "sh -c echo hi"} {
		if !strings.Contains(args, want) {
			t.Fatalf("docker args %q missing %q", args, want)
		}
	}
	if !strings.Contains(args, protected+":/workspace/relurpify_cfg/agent.manifest.yaml:ro") {
		t.Fatalf("expected protected path bind mount in docker args %q", args)
	}
}

func TestRunnerRejectsOutsideWorkspaceWorkdir(t *testing.T) {
	dir := t.TempDir()
	backend := NewBackend(Config{DockerPath: "docker", Workspace: dir})
	runner, err := NewRunner(backend)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if _, _, err := runner.Run(context.Background(), sandbox.CommandRequest{
		Workdir: "../escape",
		Args:    []string{"sh", "-c", "true"},
	}); err == nil {
		t.Fatal("expected outside workspace workdir to fail")
	}
}
