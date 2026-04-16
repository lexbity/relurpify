package langdetect

import (
	"path/filepath"
	"strings"
)

func detectFile(name string, langs *WorkspaceLanguages) bool {
	matched := false
	base := filepath.Base(name)

	if base == "go.mod" || strings.EqualFold(filepath.Ext(base), ".go") {
		langs.Go = true
		matched = true
	}

	if base == "pyproject.toml" || base == "setup.py" || base == "setup.cfg" || base == "requirements.txt" || strings.EqualFold(filepath.Ext(base), ".py") {
		langs.Python = true
		matched = true
	}

	if base == "Cargo.toml" {
		langs.Rust = true
		matched = true
	}

	if base == "package.json" || base == "tsconfig.json" || base == "deno.json" {
		langs.JS = true
		matched = true
	}

	return matched
}

