package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptsCheckNoDirectory(t *testing.T) {
	workspace := t.TempDir()
	c := promptsCheck{}
	diags := c.Run(workspace)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for missing prompt dir, got %d", len(diags))
	}
}

func TestPromptsCheckValidPrompt(t *testing.T) {
	workspace := t.TempDir()
	promptsDir := filepath.Join(workspace, "templates", "prompts")
	os.MkdirAll(promptsDir, 0o755)
	mustWrite(t, filepath.Join(promptsDir, "test.prompt"), `---
schema framework.prompt/v2
id test_prompt
tag "test"
---
Hello, world!
`)

	c := promptsCheck{}
	diags := c.Run(workspace)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for valid prompt, got %d: %+v", len(diags), diags)
	}
}

func TestPromptsCheckInvalidSchema(t *testing.T) {
	workspace := t.TempDir()
	promptsDir := filepath.Join(workspace, "templates", "prompts")
	os.MkdirAll(promptsDir, 0o755)
	mustWrite(t, filepath.Join(promptsDir, "bad.prompt"), `---
schema unknown/v1
id bad_prompt
tag "test"
---
Hello
`)

	c := promptsCheck{}
	diags := c.Run(workspace)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for invalid schema, got none")
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "unsupported schema") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'schema' diagnostic, got: %+v", diags)
	}
}

func TestPromptsCheckMissingID(t *testing.T) {
	workspace := t.TempDir()
	promptsDir := filepath.Join(workspace, "templates", "prompts")
	os.MkdirAll(promptsDir, 0o755)
	mustWrite(t, filepath.Join(promptsDir, "noid.prompt"), `---
schema framework.prompt/v2
tag "test"
---
Hello
`)

	c := promptsCheck{}
	diags := c.Run(workspace)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for missing ID, got none")
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "id") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'id' diagnostic, got: %+v", diags)
	}
}

func TestPromptsCheckEmptyBody(t *testing.T) {
	workspace := t.TempDir()
	promptsDir := filepath.Join(workspace, "templates", "prompts")
	os.MkdirAll(promptsDir, 0o755)
	mustWrite(t, filepath.Join(promptsDir, "empty.prompt"), `---
schema framework.prompt/v2
id empty_prompt
tag "test"
---
`)

	c := promptsCheck{}
	diags := c.Run(workspace)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for empty body, got none")
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "body") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'body' diagnostic, got: %+v", diags)
	}
}

func TestPromptsCheckCleanRepo(t *testing.T) {
	c := promptsCheck{}
	diags := c.Run(repoRoot())
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics for clean repo, got %d: %+v", len(diags), diags)
	}
}
