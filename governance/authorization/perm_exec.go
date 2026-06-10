package authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// CheckExecutable validates binary execution.
func (m *PermissionManager) CheckExecutable(ctx context.Context, agentID, binary string, args []string, env []string) error {
	if m == nil {
		return errors.New("permission manager missing")
	}
	perm := m.findExecutablePermission(binary)
	if perm == nil {
		desc := permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeExecutable,
			Action:   fmt.Sprintf("exec:binary:%s", binary),
			Resource: binary,
		}
		switch m.effectiveDefaultPolicy() {
		case "deny":
			return m.deny(ctx, agentID, desc, "binary not declared")
		default: // AgentPermissionAsk (Allow is rejected at registration time)
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if len(perm.Args) > 0 && !matchArgs(perm.Args, args) {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeExecutable,
			Action:   fmt.Sprintf("exec:args:%s", strings.Join(args, " ")),
			Resource: binary,
		}, "arguments rejected")
	}
	if len(perm.Env) > 0 && !matchEnv(perm.Env, env) {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeExecutable,
			Action:   "exec:env",
			Resource: binary,
		}, "environment rejected")
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeExecutable,
			Action:       fmt.Sprintf("exec:binary:%s", binary),
			Resource:     binary,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeExecutable,
		Action:   fmt.Sprintf("exec:%s", binary),
		Resource: binary,
	}, "granted", map[string]any{
		"args": args,
		"env":  env,
	})
	return nil
}

// findExecutablePermission locates the manifest entry authorizing a binary.
func (m *PermissionManager) findExecutablePermission(binary string) *permissions.ExecutablePermission {
	if m == nil || m.declared == nil {
		return nil
	}
	cacheKey := strings.TrimSpace(binary)
	m.mu.RLock()
	if perm, ok := m.execPermCache[cacheKey]; ok {
		m.mu.RUnlock()
		return perm
	}
	m.mu.RUnlock()
	var matched *permissions.ExecutablePermission
	for _, perm := range m.declared.Executables {
		if perm.Binary == binary {
			permCopy := perm
			matched = &permCopy
			break
		}
	}
	m.mu.Lock()
	m.execPermCache[cacheKey] = matched
	m.mu.Unlock()
	return matched
}

// matchArgs compares declared argument patterns with a runtime invocation while
// supporting simple globbing for flags.
func matchArgs(patterns, args []string) bool {
	if len(patterns) == 0 {
		return true
	}
	if len(patterns) == 1 && patterns[0] == "*" {
		return true
	}
	hasTrailingWildcard := len(patterns) > 0 && patterns[len(patterns)-1] == "*"
	if !hasTrailingWildcard && len(patterns) != len(args) {
		return false
	}
	if hasTrailingWildcard && len(args) < len(patterns)-1 {
		return false
	}
	limit := len(patterns)
	if len(args) < limit {
		limit = len(args)
	}
	for i := 0; i < limit; i++ {
		pattern := patterns[i]
		if hasTrailingWildcard && i == len(patterns)-1 {
			break
		}
		if pattern == "*" {
			continue
		}
		if strings.HasPrefix(pattern, "--") && strings.HasSuffix(pattern, "*") {
			if !strings.HasPrefix(args[i], strings.TrimSuffix(pattern, "*")) {
				return false
			}
			continue
		}
		if pattern != args[i] {
			return false
		}
	}
	return true
}

// matchEnv verifies required environment variables match the expected values or
// contain wildcards where any value is acceptable.
func matchEnv(patterns, env []string) bool {
	if len(patterns) == 0 {
		return true
	}
	m := map[string]string{}
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	for _, pattern := range patterns {
		parts := strings.SplitN(pattern, "=", 2)
		if len(parts) != 2 {
			continue
		}
		val, ok := m[parts[0]]
		if !ok {
			return false
		}
		if parts[1] != "*" && parts[1] != val {
			return false
		}
	}
	return true
}
