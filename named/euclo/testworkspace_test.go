package euclo

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// testRecipeSource is a minimal valid thoughtrecipe: enough DSL for the real
// loader (parse, semantics, lowering) to register exactly one recipe, with no
// capability steps so it loads against any capability registry.
const testRecipeSource = `thoughtrecipe euclo.thoughtrecipe.probe
"Minimal probe recipe for agent boot tests."

trigger as capability:
  may read workspace

input prompt: user.request

agent worker uses react

run worker:
  from input.prompt
  goal "Probe the workspace."
`

// writeRecipeWorkspace materializes a workspace whose relurpify_cfg/euclo
// carries exactly one valid recipe, and returns the workspace root.
func writeRecipeWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "relurpify_cfg", "euclo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "relurpify_cfg", "euclo", "probe.erpe"), []byte(testRecipeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

// newTestIndexManager builds an IndexManager whose declared workspace is the
// given root — the deps-side workspace truth that Initialize resolves against.
func newTestIndexManager(t *testing.T, workspace string) *ast.IndexManager {
	t.Helper()
	return ast.NewIndexManager(nil, ast.IndexConfig{WorkspacePath: workspace})
}

// initializeAgentIn initializes the agent against an explicit workspace config.
func initializeAgentIn(t *testing.T, a *Agent, workspace string) error {
	t.Helper()
	return a.Initialize(&execution.Config{Workspace: workspace})
}
