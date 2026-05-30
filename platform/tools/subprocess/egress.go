package subprocess

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// isNetworkTool reports whether a manifest declares network access and must
// therefore have its target hosts screened against the SSRF denylist.
func isNetworkTool(manifest contracts.ToolManifest) bool {
	return manifest.Execution.Sandbox != nil && manifest.Execution.Sandbox.NetworkAccess
}

// firstBlockedEgressHost scans CLI arguments for a network target (a URL or a
// bare host[:port]) that resolves to a private, loopback, or link-local address
// and returns it. An empty string means no blocked host was found.
//
// allowHosts is an optional allowlist: hosts that match any entry here are
// never blocked even if they would otherwise be denied by the mandatory
// denylist (contracts.IsPrivateOrLoopbackHost).
func firstBlockedEgressHost(args []string, allowHosts []string) string {
	allowSet := make(map[string]struct{}, len(allowHosts))
	for _, h := range allowHosts {
		h = strings.TrimSpace(strings.ToLower(h))
		if h != "" {
			allowSet[h] = struct{}{}
		}
	}

	for _, arg := range args {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue // skip flags; flag values are handled as their own args
		}
		host := extractHost(arg)
		if host == "" {
			continue
		}
		// Check allowlist first.
		if _, allowed := allowSet[strings.ToLower(host)]; allowed {
			continue
		}
		if contracts.IsPrivateOrLoopbackHost(host) {
			return host
		}
	}
	return ""
}

// extractHost pulls a hostname/IP out of a single CLI argument. It understands
// full URLs (scheme://host[:port]/...), host:port pairs, bracketed IPv6, and
// bare hosts. Arguments that are not host-like return empty string.
func extractHost(arg string) string {
	if strings.Contains(arg, "://") {
		if u, err := url.Parse(arg); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	candidate := arg
	// Drop any path/query/fragment.
	if i := strings.IndexAny(candidate, "/?#"); i >= 0 {
		candidate = candidate[:i]
	}
	// Drop userinfo (user:pass@host).
	if i := strings.LastIndex(candidate, "@"); i >= 0 {
		candidate = candidate[i+1:]
	}
	if candidate == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(candidate); err == nil {
		return host
	}
	// Bare IPv6 without a port may still be bracketed: [::1].
	candidate = strings.TrimPrefix(candidate, "[")
	candidate = strings.TrimSuffix(candidate, "]")
	return candidate
}

// checkEgress returns an error if the command args reference a blocked
// network host and the manifest declares network access. Returns nil
// when the tool has no network access or all targets are allowed.
func checkEgress(manifest contracts.ToolManifest, cmd []string) error {
	if !isNetworkTool(manifest) {
		return nil
	}

	var allowHosts []string
	if manifest.Execution.Sandbox != nil {
		allowHosts = manifest.Execution.Sandbox.AllowHosts
	}

	if host := firstBlockedEgressHost(cmd, allowHosts); host != "" {
		return fmt.Errorf(
			"network egress to %q denied: private, loopback, and link-local addresses are blocked (SSRF protection)",
			host,
		)
	}
	return nil
}
