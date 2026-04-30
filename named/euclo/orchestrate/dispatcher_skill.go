package orchestrate

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/skills"
)

func applySkillFilterToRegistry(workspace, skillName string, caps *capability.CapabilityRegistry) (*capability.CapabilityRegistry, error) {
	workspace = strings.TrimSpace(workspace)
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return caps, nil
	}
	if workspace == "" {
		return nil, fmt.Errorf("skill filter %q requires a workspace root", skillName)
	}
	if caps == nil {
		return nil, fmt.Errorf("skill filter %q requires a capability registry", skillName)
	}

	spec, _, results := skills.ResolveSkills(workspace, nil, []string{skillName})
	applied := false
	var failureReason string
	for _, result := range results {
		if result.Applied {
			applied = true
			break
		}
		if strings.TrimSpace(result.Error) != "" && failureReason == "" {
			failureReason = strings.TrimSpace(result.Error)
		}
	}
	if !applied {
		if failureReason == "" {
			failureReason = fmt.Sprintf("skill %q not found or failed to load", skillName)
		}
		return nil, fmt.Errorf("skill %q: %s", skillName, failureReason)
	}

	selectors := spec.AllowedCapabilities
	if len(selectors) == 0 {
		return nil, fmt.Errorf("skill %q has no allowed capabilities", skillName)
	}

	snapshots := caps.AllCapabilitySnapshots()
	allowedIDs := make([]string, 0, len(snapshots))
	for _, snap := range snapshots {
		for _, selector := range selectors {
			if core.SelectorMatchesDescriptor(core.CapabilitySelectorFromAgentSpec(selector), snap.Descriptor) {
				allowedIDs = append(allowedIDs, snap.Descriptor.ID)
				break
			}
		}
	}
	if len(allowedIDs) == 0 {
		return nil, fmt.Errorf("skill %q matched no registered capabilities", skillName)
	}

	return caps.WithAllowlist(allowedIDs), nil
}
