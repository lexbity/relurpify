package security

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// SandboxPolicyPath returns the canonical security sandbox policy location.
func SandboxPolicyPath(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "security", "sandbox.policy.yaml")
}

type sandboxPolicyFile struct {
	ReadOnlyRoot    bool                  `yaml:"read_only_root,omitempty"`
	ProtectedPaths  []string              `yaml:"protected_paths,omitempty"`
	NoNewPrivileges bool                  `yaml:"no_new_privileges,omitempty"`
	SeccompProfile  string                `yaml:"seccomp_profile,omitempty"`
	AllowedEnvKeys  []string              `yaml:"allowed_env_keys,omitempty"`
	DeniedEnvKeys   []string              `yaml:"denied_env_keys,omitempty"`
	NetworkRules    []sandbox.NetworkRule `yaml:"network_rules,omitempty"`
}

// LoadSandboxPolicy loads and validates the sandbox policy file.
func LoadSandboxPolicy(path, workspace string, decode Decoder) (*sandbox.SandboxPolicy, error) {
	var file sandboxPolicyFile
	if err := loadAndDecode(path, workspace, decode, SandboxPolicyPath, &file); err != nil {
		return nil, err
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	policy := &sandbox.SandboxPolicy{
		ReadOnlyRoot:    file.ReadOnlyRoot,
		ProtectedPaths:  normalizeProtectedPaths(absWorkspace, file.ProtectedPaths),
		NoNewPrivileges: file.NoNewPrivileges,
		SeccompProfile:  strings.TrimSpace(file.SeccompProfile),
		AllowedEnvKeys:  append([]string(nil), file.AllowedEnvKeys...),
		DeniedEnvKeys:   append([]string(nil), file.DeniedEnvKeys...),
		NetworkRules:    append([]sandbox.NetworkRule(nil), file.NetworkRules...),
	}
	policy.ProtectedPaths = injectProtectedRoot(absWorkspace, policy.ProtectedPaths)
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("validate sandbox policy: %w", err)
	}
	return policy, nil
}

func normalizeProtectedPaths(workspace string, paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		path = strings.ReplaceAll(path, "${workspace}", workspace)
		if !filepath.IsAbs(path) {
			path = filepath.Join(workspace, path)
		}
		out = append(out, filepath.Clean(path))
	}
	return out
}

func injectProtectedRoot(workspace string, paths []string) []string {
	root := filepath.Clean(filepath.Join(workspace, "relurpify_cfg"))
	seen := make(map[string]struct{}, len(paths)+1)
	out := make([]string, 0, len(paths)+1)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	add(root)
	for _, path := range paths {
		add(path)
	}
	return out
}
