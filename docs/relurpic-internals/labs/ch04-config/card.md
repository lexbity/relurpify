# Lab — ch04-config

**Chapter:** *Relurpic Internals — Inside Look*, Chapter 4
("Configuration and the One Boundary"). **Pin:** tag
`BETA-RC1-FALL2026`; captured at commit
`1bc57c62029641dcc919c04a8076593794246409` while the tag is pending.

**What this lab is:** the regenerate + drift transcript. You run the
drift loop in both directions — edit the template bundle (the source of
truth), then hand-edit the generated tree — and watch
`check-config-tree-drift` fail both, pass after regeneration, and pass
again after the full revert. Everything dirties tracked files and is
reverted in-checkout; `git status` is clean at the end. No model backend
needed.

## Files

- `card.md` — this card
- `template-edit.diff` — the one-line template edit (step 2)
- `transcript.txt` — captured outputs from the pin

## Command card

| # | Command | Deterministic? | Expected |
|---|---|---|---|
| 1 | `make check-config-tree-drift` | yes | `[PASS] relurpify_cfg/ matches embedded templates` |
| 2 | `git apply template-edit.diff && make check-config-tree-drift` | yes | `[FAIL] relurpify_cfg/ drifts from embedded templates. Run: make generate-config`, naming the differing `workspace.yaml` pair; exit 1 |
| 3 | `make generate-config && make check-config-tree-drift` | yes | `[PASS] relurpify_cfg/ matches embedded templates` (regeneration absorbs the template change) |
| 4 | hand-edit `relurpify_cfg/workspace.yaml` (any tracked file), then `make check-config-tree-drift` | yes | identical `[FAIL]` shape as step 2 |
| 5 | `git checkout -- relurpify_cfg/workspace.yaml userconfig/templates/embedfs/workspace/workspace.yaml && make check-config-tree-drift` | yes | `[PASS]`; `git status --porcelain relurpify_cfg/ userconfig/templates/` is empty |

The step-4 hand edit used in the transcript is one added comment line
above `paths:` in `relurpify_cfg/workspace.yaml`; any divergence works.

## Honesty notes

- The temp paths in the step-2/4 failures (`/tmp/tmp.…/out/…`) are
  per-run mktemp directories — compare the FAIL verdict and the named
  file pair, not the paths.
- The gate compares the *embedded bundle* against the *checked-in
  tree*; that is why a template-only edit fails even though no file in
  `relurpify_cfg/` changed (Chapter 4 §4.4).
- The gate's jurisdiction is the repo checkout. A live workspace's
  `relurpify_cfg/` is supposed to diverge; nothing here polices it
  (ADR-002: "your in-workspace edits are yours").
- `make generate-config` rewrites the checked-in tree from the bundle —
  run it only when the template edit is intentional (in this lab it is
  reverted immediately after).

## Verification

`verify.sh ch04` (from the labs root) replays card items 1–5 in a
scratch clone at the pin and diffs the deterministic outputs.
