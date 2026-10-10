package permissions

import (
	"testing"

	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// The governance/permissions package owns behavior only. The declarative
// permission vocabulary is owned by userconfig/permissions and imported
// directly — no aliases, no re-exports. The var block below pins the
// package's exported surface: every name here is governance-owned behavior,
// and none of them may collide with a ucperms vocabulary name (the compiler
// rejects a duplicate declaration, so re-introducing an alias type named
// PermissionSet, PermissionDescriptor, … fails the build).
var (
	_ PermissionDeniedError    = PermissionDeniedError{}
	_ FilePermissionChecker    = (FilePermissionChecker)(nil)
	_ NetworkPermissionChecker = (NetworkPermissionChecker)(nil)
	_ CapabilityChecker        = (CapabilityChecker)(nil)
	_ Decision                 = DecisionAllow
	_ *PermissionManager       = (*PermissionManager)(nil)
)

// TestMergeAndResolveWithoutAliases proves the ucperms vocabulary flows
// through governance behavior (Merge/ResolveEffective) without any alias
// bridge: literals are spelled ucperms.-side and the governance functions
// return ucperms types.
func TestMergeAndResolveWithoutAliases(t *testing.T) {
	defaults := &ucperms.PermissionSet{
		FileSystem: []ucperms.FileSystemPermission{
			{Action: ucperms.FileSystemRead, Path: "/workspace"},
		},
		Executables: []ucperms.ExecutablePermission{
			{Binary: "go", Args: []string{"test"}},
		},
	}
	spec := &ucperms.PermissionSet{
		FileSystem: []ucperms.FileSystemPermission{
			{Action: ucperms.FileSystemWrite, Path: "/workspace/out"},
		},
		Network: []ucperms.NetworkPermission{
			{Direction: "outbound", Protocol: "https", Host: "example.com"},
		},
	}

	merged := Merge(defaults, spec)
	if len(merged.FileSystem) != 2 {
		t.Errorf("want 2 filesystem entries after merge, got %d", len(merged.FileSystem))
	}
	if len(merged.Executables) != 1 {
		t.Errorf("want 1 executable entry after merge, got %d", len(merged.Executables))
	}

	resolved := ResolveEffective(defaults, spec)
	if len(resolved.FileSystem) != 2 {
		t.Errorf("want 2 filesystem entries after resolve, got %d", len(resolved.FileSystem))
	}
	if len(resolved.Network) != 1 {
		t.Errorf("want spec network grant to override defaults, got %d", len(resolved.Network))
	}

	if err := ValidateSection(&resolved); err != nil {
		t.Errorf("resolved section must validate: %v", err)
	}
}
