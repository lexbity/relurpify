// Package thoughtrecipe implements the Euclo thoughtrecipe DSL.
package thoughtrecipe

import (
	"strings"
	"testing"
)

func TestRecipeRevision_Basic(t *testing.T) {
	buffer := NewSourceBufferFromString("test.erpe", "thoughtrecipe test")
	rev := NewRecipeRevisionFromSource(buffer)

	if rev.Path != "test.erpe" {
		t.Errorf("expected path 'test.erpe', got %q", rev.Path)
	}

	if rev.Digest != buffer.Digest() {
		t.Errorf("expected digest %q, got %q", buffer.Digest(), rev.Digest)
	}

	if rev.IsZero() {
		t.Error("expected non-zero revision")
	}
}

func TestRecipeRevision_Zero(t *testing.T) {
	zero := RecipeRevision{}

	if !zero.IsZero() {
		t.Error("expected zero revision")
	}

	if zero.String() == "" {
		t.Error("expected non-empty string for zero revision")
	}
}

func TestRecipeRevision_Equal(t *testing.T) {
	rev1 := NewRecipeRevision("path.erpe", "abc123")
	rev2 := NewRecipeRevision("path.erpe", "abc123")
	rev3 := NewRecipeRevision("other.erpe", "abc123")
	rev4 := NewRecipeRevision("path.erpe", "def456")

	if !rev1.Equal(rev2) {
		t.Error("expected rev1 == rev2")
	}

	if rev1.Equal(rev3) {
		t.Error("expected rev1 != rev3 (different path)")
	}

	if rev1.Equal(rev4) {
		t.Error("expected rev1 != rev4 (different digest)")
	}
}

func TestRecipeRevision_String(t *testing.T) {
	rev := NewRecipeRevision("relurpify_cfg/euclo/test.erpe", "abc123def456")
	s := rev.String()

	// Should contain filename and digest
	if !strings.Contains(s, "test.erpe") {
		t.Errorf("expected string to contain filename, got %q", s)
	}

	if !strings.Contains(s, "abc123") {
		t.Errorf("expected string to contain digest prefix, got %q", s)
	}
}

func TestRecipeManifest_Basic(t *testing.T) {
	root := NewRecipeRevision("main.erpe", "root123")
	manifest := NewRecipeManifest(root)

	if manifest.Root.Path != "main.erpe" {
		t.Errorf("expected root path 'main.erpe', got %q", manifest.Root.Path)
	}

	if manifest.Root.Digest != "root123" {
		t.Errorf("expected root digest 'root123', got %q", manifest.Root.Digest)
	}

	if manifest.Imports == nil {
		t.Error("expected imports map to be initialized")
	}

	if len(manifest.Imports) != 0 {
		t.Errorf("expected empty imports, got %d", len(manifest.Imports))
	}
}

func TestRecipeManifest_AddImport(t *testing.T) {
	root := NewRecipeRevision("main.erpe", "root123")
	manifest := NewRecipeManifest(root)

	imp := NewRecipeRevision("import.erpe", "import456")
	manifest.AddImport("import.erpe", imp)

	if len(manifest.Imports) != 1 {
		t.Errorf("expected 1 import, got %d", len(manifest.Imports))
	}

	stored, ok := manifest.Imports["import.erpe"]
	if !ok {
		t.Fatal("expected import to be stored")
	}

	if !stored.Equal(imp) {
		t.Error("expected stored import to match added import")
	}
}

func TestRecipeManifest_Validate(t *testing.T) {
	// Valid manifest
	root := NewRecipeRevision("main.erpe", "root123")
	manifest := NewRecipeManifest(root)
	manifest.AddImport("imp1.erpe", NewRecipeRevision("imp1.erpe", "imp123"))

	if err := manifest.Validate(); err != nil {
		t.Errorf("expected valid manifest, got error: %v", err)
	}

	// Invalid: zero root
	manifest2 := NewRecipeManifest(RecipeRevision{})
	if err := manifest2.Validate(); err == nil {
		t.Error("expected error for zero root")
	}

	// Invalid: zero import
	manifest3 := NewRecipeManifest(root)
	manifest3.AddImport("bad.erpe", RecipeRevision{})
	if err := manifest3.Validate(); err == nil {
		t.Error("expected error for zero import")
	}
}

func TestRecipeManifest_Digest(t *testing.T) {
	root := NewRecipeRevision("main.erpe", "root123")
	manifest := NewRecipeManifest(root)

	digest := manifest.Digest()
	if digest == "" {
		t.Error("expected non-empty digest")
	}

	// Same manifest should produce same digest
	digest2 := manifest.Digest()
	if digest != digest2 {
		t.Errorf("expected same digest for same manifest: %q vs %q", digest, digest2)
	}
}

func TestResolutionContext_ResolveImportPath(t *testing.T) {
	ctx := &ResolutionContext{
		WorkspaceRoot: "/home/user/workspace",
		RecipeRoot:    "/home/user/workspace/relurpify_cfg/euclo",
		CurrentFile:   "/home/user/workspace/relurpify_cfg/euclo/main.erpe",
	}

	// Test relative path
	path := ctx.ResolveImportPath("utils.erpe")
	expected := "/home/user/workspace/relurpify_cfg/euclo/utils.erpe"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}

	// Test absolute path
	absPath := ctx.ResolveImportPath("/absolute/path.erpe")
	if absPath != "/absolute/path.erpe" {
		t.Errorf("expected absolute path preserved: %q", absPath)
	}
}

func TestWorkspaceRelativePath(t *testing.T) {
	workspaceRoot := "/home/user/workspace"

	// File in workspace
	absPath := "/home/user/workspace/relurpify_cfg/euclo/test.erpe"
	rel := WorkspaceRelativePath(workspaceRoot, absPath)
	if rel != "relurpify_cfg/euclo/test.erpe" {
		t.Errorf("expected 'relurpify_cfg/euclo/test.erpe', got %q", rel)
	}

	// File outside workspace (should return absolute)
	absPath2 := "/tmp/test.erpe"
	rel2 := WorkspaceRelativePath(workspaceRoot, absPath2)
	if rel2 == "relurpify_cfg/euclo/test.erpe" {
		t.Error("expected different path for file outside workspace")
	}
}

func TestRecipeIdentity(t *testing.T) {
	buffer := NewSourceBufferFromString("test.erpe", "thoughtrecipe test")
	rev := NewRecipeRevisionFromSource(buffer)
	identity := NewRecipeIdentity("test", rev)

	if identity.Name != "test" {
		t.Errorf("expected name 'test', got %q", identity.Name)
	}

	if !identity.Revision.Equal(rev) {
		t.Error("expected revision to match")
	}

	if identity.String() == "" {
		t.Error("expected non-empty string")
	}

	// Test equality
	identity2 := NewRecipeIdentity("test", rev)
	if !identity.Equal(identity2) {
		t.Error("expected identical identities to be equal")
	}

	identity3 := NewRecipeIdentity("other", rev)
	if identity.Equal(identity3) {
		t.Error("expected different names to not be equal")
	}
}
