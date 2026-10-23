# Chapter 9 — Address It From the DSL

Everything exists; nothing is addressable. `paradigm.Registry` carries the
critic's contract, the agent runs under governance, the prompt file
validates — and `agent fixer uses critic` still fails at load with
`unsupported agent paradigm` in any runtime that has not linked the
package. This chapter closes that gap: four small edits that teach the
framework to believe, the goldens that pin the grammar, the linter that
replays the loader, and the payoff — a recipe, loaded and run against
stringkit, with the telemetry receipts to prove which route the runtime
took. Part III ends here: `critic` is a DSL-addressable paradigm.

## Where you are

Thirty-two tests green; the paradigm complete. This chapter is the one
sanctioned exception to the book's self-containment rule (D13) — the
registration wiring cannot live in `cognitionzoo/critic/`, because it is
precisely the code outside the package that must know the package exists.
It ships through the overlay at the files' true repository paths, applied
to a scratch clone; the main repository is never modified.

## The composition site, pinned by reading

Chapter 1 carried a `[NEEDS CONFIRMATION]`: the constructor's callers,
pinned by reading rather than grep. Here is what the reading found, and
it is worth understanding as architecture, not just location.

Euclo's runtime never switches on paradigms at the top. `instantiateAgent`
(`app/relurpish/runtime/runtime.go`) builds exactly one agent — Euclo
itself, from `paradigmDeps()`: the fifteen-field `Deps` you met in
Chapter 8, wired field by field (`Model: r.Model`,
`PermissionChecker: r.permissionChecker()`,
`Registry: r.Tools`, …). Paradigms are reached one level down, when a
*recipe step* executes. The lowering lives in
`named/euclo/thoughtrecipes/step_core.go`, in `buildAgent` — a switch
with one case per shipped paradigm and a default that returns
`ErrContractViolation` ("reachable only when a caller constructs an
ExecutionStep bypassing both [the loader and registration validation]").
Each case does the same three things: build the delegate, lower the
step's typed directive payloads into typed options, construct the agent.

So the wiring is four edits, each following the shipped pattern:

1. **`lexer.go`** — `"critique": {}` joins the reserved keyword set. The
   DSL's directive names are lexer keywords, one per paradigm directive;
   `critique` slots between `review` and `revise`, its declaration-order
   neighbors.
2. **`parser.go`** — `critique` joins the directive-item case list. The
   grammar itself is generic: `parseDirectiveItem` reads any directive
   name, an optional `when` predicate, and arguments, and the contract
   check decides later whether the bound paradigm declares it.
3. **`paradigm_options.go`** — `criticOptions(step, deps)` mirrors
   `reflectionOptions` line for line: at most one `critique` (its text
   becomes `WithCritiqueCriteria`), at most one `revise` (predicate
   required, at least one body step, lowered through
   `newCriticReviseBody` — authored body steps run in-process via
   `ExecuteDelegateCore`). Absent directives yield no options; the
   library behavior survives (FR-9, again).
4. **`step_core.go`** — the case, mirroring reflection's:

```go
case "critic":
	delegate := reactagent.New(deps, c.streamOptions()...)
	criticOpts, err := criticOptions(c.step, deps)
	if err != nil {
		return nil, &paradigm.ErrContractViolation{Step: c.step.ID, Paradigm: c.step.Paradigm, Cause: err}
	}
	return criticagent.New(deps, delegate, criticOpts...), nil
```

Notice what the runtime hands the critic without being asked: the full
`Deps`, including the permission checker whose absence the agent refuses
to run without. Nobody wires governance for the critic; the runtime wires
it for everyone, and the paradigm enforces it.

## The goldens: byte-identical, then one deliberate diff

The DSL goldens (`named/euclo/thoughtrecipes/testdata/golden/`) are
plan/graph/trace triples — one per fixture recipe — and the policy is
stated in the Makefile: *each slice after Slice 1 must keep these goldens
byte-identical; `UPDATE_GOLDEN=1` re-baselines.* A golden that changes
without a deliberate decision is a behavior change nobody approved.

With the wiring applied, every existing triple passes byte-identical —
the edits are purely additive, which is what "mirroring the shipped
pattern" buys. Then the new fixture (`11_critic.erpe`: two agents, a
critique directive, a bounded revise delegating to the react agent) gets
its triple, deliberately:

```text
$ UPDATE_GOLDEN=1 go test ./named/euclo/thoughtrecipes/ -run TestGolden
$ go test ./named/euclo/thoughtrecipes/ -run TestGolden -v   # all PASS
```

And the new trace's content is the chapter's best surprise. The golden
harness runs with minimal deps — no model, no permission checker — and
records what actually happens with the honesty the harness is known for
(the shipped traces already record `react agent missing language model`
for the same reason):

```json
{
  "graph_valid": true,
  "start_node": "run.10.5.reviewer.0.execute",
  "exec_success": false,
  "exec_error": "critic agent requires a permission checker (fail closed)"
}
```

Read that as the integration proof it is: the recipe lowered, the graph
built and validated, the step dispatched to the critic agent through the
new case — and the governance bar, which Chapter 8 stated as two lines in
a paradigm, reached all the way through the DSL and refused in the
runtime's own failure protocol. The bar is real end to end. A golden that
recorded success here would mean somebody bent the harness; this one
records the truth.

## Lint parity, both directions

`relurplint` validates recipes through the same contract registry the
loader uses (`check_recipes.go` validates every recipe via
`thoughtrecipe.ValidateAgainstContracts`, "so a retired directive or an
unknown paradigm surfaces at lint time with the same errors the loader
would surface at boot"). Because the linter links `thoughtrecipes`, the
critic's contract rides along — no separate linter registration exists,
which is the parity guarantee *by construction*.

Prove it in both directions. Positive: a workspace with the critic
recipe lints clean.

```text
$ relurplint --check recipes --workspace ~/code/stringkit
relurplint: all checks passed (0 diagnostics)
```

Negative: a `revise` block with no `critique` before it — the D1 mixing
rule — is rejected by the *linter*, with the loader's own error and a
position:

```text
relurpify_cfg/euclo/critic_broken.erpe: [error recipes.contract]
  …:11:3: directive "revise" requires directive "critique"
  in the critic paradigm contract
```

The same text the loader would emit at boot. Lint parity is not a goal
here; it is a consequence of one registry.

## The payoff: a pinned offline run

The workspace recipe (`critic_review.erpe`) is book 1's DSL fluency spent
on your own paradigm: investigation family, keyword triggers, a read-only
tool scope, the critique criterion, and a revise body that delegates back
to the react drafter with the critique's issues interpolated into its
goal. Materialize the workspace, boot offline, and instruct:

> explain and critique the slugify function in stringkit.py

The deterministic route does its job — investigation family (from
`explain`), and the telemetry route record names the winner:

```text
family_id:investigation  recipe_id:euclo.thoughtrecipe.critic_review
route_kind:capability    name:euclo.thoughtrecipe.critic_review
```

One approval banner — `kind=execution | risk=medium` — answered `[s]` for
the session, because governed mode pauses the run for a human regardless
of how deterministic the route was. Then the graph executes, and the run
completes with the loop-close honesty guard firing — the same flag
Chapter 1 met on this same workspace. The reason is the honest one: the
offline backend's scripted responses are not valid verdict JSON, so the
critic's strict decode fails, the D6 retry fires exactly once, and the
failure protocol records the outcome. The receipts say all of it:
`node euclo.execute_thoughtrecipe`, the route record naming
`critic_review`, `critic` as the executing paradigm.

The honest reading, stated plainly: **addressability is proven end to
end** — load, lint, route, approve, execute, record. Critique *quality*
under the offline backend is not, and cannot be: a scripted model that
cannot emit verdict JSON cannot pass strict decoding, and the receipts
say so rather than hiding it. The deterministic behavioral proof remains
`critic-runner` (Chapter 6); real-model behavior is Chapter 12's subject.
`[NEEDS CONFIRMATION: recipe dispatch selection rules — an instruction
with no family keyword routes to the default recipe regardless of recipe
keywords; the dispatcher's exact keyword-vs-family mechanics get pinned
during Chapter 10 research.]`

## Gotchas

- **The wiring is four files, not one.** A new directive name touches the
  lexer, the parser's case list, the options lowering, and the factory
  switch. Miss the lexer and `critique` is a bare identifier that parses
  but never checks; miss the switch and the loader validates a paradigm
  the runtime cannot build — surfacing only as `ErrContractViolation` at
  execution, the defense-in-depth branch.
- **Re-baseline deliberately, and only after the additive proof.** Run
  the goldens *without* `UPDATE_GOLDEN` first: every existing triple must
  pass byte-identical before you create a new one. A re-baseline that
  changes an existing golden is a behavior change, not a baseline.
- **The golden harness's failures are honest failures.** Minimal deps
  mean shipped traces record missing models and the critic records its
  missing checker. Do not "fix" the harness to make a golden green; the
  recorded refusal is the proof.
- **Routing is deterministic and keyword-shaped.** A recipe is selected
  within a classified family; an instruction with no family keyword goes
  to the default route no matter what your recipe's keywords say. Write
  instructions (and keywords) the dispatcher can actually match.
- **Governed mode pauses for approval even for read-only routes.** The
  banner is the work permit working; `[s]` (session) is the usual answer
  during active work.

## Summary

- The composition site is `buildAgent`'s switch: one case per paradigm,
  delegate → options → construct, with `ErrContractViolation` guarding
  the unreachable default. The critic's case mirrors reflection's, and
  the runtime's own `Dps` wiring hands it the governance bar.
- Four files change, all following the shipped pattern; the edits are
  purely additive, proven by every existing golden passing
  byte-identical before the new triple was baselined with
  `UPDATE_GOLDEN=1`.
- The new trace records the fail-closed refusal reaching the DSL — the
  governance bar as an integration proof — while the offline run proves
  addressability end to end: load, lint (both directions, one registry),
  route (the telemetry record names `critic_review`), approve, execute,
  record.
- **End of Part III:** `critic` is a DSL-addressable paradigm. What
  remains is proof at the framework's own test layers — Part IV.

## Check yourself

1. Why can the registration wiring not live in `cognitionzoo/critic/`?
   Say it in terms of the dependency direction, not the book's rule.
2. Name the four files the wiring touches and the one job of each.
3. Why does the critic's golden trace record an exec error, and why is
   that the *stronger* proof?
4. Your teammate deletes the `critique` keyword from the lexer but leaves
   the rest. What breaks, where — at parse time, load time, or execution
   time — and with which error?
5. The offline run's critique failed strict decoding. Which three
   mechanisms handled it, in order, and what did each contribute?

*Answers.* 1. `cognitionzoo` paradigms are imported by
`named/euclo/thoughtrecipes` (named imports cognitionzoo, never the
reverse); the switch that constructs agents lives above the paradigm, so
the knowledge of a new paradigm must live above it too. 2. `lexer.go`
(reserve the directive name), `parser.go` (route it to the generic
directive-item grammar), `paradigm_options.go` (lower typed payloads into
typed options), `step_core.go` (construct the agent per step, with the
contract-violation guard behind it). 3. The harness's minimal deps have
no permission checker, so the agent's fail-closed bar fired through the
full DSL path; it is stronger because it proves the bar survives load,
lowering, dispatch, and execution — a success would have proven less. 4.
`critique` lexes as a bare identifier; the generic directive-item
fallback may still parse it, but the keyword-based paths (and any grammar
position where an identifier is invalid) become unpredictable — the
failure class is parse/load, and the honest fix is restoring the keyword,
not relying on the fallback. 5. Strict decode
(`ErrInvalidCritique`), the D6 model-invalid-output retry (exactly one),
and the run-node failure protocol (recorded outcome, run continues) —
malformed model output became a recorded, bounded, honest failure instead
of a garbage verdict.

## Try it yourself

1. **The order rule, from the linter.** In a scratch workspace, write a
   critic recipe whose `revise` appears *before* its `critique` and run
   `relurplint --check recipes`. Match the error against
   `ErrDirectiveOrder`'s format from Chapter 7 — the linter quotes the
   directive and the predecessor it must precede.
2. **A second fixture.** Add `12_critic_goal_only.erpe` — a goal-only
   critic run with no directives — and baseline its triple. Read the
   plan JSON: the directives list should be empty and the paradigm still
   `critic`. That is FR-9, pinned in a golden.

## Where next

The paradigm is addressable; now it must survive the framework's own
proof layers. Chapter 10 applies the three test layers to your agent:
unit (done, but now formalized), framework integration through the
`testsuite/` builders, and agenttest — the OSB assertion model with
Outcome and Security fail cases and advisory Benchmarks. You will write
`euclo.critic.testsuite.yaml`, add a subject adapter, and record a tape.

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch09` now includes `make test-euclo-golden` in the
scratch clone, and `transcripts/ch09-registration.md` holds the wiring
summary, the golden re-baseline, the lint captures, and the offline run's
receipts.
