package command

import (
	"net"
	"net/url"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// hasNetworkTag reports whether the tool is part of the network family and must
// therefore have its target hosts screened against the SSRF denylist.
func hasNetworkTag(tags []string) bool {
	for _, tag := range tags {
		if tag == contracts.TagNetwork {
			return true
		}
	}
	return false
}

// firstBlockedEgressHost scans CLI arguments for a network target (a URL or a
// bare host[:port]) that resolves to a private, loopback, or link-local address
// and returns it. An empty string means no blocked host was found.
//
// This is the SF-1 SSRF guard applied to network-tagged tools (curl, wget, nc,
// dig, ping, ...). The denylist itself is owned by the sandbox contract layer
// (contracts.IsPrivateOrLoopbackHost); enforcing it here, before the command
// reaches the runner, blocks ad-hoc tool calls to cloud-metadata endpoints and
// internal services even when the sandbox network is not fully closed.
func firstBlockedEgressHost(args []string) string {
	for _, arg := range args {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue // skip flags; flag values are handled as their own args
		}
		host := extractHost(arg)
		if host != "" && contracts.IsPrivateOrLoopbackHost(host) {
			return host
		}
	}
	return ""
}

// extractHost pulls a hostname/IP out of a single CLI argument. It understands
// full URLs (scheme://host[:port]/...), host:port pairs, bracketed IPv6, and
// bare hosts. Arguments that are not host-like resolve to a value that the
// denylist treats as public (unresolvable), so they are not blocked.
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
