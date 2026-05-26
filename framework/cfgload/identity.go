package cfgload

import (
	"os/user"
	"strings"
)

// ResolveAuthorName returns the local username used when scaffolding manifests.
func ResolveAuthorName() string {
	u, err := user.Current()
	if err != nil || strings.TrimSpace(u.Username) == "" {
		return "unknown"
	}
	return strings.TrimSpace(u.Username)
}
