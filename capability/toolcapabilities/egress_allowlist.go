package toolcapabilities

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/netpolicy"
)

// AllowlistValidation reports problems with a manifest's egress allowlists.
// Errors block the manifest; warnings flag dead or risky configuration.
type AllowlistValidation struct {
	Errors   []string
	Warnings []string
}

// ValidateNetworkAllowlists checks execution.sandbox.allow_hosts and
// execution.sandbox.allow_private_hosts against the canonical host classifier.
//
//   - a non-public IP literal in allow_hosts is a hard error: the mandatory
//     denylist cannot be bypassed through allow_hosts;
//   - a host present in both lists is a hard error (contradiction);
//   - an allowlist declared without network_access is dead config (warning).
//
// Hostnames are not resolved here (no I/O at load). They are classified
// per-invocation by the egress scanner, which fails closed on resolution
// failure.
//
// It lives in the capability layer because userconfig may not import
// governance (that edge would form a governance → userconfig → governance
// domain cycle); the capability build path is the manifest's admission point.
func ValidateNetworkAllowlists(m *ToolManifest) AllowlistValidation {
	var v AllowlistValidation
	if m == nil || m.Execution.Sandbox == nil {
		return v
	}
	sb := m.Execution.Sandbox
	allow := normalizeHostSet(sb.AllowHosts)
	private := normalizeHostSet(sb.AllowPrivateHosts)

	for _, host := range sb.AllowHosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if class, literal := netpolicy.ClassifyToken(host); literal && class != netpolicy.ClassPublic {
			v.Errors = append(v.Errors, fmt.Sprintf(
				"execution.sandbox.allow_hosts entry %q is %s — use allow_private_hosts (HITL-bound)", host, class))
		}
	}

	var both []string
	for host := range allow {
		if _, ok := private[host]; ok {
			both = append(both, host)
		}
	}
	sort.Strings(both)
	for _, host := range both {
		v.Errors = append(v.Errors, fmt.Sprintf("host %q appears in both allow_hosts and allow_private_hosts", host))
	}

	if !sb.NetworkAccess && (len(allow) > 0 || len(private) > 0) {
		v.Warnings = append(v.Warnings, "execution.sandbox allowlists declared without network_access (dead config)")
	}

	sort.Strings(v.Errors)
	return v
}

// normalizeHostSet lowercases and trims a host list into a set for
// intersection checks.
func normalizeHostSet(hosts []string) map[string]struct{} {
	if len(hosts) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			set[host] = struct{}{}
		}
	}
	return set
}
