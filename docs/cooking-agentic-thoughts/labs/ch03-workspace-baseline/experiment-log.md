# Chapter 3 experiment log — captured outputs

## 1. First relurplint run

```text
$ relurplint --check all --workspace .
relurplint: all checks passed (0 diagnostics)
(exit 0)
```

## 2. Tree inventory (counted)

```text
model/provider/  : 4 (ollama, lmstudio, openai_compatible, offline)
model/profiles/  : 3 (default.llm.yaml, gemma4.llm.yaml, qwen2.5-coder.llm.yaml)
security/        : 4 (sandbox, shell, localtool, workspaceingestion .policy.yaml)
tools/           : 99 manifests — file 6, git 5, lsp 7, search 3, test 5,
                   shell 73 (in 8 category subdirectories under tools/shell/)
prompts/         : 29 files (agents/, framework/, named/)
euclo/           : 10 .erpe recipes + families.yaml
```

Verification: `find relurpify_cfg/tools -name "*.yaml" | wc -l` → `99`;
`find relurpify_cfg/tools/shell -name "*.yaml" | wc -l` → `73`;
`find relurpify_cfg/prompts -name "*.prompt" | wc -l` → `29`.

## 3. The posture dial

Edit: `selection.posture: deterministic → model_assisted` in
`workspace.yaml`.

```text
$ relurpish status --format json | grep -o "model_assisted\|deterministic"
model_assisted
```

Real boot banners (both captured at 220×50):

```text
posture: model_assisted  ->  Postures — approvals: governed; intent selection: model_assisted
posture: deterministic   ->  Postures — approvals: governed; intent selection: deterministic
```

Doctor's `selection_posture` row printed the same static `deterministic: …`
explanation in both states — known rough edge, noted in the chapter.

## 4. Precedence probes

- `RELURPIFY_OLLAMA_HOST=http://localhost:9999 relurpish doctor …` → the
  ollama row still probed `http://localhost:11434` and reported
  `ollama [ready]`. No visible delta.
- `RELURPIFY_MODEL_PROVIDER=offline RELURPIFY_MODEL_NAME=offline-synthetic
  relurpish doctor …` → inference section still `provider: ollama`.
  No visible delta.
- Chapter 1's `--offline` CLI flag, by contrast, flipped the inference
  section to `provider: offline … selected: offline-synthetic`.

Conclusion carried in the chapter: CLI and workspace rungs proven; env
rung verify-first.

## 5. doctor --fix prompt

```text
$ relurpish doctor --fix --workspace . < /dev/null
Result: ready
Overwrite current starter config and manifest from templates? [y/N]:
(exit 0; EOF took the default N; edits intact)
```

## 6. RELURPIFY_STRICT

With a `workspace.yaml` copy missing `paths`, `logging`, `audit`, and
`telemetry`:

```text
$ RELURPIFY_STRICT=1 relurpish doctor --workspace .
Result: blocking issues detected
Error: doctor found blocking issues
```

Restored the keys:

```text
Result: ready
```

## 7. Sandbox image digest

`security/sandbox.policy.yaml` ships `image_digest: ""` with the comment:
leave empty to resolve from the local daemon at boot; set `sha256:…` to
pin. The verification machine's local sandbox image was built locally
(`docker images --digests` shows `<none>` for its repository digest), so
the pin exercise is left to the reader; the warning stays informational
either way.

## Status of this lab

Fully captured — every output above is from a real run. The one honest
gap: the TUI banner captures used an expect-style pty driver (220×50) as
in Chapter 2; on a normal terminal the same text renders in your session.
