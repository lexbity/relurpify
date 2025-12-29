package manifest

import "github.com/lexcodex/relurpify/framework/core"

// ResolveEffectivePermissions merges defaults, skills, and manifest permissions.
func ResolveEffectivePermissions(workspace string, m *AgentManifest) (core.PermissionSet, error) {
	var sets []*core.PermissionSet
	if m != nil && m.Spec.Defaults != nil && m.Spec.Defaults.Permissions != nil {
		sets = append(sets, m.Spec.Defaults.Permissions)
	}
	if m != nil && len(m.Spec.Skills) > 0 {
		skills, err := ResolveSkillList(workspace, m.Spec.Skills)
		if err != nil {
			return core.PermissionSet{}, err
		}
		for _, skill := range skills {
			if skill != nil && skill.Spec.Permissions != nil {
				sets = append(sets, skill.Spec.Permissions)
			}
		}
	}
	if m != nil {
		sets = append(sets, &m.Spec.Permissions)
	}
	return MergePermissionSets(sets...), nil
}

// ResolveEffectiveResources merges defaults, skills, and manifest resources.
func ResolveEffectiveResources(workspace string, m *AgentManifest) (ResourceSpec, error) {
	base := ResourceSpec{}
	var overlays []*ResourceSpec
	if m != nil && m.Spec.Defaults != nil && m.Spec.Defaults.Resources != nil {
		base = *m.Spec.Defaults.Resources
	}
	if m != nil && len(m.Spec.Skills) > 0 {
		skills, err := ResolveSkillList(workspace, m.Spec.Skills)
		if err != nil {
			return ResourceSpec{}, err
		}
		for _, skill := range skills {
			if skill != nil && skill.Spec.Resources != nil {
				overlays = append(overlays, skill.Spec.Resources)
			}
		}
	}
	if m != nil {
		overlays = append(overlays, &m.Spec.Resources)
	}
	return MergeResourceSpecs(base, overlays...), nil
}
