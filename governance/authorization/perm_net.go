package authorization

import (
	"codeburg.org/lexbit/relurpify/governance/sandbox"
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/governance/netpolicy"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// Network permission directions, named to keep the matcher readable.
const (
	directionEgress  = "egress"
	directionIngress = "ingress"
	directionDNS     = "dns"
)

// ResolveTarget resolves a host token into a policy-layer target using the
// canonical netpolicy budgets. The manager (not the pure Enforcer) owns this
// I/O, so a hostname is classified before it reaches the decision layer.
func (m *PermissionManager) ResolveTarget(ctx context.Context, token string) (netpolicy.Target, error) {
	return netpolicy.ResolveTarget(ctx, token, netpolicy.DefaultResolveOptions())
}

// networkDescriptor builds the audit/policy descriptor for a network target.
func networkDescriptor(direction, protocol, host string, port int) ucperms.PermissionDescriptor {
	return ucperms.PermissionDescriptor{
		Type:     ucperms.PermissionTypeNetwork,
		Action:   fmt.Sprintf("net:%s:%s:%s:%d", direction, protocol, host, port),
		Resource: host,
	}
}

// networkBlockReason names the class that triggered the mandatory denylist.
func networkBlockReason(class netpolicy.HostClass) string {
	return fmt.Sprintf("%s addresses are blocked (ssrf protection)", class)
}

// CheckNetwork validates network access against an already-classified target.
//
// A non-public class is a hard mandatory block: private, loopback, link-local,
// unspecified, and reserved targets are never reachable regardless of agent
// configuration or allowlists. The class is produced by netpolicy, either
// directly from an IP literal or by explicit resolution in the caller.
func (m *PermissionManager) CheckNetwork(ctx context.Context, agentID string, direction string, protocol string, target netpolicy.Target, port int) error {
	host := target.Token
	if target.Class != netpolicy.ClassPublic {
		return m.deny(ctx, agentID, networkDescriptor(direction, protocol, host, port), networkBlockReason(target.Class))
	}
	perm := m.findNetworkPermission(direction, protocol, host, port)
	if perm == nil {
		desc := networkDescriptor(direction, protocol, host, port)
		switch m.effectiveDefaultDecision() {
		case permissions.DecisionDeny:
			return m.deny(ctx, agentID, desc, "network scope missing")
		default: // DecisionAsk (allow is rejected at registration time)
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, ucperms.PermissionDescriptor{
			Type:         ucperms.PermissionTypeNetwork,
			Action:       fmt.Sprintf("net:%s:%s", direction, protocol),
			Resource:     fmt.Sprintf("%s:%d", host, port),
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	if err := m.log(ctx, agentID, ucperms.PermissionDescriptor{
		Type:     ucperms.PermissionTypeNetwork,
		Action:   fmt.Sprintf("net:%s", direction),
		Resource: fmt.Sprintf("%s:%d", host, port),
	}, "granted", nil); err != nil {
		return err
	}
	m.recordNetworkRule(ctx, direction, protocol, host, port)
	return nil
}

// recordNetworkRule stores approved network scopes and forwards them to the
// sandbox runtime so OS-level enforcement mirrors permission checks.
func (m *PermissionManager) recordNetworkRule(ctx context.Context, direction, protocol, host string, port int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rule := sandbox.NetworkRule{
		Direction: direction,
		Protocol:  protocol,
		Host:      host,
		Port:      port,
	}
	m.netPolicy = append(m.netPolicy, rule)
	m.applyRuntimePolicyLocked(ctx)
}

// findNetworkPermission resolves whether the host/port pair is authorized for
// the given direction/protocol combination.
func (m *PermissionManager) findNetworkPermission(direction, protocol, host string, port int) *ucperms.NetworkPermission {
	if m == nil || m.declared == nil {
		return nil
	}
	target := fmt.Sprintf("%s:%d", host, port)
	for _, perm := range m.declared.Network {
		if perm.Direction != direction || perm.Protocol != protocol {
			continue
		}
		switch {
		case perm.Direction == directionEgress:
			if perm.Port != 0 && perm.Port != port {
				continue
			}
			if perm.Host == host || perm.Host == permissionMatchAll || matchGlob(perm.Host, host) {
				return &perm
			}
		case perm.Direction == directionIngress:
			if perm.Port == port || perm.Port == 0 {
				return &perm
			}
		case perm.Direction == directionDNS && perm.Host == "":
			return &perm
		}
		if perm.Host == target {
			return &perm
		}
	}
	return nil
}
