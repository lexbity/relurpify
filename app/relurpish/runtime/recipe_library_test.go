package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/ports"
)

func TestV2RecipeLibraryConfinesReadsAndHasNoEffectCapabilities(t *testing.T) {
	workspace := t.TempDir()
	library := NewRecipeLibrary(workspace)
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatal("constructing a source library changed the workspace")
	}
	if _, ok := any(library).(ports.RecipeWriter); ok {
		t.Fatal("unsafe live source saving was advertised")
	}
	if _, ok := any(library).(ports.RecipeRunner); ok {
		t.Fatal("validation was advertised as execution")
	}
	dir := filepath.Join(workspace, "relurpify_cfg", "euclo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	const source = "thoughtrecipe example.flow\n\"Example\"\ninput prompt: user.request\n"
	if err := os.WriteFile(filepath.Join(dir, "example.erpe"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := library.ListRecipes(t.Context())
	if err != nil || len(files) != 1 || files[0].Path != "relurpify_cfg/euclo/example.erpe" {
		t.Fatalf("source list: %+v %v", files, err)
	}
	file, err := library.OpenRecipe(t.Context(), files[0].Path)
	if err != nil || file.Source != source || len(file.Revision) != 64 {
		t.Fatalf("source identity: %+v %v", file, err)
	}
	outside := filepath.Join(t.TempDir(), "secret.erpe")
	if err := os.WriteFile(outside, []byte("outside secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape.erpe")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{outside, "../secret.erpe", "relurpify_cfg/euclo/escape.erpe"} {
		if _, err := library.OpenRecipe(t.Context(), name); err == nil {
			t.Fatalf("source read escaped workspace through %q", name)
		}
	}
	catalog, err := library.RecipeCatalog(t.Context())
	if err != nil || len(catalog.Strategies) == 0 {
		t.Fatalf("installed strategy catalog unavailable: %v", err)
	}
	files, err = library.ListRecipes(t.Context())
	if err != nil || len(files) != 1 {
		t.Fatal("symlink was included as regular source")
	}
}

func TestV2RecipeLibraryLimitsAndCancellation(t *testing.T) {
	workspace := t.TempDir()
	library := NewRecipeLibrary(workspace)
	filename := filepath.Join(workspace, "large.erpe")
	if err := os.WriteFile(filename, []byte(strings.Repeat("x", (1<<20)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := library.OpenRecipe(t.Context(), "large.erpe"); err == nil {
		t.Fatal("oversized source accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := library.OpenRecipe(ctx, "large.erpe"); err == nil {
		t.Fatal("canceled read accepted")
	}
	if _, err := library.ListRecipes(ctx); err == nil {
		t.Fatal("canceled listing accepted")
	}
}
