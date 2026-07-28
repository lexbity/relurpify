# Templates (legacy)

This directory previously held repo-local template fallbacks. The canonical
source of truth is now the embedded FS at `userconfig/templates/embedfs/`.

- workspace config/profiles/policies/tools are served from `//go:embed all:workspace`
- prompt templates are served from `//go:embed all:prompts`

No subdirectories in `templates/` are used at runtime. The directory exists
only for this README.
