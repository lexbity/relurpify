# Chapter 7 — Contracts Are the DSL

Your pipeline runs from a shell. Now comes the question that separates a
program from a paradigm: can a *recipe* say so? By the end of Part III,
`agent fixer uses critic` will load, lint, and run — and the foundation is
laid here, in a struct. In this framework the recipe grammar for your
paradigm is not a parser you write; it is data you declare. The loader
reads it, the linter replays it, and the contract's own unit test polices
it. The DSL is a registry lookup away.

We earn that by reading first: one shipped paradigm end to end —
reflection, the review loop — agent side included. Then the contract
system. Then you design the critic's grammar and watch the validator
refuse six ways to get it wrong.

## Where you are

Thirty tests will be green at the end of this chapter (twenty-three plus
seven new ones). This chapter adds the package's second production file —
`contract.go` — and touches nothing else in the repository. The consumer
gate stays satisfied by Chapter 6's runner.

## Reading reflection end to end

You saw reflection's thirty-line contract in Chapter 1. The agent side is
where the contract's words become behavior, and three files tell the
whole story.

**`new.go` — the option surface.** The constructor is
`New(deps *paradigm.Deps, delegate graph.WorkflowExecutor, opts …Option)`
— and when the delegate is nil, it defaults to `reactpkg.New(deps)`. That
one line *is* the `Composes: []string{"react"}` declaration from the
contract: the review loop runs over a react delegate. Two options carry
the authored directives:

- `WithReviewCriteria(text)` installs the authored review criterion that
  drives the directive-mode review phase; unset, the library review
  prompt governs.
- `WithReviseCycle(body ReviseBodyFunc, predicate)` installs the authored
  revise cycle: after each review the predicate is evaluated against the
  envelope — scratch-aware — and when it reads true the body runs
  in-process and the loop re-reviews.

**`reflection.go` — the loop.** The review phase writes its verdict to
the ephemeral scratch overlay — `scratch.review` and
`scratch.review_issues` in working memory, task-class — which is what the
canonical `revise when scratch.review contains issues:` predicate routes
on. The loop is `(predicate → body → review)ⁿ` with `n ≤
MaxRevisionCycles`, a contract constant set to 2, and when the cap bites
the agent emits a `reflection.revision_capped` event. Nothing about the
bound is implicit: it is named in the contract's guarantees ("bounded
revision iterations"), enforced in the loop, and observable in telemetry.

**The grammar rules, as behavior.** `review` is deliberately *not*
`Required` — a goal-only reflection recipe (no directives at all) keeps
the library review loop. That is guarantee-shaped restraint: the DSL
stays clean for the common case. But `revise` *Requires* `review` — an
authored revise block with no review before it is a load error, not a
silent mode switch. The contract's comment names this the **D1 mixing
rule**: authored structure without its governing objective does not load.

## The contract system

`cognitionzoo/paradigm/contract.go` is the vocabulary owner, and its
types are small. `Contract` — paradigm, summary, shape, directives,
composes, guarantees, conformance, order. `DirectiveSpec` — one
directive's grammar: `Form` (`line` like `until 3`, or `block` like
`step:` with a body), `Text` (argument arity: `none`/`one`/`many`),
`Predicate` (may it carry a `when` clause?), `ArgsInteger` (must the
argument be a positive integer — the until-style caps?), `Body` (the
closed set of nested items a block may hold: `run`, `delegate`, `do`,
`capture`, `directive`, plus the clauses `from`, `goal`, `stream`, `may`),
`Repeatable`, `Required`, and `Requires` (the D1 obligations).
`OrderRule` — the canonical declaration order, checked per run block,
never against an aggregate.

Every paradigm package registers its contract from `init()` into the
process-wide registry, and registration **validates before it trusts**.
The intrinsic checks, each of which the lab triggers:

- unknown execution shape — the set is closed;
- duplicate directive names;
- a `Requires` naming an undeclared directive;
- an `OrderRule` referencing an undeclared directive;
- a conformance case pinning an undeclared directive — and the reverse:
  a declared directive with *no* conformance case;
- duplicate registration of the same paradigm name.

`Composes` resolution is deliberately *not* checked at registration time
— init order would make it flaky — so each paradigm's
`TestContractCompleteness` checks it after all inits have run. The
framework's own tests are part of the contract system; when you mirror
them in this chapter, you are joining a mechanism, not inventing one.

When a recipe *loads*, the loader resolves `uses X` against this registry
and validates every directive against the bound contract, with typed,
position-carrying errors — `ErrUnknownParadigm` (which lists the valid
paradigm names, keeping its message "unsupported agent paradigm" so old
diagnostics stay greppable), `ErrDirectiveNotInContract`,
`ErrDirectiveShape`, `ErrDirectiveOrder`, `ErrDirectiveRequires`. And for
a paradigm outside the registry reached past the loader, `ErrContractViolation`
is the runtime guard — *"reachable only when a caller constructs an
ExecutionPlan bypassing the loader; an .erpe-reachable authoring path
rejects the same mistake at load time."* Load time is where mistakes are
meant to die.

### The shape decision

The closed set of execution shapes carries this comment, and it frames
the whole conversation:

> The value is display/provenance vocabulary only; the shape a paradigm
> executes is a conformance-test obligation, not a runtime switch here.

So the honest question is not "which shape makes the runtime do X" — no
shape does anything at runtime. The question is: *what does the provenance
claim to a reader?* The critic is a draft → critique → conditional
revise loop. `ShapeReviewLoop` names that truthfully, and reflection —
the nearest neighbor — already claims it. Adding a shape to the closed
set would create a new display category without new runtime semantics,
which is exactly the kind of surface growth the charter culture resists.
**Decision: the critic reuses `ShapeReviewLoop`**, and the argument rides
along in the contract's own comment. If a maintainer ever wants a distinct
category for separated-critic loops, that is a PR-shaped conversation
with real vocabulary stakes — this book's Chapter 13 frames it exactly
that way, and the decision log (D10) records where it landed.

## Designing the critic's grammar

The mirror of reflection's two directives, in the critic's own voice:

```go
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "critic",
		Summary:  "Independent critique loop over a react delegate: separate critique phase, bounded conditional revision.",
		Shape:    paradigm.ShapeReviewLoop,
		Composes: []string{"react"},
		Directives: []paradigm.DirectiveSpec{
			{Name: "critique", Form: paradigm.FormLine, Text: paradigm.ArgOne},
			{Name: "revise", Form: paradigm.FormBlock, Text: paradigm.ArgNone, Predicate: true,
				Body:     []paradigm.BodyItem{paradigm.BodyItemRun, paradigm.BodyItemDelegate},
				Requires: []string{"critique"}},
		},
		Order: &paradigm.OrderRule{Sequence: []string{"critique", "revise"}},
		Guarantees: []string{
			"bounded revision iterations",
			"critique is a separate phase with its own model and criteria — it never edits the draft",
			"authored critique verdicts live in the ephemeral scratch namespace",
			"composes the react delegate through the scoped-registry path",
		},
		Conformance: []paradigm.ConformanceCase{ /* four cases, mirrored from reflection */ },
	}
}
```

Every choice has a reflection-shaped reason:

- **`critique <text>`** (line, one argument) is the authored critique
  criterion — the independent phase's instructions, playing the role
  `review` plays for reflection. Not `Required`, for the same FR-9
  reason: a goal-only critic recipe keeps the library critique loop.
- **`revise when …:`** (block, predicate, body of `run`/`delegate`)
  mirrors `revise` exactly, including `Requires: critique` — the D1 rule
  in the critic's own grammar.
- **The verdict namespace is `scratch.critic.*`** — the same ephemeral
  overlay pattern, namespaced so a critic and a reflection can coexist in
  one run without arguing over `scratch.review`.
- **Four conformance cases**, mirrored one-for-one:
  `critic/critique_writes_scratch`, `critic/revise_fires_on_predicate`,
  `critic/revision_cap_bounded` (at most `MaxRevisionCycles` bodies and a
  `critic.revision_capped` event), and `critic/critique_pass_ends_loop`.
  Every declared directive is pinned; every pin names a declared
  directive.

And the registration is the shipped mechanism, one function:

```go
func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register critic paradigm contract: " + err.Error())
	}
}
```

## Hands-on

The overlay adds two files: `contract.go` (production — the package's
second) and `contract_test.go`.

```bash
./verify.sh build ch07
```

```text
== tier 1 (build): ch07 ==
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.061s
[PASS] build: ch07 compiles clean and its tests pass
```

Thirty tests now. The three new ones:

- **`TestCriticContractRegistered`** — the process-wide registry holds
  the contract: shape, composes, directive names in declaration order,
  the revise spec's predicate and `Requires`, the order rule, and no
  required directives.
- **`TestContractCompleteness`** — the mirror of the shipped paradigms'
  own test, including the blank import of react so Composes resolves
  against a fully-populated registry. The comment above that import is
  the mechanism speaking: *the real runtime always links every paradigm;
  composes resolution is only meaningful against the fully-populated
  registry.*
- **`TestRegistrationValidatesIntrinsics`** — a scoped registry
  (`paradigm.NewContractRegistry()`) driven through seven refusals, from
  `unknown execution shape "Whirlpool"` to
  `duplicate paradigm contract registration`. The scoped registry keeps
  the process-wide collection untouched and keeps a would-be panic a
  test error.

And one quiet milestone, visible in the transcript: the runner binary
now links the critic package, so its process registry carries **nine**
paradigm contracts. What no binary can do yet is *load a recipe* — the
runtime composition, the linter parity, and the goldens are Chapter 9's
work, and the runtime-side wiring (`switchAgentDeps`) is the
`[NEEDS CONFIRMATION]` item that chapter's research pins down.

```text
$ ./verify.sh gates ch07
[PASS] gates: ch07 holds the framework's law
```

## Gotchas

- **Register validates; it does not resolve Composes.** A typo in
  `Composes` passes registration and fails the completeness test. Write
  the completeness test before you think you need it.
- **Use a scoped registry in tests.** `paradigm.Register` writes the
  process-wide collection; a duplicate there is a panic in an `init` —
  the least debuggable crash in Go. `NewContractRegistry()` keeps
  experiments out of the shared state.
- **Directive names are closed per paradigm, open across paradigms.**
  `critique` is not declared by any shipped paradigm, so recipes can't
  confuse it — but if you name a directive `review`, you had better be
  ready to explain the collision with reflection's vocabulary in review.
- **`ArgsInteger` is for until-style caps only.** The critique criterion
  is free text (`ArgOne`); do not reach for integer arguments where a
  quoted string is the honest shape.
- **The shape is provenance.** Do not add a shape to "make something
  work" — nothing works differently. Add one only when the vocabulary
  itself is missing a category, and expect that conversation to happen in
  review.

## Summary

- Reflection read end to end: the contract's words are the agent's
  behavior — options lower authored directives, the loop writes verdicts
  to the scratch namespace, the bound is a contract constant, the D1
  mixing rule is a load error, and goal-only recipes survive because
  nothing is `Required`.
- The contract system is data plus validation: `Contract`,
  `DirectiveSpec`, `OrderRule`, and executable conformance cases, checked
  intrinsically at registration and cross-checked by each paradigm's own
  completeness test.
- The critic's grammar — `critique` (line, not required) and
  `revise when:` (block, predicate, requires critique), ordered, with
  four mirrored conformance cases — is now registered in the process-wide
  registry by the same `init` mechanism the eight shipped paradigms use.
- The shape decision lands on `ShapeReviewLoop` by convention, with the
  argument in the contract's comment and the PR conversation deferred to
  Chapter 13 — because `ExecutionShape` is display/provenance vocabulary,
  and nothing at runtime switches on it.

## Check yourself

1. Reflection's review phase writes `scratch.review`. What does the
   critic's write, why the different namespace, and what routes on it?
2. `revise` is not `Required`, yet a recipe with a `revise` block and no
   `critique` fails to load. Which mechanism rejects it, and which
   contract rule would have caught a misspelled `Requires` target at
   *registration* time?
3. Why does the completeness test blank-import react?
4. Your teammate proposes `ShapeIndependentCritique` for the closed set.
   Using the source comment as your authority, write the two-sentence
   review response.
5. What does the runner binary's process registry now contain that it did
   not last chapter — and what can still not happen in that process?

*Answers.* 1. `scratch.critic.verdict` / `scratch.critic.issues` —
namespaced so a critic and a reflection coexist in one run; the authored
`revise when scratch.critic.verdict contains issues:` predicate routes on
it. 2. The loader's `ErrDirectiveRequires` (the D1 mixing rule, load
time); `validateContractIntrinsic`'s "requires undeclared directive"
check. 3. Composes resolution runs after all `init`s — the blank import
guarantees react's contract is registered before the test looks it up,
mirroring the runtime, which links every paradigm. 4. "The closed set's
own comment says shapes are display/provenance vocabulary only — what
does `ShapeIndependentCritique` claim that `ShapeReviewLoop` doesn't, and
what conformance obligation does it add?" And: "If the category is real,
propose it in the PR with the conformance cases that would fail without
it; otherwise reuse `ShapeReviewLoop`." 5. The `critic` contract,
registered by the package's `init` alongside the eight shipped ones; what
cannot happen is loading a recipe against it — no runtime composition,
no lint parity, no goldens until Chapter 9.

## Try it yourself

1. **Break the mirror on purpose.** Change the critic's `revise` spec to
   drop `Requires: critique` and run `TestCriticContractRegistered`.
   Read which assertion catches it, then restore. You are learning which
   test guards which declaration — the map you will need in review.
2. **A third directive.** Sketch (on paper or in a scoped registry test)
   a `focus <text>` directive for the critic: line form, one argument,
   repeatable. What breaks in `validateContractIntrinsic`? Nothing — but
   what must you add to `Conformance`, and what does the OrderRule say
   about where `focus` may appear? Grammar design is obligation design.

## Where next

The contract promises an agent that doesn't exist yet. Chapter 8 writes
it: `cognitionzoo/critic`'s agent against `paradigm.Deps` — field by
field through the curated subset (D12), the react delegate, the three
phases as prompts following the `agent.<agent-name>.default` convention,
the bounded revise loop, and unit tests driven by the Chapter 2 fake
model. The governance bar comes with it: no permission checker, no run.

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch07` re-runs everything this chapter proved, and
`transcripts/ch07-critic-contract.md` holds the captured run.
