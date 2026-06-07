package capability

import (
	"strconv"
	"strings"
)

// parseSemver parses a semver string into its numeric components.
// Returns (0,0,0) for invalid or empty strings. Supports "v" prefix,
// "major.minor.patch" format with optional pre-release suffix.
func parseSemver(version string) (major, minor, patch int) {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 1 || parts[0] == "" {
		return 0, 0, 0
	}
	major, _ = strconv.Atoi(parts[0])
	if len(parts) < 2 {
		return major, 0, 0
	}
	minor, _ = strconv.Atoi(parts[1])
	if len(parts) < 3 {
		return major, minor, 0
	}
	// Strip pre-release and build suffixes (e.g., "1.2.3-alpha" or "1.2.3+build")
	patchStr := strings.SplitN(parts[2], "-", 2)[0]
	patchStr = strings.SplitN(patchStr, "+", 2)[0]
	patch, _ = strconv.Atoi(patchStr)
	return major, minor, patch
}

// versionGreater returns true if v1 represents a higher semver version than v2.
func versionGreater(v1, v2 string) bool {
	m1, n1, p1 := parseSemver(v1)
	m2, n2, p2 := parseSemver(v2)
	if m1 != m2 {
		return m1 > m2
	}
	if n1 != n2 {
		return n1 > n2
	}
	return p1 > p2
}

// bestVersion returns the highest version from a slice. Empty versions
// are treated as the lowest possible (0.0.0).
func bestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	best := versions[0]
	for _, v := range versions[1:] {
		if versionGreater(v, best) {
			best = v
		}
	}
	return best
}
