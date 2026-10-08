# Known Limitations

This file records verified sharp edges that should stay visible in the docs.

- The repository does not ship a Dockerfile, compose file, Helm chart, or
  Terraform configuration. Distribution is source build plus local binary
  execution only.
- The runtime uses more than one persistence mechanism. The job store and graph
  layers use Badger-backed storage, while other artifacts and checkpoints are
  filesystem-backed or in-memory. Do not describe the project as a single-store
  system.
- `nexus` is stale in older comments and docs. The current runtime notes say the
  gateway and node-provider registration were removed or shelved. Treat existing
  `nexus` mentions as historical unless a future change reintroduces them.
- `ayenitd` is internal infrastructure, not a user-facing command. If a doc
  needs a launch path, use `relurpish`.
- The repository contains mixed model-name examples in older docs and tests,
  including `gemma4:e4b` and `gemma4:12b`. The suite YAML for a given run is the
  authoritative source.
- The built-in `offline` backend is intentionally limited to deterministic
  plumbing and CI use. It is not a substitute for a real interactive model.
- Documentation may describe command shapes that were verified in code but not
  re-run in this workspace. Those should be treated as verified-from-source, not
  as freshly executed session output.
