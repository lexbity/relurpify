package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/stretchr/testify/require"
)

func TestSandboxCommandRunnerRunExecutesRuntimeBinary(t *testing.T) {
	dir := t.TempDir()
	fakeDocker := writeScript(t, dir, "docker", "#!/bin/sh\nprintf '%s' \"$*\" > \"$TMPDIR/docker-args\"\nprintf 'hello from sandbox'\n")
	fakeRunsc := writeScript(t, dir, "runsc", "#!/bin/sh\nprintf 'runsc version'\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMPDIR", dir)

	rt := &stubSandboxRuntime{config: SandboxConfig{RunscPath: fakeRunsc, ContainerRuntime: fakeDocker, NetworkIsolation: true}}
	man := &manifest.AgentManifest{
		Spec: manifest.ManifestSpec{
			Image: "example/runtime:latest",
			Security: manifest.SecuritySpec{
				RunAsUser:       1001,
				ReadOnlyRoot:    true,
				NoNewPrivileges: true,
			},
		},
	}
	runner, err := NewSandboxCommandRunner(man, rt, dir)
	require.NoError(t, err)
	rt.policy = SandboxPolicy{}
	stdout, stderr, err := runner.Run(context.Background(), CommandRequest{
		Workdir: "subdir",
		Args:    []string{"sh", "-lc", "printf hello"},
		Env:     []string{"HELLO=WORLD"},
		Input:   "ignored",
		Timeout: time.Second,
	})
	require.NoError(t, err)
	require.NotEmpty(t, stdout)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "hello from sandbox")

	argsData, err := os.ReadFile(filepath.Join(dir, "docker-args"))
	require.NoError(t, err)
	args := string(argsData)
	require.Contains(t, args, "--runtime")
	require.Contains(t, args, "runsc")
	require.Contains(t, args, "-w /workspace/subdir")
	require.Contains(t, args, "--read-only")
	require.Contains(t, args, "--security-opt no-new-privileges")
	require.Contains(t, args, "-e HELLO=WORLD")
	require.Contains(t, args, "example/runtime:latest")
}

func TestSandboxCommandRunnerContainerWorkdir(t *testing.T) {
	runner := &SandboxCommandRunner{workspace: filepath.Clean("/tmp/work"), workspaceSlash: "/tmp/work"}
	got, err := runner.containerWorkdir("")
	require.NoError(t, err)
	require.Equal(t, "/workspace", got)
	got, err = runner.containerWorkdir("child")
	require.NoError(t, err)
	require.Equal(t, "/workspace/child", got)
	_, err = runner.containerWorkdir("../escape")
	require.Error(t, err)
}

func TestSandboxCommandRunnerProtectedMounts(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "relurpify_cfg", "agent.manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(protected), 0o755))
	require.NoError(t, os.WriteFile(protected, []byte("manifest"), 0o644))

	rt := &stubSandboxRuntime{policy: SandboxPolicy{ProtectedPaths: []string{protected}}}
	runner := &SandboxCommandRunner{workspace: dir, rt: rt}
	mounts := runner.protectedMounts()
	require.Len(t, mounts, 1)
	require.Contains(t, mounts[0], ":/workspace/relurpify_cfg/agent.manifest.yaml:ro")
}

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
	return path
}
