package templates

import (
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestResolverPrefersSharedRoot(t *testing.T) {
	shared := t.TempDir()
	repo := t.TempDir()
	sharedTemplate := filepath.Join(shared, "templates", "skills", "skill.yaml")
	repoTemplate := filepath.Join(repo, "templates", "skills", "skill.yaml")
	if err := fs.MkdirAllSecure(filepath.Dir(sharedTemplate)); err != nil {
		t.Fatal(err)
	}
	if err := fs.MkdirAllSecure(filepath.Dir(repoTemplate)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(sharedTemplate, []byte("shared")); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(repoTemplate, []byte("repo")); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(shared)
	r.roots = []string{shared, repo}
	got, err := r.ResolveSkillTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if got != sharedTemplate {
		t.Fatalf("ResolveSkillTemplate() = %q, want %q", got, sharedTemplate)
	}
}

func TestResolverWorkspaceConfigTemplate(t *testing.T) {
	root := t.TempDir()
	configTemplate := filepath.Join(root, "templates", "workspace", "workspace.yaml")
	if err := fs.MkdirAllSecure(filepath.Dir(configTemplate)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(configTemplate, []byte("model: test")); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(root)
	got, err := r.ResolveWorkspaceConfigTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if got != configTemplate {
		t.Fatalf("ResolveWorkspaceConfigTemplate() = %q, want %q", got, configTemplate)
	}
}

func TestResolverWorkspaceSecurityTemplate(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "templates", "workspace", "security", "sandbox.policy.yaml")
	if err := fs.MkdirAllSecure(filepath.Dir(templatePath)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(templatePath, []byte("schema: relurpify/policy/sandbox/v1")); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(root)
	got, err := r.ResolveWorkspaceSecurityTemplate("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	if got != templatePath {
		t.Fatalf("ResolveWorkspaceSecurityTemplate() = %q, want %q", got, templatePath)
	}
}

func TestResolverStarterAgentPrefersTemplatesDir(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "templates", "agents", "coding-go.yaml")
	if err := fs.MkdirAllSecure(filepath.Dir(canonical)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(canonical, []byte("canonical")); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(root)
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
	if err := fs.MkdirAllSecure(profile); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(root)
	got, err := r.ResolveTestsuiteTemplateProfile("")
	if err != nil {
		t.Fatal(err)
	}
	if got != profile {
		t.Fatalf("ResolveTestsuiteTemplateProfile() = %q, want %q", got, profile)
	}
}
