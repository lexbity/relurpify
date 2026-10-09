package arch

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

// The forbidden patterns, named once so the table below stays readable.
const (
	patternShim          = "shim"
	patternCompatibility = "compatibility"
	patternStub          = "stub"
	patternCompat        = "compat"
)

func TestCheckShimLanguageFlagsForbiddenVocabulary(t *testing.T) {
	cases := []struct {
		name    string
		literal string
		want    string
	}{
		{patternShim, `"shim"`, patternShim},
		{patternCompatibility, `"backward compatibility"`, patternCompatibility},
		{patternStub, `"stub"`, patternStub},
		{"mixed case", `"Do Not Add A SHIM here"`, patternShim},
		{"inside a longer literal", `"legacy shim for the old API"`, patternShim},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := []gatescan.File{parse(t, "app/x/a.go", "package a\n\nvar _ = "+tc.literal+"\n")}
			got := CheckShimLanguage(files, Exemption{})
			if len(got) != 1 {
				t.Fatalf("want 1 violation, got %v", got)
			}
			if !strings.Contains(got[0], tc.want) {
				t.Errorf("violation %q should name pattern %q", got[0], tc.want)
			}
			if !strings.HasPrefix(got[0], "shim: app/x/a.go:") {
				t.Errorf("violation %q should carry file:line", got[0])
			}
		})
	}
}

// TestCheckShimLanguageAllowsProviderVocabulary is the regression test for the
// false-positive class that made the first draft of this gate unusable: the
// short spelling "compat" is the project's canonical provider vocabulary, so
// only the long form is forbidden.
func TestCheckShimLanguageAllowsProviderVocabulary(t *testing.T) {
	files := []gatescan.File{parse(t, "platform/llm/a.go", `package a

var (
	kind    = "openai_compatible"
	pkg     = "codeburg.org/lexbit/relurpify/platform/llm/openaicompat"
	altKind = "openai-compatible"
	id      = "euclo:cap.api_compat"
	cat     = "migration_compat"
)
`)}

	if got := CheckShimLanguage(files, Exemption{}); len(got) != 0 {
		t.Fatalf("provider vocabulary must not be flagged, got %v", got)
	}
}

func TestCheckShimLanguageInspectsLiteralsOnly(t *testing.T) {
	files := []gatescan.File{parse(t, "a.go", `package a

// shimHelper is only a comment mentioning a shim.
func shimHelper() string {
	// stubbed out on purpose
	return "sh" + "im"
}
`)}

	if got := CheckShimLanguage(files, Exemption{}); len(got) != 0 {
		t.Fatalf("comments, identifiers and concatenations are not literals, got %v", got)
	}
}

func TestCheckShimLanguageHonoursExemptions(t *testing.T) {
	violation := "package a\n\nvar _ = \"shim\"\n"
	files := []gatescan.File{
		parse(t, archToolingDir+"/shimcheck.go", violation),
		parse(t, "named/euclo/relurpicabilities/api_compat.go", violation),
		parse(t, "named/euclo/relurpicabilities/register.go", violation),
		parse(t, "tooling/archx/a.go", violation),
	}

	exempt := Exemption{
		Prefixes: []string{archToolingDir},
		Files:    []string{"named/euclo/relurpicabilities/api_compat.go"},
	}
	got := CheckShimLanguage(files, exempt)
	if len(got) != 2 {
		t.Fatalf("want only the two unexempt files flagged, got %v", got)
	}
	for _, v := range got {
		if strings.Contains(v, "api_compat.go") || strings.Contains(v, "tooling/arch/") {
			t.Errorf("exempt file flagged: %s", v)
		}
	}
}

func TestForbiddenLanguageTracksTheGrepGate(t *testing.T) {
	// The grep gate retained in grep-architecture-gates matches these four
	// patterns; the AST gate must police the same vocabulary or the two layers
	// disagree about what is forbidden.
	want := []string{patternShim, patternCompatibility, patternStub, "backward compatibility"}
	if len(ForbiddenLanguage) != len(want) {
		t.Fatalf("ForbiddenLanguage = %v, want %v", ForbiddenLanguage, want)
	}
	for i := range want {
		if ForbiddenLanguage[i] != want[i] {
			t.Fatalf("ForbiddenLanguage = %v, want %v", ForbiddenLanguage, want)
		}
	}
}

// TestForbiddenCompatWordMatchesWholeWordsOnly covers the short spelling. As a
// substring it is the project's provider vocabulary, so it is forbidden only as
// a whole word.
func TestForbiddenCompatWordMatchesWholeWordsOnly(t *testing.T) {
	cases := []struct {
		name    string
		literal string
		want    string
	}{
		{"compat alias", `"compat alias for the v1 API"`, patternCompat},
		{"hyphenated", `"back-compat"`, patternCompat},
		{"upper case", `"COMPAT layer"`, patternCompat},
		{"provider kind", `"openai_compatible"`, ""},
		{"package path", `"codeburg.org/lexbit/relurpify/platform/llm/openaicompat"`, ""},
		{"capability id", `"euclo:cap.api_compat"`, ""},
		{"category", `"migration_compat"`, ""},
		{"long form", `"compatibility"`, "compatibility"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := []gatescan.File{parse(t, "a.go", "package a\n\nvar _ = "+tc.literal+"\n")}
			got := CheckShimLanguage(files, Exemption{})
			switch {
			case tc.want == "" && len(got) != 0:
				t.Fatalf("want no violation, got %v", got)
			case tc.want == "":
			case len(got) != 1:
				t.Fatalf("want 1 violation, got %v", got)
			case !strings.Contains(got[0], tc.want):
				t.Errorf("violation %q should name %q", got[0], tc.want)
			}
		})
	}
}
