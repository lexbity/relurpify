package ports

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOriginDerivation_CreatesSingleOriginStep(t *testing.T) {
	chain := OriginDerivation("workspace-loader")

	require.Len(t, chain.Steps, 1)
	step := chain.Steps[0]
	assert.Equal(t, "origin", step.Transform)
	assert.Equal(t, "workspace-loader", step.SourceSystem)
	assert.Equal(t, DerivationID(""), step.ParentID)
	assert.Equal(t, 0.0, step.LossMagnitude)
	assert.False(t, step.Timestamp.IsZero(), "origin step must carry a timestamp")
	assert.Len(t, step.ID, 16, "derivation IDs are the first 16 hex chars of a sha256")
}

func TestDerivationChain_DeriveAppendsWithParentLink(t *testing.T) {
	chain := OriginDerivation("loader")
	originID := chain.Steps[0].ID

	next := chain.Derive("redact", "governance", 0.25, "removed secrets")

	require.Len(t, next.Steps, 2)
	step := next.Steps[1]
	assert.Equal(t, originID, step.ParentID, "derived step must link to the previous head")
	assert.Equal(t, "redact", step.Transform)
	assert.Equal(t, "governance", step.SourceSystem)
	assert.Equal(t, 0.25, step.LossMagnitude)
	assert.Equal(t, "removed secrets", step.Detail)
	assert.NotEqual(t, originID, step.ID)
	assert.False(t, step.Timestamp.IsZero())

	// Deriving returns a new chain value; the receiver chain is unchanged.
	assert.Len(t, chain.Steps, 1)
}

func TestDerivationChain_DeriveClampsLossMagnitude(t *testing.T) {
	chain := OriginDerivation("loader").
		Derive("t1", "sys", -0.5, "").
		Derive("t2", "sys", 1.5, "")

	require.Len(t, chain.Steps, 3)
	assert.Equal(t, 0.0, chain.Steps[1].LossMagnitude, "negative loss must clamp to 0")
	assert.Equal(t, 1.0, chain.Steps[2].LossMagnitude, "loss above 1 must clamp to 1")
}

func TestDerivationChain_DeriveUniqueIDsPerStep(t *testing.T) {
	chain := OriginDerivation("sys")
	a := chain.Derive("transform-a", "sys", 0, "")
	b := chain.Derive("transform-b", "sys", 0, "")

	require.Len(t, a.Steps, 2)
	require.Len(t, b.Steps, 2)
	assert.NotEqual(t, a.Steps[1].ID, b.Steps[1].ID, "sibling derivations must get distinct IDs")
}

func TestDerivationChain_TimestampsAreUTC(t *testing.T) {
	chain := OriginDerivation("sys")
	before := time.Now().UTC().Add(-time.Minute)
	after := time.Now().UTC().Add(time.Minute)

	step := chain.Steps[0]
	require.True(t, step.Timestamp.After(before) && step.Timestamp.Before(after))
	assert.Equal(t, time.UTC, step.Timestamp.Location())
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"empty pattern never matches", "", "anything.txt", false},
		{"empty pattern does not match empty path", "", "", false},
		{"bare star matches everything", "*", "/deeply/nested/file", true},
		{"double star matches everything", "**", "x", true},
		{"all-star pattern fails closed", "***", "x", false},
		{"all-star pattern fails closed on empty path", "***", "", false},
		{"exact literal match", "main.go", "main.go", true},
		{"exact literal mismatch", "main.go", "other.go", false},
		{"literal mismatch by length", "main.go", "main.go2", false},
		{"prefix glob matches", "file*.txt", "file.txt", true},
		{"prefix glob matches span", "file*.txt", "file12345.txt", true},
		{"prefix glob rejects other suffix", "file*.txt", "file12345.tsv", false},
		{"prefix glob rejects other prefix", "file*.txt", "data12345.txt", false},
		{"prefix glob is case-insensitive", "FILE*.TXT", "file12345.txt", true},
		{"interior glob matches in order", "a*X*b", "a1x2b", true},
		{"interior glob enforces order", "a*X*b*Y", "abX_Y", false},
		{"interior glob ordered match", "a*X*b*Y", "aXbY", true},
		{"suffix longer than remainder fails", "a*bcd", "abc", false},
		{"path shorter than prefix fails", "src*", "sr", false},
		{"leading star degenerates to exact match", "*.go", ".go", true},
		{"leading star does not span", "*.go", "main.go", false},
		{"trailing star degenerates to exact match", "src/*", "src/", true},
		{"trailing star does not span", "src/*", "src/main.go", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, MatchGlob(tc.pattern, tc.path),
				"MatchGlob(%q, %q)", tc.pattern, tc.path)
		})
	}
}

func TestSplitGlob(t *testing.T) {
	cases := []struct {
		pattern string
		want    []string
	}{
		{"", nil},
		{"*", nil},
		{"**", nil},
		{"abc", []string{"abc"}},
		{"*abc", []string{"abc"}},
		{"abc*", []string{"abc"}},
		{"a*b", []string{"a", "b"}},
		{"a*b*c", []string{"a", "b", "c"}},
		{"a**b", []string{"a", "b"}},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, splitGlob(tc.pattern), "splitGlob(%q)", tc.pattern)
	}
}

func TestMatchGlob_FoldHelpers(t *testing.T) {
	assert.True(t, hasPrefixFold("HelloWorld", "hello"))
	assert.False(t, hasPrefixFold("Hello", "hello world"))
	assert.False(t, hasPrefixFold("Jello", "hello"))
	assert.True(t, hasSuffixFold("HelloWorld", "WORLD"))
	assert.False(t, hasSuffixFold("World", "helloworld"))
	assert.Equal(t, 2, indexFold("abCDE", "cd"))
	assert.Equal(t, 0, indexFold("abc", ""))
	assert.Equal(t, -1, indexFold("abc", "zz"))
	assert.True(t, equalFold("MiXeD", "mixed"))
	assert.False(t, equalFold("abc", "ab"))
	assert.False(t, equalFold("abc", "abd"))
	assert.Equal(t, byte('a'), toLower('A'))
	assert.Equal(t, byte('z'), toLower('Z'))
	assert.Equal(t, byte('5'), toLower('5'))
}

func FuzzMatchGlob(f *testing.F) {
	f.Add("file*.txt", "file123.txt")
	f.Add("", "")
	f.Add("*", "anything")
	f.Add("**", "")
	f.Add("src/**", "src/x/y")
	f.Add("a*X*b*Y", "a1x2b3y")
	f.Add("MAIN.GO", "main.go")

	f.Fuzz(func(t *testing.T, pattern, path string) {
		// The matcher must be total: no panics on any input pair.
		got := MatchGlob(pattern, path)

		// Documented hard cases.
		if pattern == "" {
			require.False(t, got, "empty pattern must never match")
			return
		}
		if pattern == "*" || pattern == "**" {
			require.True(t, got, "bare star patterns must match every path")
			return
		}
		// A star-free pattern must behave as exact literal equality.
		if !strings.Contains(pattern, "*") {
			require.Equal(t, pattern == path, got,
				"star-free pattern %q must match only the exact path", pattern)
		}
	})
}
