# Chapter 11 — Passing the Gates

Everything you have built — the package, the contract, the agent, the
wiring, the suite — now faces the gauntlet: the same gates every
contributor's PR runs. This chapter is the gauntlet as curriculum. You
will run each gate, meet the failures your contribution causes, and fix
every one at the cause — never at the gate. Three of the failures you
meet are the best teachers in the book: one catches a real integration
gap in your agent, one teaches how the framework keeps its configuration
honest, and one hands you the coverage philosophy in a single output
line. When the last gate goes green, the contribution is real in the
only currency this repository trades in.

## Where you are

Thirty-four tests green; the suite green live; the paradigm complete.
This chapter's overlay is the thickest yet, because the gauntlet touches
everything: conformance runners and fixtures, the regenerated matrix, a
surface parity test, a coverage override, two embedded templates — and
one two-line fix to the agent itself that a gate caught.

## The conformance harness: your contract, executable

Chapter 7 declared four conformance cases as data. The harness
(`testsuite/conformance/`) makes them executable — and it is
**registry-driven**:

```go
for _, contract := range paradigm.Registry.All() {
	for _, cas := range contract.Conformance {
		t.Run(contract.Paradigm+"/"+cas.ID, func(t *testing.T) {
			runner := conformanceRunners[cas.ID]
			if runner == nil {
				t.Fatalf("conformance case %q has no runner (contract declared, runtime drift)")
			}
			runner(t)
		})
	}
}
```

Run it cold and your contribution fails exactly as designed:

```text
conformance case "critic/critique_writes_scratch" has no runner
(contract declared, runtime drift)
```

*Contract declared, runtime drift* — the phrase is the harness's whole
philosophy: a declaration without a proof is drift, and drift fails CI,
not production. The fix is four runners mirroring reflection's line for
line: a scripted model, a fixture recipe run through the **real recipe
path** (parser → contract validation → lowering → graph → Execute), and
assertions on the envelope. The fixtures are `.erpe` files in the
committed fixture directory — `critic_critique.erpe` (a lone `critique`
directive) and `critic_revise.erpe` (critique + predicate revise +
capture).

One failure here deserves retelling because it is a fixture-design
lesson: the pass case first ran the *revise* fixture (which carries
`capture revisions -> state.critic_revisions`) and failed — the capture
had bound the result field `revisions: 0`. Present, not absent.
Reflection ships a separate pass fixture for exactly this reason, and
now so does the critic. A fixture asserts by shape as much as by
content; share shapes only when the assertions share semantics.

### The matrix: generated, never hand-written

The committed `paradigm_matrix.yaml` is regenerated from the registry
(`UPDATE_MATRIX=1`, the goldens' discipline again) and its invariants
test refuses unannotated rows — the status vocabulary (`honored` for
effects that predated the harness, `implemented` for effects built with
it) is part of the record. Your four cases are `implemented`, because
you implemented them, this week, in this book.

### The stream matrix: the gate that caught the agent

The deepest failure of the chapter:

```text
paradigm "critic" is registered but has no streamed-context matrix
runner (paradigm_not_integrated)
```

Adding the runner was not enough — running it proved the critic **never
rendered the compiled context slice into its critique prompt**.
Reflection does: `paradigm.StreamedSection(ctx, env, "reflection")`
renders the substrate's compiled slice into the review's system slot
(the D-2 seam), and the matrix proves it by seeding a sentinel chunk and
asserting the sentinel reaches a model call verbatim. Your Chapter 8
agent skipped the seam entirely — and nothing up to now could see the
omission, because unit tests never wire a compiler.

The fix is in the **agent**, not the test:

```go
streamed, err := paradigm.StreamedSection(ctx, env, "critic")
if err != nil {
	return "", nil, err
}
critiqueMessages := []model.Message{
	{Role: "system", Content: critiqueSystemPrompt},
}
if streamed != "" {
	critiqueMessages = append(critiqueMessages, model.Message{Role: "system", Content: streamed})
}
```

Then `streamMatrixCritic` seeds the sentinel and the matrix passes —
with all thirty-four package tests still green. Read what happened
there: a conformance gate *outside your package* found an integration
gap *inside* it that every test inside it was blind to. That is what
conformance is for, and it is why the runner table is checked against
the registry dynamically — "a paradigm added to the registry without a
stream integration fails this gate in CI instead of silently dropping
the slice in a live run."

## The ratchets you meet on the way out

Two count-and-vector gates catch the contribution's breadth, and both
are fixed by deliberate, one-line-each edits that a PR carries openly:

**The surface parity vectors** (`named/euclo/surface/paradigm_test.go`):
the canonical paradigm lists grew `critic` in Chapter 9; the legacy
parity vectors grow with them (`AllParadigms` 9→10, `AgentParadigms`
8→9). The test exists to make list growth visible; growing it is the
point.

**The config-tree drift gate** (ADR-002) — the subtlest gate in the
repository, and the one most worth internalizing:

```text
Only in relurpify_cfg/euclo: critic_review.erpe
Only in relurpify_cfg/prompts/agents: agent.critic.default.prompt
```

The checked-in `relurpify_cfg/` is not editable content — it is the
*materialization* of the embedded template bundle, and the drift gate
diffs the two. Dropping files into `relurpify_cfg/` works right up until
this gate, and then it doesn't. The honest fix is the ADR's own recipe:
add the recipe and the prompt as **template sources**
(`userconfig/templates/embedfs/workspace/euclo/critic_review.erpe`,
`…/prompts/agents/agent.critic.default.prompt`), then regenerate:

```text
$ make generate-config
$ make check-config-tree-drift
[PASS] relurpify_cfg/ matches embedded templates
```

After regeneration, `git status relurpify_cfg/` shows exactly the two
new files — the tree is a build output, and the gate proves the build is
clean. This is why Chapter 8's prompt file was honest but incomplete:
authoring it was necessary; templating it is what makes it shippable.

## The coverage gate, read properly (D9)

The plan's decision log records the maintainer's answer on coverage, and
the gate's own output states it perfectly:

```text
  [PASS] codeburg.org/lexbit/relurpify/cognitionzoo/critic           78.28% (floor  40; 209/267 stmts)
  [PASS] codeburg.org/lexbit/relurpify/cognitionzoo/critic/cmd/critic-runner   0.00% (floor   0; 0/54 stmts)
[PASS] coverage: all 151 packages at or above their floors (default floor 40%)
```

Three facts in one gate:

- **The enforced floor is 40% globally** (the Makefile's default), with
  per-package floors in `tooling/coverage/overrides.yaml` — the
  ratchet: every entry records a package's best, floors never go down
  (`coverage-baseline` refuses), and new tests ratchet them up. The 70%
  figure in `CONTRIBUTING.md` is the project's **aspirational** target
  while it is in beta — the maintainer's own words (D9): the 40% floor
  is temporary until the project is ironed out. You write tests against
  70; the gate enforces 40; and your package clears both.
- **Main packages get explicit `floor: 0` entries with a reason** — the
  house pattern (`reason: …; ratchets up from 0 as tests land`). Your
  runner main carries one. A demonstration main with zero tests is a
  documented fact, not an oversight.
- **Your package lands at 78.28%** — above the aspiration — because
  every line of it was written test-first from Chapter 2 onward. That is
  the real lesson: coverage is a *lagging* indicator of a discipline you
  either had or didn't. You had it.

## The AST gates as a writing discipline

You met `lint-arch` in Chapter 3 as law; meet it now as *writing
discipline*. Your contribution passes it without effort because the
discipline was designed in: one package under `cognitionzoo`, imports
that respect the DAG, no env reads, none of the charter's four bans.
Note what you never did: you never added an exemption. The exemption
tables are load-bearing (`check-gates-honest` verifies the gates still
bite, and the gates' own tests fail when an exemption stops being
needed) — an exemption is a debt the next contributor pays interest on.
The grep fences, the domain check, the honesty check: all green, all
untouched. The cleanest PR diff is the one that changes no gate.

## The final transcript

The chapter's lab artifact is the full gauntlet, green, in one pass:

```text
lint-arch:                  [PASS]
check-gates-honest:         [PASS]
domain-check:               [PASS]
grep-architecture-gates:    [PASS]
test-euclo-golden:          [PASS]
test-conformance:           [PASS]
test-coverage:              [PASS]  (151 packages at or above floors)
```

Every gate green, zero exemptions added, two ratchets bumped with
reasons, one agent fix that a gate forced, and a config tree regenerated
from its templates. Read the sentence back: that is what "merge-ready"
means in this repository — not "the tests pass," but "the gates that
encode the project's self-respect all still hold, with my contribution
inside them."

## Gotchas

- **Conformance cases are declarations with a debt.** Every case your
  contract declares needs a runner in the same PR, or the harness calls
  it drift — correctly.
- **Fixture shape is assertion.** Sharing a fixture between cases with
  different assertions (the pass/revise capture) produces present-but-
  zero surprises. Ship separate fixtures when the assertions differ.
- **`relurpify_cfg/` is a build output.** The drift gate is not
  bureaucracy; it is the difference between "config that exists" and
  "config that regenerates." Templates first, always.
- **Don't chase the coverage number.** Write tests for behavior (you
  did, since Chapter 2) and the number follows; a package that arrives
  at 78% test-first and a package that arrives at 41% test-backfilled
  are not the same artifact, and reviewers can tell.
- **A gate failure outside your package is still your failure.** The
  stream matrix lives in `testsuite/conformance` and caught a bug in
  `cognitionzoo/critic`. Fix the cause where the cause lives.

## Summary

- The conformance harness executes your contract's declared cases
  through the real recipe path; declared-without-runner is drift and
  fails CI. Your four cases run, with fixtures shaped for their
  assertions.
- The gauntlet's failures were curriculum: the stream matrix caught a
  real D-2 gap in the agent (now rendering the compiled slice into the
  critique's system slot); the surface vectors and the config drift
  gate forced the contribution's breadth to be deliberate (templates
  first, regenerate, diff shows exactly two files).
- Coverage enforces a 40% global floor with a per-package ratchet
  (`overrides.yaml`, floors never fall); 70% is the stated aspiration
  while the project is in beta. The critic package: 78.28%, test-first
  since Chapter 2. The runner main: an honest `floor: 0` entry with a
  reason.
- All gates green with zero exemptions — and the exemption-table
  discipline means the PR diff touches no gate's forgiveness, only its
  ratchets.

## Check yourself

1. Quote the harness's phrase for a declared conformance case without a
   runner, and explain why the failure is called *drift*.
2. The stream matrix failed your contribution before your fixture even
   ran. What integration did the sentinel prove missing, where was the
   fix, and why couldn't your unit tests have caught it?
3. Your teammate adds a prompt file straight to `relurpify_cfg/prompts/`
   and asks why CI is red. Which ADR governs, and what are the two
   commands that make the PR honest?
4. The coverage output shows `floor 40` for your package and the
   CONTRIBUTING guide says 70%. Which number is enforced, which is
   aspirational, and what does the `overrides.yaml` ratchet add that
   neither number provides?
5. Why does this chapter's PR add zero exemptions, and what would have
   been wrong with adding one to make, say, the drift gate pass?

*Answers.* 1. "Contract declared, runtime drift" — the contract is a
promise and the runner is its proof; a promise without proof has drifted
from the runtime, so CI fails instead of production. 2. The compiled
context slice was never rendered into the critique prompt (the D-2
seam, `paradigm.StreamedSection`); the fix is in the agent's
`runCritique`; unit tests never wire a compiler, so no in-package test
could see a seam it doesn't exercise. 3. ADR-002 (configuration is
generated, not hand-edited): add the prompt under
`userconfig/templates/embedfs/prompts/agents/`, then
`make generate-config` and `make check-config-tree-drift`. 4. 40% is
enforced (the Makefile's default floor), 70% is the beta-era aspiration
(D9); the ratchet records each package's best and never lets a floor
fall, turning coverage into monotonic history rather than a threshold.
5. Because every exemption is load-bearing and debt-checked — the
honesty gate and the gates' own tests police them; an exemption to pass
the drift gate would have shipped config that cannot regenerate, which
is precisely what the gate exists to prevent.

## Try it yourself

1. **Watch the ratchet refuse.** In a scratch clone, lower the critic
   entry in `overrides.yaml` from its recorded floor to 0 and run
   `make test-coverage`… then read the error, restore, and run
   `make coverage-baseline` after adding one more test to the package.
   You will see the floor try to move in both directions and be allowed
   in only one.
2. **Declare a fifth case.** Add `critic/revise_requires_critique` to
   the contract (a D1 case: a revise block with no critique fails at
   load) and follow the full chain: runner, fixture, matrix annotation,
   regeneration. You have just walked this chapter end to end on a case
   the shipped paradigms don't have.

## Where next

One chapter left. Chapter 13 (the plan's finale) shapes everything you
have built into a PR: the clean-checkout gate run from nothing, DCO and
commit style, the PR checklist (goldens deliberate, envelope keys
registered, docs updated, gaps named — including this book's carried
`[NEEDS CONFIRMATION]`s), and the review rubric a maintainer reads
first. (Chapter 12, the live-model chapter, is deliberately skipped in
this draft's sequence and lands with its own captures — the plan's
order holds; only the drafting order bent.)

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch11` runs the full gauntlet from nothing, and
`transcripts/ch11-gates.md` holds every failure and fix in this chapter,
verbatim.
