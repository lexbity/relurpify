package prompt

import (
	"errors"
	"path/filepath"

	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

const (
	Allidsvwantv_registry_test        = "All ids = %#v, want %#v"
	LoadFSv_registry_test             = "LoadFS: %v"
	Resolveoutputqwantq_registry_test = "Resolve output = %q, want %q"
	Resolvev_registry_test            = "Resolve: %v"
	Speaktone_registry_test           = "Speak {tone}."
	Aprompt_registry_test             = "a.prompt"
	Agentdup_registry_test            = "agent.dup"
	Agentone_registry_test            = "agent.one"
	Agenttwo_registry_test            = "agent.two"
	Bprompt_registry_test             = "b.prompt"
	Debug_registry_test               = "debug"
	Direct_registry_test              = "direct"
	Fixtures_registry_test            = "fixtures"
	System_registry_test              = "system"
	Tone_registry_test                = "tone"
)

func TestRegistry_LoadDirV2Prompts(t *testing.T) {
	dir := t.TempDir()
	writePromptFile(t, dir, Bprompt_registry_test, Agenttwo_registry_test, []string{Debug_registry_test}, map[string]string{Tone_registry_test: Direct_registry_test}, "Hello {tone}.")
	writePromptFile(t, dir, Aprompt_registry_test, Agentone_registry_test, []string{System_registry_test}, nil, "Alpha")

	reg := NewRegistry()
	if err := reg.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	if got, want := reg.Count(), 2; got != want {
		t.Fatalf("Count = %d, want %d", got, want)
	}
	if got, want := idsOf(reg.All()), []string{Agentone_registry_test, Agenttwo_registry_test}; !reflect.DeepEqual(got, want) {
		t.Fatalf(Allidsvwantv_registry_test, got, want)
	}
	out, err := reg.Resolve(Agenttwo_registry_test, RuntimeContext{})
	if err != nil {
		t.Fatalf(Resolvev_registry_test, err)
	}
	if out != "Hello direct." {
		t.Fatalf(Resolveoutputqwantq_registry_test, out, "Hello direct.")
	}
}

func TestRegistry_LoadFSV2Prompts(t *testing.T) {
	fsys := fstest.MapFS{
		Bprompt_registry_test: {Data: []byte(promptFile(Agenttwo_registry_test, []string{Debug_registry_test}, map[string]string{Tone_registry_test: Direct_registry_test}, "Hello {tone}."))},
		Aprompt_registry_test: {Data: []byte(promptFile(Agentone_registry_test, []string{System_registry_test}, nil, "Alpha"))},
	}

	reg := NewRegistry()
	if err := reg.LoadFS(fsys, Fixtures_registry_test); err != nil {
		t.Fatalf(LoadFSv_registry_test, err)
	}

	if got, want := idsOf(reg.All()), []string{Agentone_registry_test, Agenttwo_registry_test}; !reflect.DeepEqual(got, want) {
		t.Fatalf(Allidsvwantv_registry_test, got, want)
	}
}

func TestRegistry_DuplicateIDRejected(t *testing.T) {
	fsys := fstest.MapFS{
		Aprompt_registry_test: {Data: []byte(promptFile(Agentdup_registry_test, nil, nil, "one"))},
		Bprompt_registry_test: {Data: []byte(promptFile(Agentdup_registry_test, nil, nil, "two"))},
	}

	reg := NewRegistry()
	err := reg.LoadFS(fsys, Fixtures_registry_test)
	if err == nil {
		t.Fatal("expected duplicate-id load error")
	}
	var dupErr *DuplicateIDError
	if !errors.As(err, &dupErr) {
		t.Fatalf("error = %v, want DuplicateIDError", err)
	}
	if dupErr.ID != Agentdup_registry_test {
		t.Fatalf("DuplicateIDError.ID = %q, want agent.dup", dupErr.ID)
	}
	if got, want := reg.Count(), 1; got != want {
		t.Fatalf("Count = %d, want %d", got, want)
	}
}

func TestRegistry_FilterSingleTag(t *testing.T) {
	reg := loadTestRegistry(t)
	got := idsOf(reg.Filter(FilterOptions{Tags: []string{System_registry_test}}))
	if want := []string{Agentone_registry_test}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Filter ids = %#v, want %#v", got, want)
	}
}

func TestRegistry_FilterMultipleTags(t *testing.T) {
	reg := loadTestRegistry(t)
	got := idsOf(reg.Filter(FilterOptions{Tags: []string{Debug_registry_test, System_registry_test}}))
	if want := []string{Agentone_registry_test, Agenttwo_registry_test}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Filter ids = %#v, want %#v", got, want)
	}
}

func TestRegistry_RuntimeOverrideResolution(t *testing.T) {
	reg := NewRegistry()
	if err := reg.LoadFS(fstest.MapFS{
		"override.prompt": {Data: []byte(promptFile("agent.override", nil, map[string]string{Tone_registry_test: Direct_registry_test}, Speaktone_registry_test))},
	}, Fixtures_registry_test); err != nil {
		t.Fatalf(LoadFSv_registry_test, err)
	}

	out, err := reg.Resolve("agent.override", RuntimeContext{Variables: map[string]string{Tone_registry_test: "gentle"}})
	if err != nil {
		t.Fatalf(Resolvev_registry_test, err)
	}
	if out != "Speak gentle." {
		t.Fatalf(Resolveoutputqwantq_registry_test, out, "Speak gentle.")
	}
}

func TestRegistry_DefaultVariableFallback(t *testing.T) {
	reg := NewRegistry()
	if err := reg.LoadFS(fstest.MapFS{
		"default.prompt": {Data: []byte(promptFile("agent.default", nil, map[string]string{Tone_registry_test: Direct_registry_test}, Speaktone_registry_test))},
	}, Fixtures_registry_test); err != nil {
		t.Fatalf(LoadFSv_registry_test, err)
	}

	out, err := reg.Resolve("agent.default", RuntimeContext{})
	if err != nil {
		t.Fatalf(Resolvev_registry_test, err)
	}
	if out != "Speak direct." {
		t.Fatalf(Resolveoutputqwantq_registry_test, out, "Speak direct.")
	}
}

func TestRegistry_MissingVariableFails(t *testing.T) {
	reg := NewRegistry()
	err := reg.LoadFS(fstest.MapFS{
		"missing.prompt": {Data: []byte(promptFile("agent.missing", nil, nil, Speaktone_registry_test))},
	}, Fixtures_registry_test)
	if err == nil {
		t.Fatal("expected missing variable load error")
	}
	if !strings.Contains(err.Error(), "unknown variable") {
		t.Fatalf("error = %v, want unknown variable", err)
	}
}

func TestRegistry_DeterministicLoadOrder(t *testing.T) {
	dir := t.TempDir()
	writePromptFile(t, dir, "z.prompt", "agent.z", nil, nil, "Z")
	writePromptFile(t, dir, Aprompt_registry_test, "agent.a", nil, nil, "A")
	writePromptFile(t, dir, "m.prompt", "agent.m", nil, nil, "M")

	reg := NewRegistry()
	if err := reg.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	if got, want := idsOf(reg.All()), []string{"agent.a", "agent.m", "agent.z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf(Allidsvwantv_registry_test, got, want)
	}
}

func writePromptFile(t *testing.T, dir, name, id string, tags []string, vars map[string]string, body string) {
	t.Helper()
	if err := fs.WriteFileSecure(filepath.Join(dir, name), []byte(promptFile(id, tags, vars, body))); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func promptFile(id string, tags []string, vars map[string]string, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema framework.prompt/v2\n")
	b.WriteString("id ")
	b.WriteString(id)
	b.WriteByte('\n')
	for _, tag := range tags {
		b.WriteString("tag ")
		b.WriteString(strconvQuote(tag))
		b.WriteByte('\n')
	}
	tagKeys := make([]string, 0, len(vars))
	for name := range vars {
		tagKeys = append(tagKeys, name)
	}
	sort.Strings(tagKeys)
	for _, name := range tagKeys {
		b.WriteString("var ")
		b.WriteString(name)
		b.WriteString(" = ")
		b.WriteString(strconvQuote(vars[name]))
		b.WriteByte('\n')
	}
	b.WriteString("---\n")
	if body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func strconvQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}

func idsOf(cfgs []*PromptConfig) []string {
	out := make([]string, 0, len(cfgs))
	for _, cfg := range cfgs {
		out = append(out, cfg.ID)
	}
	return out
}

func loadTestRegistry(t *testing.T) Registry {
	t.Helper()
	reg := NewRegistry()
	fsys := fstest.MapFS{
		"one.prompt":   {Data: []byte(promptFile(Agentone_registry_test, []string{System_registry_test}, map[string]string{Tone_registry_test: Direct_registry_test}, "One {tone}."))},
		"two.prompt":   {Data: []byte(promptFile(Agenttwo_registry_test, []string{Debug_registry_test}, nil, "Two"))},
		"three.prompt": {Data: []byte(promptFile("agent.three", []string{"misc"}, nil, "Three"))},
	}
	if err := reg.LoadFS(fsys, Fixtures_registry_test); err != nil {
		t.Fatalf(LoadFSv_registry_test, err)
	}
	return reg
}
