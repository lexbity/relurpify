package descriptor

import (
	"math"
	"testing"
)

// FuzzParseSemver exercises the version parser against arbitrary untrusted
// version strings. The parser must be total (never panic) and stay consistent
// with versionGreater's treatment of the empty version.
func FuzzParseSemver(f *testing.F) {
	f.Add("1.2.3")
	f.Add("v2.0.0")
	f.Add("V1.0.0-rc.1")
	f.Add("1.2.3+build.7")
	f.Add("")
	f.Add("0")
	f.Add("9.9.9.9")
	f.Add("-5")
	f.Add("1.-2.x")
	f.Add("  3.1.4  ")
	f.Add("vv1.2")
	f.Add("99999999999999999999")

	f.Fuzz(func(t *testing.T, version string) {
		major, minor, patch := parseSemver(version)

		// The parser must never produce values outside the int range it
		// promises (they are plain ints here, but downstream comparisons
		// rely on lexicographic component ordering being meaningful).
		if major < math.MinInt || minor < math.MinInt || patch < math.MinInt {
			t.Fatalf("parseSemver(%q) produced impossible negatives", version)
		}

		// versionGreater(v, "") must hold exactly when parseSemver(v) is
		// lexicographically greater than (0,0,0), because the empty string
		// parses to (0,0,0) and comparison is component-wise.
		greater := versionGreater(version, "")
		wantGreater := major > 0 || (major == 0 && minor > 0) || (major == 0 && minor == 0 && patch > 0)
		if greater != wantGreater {
			t.Fatalf("versionGreater(%q, %q) = %v, want %v (parsed %d.%d.%d)",
				version, "", greater, wantGreater, major, minor, patch)
		}

		// Parsing is idempotent through the comparison: bestVersion of a
		// single-element slice always returns that element unchanged.
		if got := bestVersion([]string{version}); got != version {
			t.Fatalf("bestVersion([%q]) = %q, want the element unchanged", version, got)
		}
	})
}
