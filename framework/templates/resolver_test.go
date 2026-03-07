package templates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolverPrefersSharedRoot(t *testing.T) {
	shared := t.TempDir()
	repo := t.TempDir()
	sharedTemplate := filepath.Join(shared, "templates", "skills", "skill.manifest.yaml")
	repoTemplate := filepath.Join(repo, "templates", "skills", "skill.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(sharedTemplate), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(repoTemplate), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sharedTemplate, []byte("shared"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repoTemplate, []byte("repo"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Resolver{roots: []string{shared, repo}}
	got, err := r.ResolveSkillManifestTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if got != sharedTemplate {
		t.Fatalf("ResolveSkillManifestTemplate() = %q, want %q", got, sharedTemplate)
	}
}

func TestResolverWorkspaceConfigTemplate(t *testing.T) {
	root := t.TempDir()
	configTemplate := filepath.Join(root, "templates", "workspace", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configTemplate), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configTemplate, []byte("model: test"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Resolver{roots: []string{root}}
	got, err := r.ResolveWorkspaceConfigTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if got != configTemplate {
		t.Fatalf("ResolveWorkspaceConfigTemplate() = %q, want %q", got, configTemplate)
	}
}

func TestResolverStarterAgentPrefersTemplatesDir(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "templates", "agents", "coding-go.yaml")
	if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canonical, []byte("canonical"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Resolver{roots: []string{root}}
	got, err := r.ResolveStarterAgent("coding-go")
	if err != nil {
		t.Fatal(err)
	}
	if got != canonical {
		t.Fatalf("ResolveStarterAgent() = %q, want %q", got, canonical)
	}
}

func TestResolverTestsuiteTemplateProfile(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "templates", "testsuite", "default", "relurpify_cfg")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	r := Resolver{roots: []string{root}}
	got, err := r.ResolveTestsuiteTemplateProfile("")
	if err != nil {
		t.Fatal(err)
	}
	if got != profile {
		t.Fatalf("ResolveTestsuiteTemplateProfile() = %q, want %q", got, profile)
	}
}
