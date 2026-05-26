# Templates

This directory is the repo-local development fallback for Relurpify starter assets.

The current model is:

- installed shared templates are the primary source for starter assets
- repo-local templates are development fallbacks
- workspace copies inside `relurpify_cfg/` become the runtime source of truth after initialization

Current categories:

- `agents/` for starter agent configs
- `workspace/` for the canonical workspace starter set
- `skills/` for starter skill files
- `testsuite/` for derived testsuite workspace profiles

The workspace starter set is intentionally minimal:

- `workspace/workspace.yaml`
- `workspace/agent.yaml`
- `workspace/security/*.policy.yaml`

`templates/` is the canonical repo-local development fallback tree.
