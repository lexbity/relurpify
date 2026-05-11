package prompt

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhase8_V2PromptAssetsOnly(t *testing.T) {
	var assets []string
	err := filepath.WalkDir(repoPath("templates", "prompts"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".prompt") {
			assets = append(assets, path)
			cfg, err := ParseFile(path)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			if cfg == nil || cfg.Config == nil {
				t.Fatalf("%s parsed without config", path)
			}
			if cfg.Config.Schema != "framework.prompt/v2" {
				t.Fatalf("%s schema = %q, want framework.prompt/v2", path, cfg.Config.Schema)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk templates/prompts: %v", err)
	}
	if len(assets) == 0 {
		t.Fatal("no prompt assets found under templates/prompts")
	}
}

func TestPhase1_HandoffDocsAreV2Only(t *testing.T) {
	expected := map[string]struct{}{
		repoPath("devdocs", "HANDOFF", "framework-prompt", "OVERVIEW.md"):     {},
		repoPath("devdocs", "HANDOFF", "framework-prompt", "API_SURFACES.md"): {},
		repoPath("devdocs", "HANDOFF", "framework-prompt", "ARCHITECTURE.md"): {},
		repoPath("devdocs", "HANDOFF", "framework-prompt", "DECISIONS.md"):    {},
	}

	banned := []string{
		"framework.prompt/v1",
		"YAML front matter",
		"extends:",
		"requires_providers",
		"apiVersion:",
	}

	err := filepath.WalkDir(repoPath("devdocs", "HANDOFF", "framework-prompt"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := expected[path]; !ok {
			t.Fatalf("unexpected handoff file remains: %s", path)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(data)
		for _, phrase := range banned {
			if strings.Contains(text, phrase) {
				t.Fatalf("%s still contains legacy contract language: %q", path, phrase)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk devdocs/HANDOFF/framework-prompt: %v", err)
	}
}

func TestPhase1_V2ContractDocsPresent(t *testing.T) {
	path := repoPath("devdocs", "RECIPES", "prompt-format-re-integration.md")
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s should not exist after v2 cutover", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}

	checks := []string{
		repoPath("devdocs", "HANDOFF", "framework-prompt", "OVERVIEW.md"),
		repoPath("devdocs", "HANDOFF", "framework-prompt", "API_SURFACES.md"),
		repoPath("devdocs", "HANDOFF", "framework-prompt", "ARCHITECTURE.md"),
		repoPath("devdocs", "HANDOFF", "framework-prompt", "DECISIONS.md"),
	}

	for _, path := range checks {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, phrase := range []string{
			"framework.prompt/v1",
			"apiVersion: v1",
			"extends:",
			"requires_providers",
		} {
			if strings.Contains(text, phrase) {
				t.Fatalf("%s still contains legacy contract language: %q", path, phrase)
			}
		}
	}
}

func repoPath(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}
