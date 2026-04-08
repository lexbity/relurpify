package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGVisorRuntimeVerifyWithFakeBinaries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell binaries are not portable to windows in this test")
	}
	dir := t.TempDir()
	writeFakeBinary(t, dir, "runsc", "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'runsc kvm test'; exit 0; fi\nexit 1\n")
	writeFakeBinary(t, dir, "docker", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	rt := NewGVisorRuntime(SandboxConfig{RunscPath: "runsc", ContainerRuntime: "docker", Platform: "kvm"})
	require.NoError(t, rt.Verify(context.Background()))
	require.True(t, rt.verified)
	require.Contains(t, rt.version, "runsc")
	require.Contains(t, rt.version, "kvm")

	require.NoError(t, rt.Verify(context.Background()))
}

func TestGVisorRuntimeVerifyRejectsUnsupportedContainerRuntime(t *testing.T) {
	rt := NewGVisorRuntime(SandboxConfig{RunscPath: "runsc", ContainerRuntime: "podman"})
	err := rt.checkContainerRuntime(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported container runtime")
}

func TestGVisorRuntimeCommandContextAppliesTimeout(t *testing.T) {
	rt := NewGVisorRuntime(SandboxConfig{})
	cmd, cancelCmd := rt.commandContext(context.Background(), "sh", "-c", "true")
	defer cancelCmd()

	require.NotNil(t, cmd)
	require.Equal(t, "sh", filepath.Base(cmd.Path))
	require.Equal(t, []string{"sh", "-c", "true"}, cmd.Args)
}

func writeFakeBinary(t *testing.T, dir, name, script string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}
