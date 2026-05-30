// Package toolcapabilities governs local tool admission, builds tool
// implementations from manifests, and enforces manifest/implementation
// consistency checks such as parameter-key drift detection.
//
// This package handles only subprocess, go_native, and composite backends.
// MCP tools are routed to a different subsystem.
package toolcapabilities

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// AssertParamKeys checks that every parameter key the implementation declares
// (via contracts.ParamKeysProvider) exists in the manifest's parameter list.
// Returns a descriptive error listing all mismatched keys.
//
// This is a registration-time check that catches drift between the Go impl
// and the manifest before the tool ever runs.
func AssertParamKeys(impl contracts.Tool, name string, manifestParams []contracts.ToolParameter) error {
	provider, ok := impl.(contracts.ParamKeysProvider)
	if !ok {
		return nil // impl doesn't declare consumed keys — skip check
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
