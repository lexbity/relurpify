package subprocess

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/governance/netpolicy"
)

// checkEgress returns an error if the command args reference a blocked
// network host. allowHosts is an optional per-tool allowlist consulted before
// the mandatory denylist (ordering is inverted in a follow-on change); hosts
// in this list are skipped. Returns nil when no network target is blocked.
func checkEgress(allowHosts []string, cmd []string) error {
	if host := firstBlockedEgressHost(cmd, allowHosts); host != "" {
		return fmt.Errorf(
			"network egress to %q denied: private, loopback, and link-local addresses are blocked (SSRF protection)",
			host,
		)
	}
	return nil
}

// isNetworkTool reports whether a manifest declares network access and must
// therefore have its target hosts screened against the SSRF denylist.
func isNetworkTool(manifest ports.ToolManifest) bool {
	return manifest.Execution.Sandbox != nil && manifest.Execution.Sandbox.NetworkAccess
}

// firstBlockedEgressHost scans CLI arguments for a network target (a URL or a
// bare host[:port]) whose classification is non-public and returns it. An empty
// string means no blocked host was found.
//
// Classification is delegated to the canonical netpolicy classifier and is
// fail-closed: a hostname that cannot be resolved is treated as blocked. The
// allowlist is consulted first (legacy ordering, superseded by the
// denylist-first inversion).
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
		if _, allowed := allowSet[strings.ToLower(host)]; allowed {
			continue
		}
		if isBlockedEgressHost(host, strings.Contains(arg, "://")) {
			return host
		}
	}
	return ""
}

// isBlockedEgressHost reports whether a host token is blocked by the mandatory
// denylist. IP literals are classified without I/O; hostnames are resolved with
// the canonical resolver and a resolution failure blocks the target.
//
// urlContext marks a token that was unambiguously a host (scheme://host/...),
// which is resolved even when it carries no dot; otherwise a syntactic gate
// keeps arbitrary argv tokens from being treated as hostnames.
func isBlockedEgressHost(host string, urlContext bool) bool {
	if _, ok := netpolicy.ParseHostToken(host); !ok {
		if !urlContext && !looksLikeHostname(host) {
			return false
		}
	}
	return sandbox.ClassifyEgressTarget(host, func(token string) (netpolicy.Target, error) {
		return netpolicy.ResolveTarget(context.Background(), token, netpolicy.DefaultResolveOptions())
	}) != nil
}

// looksLikeHostname is a syntactic gate that keeps arbitrary argv tokens (flag
// values, header snippets) from being treated as hostnames and resolved. A
// token must carry a DNS label separator and contain no whitespace or leftover
// port colon to be considered a hostname candidate.
func looksLikeHostname(host string) bool {
	if host == "" || strings.ContainsAny(host, " \t") || strings.Contains(host, ":") {
		return false
	}
	if host == "localhost" {
		return true
	}
	if !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			continue
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			switch {
			case c >= 'a' && c <= 'z':
			case c >= 'A' && c <= 'Z':
			case c >= '0' && c <= '9':
			case c == '-' || c == '_':
			default:
				return false
			}
		}
	}
	return true
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
