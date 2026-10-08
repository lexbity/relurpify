package sandbox

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/netpolicy"
)

// ClassifyEgressTarget reports whether an egress token must be blocked.
//
// IP literals are classified without I/O by the canonical netpolicy
// classifier. Hostnames require the supplied resolver (the sandbox layer may
// perform I/O); a nil resolver — or a resolver that returns an error — blocks
// the target (fail closed). Any non-public class is always blocked, regardless
// of allowlists.
func ClassifyEgressTarget(token string, resolve func(string) (netpolicy.Target, error)) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("egress target empty")
	}
	if ip, ok := netpolicy.ParseHostToken(token); ok {
		if class := netpolicy.ClassifyIP(ip); class != netpolicy.ClassPublic {
			return fmt.Errorf("egress target %q is %s — blocked (ssrf protection)", token, class)
		}
		return nil
	}
	if resolve == nil {
		return fmt.Errorf("egress target %q unresolved (no resolver) — blocked", token)
	}
	target, err := resolve(token)
	if err != nil {
		return fmt.Errorf("egress target %q unresolved: %w", token, err)
	}
	if target.Class != netpolicy.ClassPublic {
		return fmt.Errorf("egress target %q is %s — blocked (ssrf protection)", token, target.Class)
	}
	return nil
}
