package conformance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
)

// spillManifest declares a subprocess tool whose output ceiling is 1 MiB and
// whose command produces effectively unbounded output.
func spillManifest() *ports.ToolManifest {
	return &ports.ToolManifest{
		Name:        "noisy_worker",
		Family:      "test",
		Description: "ceiling enforcement conformance",
		Parameters:  []ports.ToolParameter{},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{"noisy_worker"}},
			Sandbox: &ports.ToolManifestSandbox{OutputCeiling: 1 << 20},
		},
	}
}

// runSpillMatrixRow drives ceiling enforcement through the composition path:
// a manifest-limited tool on a REAL sandbox runner (fake docker), asserting
// the tool envelope carries truncated=true with a populated stdout_ref
// (SBH-1 D-12; envelope contract executor.go).
func runSpillMatrixRow(t *testing.T) {
	t.Helper()
	fakeDir, err := filepath.Abs(filepath.Join("..", "..", "capability", "sandbox", "testdata", "fake-docker"))
	require.NoError(t, err)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	spillDir := filepath.Join(t.TempDir(), "spill")
	t.Setenv("FAKE_DOCKER_MODE", "spam")
	t.Setenv("FAKE_DOCKER_SPAM_BYTES", "2097152")

	runner, err := sandbox.NewSandboxCommandRunner(&sandbox.CommandRunnerConfig{
		Workspace: t.TempDir(),
		SpillDir:  spillDir,
	}, sandbox.NewSandboxRuntime(sandbox.SandboxConfig{}))
	require.NoError(t, err)

	tools := toolcapabilities.Build(t.TempDir(), runner, []*ports.ToolManifest{spillManifest()},
		toolcapabilities.StrictMode(),
		toolcapabilities.WithBackendBuilder("subprocess", subprocess.BackendBuilder()),
	)
	require.Len(t, tools, 1)

	result, err := tools[0].Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, result)

	truncated, _ := result.Data["truncated"].(bool)
	require.True(t, truncated, "ceiling exceed must surface truncated in the envelope")
	stdoutRef, _ := result.Data["stdout_ref"].(string)
	require.NotEmpty(t, stdoutRef, "spilled stdout_ref must be populated when truncated")
	spilled, err := os.ReadFile(stdoutRef)
	require.NoError(t, err)
	require.Len(t, spilled, 1<<20, "spill file must hold the retained ceiling prefix")

	// Without SpillDir the truncation is still reported; the refs stay empty.
	noSpillRunner, err := sandbox.NewSandboxCommandRunner(&sandbox.CommandRunnerConfig{
		Workspace: t.TempDir(),
	}, sandbox.NewSandboxRuntime(sandbox.SandboxConfig{}))
	require.NoError(t, err)
	tools2 := toolcapabilities.Build(t.TempDir(), noSpillRunner, []*ports.ToolManifest{spillManifest()},
		toolcapabilities.StrictMode(),
		toolcapabilities.WithBackendBuilder("subprocess", subprocess.BackendBuilder()),
	)
	require.Len(t, tools2, 1)
	result2, err := tools2[0].Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, result2)
	truncated2, _ := result2.Data["truncated"].(bool)
	require.True(t, truncated2)
	ref2, _ := result2.Data["stdout_ref"].(string)
	require.Empty(t, ref2, "no SpillDir ⇒ no ref")
}
