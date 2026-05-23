package prompt

import (
	"io/fs"
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

func repoPath(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}
