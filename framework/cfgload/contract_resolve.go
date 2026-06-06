package cfgload

import (
	"fmt"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// EffectiveAgentContract captures the resolved runtime-facing contract derived
// from the manifest and any later overlays.
type EffectiveAgentContract struct {
	AgentID      string
	Manifest     *AgentManifest
	AgentSpec    *agentspec.AgentRuntimeSpec
	Permissions  contracts.PermissionSet
	Resources    ResourceSpec
	Sources      SourceSummary
}

// SourceSummary records which inputs contributed to the effective contract so
// callers can inspect how a runtime was resolved.
type SourceSummary struct {
	ManifestName     string
	ManifestVersion  string
	Workspace        string
	GlobalDefaults   bool
	OverlayCount     int
	RuntimeOverrides int
}

// ResolveOptions provides optional inputs layered on top of the raw manifest.
type ResolveOptions struct {
}

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

// MergePermissionSets unions multiple permission sets in order, de-duping
// entries.
func MergePermissionSets(sets ...*contracts.PermissionSet) contracts.PermissionSet {
	var merged contracts.PermissionSet
	fsSeen := make(map[string]struct{})
	execSeen := make(map[string]struct{})
	netSeen := make(map[string]struct{})
	capSeen := make(map[string]struct{})
	ipcSeen := make(map[string]struct{})

	for _, set := range sets {
		if set == nil {
			continue
		}
		for _, perm := range set.FileSystem {
			key := string(perm.Action) + ":" + perm.Path
			if _, ok := fsSeen[key]; ok {
				continue
			}
			fsSeen[key] = struct{}{}
			merged.FileSystem = append(merged.FileSystem, perm)
		}
		for _, perm := range set.Executables {
			key := perm.Binary + ":" + strings.Join(perm.Args, "|") + ":" + strings.Join(perm.Env, "|") + ":" + perm.Checksum
			if perm.HITLRequired {
				key += ":hitl"
			}
			if perm.ProxyRequired {
				key += ":proxy"
			}
			if _, ok := execSeen[key]; ok {
				continue
			}
			execSeen[key] = struct{}{}
			merged.Executables = append(merged.Executables, perm)
		}
		for _, perm := range set.Network {
			key := perm.Direction + ":" + perm.Protocol + ":" + perm.Host
			if perm.Port > 0 {
				key += ":" + strconv.Itoa(perm.Port)
			}
			if perm.HITLRequired {
				key += ":hitl"
			}
			if _, ok := netSeen[key]; ok {
				continue
			}
			netSeen[key] = struct{}{}
			merged.Network = append(merged.Network, perm)
		}
		for _, perm := range set.Capabilities {
			key := perm.Capability
			if _, ok := capSeen[key]; ok {
				continue
			}
			capSeen[key] = struct{}{}
			merged.Capabilities = append(merged.Capabilities, perm)
		}
		for _, perm := range set.IPC {
			key := perm.Kind + ":" + perm.Target
			if _, ok := ipcSeen[key]; ok {
				continue
			}
			ipcSeen[key] = struct{}{}
			merged.IPC = append(merged.IPC, perm)
		}
		merged.HITLRequired = append(merged.HITLRequired, set.HITLRequired...)
	}
	return merged
}

// MergeResourceSpecs overlays non-empty resource fields on top of a base spec.
func MergeResourceSpecs(base ResourceSpec, overlays ...*ResourceSpec) ResourceSpec {
	merged := base
	for _, overlay := range overlays {
		if overlay == nil {
			continue
		}
		if overlay.Limits.CPU != "" {
			merged.Limits.CPU = overlay.Limits.CPU
		}
		if overlay.Limits.Memory != "" {
			merged.Limits.Memory = overlay.Limits.Memory
		}
		if overlay.Limits.DiskIO != "" {
			merged.Limits.DiskIO = overlay.Limits.DiskIO
		}
		if overlay.Limits.Network != "" {
			merged.Limits.Network = overlay.Limits.Network
		}
	}
	return merged
}

// ResolveEffectiveAgentContract merges manifest defaults and optional overlays
// into one runtime-facing contract.
func ResolveEffectiveAgentContract(workspace string, m *AgentManifest, opts ResolveOptions) (*EffectiveAgentContract, error) {
	if m == nil {
		return nil, fmt.Errorf("agent manifest required")
	}
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}

	permissions, err := ResolveEffectivePermissions(workspace, m)
	if err != nil {
		return nil, fmt.Errorf("resolve permissions: %w", err)
	}
	resources, err := ResolveEffectiveResources(workspace, m)
	if err != nil {
		return nil, fmt.Errorf("resolve resources: %w", err)
	}

	baseSpec := m.Spec.Agent
	if baseSpec == nil {
		baseSpec = &agentspec.AgentRuntimeSpec{}
	}

	sources := SourceSummary{
		ManifestName:    m.Metadata.Name,
		ManifestVersion: m.Metadata.Version,
		Workspace:       workspace,
		GlobalDefaults:  false,
	}

	return &EffectiveAgentContract{
		AgentID:     m.Metadata.Name,
		Manifest:    m,
		AgentSpec:   baseSpec,
		Permissions: permissions,
		Resources:   resources,
		Sources:     sources,
	}, nil
}
