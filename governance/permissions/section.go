package permissions

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// DecodeSection decodes a permissions YAML section node into a typed
// PermissionSet. This is the governance-owned decoder — each domain that
// owns a manifest section provides its own DecodeSection function.
func DecodeSection(node yaml.Node) (*PermissionSet, error) {
	if node.Kind == 0 {
		return nil, fmt.Errorf("permissions section node is absent")
	}
	var ps PermissionSet
	if err := node.Decode(&ps); err != nil {
		return nil, fmt.Errorf("decode permissions section: %w", err)
	}
	return &ps, nil
}

// Merge combines multiple PermissionSets into a single resolved set.
// Defaults are passed first, then spec-level overrides. Later entries with
// the same dedup key as earlier entries are skipped (first wins within each
// insertion order). This is the canonical permission merge operation,
// relocated from userconfig/config/contract_resolve.go (Slice 8).
func Merge(sets ...*PermissionSet) PermissionSet {
	var out PermissionSet

	seenFS := make(map[string]bool)
	seenExec := make(map[string]bool)
	seenNet := make(map[string]bool)
	seenCap := make(map[string]bool)
	seenIPC := make(map[string]bool)

	for _, set := range sets {
		if set == nil {
			continue
		}
		for _, p := range set.FileSystem {
			key := string(p.Action) + ":" + p.Path
			if seenFS[key] {
				continue
			}
			seenFS[key] = true
			out.FileSystem = append(out.FileSystem, p)
		}
		for _, p := range set.Executables {
			key := p.Binary + ":" + joinArgsEnv(p) + ":" + p.Checksum
			if p.HITLRequired {
				key += ":hitl"
			}
			if p.ProxyRequired {
				key += ":proxy"
			}
			if seenExec[key] {
				continue
			}
			seenExec[key] = true
			out.Executables = append(out.Executables, p)
		}
		for _, p := range set.Network {
			key := p.Direction + ":" + p.Protocol + ":" + p.Host + ":" + fmt.Sprint(p.Port)
			if p.HITLRequired {
				key += ":hitl"
			}
			if seenNet[key] {
				continue
			}
			seenNet[key] = true
			out.Network = append(out.Network, p)
		}
		for _, p := range set.Capabilities {
			key := p.Capability
			if seenCap[key] {
				continue
			}
			seenCap[key] = true
			out.Capabilities = append(out.Capabilities, p)
		}
		for _, p := range set.IPC {
			key := p.Kind + ":" + p.Target
			if seenIPC[key] {
				continue
			}
			seenIPC[key] = true
			out.IPC = append(out.IPC, p)
		}
		out.HITLRequired = append(out.HITLRequired, set.HITLRequired...)
	}

	return out
}

// ResolveEffective resolves the effective PermissionSet for an agent by
// merging defaults with spec-level overrides. Any non-nil pointer is
// included; nil pointers are skipped.
func ResolveEffective(defaults, spec *PermissionSet) PermissionSet {
	return Merge(defaults, spec)
}

// ValidateSection is a thin wrapper around ValidatePermissionSet for use as
// a manifest section validator.
func ValidateSection(ps *PermissionSet) error {
	if ps == nil {
		return nil
	}
	return ValidatePermissionSet(ps)
}

func joinArgsEnv(p ExecutablePermission) string {
	parts := make([]string, 0, len(p.Args)+len(p.Env))
	parts = append(parts, p.Args...)
	parts = append(parts, p.Env...)
	out := ""
	for _, part := range parts {
		out += "|" + part
	}
	return out
}
