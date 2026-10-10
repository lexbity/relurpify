# prep — the hermetic dry-run tier

`prep` drives real `.erpe` recipe text through the real loader, the real
`euclo.Agent.Execute`, the real RootGraph, real paradigm agents, and scripted
capabilities against a scripted model — with no LLM, no network, and no
security bundle. It is the middle test tier between hermetic conformance
(no boot) and the live lane (real model, governed).

## What it proves

- **Dispatch**: an authored recipe is selected for an instruction (or the
  built-in default route is taken when nothing matches), with the decision
  provenance on the report.
- **Execution**: the recipe's graph runs to completion against scripted
  capabilities, including the canonical 7 recipes extracted from the embedded
  template.
- **Capability invocation**: the DSL's `may invoke` scoping is exercised for
  real — scripted capabilities are admitted through workspace tool manifests and invoked
  through the same registry surface a live run uses.
- **Prompt content**: every model message is recorded verbatim. A step with a
  `stream` clause must have the seeded chunk's body in the following model
  call, verbatim and in compiler rank order; budget headers must match the
  compiler's accounting.
- **The full BKC loop**: seed → prompt (backward pass) and capture →
  grounding → next prompt (forward pass), hermetically.

## What it does NOT prove

**This is not a governance tier.** The dry run loads no security bundle, the
fake command runner denies by default, and no assertion here says anything
about authorization or sandbox behavior. A capability that behaves differently
under a real policy will pass dry and fail live — that divergence is exactly
what the live tier exists to catch.

## Running

```
go test ./testsuite/agenttest/prep/... -count=1
```

Hermetic: no tags, no network. The full table completes in well under a
minute.

## Adding a case

1. Author a fixture under `testdata/relurpify_cfg/euclo/` (the workspace
   layout the real loader scans). Give the recipe's trigger block a
   `keyword ["<distinctive-token>"]` association and mention that token in the
   case instruction — route selection scores token overlap between the
   intent evidence and the recipe's keyword vocabulary, which keeps dispatch
   deterministic without an LLM.
2. Script the mind: `testhelper.ModelTurn` values are consumed in order and
   the last repeats. **The first turn is consumed by the route-disambiguation
   model call** before any paradigm runs. Paradigms run with native tool
   calling disabled, so react decisions arrive as prompt JSON:
   `{"thought":..., "action":"tool|complete", "tool":..., "arguments":{...}}`.
3. If the recipe invokes capabilities, declare a `ScriptedCapability` (the
   harness generates its workspace tool manifest automatically so the
   registry's deny-by-default admission lets it through.

## Assertion vocabulary

The `DryRunReport` carries everything a case asserts on:

| Field | Meaning |
|---|---|
| `Dispatched` / `DecidedBy` / `FallbackTaken` | route selection and provenance |
| `ParadigmRuns` | paradigms observed in telemetry, in order |
| `CapabilityCalls` | scripted capability invocations with arguments |
| `ModelMessages` | every prompt, verbatim (debugging tier; no redaction) |
| `StreamedSections` | rendered streamed-context sections per model call |
| `Knowledge` | grounded chunks (IDs, hashes, bodies, token counts) |
| `Events` | the full telemetry trail |
| `Errors` / `Success` | run-level failure surface (model starvation fails loudly) |

Model starvation (the script runs out before the agent stops calling) returns
`ErrTurnsExhausted` through the report — an under-scripted case fails with a
clear diagnosis, never a hang.
