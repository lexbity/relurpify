# Chapter 3 lab — workspace baseline

The captured state of every experiment in "Own the Kitchen," on the
verification machine (Linux, Ollama 0.40.8 serving `gemma4:e4b`, binaries
built from the tree that became `BETA-RC1-FALL2026`).

## Files

- `workspace-annotated.yaml` — the materialized `workspace.yaml`, trimmed
  to the keys the chapter teaches, with the captured values.
- `experiment-log.md` — every hands-on step with its captured output.

## Reproduce

From your workspace root, with your built binaries:

```bash
relurplint --check all --workspace .        # expect: all checks passed (0 diagnostics)
relurpish status --format json | grep -o "model_assisted\|deterministic"
relurpish doctor --fix --workspace . </dev/null   # observe the [y/N] prompt; default declines
RELURPIFY_STRICT=1 relurpish doctor --workspace . # with a key-stripped copy: blocking issues
```

Both posture banners were captured from real boots and are quoted in the
experiment log.
