package configcheck

import "codeburg.org/lexbit/relurpify/capability/toolcapabilities"

// CheckNetworkAllowlists runs the canonical egress-allowlist validation over
// every manifest and returns blocking problems keyed by tool name.
func CheckNetworkAllowlists(manifests []*toolcapabilities.ToolManifest) map[string][]string {
	results := make(map[string][]string)
	for _, m := range manifests {
		if m == nil {
			continue
		}
		v := toolcapabilities.ValidateNetworkAllowlists(m)
		if len(v.Errors) > 0 {
			results[m.Name] = append([]string(nil), v.Errors...)
		}
	}
	if len(results) == 0 {
		return nil
	}
	return results
}

// Note on SF-3 (network_access vs. container isolation honesty): the runtime
// hard-defaults container network isolation on and exposes no config knob to
// disable it — the SBH-1 non-goal keeps `--network none` at the container
// boundary. A live "network_access declared while isolated" diagnostic would
// therefore fire for every shipped network tool with no non-destructive
// remediation, regressing the repo's zero-diagnostic invariant. The check is
// deferred until isolation becomes configurable (a resource-plumbing change);
// the egress scanner below still screens network-access tools, so the
// declaration is defense-in-depth rather than silently trusted.
