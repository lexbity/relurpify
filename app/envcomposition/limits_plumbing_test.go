package envcomposition

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
)

// recordRunner implements ports.CommandRunner by capturing every request —
// the regression signal for the deleted sandbox command-runner adapter, which
// silently dropped MemoryBytes/PidsLimit/CPUs/OutputCeiling/GracePeriod.
type recordRunner struct {
	requests []ports.CommandRequest
}

func (r *recordRunner) Run(_ context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	r.requests = append(r.requests, req)
	return &ports.CommandResult{ExitCode: 0}, nil
}

// TestManifestLimitsSurviveComposition proves the manifest-declared sandbox
// limits reach the runner request through the same toolcapabilities.Build
// composition path the registry uses (SBH-1 FR-16 / acceptance 10).
func TestManifestLimitsSurviveComposition(t *testing.T) {
	rec := &recordRunner{}
	manifests := []*ports.ToolManifest{{
		Name:        "limited_worker",
		Family:      "test",
		Description: "declared limits must survive composition",
		Parameters:  []ports.ToolParameter{},
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{"limited_worker"}},
			Sandbox: &ports.ToolManifestSandbox{
				MemoryMB:      1024,
				PidsLimit:     128,
				CPUs:          2.5,
				OutputCeiling: 1 << 20,
				GracePeriod:   2 * time.Second,
			},
		},
	}}
	tools := toolcapabilities.Build(t.TempDir(), rec, manifests,
		toolcapabilities.StrictMode(),
		toolcapabilities.WithBackendBuilder("subprocess", subprocess.BackendBuilder()),
	)
	require.Len(t, tools, 1, "the limited manifest must admit one tool")

	_, err := tools[0].Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.Len(t, rec.requests, 1, "execute must reach the runner exactly once")
	req := rec.requests[0]

	require.Equal(t, int64(1024*1024*1024), req.MemoryBytes)
	require.Equal(t, int64(128), req.PidsLimit)
	require.Equal(t, 2.5, req.CPUs)
	require.Equal(t, int64(1<<20), req.OutputCeiling)
	require.Equal(t, 2*time.Second, req.GracePeriod)
}
