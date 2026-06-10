package authorization

import (
	"context"
	"fmt"
	"net"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

// isPrivateOrLoopbackHost checks if the host is a private/loopback address.
func isPrivateOrLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	if ip != nil {
		return isPrivateIP(ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true
		}
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	return false
}

// CheckNetwork validates network access.
func (m *PermissionManager) CheckNetwork(ctx context.Context, agentID string, direction string, protocol string, host string, port int) error {
	// Hard mandatory block: private, loopback, and link-local IPs are never
	// reachable regardless of agent configuration. This prevents SSRF to
	// cloud metadata services, localhost services, and internal networks.
	// The denylist is owned and enforced by the sandbox package.
	if isPrivateOrLoopbackHost(host) {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeNetwork,
			Action:   fmt.Sprintf("net:%s:%s:%s:%d", direction, protocol, host, port),
			Resource: host,
		}, "private, loopback, or link-local addresses are blocked")
	}
	perm := m.findNetworkPermission(direction, protocol, host, port)
	if perm == nil {
		desc := permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeNetwork,
			Action:   fmt.Sprintf("net:%s:%s:%s:%d", direction, protocol, host, port),
			Resource: host,
		}
		switch m.effectiveDefaultPolicy() {
		case "deny":
			return m.deny(ctx, agentID, desc, "network scope missing")
		default: // AgentPermissionAsk (Allow is rejected at registration time)
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeNetwork,
			Action:       fmt.Sprintf("net:%s:%s", direction, protocol),
			Resource:     fmt.Sprintf("%s:%d", host, port),
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeNetwork,
		Action:   fmt.Sprintf("net:%s", direction),
		Resource: fmt.Sprintf("%s:%d", host, port),
	}, "granted", nil)
	m.recordNetworkRule(direction, protocol, host, port)
	return nil
}

// recordNetworkRule stores approved network scopes and forwards them to the
// sandbox runtime so OS-level enforcement mirrors permission checks.
func (m *PermissionManager) recordNetworkRule(direction, protocol, host string, port int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rule := governanceports.SandboxNetworkRule{
		Direction: direction,
		Protocol:  protocol,
		Host:      host,
		Port:      port,
	}
	m.netPolicy = append(m.netPolicy, rule)
	m.applyRuntimePolicyLocked()
}

// findNetworkPermission resolves whether the host/port pair is authorized for
// the given direction/protocol combination.
func (m *PermissionManager) findNetworkPermission(direction, protocol, host string, port int) *permissions.NetworkPermission {
	if m == nil || m.declared == nil {
		return nil
	}
	target := fmt.Sprintf("%s:%d", host, port)
	for _, perm := range m.declared.Network {
		if perm.Direction != direction || perm.Protocol != protocol {
			continue
		}
		if perm.Direction == "egress" {
			if perm.Port != 0 && perm.Port != port {
				continue
			}
			if perm.Host == host || perm.Host == permissionMatchAll || matchGlob(perm.Host, host) {
				return &perm
			}
		} else if perm.Direction == "ingress" {
			if perm.Port == port || perm.Port == 0 {
				return &perm
			}
		} else if perm.Direction == "dns" && perm.Host == "" {
			return &perm
		}
		if perm.Host == target {
			return &perm
		}
	}
	return nil
}
