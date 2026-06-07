package configcheck

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
)

// DeriveExpectedCapability computes the expected risk_class and effect_class
// for a subprocess tool based on its binary command and sandbox config.
//
// This catches under-declared manifests: a tool that runs curl with
// network_access: true must declare risk_class=[execute, network].
func DeriveExpectedCapability(manifest toolcapabilities.ToolManifest) (expectedRisk []string, expectedEffect []string) {
	if manifest.Execution.Backend != ports.ToolBackendSubprocess {
		return nil, nil
	}

	riskSet := make(map[string]struct{})
	effectSet := make(map[string]struct{})

	// Every subprocess tool executes a binary.
	riskSet["execute"] = struct{}{}
	effectSet["process_spawn"] = struct{}{}

	// Network access adds network risk and egress effect.
	sandbox := manifest.Execution.Sandbox
	if sandbox != nil && sandbox.NetworkAccess {
		riskSet["network"] = struct{}{}
		effectSet["network_egress"] = struct{}{}
		effectSet["external_state"] = struct{}{}
	}

	// Certain tools may also have filesystem effects based on family.
	family := toolcapabilities.NormalizeToolName(manifest.Family)
	switch family {
	case "fileops", "text", "system", "shell":
		if sandbox == nil || !sandbox.NetworkAccess {
			// If the tool already has filesystem_mutation (e.g. mkdir),
			// don't also require filesystem_read.
			hasMutation := false
			for _, ec := range manifest.Capability.EffectClass {
				if strings.TrimSpace(strings.ToLower(ec)) == "filesystem_mutation" {
					hasMutation = true
					break
				}
			}
			if !hasMutation {
				effectSet["filesystem_read"] = struct{}{}
			}
		}
	case "build":
		// Build tools spawn processes but don't necessarily read files
	}

	// Check if tool has default_args that include write-like operations.
	for _, arg := range manifest.Execution.DefaultArgs {
		arg = strings.TrimSpace(arg)
		if arg == "-p" || arg == "--parents" {
			// mkdir -p is a filesystem mutation
			effectSet["filesystem_mutation"] = struct{}{}
		}
	}

	return setToSortedSlice(riskSet), setToSortedSlice(effectSet)
}

// CheckManifest compares a manifest's declared capability with the derived
// expectation. Returns a list of issues (empty = no issues).
func CheckManifest(manifest toolcapabilities.ToolManifest) []string {
	var issues []string

	expectedRisk, expectedEffect := DeriveExpectedCapability(manifest)
	if expectedRisk == nil && expectedEffect == nil {
		return nil // go_native, composite — skip
	}

	declaredRisk := normalizeSet(manifest.Capability.RiskClass)
	declaredEffect := normalizeSet(manifest.Capability.EffectClass)

	// Each expected risk must be declared.
	for _, r := range expectedRisk {
		if !contains(declaredRisk, r) {
			issues = append(issues, fmt.Sprintf("capability.risk_class missing %q (derived from tool config)", r))
		}
	}

	// Each expected effect must be declared.
	for _, e := range expectedEffect {
		if !contains(declaredEffect, e) {
			issues = append(issues, fmt.Sprintf("capability.effect_class missing %q (derived from tool config)", e))
		}
	}

	return issues
}

// CheckAllManifests runs CheckManifest for every tool manifest.
func CheckAllManifests(manifests []*toolcapabilities.ToolManifest) map[string][]string {
	results := make(map[string][]string)
	for _, m := range manifests {
		if m == nil {
			continue
		}
		if issues := CheckManifest(*m); len(issues) > 0 {
			results[m.Name] = issues
		}
	}
	return results
}

func normalizeSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		norm := strings.TrimSpace(strings.ToLower(item))
		if norm != "" {
			out[norm] = struct{}{}
		}
	}
	return out
}

func setToSortedSlice(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}
