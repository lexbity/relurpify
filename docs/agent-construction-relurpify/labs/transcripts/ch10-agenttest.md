# Chapter 10 — agenttest transcripts (captured)

Real output from the verification machine (same environment as the earlier
transcripts; live Ollama serving `gemma4:e4b`). The Chapter 10 overlay
(`ch10-agenttest`) adds `testsuite/agenttests/euclo.critic.testsuite.yaml`
and the suite-count ratchet bump in
`testsuite/agenttest/osb_coverage_test.go` (24 → 26: +1 for this book's
suite, +1 for book 1's `euclo.stringkit.osb`, committed alongside).

## 1. The green run

```text
$ ./dev-agent agenttest run --suite testsuite/agenttests/euclo.critic.testsuite.yaml
euclo.critic.testsuite.yaml [record]: 2/2 passed (0 infra, 0 assertion)
```

Run `20261023-222227.638`, per-case receipts from `report.json`:

```text
critique_readonly_route: route=euclo.thoughtrecipe.explain  llm=1 tokens=657
                         tools=[] changed=None
critique_seeded_bug:     route=euclo.thoughtrecipe.explain  llm=1 tokens=667
                         tools=[] changed=None
```

Honest readings: the instruction routed into the investigation family
(the shipped `explain` recipe won the dispatcher's scoring — see
`[NEEDS CONFIRMATION]` below); the agent read from the inline context
(`context_file_contents` — zero tool calls needed), and **no file
changed** — which is exactly what the outcome (`no_file_changes`) and
security (`tools_must_not_call: [file_write, file_delete]`) assertions
pin. The benchmark tier recorded the advisory gap (`tools_expected:
[file_read]` not satisfied — presence mismatches are advisory, by design).

`[NEEDS CONFIRMATION: dispatcher recipe scoring — the critic_review
recipe (family investigation, keyword critique) did not win the headless
dispatch against the shipped explain/investigation recipes; pinned with
open item 4.]`

## 2. The debugging arc (all real, all instructive)

Three failed drafts preceded the green run, each caught by a different
subsystem reading the same prompt:

1. **Intent tie → clarification.** Prompt containing "Do not edit any
   files": the word *edit* (an implementation-family keyword) tied against
   *explain* (investigation); the run routed to
   `euclo.thoughtrecipe.intent.clarify`, emitted an interaction frame,
   and closed with zero LLM calls:

   ```text
   route: euclo.thoughtrecipe.intent.clarify  decided_by: explicit
   LLM calls: 0        result: 0/2 passed (assertion)
   ```

2. **Edit-shape heuristic.** Reworded with "change nothing": routed
   correctly, 2 LLM calls — but *change* matches the edit-shape pattern,
   the loop-close honesty rule fired (`unverified` in telemetry), and
   `must_succeed: true` failed.

3. **Clarification frame, headless.** Case B's prompt containing "name
   the punctuation bug": *bug* tied debug against investigation; the run
   emitted `interaction.frame.emitted` and blocked for the full 180s
   timeout (no human answers a headless harness):

   ```text
   Success: false  FailureKind: timeout  dur: 180160ms  LLM: 0
   ```

The fix — prompts with investigation-family vocabulary only ("explain
and critique …") — is itself the lesson: the family classifier, the
edit-shape heuristic, and the interaction gate all read the prompt, and
a suite author must satisfy all three.

## 3. The read-only honesty interplay

Recipe-routed read-only runs close **unverified** by the loop-close
honesty rule (only chat-mode task types — review/explain/analysis/plan —
are exempt), so `must_succeed: true` on a read-only recipe run asserts
against the framework's own honesty. The suite therefore asserts what is
true: the security envelope (`no_file_changes`, `tools_must_not_call`)
and the advisory benchmark record. `must_succeed` is reserved for cases
whose outcome is verifiable.

## 4. The OSB coverage ratchet

Adding a committed suite trips the catalog-count ratchet:

```text
--- FAIL: TestCommittedSuitesOSBCoverage
    osb_coverage_test.go:20: expected 24 committed suites, found 26
```

The fix is the deliberate bump (the PR carries `+1` per added suite):

```text
$ go test ./testsuite/agenttest/ -count=1
ok  	codeburg.org/lexbit/relurpify/testsuite/agenttest	0.903s
```

## 5. Tapes: the seam, and an honest limit

Promotion from the green run fails — the run produced no `tape.jsonl`:

```text
$ ./dev-agent agenttest promote --suite … --run … --case critique_readonly_route
case "critique_readonly_route" tape invalid: open …/execution/artifacts/tape.jsonl:
no such file or directory
```

Source-verified why: `buildModel` passes the suite's provider family to
the model runtime (`Kind: desc.BackendFamily`); the ollama provider
ignores `TapePath`, and the registered `tape` provider kind constructs
replay-only backends (`NewTapeModel(nil, path, TapeReplay)` in
`platform/llm/providers_tape.go`). At this tree, suite runs against
ollama do not emit tapes; promote requires one. The committed lineage
fixture demonstrates the promoted shape and the staleness vocabulary:

```json
{
  "suite_name": "euclo.gemma4.smoke",
  "provider": "tape",
  "promoted_artifacts": ["tape.jsonl"],
  "destination_tape": "testsuite/agenttests/tapes/euclo.gemma4.smoke/simple_code_edit__gemma4_e4b.tape.jsonl",
  "status": "stale-awaiting-rerecord",
  "created_at": "2026-06-13T00:00:00Z"
}
```

`[NEEDS CONFIRMATION: the reachable tape-recording path at this tree —
pinned for Chapter 12 research.]`

## 6. Gates

```text
$ ./verify.sh gates ch10
[PASS] gates: ch10 holds the framework's law
```
