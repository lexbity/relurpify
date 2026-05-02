package manifest

import "codeburg.org/lexbit/relurpify/platform/contracts"

// ResolveEffectivePermissions merges defaults and manifest permissions.
// Skills no longer contribute a Permissions block; that is handled by the
// gVisor allowlist derived from the tool set.
func ResolveEffectivePermissions(_ string, m *AgentManifest) (contracts.PermissionSet, error) {
	var sets []*contracts.PermissionSet
	if m != nil && m.Spec.Defaults != nil && m.Spec.Defaults.Permissions != nil {
		sets = append(sets, m.Spec.Defaults.Permissions)
	}
	if m != nil {
		sets = append(sets, &m.Spec.Permissions)
	}
	return MergePermissionSets(sets...), nil
}

// ResolveEffectiveResources merges defaults and manifest resources.
func ResolveEffectiveResources(_ string, m *AgentManifest) (ResourceSpec, error) {
	base := ResourceSpec{}
	var overlays []*ResourceSpec
	if m != nil && m.Spec.Defaults != nil && m.Spec.Defaults.Resources != nil {
		base = *m.Spec.Defaults.Resources
	}
	if m != nil {
		overlays = append(overlays, &m.Spec.Resources)
	}
	return MergeResourceSpecs(base, overlays...), nil
}
