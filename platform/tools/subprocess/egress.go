package subprocess

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/netpolicy"
)

// Egress effect values returned by checkEgress.
const (
	EgressAllow           = "allow"
	EgressDeny            = "deny"
	EgressRequireApproval = "require_approval"
)

// SandboxSpec is the egress-relevant slice of a tool manifest's sandbox plus
// the effective container isolation. It is the input to checkEgress.
type SandboxSpec struct {
	// NetworkAccess mirrors manifest execution.sandbox.network_access.
	NetworkAccess bool
	// NetworkIsolation is the effective container isolation. When false the
	// scanner runs for every command with no allowlist bypass (D-15).
	NetworkIsolation bool
	// AllowHosts are public egress targets granted without prompting.
	AllowHosts []string
	// AllowPrivateHosts are non-public egress targets that require HITL.
	AllowPrivateHosts []string
}

// EgressDecision is the typed result of the egress scan.
type EgressDecision struct {
	Effect string   // EgressAllow | EgressDeny | EgressRequireApproval
	Hosts  []string // offending hosts, or hosts needing approval
	Reason string
}

// PrivateEgressApprover resolves a private-egress (or unisolated public-egress)
// approval through the governance HITL broker. Implementations are wired at the
// composition root over governance/authorization.PermissionManager.
type PrivateEgressApprover interface {
	ApprovePrivateEgress(ctx context.Context, agentID string, hosts []string) error
}

// proxyEnvKeys are the environment variables whose values carry an egress
// carrier (a proxy URL or, for NO_PROXY, a bypass host list).
var proxyEnvKeys = map[string]struct{}{
	"http_proxy":  {},
	"https_proxy": {},
	"all_proxy":   {},
	"ftp_proxy":   {},
	"ws_proxy":    {},
	"wss_proxy":   {},
	"no_proxy":    {},
}

// checkEgress scans every command token and proxy environment value for network
// targets and returns a typed decision.
//
// Evaluation order (D-4): the mandatory denylist is evaluated before any
// allowlist. A non-public target is denied unless it is declared in
// allow_private_hosts, in which case it requires HITL approval. Public targets
// in allow_hosts are granted; when isolation is off the allowlists are ignored
// and every public target requires approval (the scanner is the only boundary).
// An unresolved hostname is a denial (fail closed), with one exception: a bare
// single-label token (D10) is only a host if it resolves, so an unresolvable
// bare token is ignored rather than denied.
func checkEgress(ctx context.Context, spec SandboxSpec, env, cmd []string) EgressDecision {
	if !spec.NetworkAccess && spec.NetworkIsolation {
		return EgressDecision{Effect: EgressAllow}
	}

	allowHosts := hostSet(spec.AllowHosts)
	allowPrivate := hostSet(spec.AllowPrivateHosts)
	toolDefaultDenies := false
	if !spec.NetworkIsolation {
		allowHosts = nil
		allowPrivate = nil
		toolDefaultDenies = true
	}

	var denied []string
	var pending []string

	scan := func(candidate hostCandidate) {
		class, err := classifyEgressHost(ctx, candidate.name)
		if err != nil {
			if candidate.bare {
				// An unresolvable scheme-less label (e.g. an ordinary argv
				// argument) is not a host claim: ignore it rather than failing
				// closed, which would block every bare-word argument (D10).
				return
			}
			denied = append(denied, candidate.name)
			return
		}
		key := strings.ToLower(candidate.name)
		if class != netpolicy.ClassPublic {
			if _, ok := allowPrivate[key]; ok {
				pending = append(pending, candidate.name)
				return
			}
			denied = append(denied, candidate.name)
			return
		}
		if _, ok := allowHosts[key]; !ok && toolDefaultDenies {
			pending = append(pending, candidate.name)
		}
	}

	for _, token := range cmd {
		for _, candidate := range extractHostCandidates(token) {
			scan(candidate)
		}
	}
	for _, entry := range env {
		for _, host := range proxyEnvHosts(entry) {
			scan(hostCandidate{name: host})
		}
	}

	if len(denied) > 0 {
		hosts := dedupeSorted(denied)
		return EgressDecision{
			Effect: EgressDeny,
			Hosts:  hosts,
			Reason: fmt.Sprintf(
				"network egress to %s denied: private, loopback, link-local, unspecified, reserved, and unresolved hosts are blocked (SSRF protection)",
				strings.Join(hosts, ", ")),
		}
	}
	if len(pending) > 0 {
		hosts := dedupeSorted(pending)
		return EgressDecision{
			Effect: EgressRequireApproval,
			Hosts:  hosts,
			Reason: fmt.Sprintf("network egress to %s requires approval", strings.Join(hosts, ", ")),
		}
	}
	return EgressDecision{Effect: EgressAllow}
}

// classifyEgressHost classifies a host token through the canonical netpolicy
// classifier with the fail-closed resolution policy. The caller's context
// bounds and cancels resolution (D11).
func classifyEgressHost(ctx context.Context, host string) (netpolicy.HostClass, error) {
	if ip, ok := netpolicy.ParseHostToken(host); ok {
		return netpolicy.ClassifyIP(ip), nil
	}
	target, err := netpolicy.ResolveTarget(ctx, host, netpolicy.DefaultResolveOptions())
	if err != nil {
		return "", err
	}
	return target.Class, nil
}

// hostCandidate is one host token extracted from an argv entry. bare marks a
// scheme-less single-label token (D10): it is a host only if it resolves.
// Dotted, scheme-qualified, and literal candidates are committed host claims
// and fail closed on resolution failure.
type hostCandidate struct {
	name string
	bare bool
}

// extractHostCandidates pulls zero or one host out of a single argv token. It
// understands full URLs (scheme://host[:port]/...), host:port pairs, bracketed
// IPv6, bare hosts, and config carriers of the form k=value where value is a
// URL or a dotted host — e.g. `git -c http.proxy=http://127.0.0.1:9` or
// `--url=http://10.0.0.1/`.
//
// Tokens that are not host-like return nil. A scheme:// token is always
// treated as a host (even single-label), because the scheme disambiguates it.
// A standalone single-label token (RFC-1123 label, no scheme, not
// flag-prefixed) is returned as a bare candidate: the scanner resolves it and
// treats resolution failure as "not a host", so ordinary argv words are not
// blocked (D10).
func extractHostCandidates(token string) []hostCandidate {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	// A genuine URL parses as a whole (its query may contain '=').
	if idx := strings.Index(token, "://"); idx >= 0 && !strings.Contains(token[:idx], "=") {
		if u, err := url.Parse(token); err == nil && u.Hostname() != "" {
			return []hostCandidate{{name: u.Hostname()}}
		}
	}
	// k=value carrier (flag/config): recurse on the value.
	if i := strings.IndexByte(token, '='); i >= 0 {
		value := strings.TrimSpace(token[i+1:])
		if host := extractHost(value); host != "" && (strings.Contains(value, "://") || looksLikeHost(host)) {
			return []hostCandidate{{name: host}}
		}
	}
	// Bare host, host:port, or scheme-less single label.
	if host := extractHost(token); host != "" {
		if looksLikeHost(host) {
			return []hostCandidate{{name: host}}
		}
		if looksLikeBareLabel(host) {
			return []hostCandidate{{name: host, bare: true}}
		}
	}
	return nil
}

// proxyEnvHosts extracts zero or more hosts from an environment entry whose key
// is a proxy variable. NO_PROXY-style comma lists are split.
func proxyEnvHosts(entry string) []string {
	key, value, ok := strings.Cut(entry, "=")
	if !ok {
		return nil
	}
	if _, isProxy := proxyEnvKeys[strings.ToLower(strings.TrimSpace(key))]; !isProxy {
		return nil
	}
	var hosts []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if host := extractHost(part); host != "" && (strings.Contains(part, "://") || looksLikeHost(host)) {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// hostSet lowercases and trims an allowlist into a set.
func hostSet(hosts []string) map[string]struct{} {
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

// looksLikeHost reports whether host is an IP literal or a plausible hostname.
func looksLikeHost(host string) bool {
	if host == "" {
		return false
	}
	if _, ok := netpolicy.ParseHostToken(host); ok {
		return true
	}
	return looksLikeHostname(host)
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
		if !isHostnameLabel(label, true) {
			return false
		}
	}
	return true
}

// isHostnameLabel reports whether label contains only characters legal in a DNS
// label under the scanner's syntax. allowUnderscore admits the legacy
// underscore spelling that dotted hostnames tolerate; bare labels stay strict
// RFC-1123 (D10).
func isHostnameLabel(label string, allowUnderscore bool) bool {
	for i := 0; i < len(label); i++ {
		c := label[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-':
		case allowUnderscore && c == '_':
		default:
			return false
		}
	}
	return true
}

// looksLikeBareLabel reports whether s is a scheme-less RFC-1123 single-label
// token: 1-63 characters drawn from [A-Za-z0-9-], with no leading or trailing
// hyphen and no dot, colon, or whitespace. Such a token is only a host
// candidate if it resolves (D10); resolution failure means it is an ordinary
// argv argument, not a host claim.
func looksLikeBareLabel(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	if strings.ContainsAny(s, " \t.:") {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	return isHostnameLabel(s, false)
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

func dedupeSorted(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
