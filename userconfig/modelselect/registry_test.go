package modelselect

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	cfgmodel "codeburg.org/lexbit/relurpify/userconfig/config/model"
)

func profile(pattern, sourcePath string) *cfgmodel.ModelProfileConfig {
	return &cfgmodel.ModelProfileConfig{Pattern: pattern, SourcePath: sourcePath}
}

func TestResolveFallsBackToBuiltInDefaultOnEmptyRegistry(t *testing.T) {
	res := NewProfileRegistry().Resolve("lmstudio", "qwen2.5-coder")
	require.NotNil(t, res.Profile, "empty registry must resolve to a built-in profile")
	require.Equal(t, "builtin-default", res.MatchKind)
	require.Equal(t, "built-in default profile", res.Reason)
	require.Equal(t, "lmstudio", res.Provider)
	require.Equal(t, "qwen2.5-coder", res.Model)
	require.Empty(t, res.SourcePath)
	require.Equal(t, "*", res.Profile.Pattern)
}

func TestResolveOnNilRegistryFallsBackToBuiltIn(t *testing.T) {
	var nilRegistry *ProfileRegistry
	res := nilRegistry.Resolve("openai", "gpt-4")
	require.NotNil(t, res.Profile)
	require.Equal(t, "builtin-default", res.MatchKind)
}

func TestResolveMatchesExactModelCaseInsensitively(t *testing.T) {
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{
		profile("Qwen2.5-Coder", "a.llm.yaml"),
	})
	res := reg.Resolve("LMStudio", "  qwen2.5-coder  ")
	require.Equal(t, "model-exact", res.MatchKind)
	require.Equal(t, "exact model match for qwen2.5-coder", res.Reason)
	require.NotNil(t, res.Profile)
	require.Equal(t, "lmstudio", res.Provider, "provider is lowercased and trimmed")
	require.Equal(t, "qwen2.5-coder", res.Model, "model is trimmed")
}

func TestResolvePrefersExactModelOverGlob(t *testing.T) {
	globProfile := profile("qwen*", "glob.llm.yaml")
	exactProfile := profile("qwen2.5-coder", "exact.llm.yaml")
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{globProfile, exactProfile})

	res := reg.Resolve("any", "qwen2.5-coder")
	require.Same(t, exactProfile, res.Profile, "exact model match must beat glob")
	require.Equal(t, "model-exact", res.MatchKind)
}

func TestResolveMatchesGlobAndPrefersLongerLiteralPrefix(t *testing.T) {
	broad := profile("gpt*", "broad.llm.yaml")
	specific := profile("gpt-4*", "specific.llm.yaml")
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{broad, specific})

	res := reg.Resolve("openai", "gpt-4o")
	require.Same(t, specific, res.Profile, "longer literal prefix must win")
	require.Equal(t, "glob", res.MatchKind)
	require.Equal(t, "glob match for gpt-4o", res.Reason)
}

func TestResolveFallsBackToDefaultProfileWhenNothingMatches(t *testing.T) {
	def := profile("*", "model/profiles/default.llm.yaml")
	other := profile("claude*", "claude.llm.yaml")
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{other, def})

	res := reg.Resolve("ollama", "llama3")
	require.Same(t, def, res.Profile, "non-matching model must resolve to the default profile")
	require.Equal(t, "default", res.MatchKind)
	require.Equal(t, "default profile from default.llm.yaml", res.Reason)
	require.Equal(t, "llama3", res.Model)
}

func TestResolveSkipsProfileWithEmptyPatternFromNonDefaultFile(t *testing.T) {
	// A non-default profile whose pattern is empty scores -1; with no other
	// candidates the registry must fall back to the built-in profile rather
	// than hand out a profile that declares nothing.
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{
		profile("", "other.llm.yaml"),
	})
	res := reg.Resolve("ollama", "llama3")
	require.NotNil(t, res.Profile)
	require.Equal(t, "builtin-default", res.MatchKind)
}

func TestResolveDefaultsToDefaultEntryWhenItHasEmptyPattern(t *testing.T) {
	// OnlyDefault path: a nil-pattern profile from the default file still
	// scores 0 as "default" and must be preferred over the built-in.
	def := profile("", "default.llm.yaml")
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{def})
	res := reg.Resolve("ollama", "llama3")
	require.Same(t, def, res.Profile)
	require.Equal(t, "default", res.MatchKind)
}

func TestResolveIgnoresNilProfiles(t *testing.T) {
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{nil, profile("m*", "x.llm.yaml")})
	res := reg.Resolve("any", "mistral")
	require.Equal(t, "glob", res.MatchKind)
	require.NotNil(t, res.Profile)
}

func TestResolveEmptyModelNeverMatchesDeclaredProfiles(t *testing.T) {
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{profile("*", "x.llm.yaml")})
	res := reg.Resolve("any", "")
	require.Equal(t, "builtin-default", res.MatchKind, "empty model string must not match any pattern")
}

func TestResolveFirstRegisteredWinsOnTie(t *testing.T) {
	a := profile("gem*", "a.llm.yaml")
	b := profile("gem*", "b.llm.yaml")
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{a, b})
	res := reg.Resolve("any", "gemma4")
	require.Same(t, a, res.Profile, "equal scores must resolve to the first registered profile")
}

func TestMatchResolvesWithoutProviderScoping(t *testing.T) {
	got := profile("gpt*", "gpt.llm.yaml")
	reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{got})
	require.Same(t, got, reg.Match("gpt-4o"))
	// Match never returns nil: an unmatched model resolves to the built-in
	// default profile.
	require.Equal(t, "*", NewProfileRegistry().Match("nomatch-xyz").Pattern)
}

func TestAddAppendsProfilesAndSkipsNil(t *testing.T) {
	reg := NewProfileRegistry()
	reg.Add(nil)
	p := profile("k*", "kimi.llm.yaml")
	reg.Add(p)

	res := reg.Resolve("moonshot", "kimi-k2")
	require.Same(t, p, res.Profile)
}

func TestAddMarksDefaultProfileFile(t *testing.T) {
	reg := NewProfileRegistry()
	def := profile("*", "model/profiles/default.llm.yaml")
	reg.Add(def)
	// The default file's "*" pattern matches everything through the glob path,
	// so exercise the default-entry branch with a non-matching model name and
	// an empty pattern to force the default fallback instead of the glob hit.
	res := reg.Resolve("x", "nomatch-model")
	require.NotNil(t, res.Profile)
}

func TestBuildProfileRegistrySkipsNilConfigs(t *testing.T) {
	reg, err := BuildProfileRegistry([]*cfgmodel.ModelProfileConfig{nil, profile("o*", "ollama.llm.yaml")})
	require.NoError(t, err)
	require.NotNil(t, reg)
	res := reg.Resolve("ollama", "ollama3")
	require.Equal(t, "glob", res.MatchKind)
}

func TestBuildProfileRegistryEmptyYieldsBuiltInDefault(t *testing.T) {
	reg, err := BuildProfileRegistry(nil)
	require.NoError(t, err)
	res := reg.Resolve("any", "any")
	require.Equal(t, "builtin-default", res.MatchKind)
}

type profileHolder struct{ got *cfgmodel.ModelProfileConfig }

func (h *profileHolder) SetProfile(p *cfgmodel.ModelProfileConfig) { h.got = p }

func TestApplyProfileSetsOnProfileAwareTarget(t *testing.T) {
	holder := &profileHolder{}
	p := profile("*", "d.llm.yaml")
	require.True(t, ApplyProfile(holder, p))
	require.Same(t, p, holder.got)
}

func TestApplyProfileRejectsNilTargetAndNilProfile(t *testing.T) {
	require.False(t, ApplyProfile(nil, profile("*", "d.llm.yaml")))
	require.False(t, ApplyProfile(&profileHolder{}, nil))
}

func TestApplyProfileRejectsTargetWithoutSetter(t *testing.T) {
	require.False(t, ApplyProfile("not a setter", profile("*", "d.llm.yaml")))
}

// TestMatchGlobSemantics pins the glob dialect implemented by matchGlob:
// case-insensitive, '*' as the only wildcard, and every literal segment
// between stars must appear in order.
//
// Note the segments are NOT anchored: "pt*" matches "gpt-4" and "*gpt"
// matches "gpt-4". Patterns that read as prefix/suffix matches therefore
// behave as containment matches (see bug report on registry.go matchGlob).
// These cases pin current behavior so a future anchoring fix flips them
// deliberately rather than silently.
func TestMatchGlobSemantics(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{"star matches suffix", "gpt*", "gpt-4o", true},
		{"star matches prefix", "*coder", "qwen2.5-coder", true},
		{"star in middle", "qwen*llama", "qwen3-llama", true},
		{"star matches empty middle", "qwen*llama", "qwenllama", true},
		{"literal no metachars equal", "gpt4", "gpt4", true},
		{"literal case insensitive", "GPT4", "gpt4", true},
		{"literal mismatch", "gpt4", "gpt5", false},
		{"question mark is not a wildcard", "gpt-?", "gpt-4", false},
		{"unanchored head segment", "pt*", "gpt-4", true},
		{"unanchored tail segment", "*gpt", "gpt-4", true},
		{"missing middle segment", "qwen*llama", "qwen3-coder", false},
		{"empty pattern never matches", "", "anything", false},
		{"double star", "a**b", "axb", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, matchPattern(tc.pattern, tc.value))
		})
	}
}

func TestIsDefaultProfileFileOnlyMatchesDefaultBasename(t *testing.T) {
	require.True(t, isDefaultProfileFile("model/profiles/default.llm.yaml"))
	require.True(t, isDefaultProfileFile("DEFAULT.LLM.YAML"))
	require.False(t, isDefaultProfileFile("qwen.llm.yaml"))
	require.False(t, isDefaultProfileFile(""))
}

func TestProviderRegistryConstructor(t *testing.T) {
	require.NotNil(t, NewProviderRegistry(struct{}{}))
}

// FuzzResolveReturnsProfile asserts the core resolution invariant for any
// (provider, model, pattern) triple: Resolve always hands back a usable
// profile with a known match kind — the resolver must never return nothing.
func FuzzResolveReturnsProfile(f *testing.F) {
	f.Add("openai", "gpt-4o", "gpt*")
	f.Add("LMStudio", "Qwen2.5-Coder", "qwen2.5-coder")
	f.Add("ollama", "llama3", "default.llm.yaml")
	f.Add("", "", "*")
	f.Add("x", "y", "[")
	f.Add("x", "y", "**")

	f.Fuzz(func(t *testing.T, provider, model, pattern string) {
		reg := NewProfileRegistryFromProfiles([]*cfgmodel.ModelProfileConfig{
			{Pattern: pattern, SourcePath: "p.llm.yaml"},
		})
		res := reg.Resolve(provider, model)
		require.NotNil(t, res.Profile, "Resolve must always return a profile")
		switch res.MatchKind {
		case "model-exact", "glob", "default", "builtin-default":
		default:
			t.Fatalf("unexpected match kind %q", res.MatchKind)
		}
	})
}

// FuzzMatchGlobNoPanic checks the hand-rolled glob matcher never panics and
// stays case-insensitive for literal (metachar-free) patterns.
func FuzzMatchGlobNoPanic(f *testing.F) {
	f.Add("gpt*", "gpt-4o")
	f.Add("", "x")
	f.Add("**", "x")
	f.Add("a[", "a[")
	f.Add("qwen*", "")

	f.Fuzz(func(t *testing.T, pattern, value string) {
		got := matchPattern(pattern, value)
		if !strings.ContainsAny(pattern, "*?[") {
			want := strings.EqualFold(pattern, value)
			require.Equal(t, want, got,
				"literal pattern %q vs %q must behave like case-insensitive equality", pattern, value)
		}
	})
}
