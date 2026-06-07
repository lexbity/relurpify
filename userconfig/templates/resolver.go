package templates

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolver discovers installed shared templates and falls back to repo-local
// development templates while the install model is being phased in.
type Resolver struct {
	roots      []string
	sharedRoot string
}

// NewResolver returns a resolver that checks an explicit shared template root
// first, then repo-local fallback locations.
func NewResolver(sharedRoot string) Resolver {
	return Resolver{roots: defaultRoots(sharedRoot), sharedRoot: sharedRoot}
}

// SearchRoots returns the ordered template search roots.
func (r Resolver) SearchRoots() []string {
	return append([]string(nil), r.roots...)
}

// ResolveWorkspaceAgentTemplate resolves the generic starter workspace agent template.
func (r Resolver) ResolveWorkspaceAgentTemplate() (string, error) {
	return r.resolve(
		filepath.Join("templates", "workspace", "agent.yaml"),
	)
}

// ResolveWorkspaceConfigTemplate resolves the starter workspace root config.
func (r Resolver) ResolveWorkspaceConfigTemplate() (string, error) {
	return r.resolve(
		filepath.Join("templates", "workspace", "workspace.yaml"),
	)
}

// ResolveSkillTemplate resolves the generic skill template.
func (r Resolver) ResolveSkillTemplate() (string, error) {
	return r.resolve(
		filepath.Join("templates", "skills", "skill.yaml"),
	)
}

// ResolveWorkspaceSecurityTemplate resolves a starter workspace security policy template.
func (r Resolver) ResolveWorkspaceSecurityTemplate(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("security template name required")
	}
	return r.resolve(
		filepath.Join("templates", "workspace", "security", name+".policy.yaml"),
	)
}

// ResolveStarterAgent resolves a named starter agent template.
func (r Resolver) ResolveStarterAgent(name string) (string, error) {
	if name == "" {
		return "", errors.New("starter agent name required")
	}
	filename := name + ".yaml"
	return r.resolve(
		filepath.Join("templates", "agents", filename),
	)
}

// ResolveTestsuiteTemplateProfile resolves the relurpify_cfg root for a named
// testsuite template profile.
func (r Resolver) ResolveTestsuiteTemplateProfile(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		name = "default"
	}
	return r.resolve(
		filepath.Join("templates", "testsuite", name, "relurpify_cfg"),
	)
}

func (r Resolver) resolve(relPaths ...string) (string, error) {
	for _, root := range r.roots {
		for _, rel := range relPaths {
			candidate := filepath.Join(root, rel)
			if info, err := os.Stat(candidate); err == nil {
				if info.IsDir() && strings.HasSuffix(filepath.ToSlash(rel), "/relurpify_cfg") {
					return candidate, nil
				}
				if !info.IsDir() {
					return candidate, nil
				}
			}
		}
	}
	return "", os.ErrNotExist
}

func defaultRoots(sharedRoot string) []string {
	roots := []string{filepath.Clean(sharedRoot)}
	if repo := repoRoot(); repo != "" {
		roots = append(roots, repo)
	}
	return unique(roots)
}

func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	// framework/templates/resolver.go -> repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func unique(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
