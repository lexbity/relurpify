package agenttest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testRecipeSource is a minimal valid thoughtrecipe: enough DSL for the real
// loader (parse, semantics, lowering) to register exactly one recipe, with no
// capability steps so it loads against any capability registry.
const testRecipeSource = `thoughtrecipe euclo.thoughtrecipe.probe
"Minimal probe recipe for agenttest boot tests."

trigger as capability:
  may read workspace

input prompt: user.request

agent worker uses react

run worker:
  from input.prompt
  goal "Probe the workspace."
`

// materializeRecipeWorkspace writes relurpify_cfg/euclo with one valid recipe
// into the given workspace. Boots require it after workspace-truth recipe
// resolution (D-8): a resolved workspace without a recipe directory is a boot
// error, never a silently empty registry.
func materializeRecipeWorkspace(t *testing.T, workspace string) {
	t.Helper()
	// Tests that deliberately pass an invalid workspace (file-as-workspace,
	// nonexistent path) exercise their own later failure; skip them here.
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		return
	}
	if err := os.MkdirAll(filepath.Join(workspace, "relurpify_cfg", "euclo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "relurpify_cfg", "euclo", "probe.erpe"), []byte(testRecipeSource), 0o644); err != nil {
		t.Fatal(err)
	}
}

// copyCanonicalRecipes copies the repo's canonical relurpify_cfg/euclo recipes
// into the workspace so boots see the full canonical set.
func copyCanonicalRecipes(t *testing.T, workspace string) {
	t.Helper()
	src := filepath.Join("..", "..", "relurpify_cfg", "euclo")
	dst := filepath.Join(workspace, "relurpify_cfg", "euclo")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".erpe" {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, entry.Name()), contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestExecutorBootsWithWorkspaceRecipesFromNonRepoCWD is acceptance
// criterion 4's hermetic proof: the executor resolves recipes from the
// descriptor's workspace (never the process CWD), so a boot whose workspace
// is a temp dir with the canonical recipe set exposes those IDs on the agent.
// The package's CWD (testsuite/agenttest) is not the repo root, and the
// workspace is nowhere near it.
func TestExecutorBootsWithWorkspaceRecipesFromNonRepoCWD(t *testing.T) {
	ws := t.TempDir()
	copyCanonicalRecipes(t, ws)
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exec.agent == nil {
		t.Fatal("agent is nil after Execute")
	}

	loaded := map[string]bool{}
	for _, id := range exec.agent.ThoughtRecipeIDs() {
		loaded[id] = true
	}
	for _, id := range []string{
		"euclo.thoughtrecipe.default",
		"euclo.thoughtrecipe.code_review",
		"euclo.thoughtrecipe.investigation",
		"euclo.thoughtrecipe.debug_tdd_repair",
		"euclo.thoughtrecipe.dep_upgrade",
		"euclo.thoughtrecipe.test_synthesis",
		"euclo.thoughtrecipe.extract_func",
	} {
		if !loaded[id] {
			t.Errorf("canonical recipe %q missing from booted registry", id)
		}
	}
}

// TestExecutorBootFailsLoudlyWithoutRecipeDir: a resolved workspace without
// relurpify_cfg/euclo fails the boot with the contract error — the silently
// empty registry failure mode is gone.
func TestExecutorBootFailsLoudlyWithoutRecipeDir(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	if info, err := os.Stat(filepath.Join(ws, "relurpify_cfg", "euclo")); err == nil && info.IsDir() {
		// validDescriptorWithWorkspace materialized the minimal recipe tree;
		// remove it to exercise the missing-directory failure.
		if err := os.RemoveAll(filepath.Join(ws, "relurpify_cfg", "euclo")); err != nil {
			t.Fatal(err)
		}
	}
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	err := exec.Execute(context.Background(), desc, io.Discard)
	if err == nil {
		t.Fatal("expected boot error for workspace without relurpify_cfg/euclo")
	}
	if !strings.Contains(err.Error(), "no recipes: relurpify_cfg/euclo missing under workspace") {
		t.Fatalf("error does not carry the contract text: %v", err)
	}
}
