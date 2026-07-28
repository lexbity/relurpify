package templates

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestResolverTestsuiteTemplateProfile(t *testing.T) {
	r := NewResolver("")
	got, err := r.ResolveTestsuiteTemplateProfile("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(got, "workspace.yaml"); err != nil {
		t.Errorf("expected workspace.yaml in profile: %v", err)
	}
	if _, err := fs.Stat(got, "security/sandbox.policy.yaml"); err != nil {
		t.Errorf("expected security/sandbox.policy.yaml in profile: %v", err)
	}
}

func TestResolverTestsuiteTemplateProfileExplicitDefault(t *testing.T) {
	r := NewResolver("")
	got, err := r.ResolveTestsuiteTemplateProfile("default")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(got, "workspace.yaml"); err != nil {
		t.Errorf("expected workspace.yaml in default profile: %v", err)
	}
}

func TestResolverTestsuiteTemplateProfileUnknown(t *testing.T) {
	r := NewResolver("")
	_, err := r.ResolveTestsuiteTemplateProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestResolverTestsuiteTemplateProfileCleanup(t *testing.T) {
	tmpDir := t.TempDir()

	outDir := filepath.Join(tmpDir, "generated")
	if err := GenerateConfig(outDir); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "tools")); err != nil {
		t.Errorf("generated tools dir missing: %v", err)
	}
}
