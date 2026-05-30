package toolcapabilities

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/tools/composite"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
)

// BuildOption controls Build behaviour.
type BuildOption func(*buildConfig)

type buildConfig struct {
	strict bool
}

// StrictMode makes Build hard-fail when a go_native manifest references an
// unregistered implementation key instead of logging a warning and skipping.
func StrictMode() BuildOption {
	return func(c *buildConfig) { c.strict = true }
}

// Build constructs the full set of local tool implementations from their
// manifests. Each manifest is dispatched to the appropriate backend:
//
//	subprocess → subprocess.NewTool
//	go_native  → nativereg.Lookup + constructor call
//	composite  → composite.New
//	mcp        → skipped (handled by a separate subsystem)
//
// Tools whose manifests are not admitted (missing Go impl, strict-mode
// violations, or unlisted backends) are excluded with a warning log.
func Build(workspace string, runner contracts.CommandRunner, manifests []*contracts.ToolManifest, opts ...BuildOption) []contracts.Tool {
	var cfg buildConfig
	for _, o := range opts {
		o(&cfg)
	}

	admission := newAdmission(manifests)
	var result []contracts.Tool

	for _, m := range manifests {
		if m == nil {
			continue
		}
		name := contracts.NormalizeToolName(m.Name)
		if name == "" {
			continue
		}

		tool, err := buildOne(workspace, runner, *m, cfg.strict)
		if err != nil {
			log.Printf("tool build: skipping %q: %v", name, err)
			continue
		}
		if tool == nil {
			continue
		}

		if ok, admitErr := admission.Admit(tool); !ok {
			if admitErr != nil {
				log.Printf("tool admission: skipping %q: %v", name, admitErr)
			}
			continue
		}

		result = append(result, tool)
	}

	return result
}

// buildOne constructs a single tool from its manifest.
func buildOne(workspace string, runner contracts.CommandRunner, manifest contracts.ToolManifest, strict bool) (contracts.Tool, error) {
	switch manifest.Execution.Backend {
	case contracts.ToolBackendSubprocess:
		return subprocess.NewTool(manifest, runner), nil

	case contracts.ToolBackendGoNative:
		return buildGoNative(workspace, manifest, strict)

	case contracts.ToolBackendComposite:
		resolver := func(name string) (contracts.Tool, bool) {
			// The resolver is intentionally empty at build time — the
			// framework injects a full tool resolver at registration time.
			// Composite tools built here serve as descriptors only.
			return nil, false
		}
		return composite.New(manifest, resolver), nil

	case contracts.ToolBackendMCP:
		return nil, nil // MCP tools are handled by a separate subsystem

	default:
		return nil, fmt.Errorf("unsupported backend %q", manifest.Execution.Backend)
	}
}

// buildGoNative constructs a go_native tool from the native registry.
func buildGoNative(workspace string, manifest contracts.ToolManifest, strict bool) (contracts.Tool, error) {
	impl := manifest.Execution.Implementation
	if impl == "" {
		impl = manifest.Name
	}

	ctor, ok := contracts.LookupNative(impl)
	if !ok {
		if strict {
			return nil, fmt.Errorf("go_native implementation %q not registered (strict mode)", impl)
		}
		log.Printf("tool build: go_native %q: implementation %q not registered, skipping", manifest.Name, impl)
		return nil, nil
	}

	tool := ctor(workspace)
	if tool == nil {
		return nil, fmt.Errorf("go_native constructor for %q returned nil", impl)
	}

	if err := AssertParamKeys(tool, manifest.Name, manifest.Parameters); err != nil {
		return nil, err
	}

	return tool, nil
}

// admission wraps the capability-level ToolAdmissionPolicy for use during
// Build, applying tool-level consistency checks.
type admission struct {
	manifests map[string]contracts.ToolManifest
}

func newAdmission(manifests []*contracts.ToolManifest) *admission {
	m := make(map[string]contracts.ToolManifest, len(manifests))
	for _, p := range manifests {
		if p != nil {
			m[contracts.NormalizeToolName(p.Name)] = *p
		}
	}
	return &admission{manifests: m}
}

func (a *admission) Admit(tool contracts.Tool) (bool, error) {
	if tool == nil {
		return false, fmt.Errorf("tool required")
	}
	name := contracts.NormalizeToolName(tool.Name())
	if _, ok := a.manifests[name]; !ok {
		return false, nil // not in manifest — silently skip
	}
	if strings.TrimSpace(tool.Description()) == "" {
		return false, fmt.Errorf("missing description")
	}
	return true, nil
}

// AssertParamKeys checks that every parameter key the implementation declares
// (via contracts.ParamKeysProvider) exists in the manifest's parameter list.
// Returns a descriptive error listing all mismatched keys.
//
// This is a registration-time check that catches drift between the Go impl
// and the manifest before the tool ever runs.
func AssertParamKeys(impl contracts.Tool, name string, manifestParams []contracts.ToolParameter) error {
	provider, ok := impl.(contracts.ParamKeysProvider)
	if !ok {
		return nil
	}

	consumed := provider.ParamKeys()
	if len(consumed) == 0 {
		return nil
	}

	declared := make(map[string]struct{}, len(manifestParams))
	for _, p := range manifestParams {
		key := contracts.NormalizeToolName(p.Name)
		if key != "" {
			declared[key] = struct{}{}
		}
	}

	var missing []string
	for _, key := range consumed {
		normalized := contracts.NormalizeToolName(key)
		if normalized == "" {
			continue
		}
		if _, ok := declared[normalized]; !ok {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf(
			"tool %q parameter drift: implementation consumes %d key(s) not declared in manifest: %s",
			name, len(missing), strings.Join(missing, ", "),
		)
	}
	return nil
}

// AssertParamKeysOnConstructor asserts param key consistency by constructing
// a tool from the given constructor and checking it against the manifest.
func AssertParamKeysOnConstructor(key string, ctor contracts.NativeToolConstructor, manifest contracts.ToolManifest) error {
	if ctor == nil {
		return nil
	}
	impl := ctor("")
	if impl == nil {
		return nil
	}
	return AssertParamKeys(impl, key, manifest.Parameters)
}
