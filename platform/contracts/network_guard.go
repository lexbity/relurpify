package contracts

import "net"

// privateRanges contains IP subnets that must never be reachable by sandboxed
// tool network calls. This is a hard-coded mandatory denylist that cannot be
// bypassed by agent configuration. It protects cloud metadata endpoints,
// loopback interfaces, and internal RFC-1918 addresses from SSRF.
//
// The denylist lives in platform/contracts (the sandbox contract leaf) so that
// it can be enforced by both the sandbox runtimes (framework/sandbox,
// platform/sandbox/*) and the platform-level network tools (platform/shell)
// without any layer crossing it back into a higher package. framework/sandbox
// re-exports IsPrivateOrLoopbackHost so the sandbox package surface is preserved.
var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",    // IPv4 loopback
		"::1/128",        // IPv6 loopback
		"10.0.0.0/8",     // RFC-1918 class A
		"172.16.0.0/12",  // RFC-1918 class B
		"192.168.0.0/16", // RFC-1918 class C
		"169.254.0.0/16", // Link-local / cloud metadata (AWS/GCP/Azure)
		"fc00::/7",       // IPv6 unique-local
		"fe80::/10",      // IPv6 link-local
	}
	privateRanges = make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, subnet, err := net.ParseCIDR(cidr)
		if err == nil {
			privateRanges = append(privateRanges, subnet)
		}
	}
}

// IsPrivateOrLoopbackHost reports whether the given hostname or IP address
// resolves to a private, loopback, or link-local address range. This check is
// mandatory and cannot be bypassed by agent configuration. If the host cannot
// be resolved it is treated as public (safe default for unknown hosts).
func IsPrivateOrLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	if ip != nil {
		return isPrivateIP(ip)
	}
	// Hostname — resolve to IPs.
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false // unresolvable names are treated as public
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true
		}
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	for _, block := range privateRanges {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
