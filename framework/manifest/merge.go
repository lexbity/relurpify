package manifest

import (
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// MergePermissionSets unions multiple permission sets in order, de-duping entries.
func MergePermissionSets(sets ...*core.PermissionSet) core.PermissionSet {
	var merged core.PermissionSet
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
