package config

import (
	"fmt"
	"sort"
	"strings"

	configmanifest "codeburg.org/lexbit/relurpify/platform/configmanifest"
	"codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// ToolRegistry stores the loaded tool manifests plus the resolved runtime
// implementations used to back them.
type ToolRegistry struct {
	manifests map[string]configmanifest.ToolManifest
	tools     map[string]any
	policies  map[string]security.ToolPolicy
	ordered   []string
}

// LookupTool resolves a tool definition by canonical name.
func (r *ToolRegistry) LookupTool(name string) (configmanifest.ToolManifest, bool) {
	if r == nil {
		return configmanifest.ToolManifest{}, false
	}
	manifest, ok := r.manifests[configmanifest.NormalizeToolName(name)]
	return manifest, ok
}

// ListTools returns the loaded tool definitions in deterministic order.
func (r *ToolRegistry) ListTools() []configmanifest.ToolManifest {
	if r == nil || len(r.ordered) == 0 {
		return nil
	}
	out := make([]configmanifest.ToolManifest, 0, len(r.ordered))
	for _, name := range r.ordered {
		out = append(out, r.manifests[name])
	}
	return out
}

// Tool returns the resolved runtime implementation for a tool name.
func (r *ToolRegistry) Tool(name string) (any, bool) {
	if r == nil {
		return nil, false
	}
	tool, ok := r.tools[configmanifest.NormalizeToolName(name)]
	return tool, ok
}

// Policy returns the localtool policy attached to a tool name.
func (r *ToolRegistry) Policy(name string) (security.ToolPolicy, bool) {
	if r == nil {
		return security.ToolPolicy{}, false
	}
	policy, ok := r.policies[configmanifest.NormalizeToolName(name)]
	return policy, ok
}

// BuildRegistry validates tool manifests against the local tool policy and
// attaches runtime implementations where available.
func BuildRegistry(
	defs []*configmanifest.ToolManifest,
	policy map[string]security.ToolPolicy,
	implementations map[string]any,
	subprocessToolFactory func(configmanifest.ToolManifest) any,
) (*ToolRegistry, error) {
	manifestByName := make(map[string]configmanifest.ToolManifest, len(defs))
	ordered := make([]string, 0, len(defs))
	for _, def := range defs {
		if def == nil {
			continue
		}
		name := configmanifest.NormalizeToolName(def.Name)
		if name == "" {
			return nil, fmt.Errorf("tool manifest missing name")
		}
		if _, exists := manifestByName[name]; exists {
			return nil, fmt.Errorf("tool %q declared more than once", name)
		}
		manifestByName[name] = *def
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	normalizedPolicy := make(map[string]security.ToolPolicy, len(policy))
	var missing []string
	for name, entry := range policy {
		normalized := configmanifest.NormalizeToolName(name)
		if normalized == "" {
			continue
		}
		if _, ok := manifestByName[normalized]; !ok {
			missing = append(missing, normalized)
			continue
		}
		normalizedPolicy[normalized] = entry
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("localtool.policy.yaml references unknown tool(s): %s", strings.Join(missing, ", "))
	}

	impls := make(map[string]any, len(implementations))
	for name, tool := range implementations {
		normalized := configmanifest.NormalizeToolName(name)
		if normalized == "" || tool == nil {
			continue
		}
		impls[normalized] = tool
	}

	tools := make(map[string]any, len(manifestByName))
	for _, name := range ordered {
		manifest := manifestByName[name]
		tool, ok := impls[name]
		switch manifest.Execution.Backend {
		case configmanifest.ToolBackendGoNative:
			// Go-native tools are defined by their manifests; runtime registration
			// happens in the packages that own the implementations.
		case configmanifest.ToolBackendSubprocess:
			if !ok && subprocessToolFactory != nil {
				tool = subprocessToolFactory(manifest)
			}
		case configmanifest.ToolBackendComposite:
			// Composite tools are resolved at runtime via the composition
			// runner; no tool implementation is registered here.
		default:
			return nil, fmt.Errorf("tool %q has unsupported backend %q", name, manifest.Execution.Backend)
		}
		if tool != nil {
			tools[name] = tool
		}
	}

	return &ToolRegistry{
		manifests: manifestByName,
		tools:     tools,
		policies:  normalizedPolicy,
		ordered:   ordered,
	}, nil
}
