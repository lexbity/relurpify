package templates

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const sharedRootEnv = "RELURPIFY_SHARED_DIR"

// Resolver discovers installed shared templates and falls back to repo-local
// development templates while the install model is being phased in.
type Resolver struct {
	roots []string
}

// NewResolver returns a resolver that checks installed shared templates first,
// then repo-local fallback locations.
func NewResolver() Resolver {
	return Resolver{roots: defaultRoots()}
}

// SearchRoots returns the ordered template search roots.
func (r Resolver) SearchRoots() []string {
	return append([]string(nil), r.roots...)
}

// SharedRoot returns the preferred machine-local shared directory.
func SharedRoot() string {
	if v := os.Getenv(sharedRootEnv); v != "" {
		return filepath.Clean(v)
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "relurpify")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".local", "share", "relurpify")
	}
	return filepath.Join(home, ".local", "share", "relurpify")
}

// ResolveWorkspaceManifestTemplate resolves the generic starter workspace
// manifest template.
func (r Resolver) ResolveWorkspaceManifestTemplate() (string, error) {
	return r.resolve(
		filepath.Join("templates", "workspace", "agent.manifest.yaml"),
	)
}

// ResolveWorkspaceConfigTemplate resolves the generic starter workspace config.
func (r Resolver) ResolveWorkspaceConfigTemplate() (string, error) {
	return r.resolve(
		filepath.Join("templates", "workspace", "config.yaml"),
	)
}

// ResolveSkillManifestTemplate resolves the generic skill manifest template.
func (r Resolver) ResolveSkillManifestTemplate() (string, error) {
	return r.resolve(
		filepath.Join("templates", "skills", "skill.manifest.yaml"),
	)
}

// ResolveStarterAgent resolves a named starter agent manifest template.
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

func defaultRoots() []string {
	roots := []string{SharedRoot()}
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
