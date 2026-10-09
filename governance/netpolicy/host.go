// Package netpolicy owns the canonical classification of network host tokens
// and the one resolution policy used by the governance enforcement planes.
//
// It is stdlib-only and therefore importable by every domain that needs to
// decide whether a host is safe to dial (governance, capability, platform).
// Classification is pure; DNS resolution is an explicit, separately-budgeted
// operation (resolve.go) that the pure Enforcer path never performs.
package netpolicy

import (
	"net"
	"strings"
)

// HostClass is the security classification of a network host token.
type HostClass string

const (
	// ClassLoopback is the local host (127.0.0.0/8, ::1).
	ClassLoopback HostClass = "loopback"
	// ClassLinkLocal is a link-local address (169.254.0.0/16, fe80::/10),
	// including the cloud metadata endpoint 169.254.169.254.
	ClassLinkLocal HostClass = "link-local"
	// ClassPrivate is an RFC-1918 / RFC-4193 private address.
	ClassPrivate HostClass = "private"
	// ClassUnspecified is the unspecified address (0.0.0.0, ::).
	ClassUnspecified HostClass = "unspecified"
	// ClassReserved is a special-purpose range that is not globally routable
	// (CGNAT, documentation, benchmarking, multicast, broadcast).
	ClassReserved HostClass = "reserved"
	// ClassPublic is a globally routable address.
	ClassPublic HostClass = "public"
)

// Target is a resolved (or literal) egress destination carried from the
// resolver to the pure policy layer. Class is never empty for a Target built
// by ResolveTarget: an unresolved name is an error, not a Target.
type Target struct {
	// Token is the original host token, used for policy matching and audit.
	Token string
	// Class is the security classification of the destination.
	Class HostClass
	// Literal is true when Token was an IP literal and no resolution occurred.
	Literal bool
}

// classSeverity gives the "most restrictive wins" ordering used when a name
// resolves to several addresses.
var classSeverity = map[HostClass]int{
	ClassPublic:      0,
	ClassReserved:    1,
	ClassPrivate:     2,
	ClassUnspecified: 3,
	ClassLinkLocal:   4,
	ClassLoopback:    5,
}

// unspecifiedBlocks are the "this network" ranges that are not covered by
// ip.IsUnspecified (which matches only the all-zero address).
var unspecifiedBlocks = mustParseCIDRs(
	"0.0.0.0/8",
)

// reservedBlocks are the non-public ranges that are not covered by the
// net.IP predicate helpers (IsPrivate/IsLoopback/IsLinkLocal*). Loopback,
// link-local and private ranges are handled by predicates before this table
// is consulted.
var reservedBlocks = mustParseCIDRs(
	"100.64.0.0/10",   // CGNAT (RFC 6598)
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // TEST-NET-1
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // TEST-NET-2
	"203.0.113.0/24",  // TEST-NET-3
	"224.0.0.0/4",     // multicast
	"240.0.0.0/4",     // reserved (incl. 255.255.255.255)
	"2001:db8::/32",   // documentation
	"ff00::/8",        // IPv6 multicast
)

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("netpolicy: invalid reserved CIDR " + cidr + ": " + err.Error())
		}
		out = append(out, block)
	}
	return out
}

// ClassifyIP returns the security class of an address.
//
// The class table (normative):
//
//	127.0.0.0/8, ::1/128, ip.IsLoopback()                        → loopback
//	169.254.0.0/16, fe80::/10, is-link-local-unicast/multicast   → link-local
//	10/8, 172.16/12, 192.168/16, fc00::/7, ip.IsPrivate()        → private
//	0.0.0.0/8, ::/128, ip.IsUnspecified()                        → unspecified
//	100.64/10, 192.0.0/24, 192.0.2/24, 198.51.100/24,
//	203.0.113/24, 198.18/15, 224/4, 240/4, 2001:db8::/32, ff00::/8 → reserved
//	everything else                                              → public
func ClassifyIP(ip net.IP) HostClass {
	if ip == nil {
		return ClassUnspecified
	}
	switch {
	case ip.IsUnspecified():
		return ClassUnspecified
	case ip.IsLoopback():
		return ClassLoopback
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return ClassLinkLocal
	case ip.IsPrivate():
		return ClassPrivate
	}
	for _, block := range unspecifiedBlocks {
		if block.Contains(ip) {
			return ClassUnspecified
		}
	}
	for _, block := range reservedBlocks {
		if block.Contains(ip) {
			return ClassReserved
		}
	}
	return ClassPublic
}

// ClassifyToken classifies a host token. It returns (class, true) when token
// is an IP literal and (zero, false) when token is a hostname. It performs no
// I/O: hostnames are resolved separately via ResolveHost.
//
// The IPv4-literal path allocates nothing.
func ClassifyToken(token string) (HostClass, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	if strings.HasPrefix(token, "[") {
		if !strings.HasSuffix(token, "]") || len(token) < 2 {
			return "", false
		}
		token = token[1 : len(token)-1]
	}
	if i := strings.LastIndexByte(token, '%'); i >= 0 {
		if !validZone(token[i+1:]) {
			return "", false
		}
		token = token[:i]
	}
	if strings.ContainsAny(token, "/-") {
		return "", false
	}
	if !strings.Contains(token, ":") {
		if v, ok := parseInetAtonU32(token); ok {
			return classifyIPv4(v), true
		}
	}
	ip := net.ParseIP(token)
	if ip == nil {
		return "", false
	}
	return ClassifyIP(ip), true
}

// classifyIPv4 returns the class of an IPv4 address given as a big-endian
// uint32. It allocates nothing.
func classifyIPv4(v uint32) HostClass {
	switch {
	case v == 0, v>>24 == 0: // 0.0.0.0/8 "this network"
		return ClassUnspecified
	case v>>24 == 127:
		return ClassLoopback
	case v>>16 == 0xa9fe, // 169.254.0.0/16
		v&0xffffff00 == 0xe0000000: // 224.0.0.0/24 link-local multicast
		return ClassLinkLocal
	case v&0xff000000 == 0x0a000000, // 10.0.0.0/8
		v&0xfff00000 == 0xac100000, // 172.16.0.0/12
		v&0xffff0000 == 0xc0a80000: // 192.168.0.0/16
		return ClassPrivate
	case v&0xffc00000 == 0x64400000, // 100.64.0.0/10
		v&0xffffff00 == 0xc0000000, // 192.0.0.0/24
		v&0xffffff00 == 0xc0000200, // 192.0.2.0/24
		v&0xfffe0000 == 0xc6120000, // 198.18.0.0/15
		v&0xffffff00 == 0xc6336400, // 198.51.100.0/24
		v&0xffffff00 == 0xcb007100, // 203.0.113.0/24
		v&0xf0000000 == 0xe0000000, // 224.0.0.0/4 remainder (multicast)
		v&0xf0000000 == 0xf0000000: // 240.0.0.0/4
		return ClassReserved
	default:
		return ClassPublic
	}
}

// ParseHostToken parses an IP literal in every spelling the consumer side can
// dereference, and reports false for anything that is a hostname.
//
// The grammar is a superset of net.ParseIP:
//
//   - dotted quad                      192.168.0.1
//   - inet_aton forms (1-4 parts)      127.1, 2130706433, 0x7f000001, 0177.0.0.1
//   - IPv4-mapped IPv6                 ::ffff:127.0.0.1
//   - IPv6 with zone                   fe80::1%eth0
//   - bracketed IPv6                   [::1]
//
// Tokens containing a port, a CIDR slash, or a hyphen are not host tokens and
// return false. Octal parts with digits 8 or 9 are invalid (as in inet_aton)
// and fall through to the hostname path.
func ParseHostToken(token string) (net.IP, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}
	if strings.HasPrefix(token, "[") {
		if !strings.HasSuffix(token, "]") || len(token) < 2 {
			return nil, false
		}
		token = token[1 : len(token)-1]
	}
	if i := strings.LastIndexByte(token, '%'); i >= 0 {
		if !validZone(token[i+1:]) {
			return nil, false
		}
		token = token[:i]
	}
	if strings.ContainsAny(token, "/-") {
		return nil, false
	}
	if !strings.Contains(token, ":") {
		if ip := parseInetAton(token); ip != nil {
			return ip, true
		}
	}
	if ip := net.ParseIP(token); ip != nil {
		return ip, true
	}
	return nil, false
}

// validZone reports whether s is a non-empty IPv6 zone identifier.
func validZone(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-' || c == '.':
		default:
			return false
		}
	}
	return true
}

// parseInetAton parses the POSIX inet_aton numeric forms into a net.IP.
func parseInetAton(token string) net.IP {
	v, ok := parseInetAtonU32(token)
	if !ok {
		return nil
	}
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// parseInetAtonU32 parses the POSIX inet_aton numeric forms directly into a
// big-endian uint32. It allocates nothing.
func parseInetAtonU32(token string) (uint32, bool) {
	var vals [4]uint64
	n := 0
	start := 0
	for i := 0; i <= len(token); i++ {
		if i == len(token) || token[i] == '.' {
			if i == start || n >= 4 {
				return 0, false
			}
			v, ok := parseNumericPart(token[start:i])
			if !ok {
				return 0, false
			}
			vals[n] = v
			n++
			start = i + 1
		}
	}
	if n == 0 {
		return 0, false
	}
	for i := 0; i < n-1; i++ {
		if vals[i] > 0xff {
			return 0, false
		}
	}
	shift := uint(8 * (5 - n))
	if vals[n-1] >= uint64(1)<<shift {
		return 0, false
	}
	var v uint64
	for i := 0; i < n-1; i++ {
		v = (v << 8) | vals[i]
	}
	v = (v << shift) | vals[n-1]
	return uint32(v), true
}

// parseNumericPart parses one inet_aton component: decimal, 0x-hex, or
// 0-octal. It rejects empty strings, invalid digits, and overflow past 32 bits.
func parseNumericPart(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	base := 10
	digits := s
	switch {
	case len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X'):
		base = 16
		digits = s[2:]
	case len(s) > 1 && s[0] == '0':
		base = 8
		digits = s[1:]
	}
	if digits == "" {
		return 0, false
	}
	var v uint64
	for i := 0; i < len(digits); i++ {
		d, ok := digitValue(digits[i], base)
		if !ok {
			return 0, false
		}
		v = v*uint64(base) + uint64(d)
		if v > 0xffffffff {
			return 0, false
		}
	}
	return v, true
}

func digitValue(c byte, base int) (int, bool) {
	var d int
	switch {
	case c >= '0' && c <= '9':
		d = int(c - '0')
	case base == 16 && c >= 'a' && c <= 'f':
		d = int(c-'a') + 10
	case base == 16 && c >= 'A' && c <= 'F':
		d = int(c-'A') + 10
	default:
		return 0, false
	}
	if d >= base {
		return 0, false
	}
	return d, true
}
