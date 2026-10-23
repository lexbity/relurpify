# Chapter 10 — Test It Like the Framework Does

Your paradigm has three proofs so far: thirty-eight unit tests (the fake
model's domain), a golden triple (the DSL's domain), and one live TUI run
(the operator's domain). This chapter adds the framework's own test
layer — **agenttest** — where behavior is asserted from the outside: real
runs, real receipts, structured assertions in three tiers. You will write
the critic's suite, watch three subsystems fight over your prompt's
wording, meet the ratchet that guards the committed catalog, and learn
where the tape seam is honest about its own limits. By the end you will
know what the framework considers *proof* at this layer — and what it
refuses to certify.

## Where you are

The paradigm is registered and DSL-addressable (Part III complete). This
chapter's overlay adds two files: the suite and a one-line ratchet bump.
The lab runs against your live Ollama model — `gemma4:e4b`, the same one
book 1 tested against — because agenttest's whole point is asserting on
real runs.

## The three layers, and where the seams are

The framework's test stack has three layers, and knowing which layer owns
which claim is half the discipline:

1. **Unit** (`go test ./...`) — no external services. Your thirty-eight
   tests: the fake model's contract, the graph semantics, the agent's
   loop under governance, the prompt file. Deterministic, fast, yours.
2. **Framework integration** — the `testsuite/` builders: the golden
   harness you met in Chapter 9 (plan/graph/trace triples), conformance
   (Chapter 11's harness), and the registry-level tests.
3. **Agenttest** (`dev-agent agenttest run`) — a real agent against a
   real (or replayed) model, asserting on outputs, file mutations,
   state, and metrics. This is where "the agent behaves" becomes a
   checkable claim.

One architectural fact frames everything you write today, discovered by
reading the harness: **the agenttest runner is euclo-shaped**
(`testsuite/agenttest/execution.go` builds `*euclo.Agent` directly). The
`agent_name` field resolves against exactly one subject. The "subject
adapters" (`testsuite/subjects/euclo/adapter.go`) are the euclo-specific
*assertion* extensions — the `extensions: euclo:` blocks — not a pluggable
agent seam. So the open question from the plan — *does the critic need a
subject adapter?* — resolves honestly: **no, and none is possible**: at
this layer the critic is exercised the way the runtime exercises it, as a
paradigm inside a recipe run driven by euclo. A paradigm-level subject
seam would be a framework contribution of its own — noted for the
internals book.

## The OSB model

Every `expect:` block speaks three dialects, and the harness enforces
exactly what it claims to — the enforcement tables are code
(`testsuite/agenttest/osb_coverage.go`), and a committed suite cannot
declare an assertion the harness silently ignores (the coverage gate
fails on unevaluated fields):

- **Outcome** — hard. `must_succeed`, `no_file_changes`,
  `files_changed`, `output_contains`/`output_regex`,
  `state_keys_must_exist`, memory and workflow counts.
- **Security** — hard, some runtime-enforced. `tools_must_not_call`,
  `no_writes_outside_scope`, `no_reads_outside_scope`,
  `mutation_enforced`, `expected_violations` (for negative tests: the
  sandbox correctly *blocked* something — these do not fail). The table
  distinguishes telemetry-evaluated fields from runtime-enforced ones
  (`no_exec_outside_manifest`, `no_network_outside_manifest` — enforced
  by the command policy and the sandbox's `--network none`, which no
  harness telemetry could re-assert; the table says so instead of
  pretending).
- **Benchmark** — advisory by design. `tools_expected`,
  `tools_not_expected`, `max_tool_calls_hint`: thresholds and negative
  assertions are hard, presence mismatches are recorded, not failed.
  Book 1 met this triad; today you write it.

Read the tier table once as philosophy: an agent framework that cannot
state *what must not happen* (security) separately from *what should
happen* (outcome) and *what usually happens* (benchmark) will eventually
certify the wrong thing. OSB is that separation, enforced.

## The suite

`euclo.critic.testsuite.yaml` follows the house shape — and the closest
precedent is a gift: book 1's own `euclo.stringkit.osb.testsuite.yaml`
(all three OSB kinds against the stringkit fixture, read-only path).
The critic suite, two cases:

```yaml
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agent.yaml
  execution: { profile: record, strict: false, timeout: 180s }
  models:
  - name: gemma4:e4b
    endpoint: http://localhost:11434
  recording: { mode: record }
  cases:
  - name: critique_readonly_route
    …
    prompt: explain and critique the slugify function in stringkit.py
    expect:
      outcome:  { no_file_changes: true }
      security: { tools_must_not_call: [file_write, file_delete] }
      benchmark: { max_tool_calls_hint: 8, tools_expected: [file_read], tools_not_expected: [file_write] }
  - name: critique_seeded_bug
    # the same route against book 1's seeded punctuation bug
```

Case two re-seeds the book-1 buggy `slugify` (punctuation kept) — the
critique has real material, and the lineage between the two books is
literal: the same fixture, read by the paradigm the second book built.

### The green run, and what it proves

```text
$ ./dev-agent agenttest run --suite testsuite/agenttests/euclo.critic.testsuite.yaml
euclo.critic.testsuite.yaml [record]: 2/2 passed (0 infra, 0 assertion)
```

Per-case receipts: one LLM call each (~660 tokens), zero tool calls (the
source arrived inline via `context_file_contents` — the agent never
needed the filesystem), **no files changed**, no forbidden tools touched.
What the run certifies, honestly: the critique-shaped instruction routes
into the investigation family and the run's *security envelope holds* —
read-only in fact, not just in intent. The benchmark tier recorded the
advisory gap (`file_read` never called) — advisory working as designed.

Two honesty notes the receipts force, and neither is a footnote:

**The dispatcher, not the suite, picks the recipe.** Both cases routed
to the shipped `explain` recipe — the dispatcher's token scoring chose
among the investigation-family registrants, and `critic_review` did not
win headless (it did win in Chapter 9's TUI run). Suite YAML cannot pin a
recipe at this tree. The critic *recipe's* behavioral proof therefore
remains where it already lives: the golden trace (Chapter 9), the unit
tests, and the runner. `[NEEDS CONFIRMATION: dispatcher recipe-scoring
mechanics — open item 4, pinned for ongoing research.]`

**Read-only recipe runs close unverified — by design.** The loop-close
honesty rule you met in Chapter 1 downgrades a closed run without an
edit observation unless the task type is exempt (review/explain/
analysis/plan — the chat-mode types). A recipe-routed read-only run is
not exempt, so `must_succeed: true` would assert against the framework's
own honesty. The suite asserts what is true instead — `no_file_changes`
*is* the outcome assertion for a read-only critique — and reserves
`must_succeed` for cases whose outcome is verifiable. Asserting less,
precisely, beats asserting more, wrongly.

## Three subsystems read your prompt

The suite's first drafts failed three times, and each failure is a
chapter in itself — captured in full in the transcript:

1. **The family classifier** saw "Do not edit any files" and tied
   *edit* (implementation) against *explain* (investigation). The
   dispatcher chose the clarification recipe deterministically; the run
   emitted an interaction frame; the headless harness had no human, and
   the case closed with zero LLM calls.
2. **The edit-shape heuristic** saw "change nothing" — *change* matches
   the edit pattern — and the loop-close honesty rule downgraded the
   closed run to unverified; `must_succeed` failed on a run that had
   done exactly what was asked.
3. **The interaction gate** saw "name the punctuation bug" — *bug* tied
   debug against investigation — emitted a clarification frame, and
   blocked to the 180-second timeout.

The final prompts carry investigation vocabulary only. The lesson
generalizes: an agent's prompt is read by every classifier upstream of
it, and a suite author must satisfy all of them at once. When a case
fails with zero LLM calls, do not debug the model — debug the route.

## The ratchet

Adding a committed suite trips the catalog guard:

```text
--- FAIL: TestCommittedSuitesOSBCoverage
    osb_coverage_test.go:20: expected 24 committed suites, found 26
```

The count is a ratchet, deliberately: a new suite is a new set of claims
the catalog vouches for, so it goes in the PR with its eyes open — the
bump is one line, commented with the reason (this book's +1, book 1's
+1). You met load-bearing exemptions in Chapter 3; this is the same
taste for test catalogs. Silent growth is what the count prevents.

## Tapes: the seam, read honestly

The plan promised record → promote → replay. The honest findings, all
source-verified:

- **The seam exists and is real**: `TapeModel` wraps a model in record
  or replay mode; replay matches requests by SHA-256 fingerprint; the
  promote command writes a tape plus a lineage file; the lineage records
  its own staleness against a 30-day threshold (`staleTapeThreshold =
  30 * 24 * time.Hour`), and `rerecord` plans re-records for missing or
  stale fixtures.
- **Recording is not reachable from a suite run at this tree.** The
  runner passes the suite's provider family to the model runtime; the
  ollama provider ignores the tape path, and the registered `tape`
  provider kind constructs replay-only backends. So the green run wrote
  no `tape.jsonl`, and promote says so:

  ```text
  case "critique_readonly_route" tape invalid: open …/tape.jsonl:
  no such file or directory
  ```

- **The committed lineage fixture demonstrates the promoted shape** —
  `euclo.gemma4.smoke`'s lineage shows `provider: "tape"`, the
  destination tape path, and `status: "stale-awaiting-rerecord"`: a
  promoted tape past its 30 days is *marked*, not deleted, and the
  rerecord planner picks it up. The honesty vocabulary is complete even
  where the recording path is not.

`[NEEDS CONFIRMATION: the reachable tape-recording path at this tree —
carried for Chapter 12, where live models are the subject anyway.]`

This is what "test it like the framework does" means at the limit: you
adopt not just the harness but its honesty about what the harness cannot
yet do.

## Gotchas

- **A case failing with zero LLM calls is a routing failure.** Read
  `route_selection` in the case report first: `intent.clarify` means a
  family tie; no route means setup. The model is the last thing to
  suspect.
- **Prompt wording is load-bearing three ways** (family keywords,
  edit-shape heuristic, clarification ambiguity). Prefer instructions
  that are clean in all three; when one must bend, the transcript's arc
  shows which subsystem you are bending.
- **`must_succeed` on read-only recipe runs fights the honesty rule.**
  Assert the envelope instead — `no_file_changes` is a *stronger* claim
  for a read-only critique, because it is the claim you actually mean.
- **The suite catalog count is a ratchet.** Adding a suite without the
  bump fails the coverage gate; adding the bump without the suite fails
  it too.
- **`recording.mode: record` does not make tapes against ollama** at
  this tree. Budget your proof accordingly: live run + receipts now,
  tapes when the recording path lands.

## Summary

- Three layers, three owners: unit tests own determinism, the golden
  harness owns the DSL, agenttest owns behavior-from-outside. The
  agenttest runner is euclo-shaped — the critic is tested as the runtime
  runs it, inside recipe runs; a paradigm-level subject seam is an
  internals-book contribution.
- OSB is the assertion philosophy: outcome hard, security hard (with
  runtime-enforced fields named as such), benchmark advisory — enforced
  by code tables, guarded by a catalog ratchet.
- The suite is green live: 2/2, security envelope pinned, advisory
  records honest — with two `[NEEDS CONFIRMATION]`s carried (dispatcher
  scoring; tape recording path) rather than papered over.
- Prompt authorship for agents is multi-classifier engineering: family,
  edit-shape, and ambiguity gates all read the same words.

## Check yourself

1. Why does the critic not get its own subject adapter, and what would a
   paradigm-level subject seam actually be a contribution of?
2. Name the three OSB tiers, one assertion from each, and which tier
   `tools_must_not_call` belongs to — plus the one security vocabulary
   distinction the coverage table draws *inside* that tier.
3. Your case fails with zero LLM calls and route
   `euclo.thoughtrecipe.intent.clarify`. What happened, and what in the
   prompt would you look for?
4. Why is `no_file_changes: true` a *stronger* assertion than
   `must_succeed: true` for this suite's cases — and which framework
   rule makes it so?
5. What does the committed lineage's `status: "stale-awaiting-rerecord"`
   tell you about how the framework treats aged proofs?

*Answers.* 1. The runner builds `*euclo.Agent` directly; subjects are
euclo assertion extensions, not an agent seam — the critic is exercised
as a paradigm inside recipe runs; a paradigm subject seam would be a
framework-extension contribution (internals book). 2. Outcome
(`must_succeed`), security (`tools_must_not_call`), benchmark
(`tools_expected`); security distinguishes telemetry-evaluated fields
from runtime-enforced ones (exec/network manifests — enforced by the
command policy and sandbox, recognised rather than re-asserted). 3. A
family-keyword tie sent the run to deterministic clarification; a
headless harness cannot answer the frame — look for words that score
into two families at once (edit/bug/fix…). 4. The loop-close honesty
rule: a read-only recipe run closes unverified (no edit observation,
task type not exempt), so `must_succeed` would fail honestly — while
`no_file_changes` asserts exactly the fact the case exists to prove. 5.
Aged proofs are marked, not trusted silently and not deleted — the
rerecord planner picks them up; staleness is honesty about decay.

## Try it yourself

1. **Break the envelope on purpose.** Add `file_write` to the recipe's
   allowed tools for the revise delegate and a case instructing a written
   critique report (`files_changed: [CRITIQUE.md]`,
   `no_writes_outside_scope: true`). Watch the honesty rule close the
   run *verified* this time — the edit observation exists. You will have
   built the write-path twin of this chapter's read-only cases.
2. **Read the enforcement tables.** Open
   `testsuite/agenttest/osb_coverage.go` and add a `security:` field
   that is in neither table to a scratch suite. Run the coverage test
   and read the failure — then remove it. You have just watched the
   "no silent pass" gate refuse an assertion nobody evaluates.

## Where next

The suite is green; the contribution's remaining gate is the gauntlet
itself. Chapter 11 walks the merge path as curriculum: the coverage
floor and its ratchet, the conformance harness (your contract's four
cases, executable), the AST gates as a writing discipline, grep fences,
domain check — ending with the full green gate transcript for the critic
contribution.

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch10` re-runs the tier, and
`transcripts/ch10-agenttest.md` holds the green run, the debugging arc,
the ratchet bump, and the tape findings in full.
