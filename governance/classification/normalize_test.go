package classification

import (
	"strings"
	"testing"
)

func TestNormalizeClassString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"process_spawn", string(EffectClassProcessSpawn)},
		{"filesystem_mutation", string(EffectClassFilesystemMutation)},
		{"filesystem_read", "filesystem-read"},
		{"network_egress", string(EffectClassNetworkEgress)},
		{"external_state", string(EffectClassExternalState)},
		{"session_creation", string(EffectClassSessionCreation)},
		{"context_insertion", string(EffectClassContextInsertion)},
		{"builtin_trusted", "builtin-trusted"},
		{"read_only", "read-only"},
		{"  Process_Spawn  ", string(EffectClassProcessSpawn)},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := NormalizeClassString(tc.in); got != tc.want {
			t.Errorf("NormalizeClassString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeClassStringAlreadyKebab(t *testing.T) {
	for _, s := range []string{string(EffectClassProcessSpawn), "builtin-trusted", "read-only"} {
		if got := NormalizeClassString(s); got != s {
			t.Errorf("NormalizeClassString(%q) = %q, want unchanged", s, got)
		}
	}
}

func TestNormalizeClassStringIdempotent(t *testing.T) {
	// Deterministic pseudo-random inputs (hand-rolled xorshift, no math/rand):
	// keeps the test reproducible and avoids weak-random lint findings. Bit
	// masks (not modulo) keep the index non-negative without integer narrowing
	// conversions.
	seed := 2463534242
	next := func() int {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		return seed
	}
	const alphabet = "abAB_- 0" // 8 symbols
	for i := 0; i < 1000; i++ {
		n := next() & 15
		var b strings.Builder
		for j := 0; j < n; j++ {
			b.WriteRune(rune(alphabet[next()&7]))
		}
		s := b.String()
		once := NormalizeClassString(s)
		if twice := NormalizeClassString(once); once != twice {
			t.Fatalf("NormalizeClassString not idempotent for %q: %q != %q", s, once, twice)
		}
	}
}
