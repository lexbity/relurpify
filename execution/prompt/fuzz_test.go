package prompt

import "testing"

func FuzzParseBytesHeader(f *testing.F) {
	seed := []string{
		"---\nschema framework.prompt/v2\nid agent.generic.default\n---\nbody\n",
		"---\nid agent.generic.default\n---\nbody\n",
		"---\nschema framework.prompt/v2\nid agent.generic.default\ntag [\"system\"]\n---\nbody\n",
		"---\nschema framework.prompt/v2\nid agent.generic.default\nvar tone = \"direct\"\n---\nbody\n",
	}
	for _, s := range seed {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		_, _ = ParseBytes([]byte(src), "fuzz.prompt")
	})
}

func FuzzSubstituteMarkdownText(f *testing.F) {
	f.Add("plain text")
	f.Add("{tone}")
	f.Add("\\{literal\\}")
	f.Add("{1bad}")
	f.Add("before {tone} after")
	f.Add("code `not {sub}`")

	f.Fuzz(func(t *testing.T, text string) {
		vars := map[string]string{
			"tone": "direct",
		}
		_, _ = substituteMarkdownText(text, vars)
	})
}
