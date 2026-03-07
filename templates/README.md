# Templates

This directory is the repo-local development fallback for Relurpify starter assets.

The current model is:

- installed shared templates are the primary source for starter assets
- repo-local templates are development fallbacks
- workspace copies inside `relurpify_cfg/` become the runtime source of truth after initialization

Current categories:

- `agents/` for starter agent manifests
- `workspace/` for starter workspace files
- `skills/` for starter skill files
- `testsuite/` for derived testsuite workspace profiles

`templates/` is the canonical repo-local development fallback tree.
