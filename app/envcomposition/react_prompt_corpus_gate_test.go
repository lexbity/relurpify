package envcomposition

import (
	"io/fs"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

// TestReactPromptCorpusResolvesIntoPrompts pins the prompt-DSL delivery
// contract: the shipped corpus (the same embedfs the workspace materializer
// and generate-config emit) resolves the react system and user prompts with
// the dynamic pieces the paradigm injects — tool list, guidance, instruction.
// Before this gate, the corpus never reached a real workspace and every react
// turn silently ran on inline fallbacks (prompt.resolve_failed in every live
// boot).
func TestReactPromptCorpusResolvesIntoPrompts(t *testing.T) {
	efs := embedfs.DefaultFS()
	promptsFS, err := fs.Sub(efs, "prompts")
	if err != nil {
		t.Fatalf("sub prompts: %v", err)
	}
	reg := prompt.NewRegistry()
	if err := reg.LoadFS(promptsFS, "."); err != nil {
		t.Fatalf("load shipped corpus: %v", err)
	}

	system, err := reg.Resolve("agent.react.default", prompt.RuntimeContext{
		Paradigm: "react",
		Variables: map[string]string{
			"tools":    "- file_read: Reads a file\n- file_write: Writes a file",
			"guidance": "",
		},
	})
	if err != nil {
		t.Fatalf("system prompt resolve failed: %v", err)
	}
	for _, want := range []string{"Available tools:", "- file_read: Reads a file", "Only call tools listed above"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt missing %q;\ngot:\n%s", want, system)
		}
	}

	user, err := reg.Resolve("agent.react.default.user", prompt.RuntimeContext{
		Paradigm: "react",
		Variables: map[string]string{
			"tools":       "- file_read: Reads a file",
			"instruction": "fix the misspelling in hello.py",
		},
	})
	if err != nil {
		t.Fatalf("user prompt resolve failed: %v", err)
	}
	for _, want := range []string{"Task: fix the misspelling in hello.py", `"action":"tool|complete"`, "- file_read: Reads a file"} {
		if !strings.Contains(user, want) {
			t.Fatalf("user prompt missing %q;\ngot:\n%s", want, user)
		}
	}
}
