# Chapter 8 — The Critic Agent

Last chapter the contract promised an agent. This chapter you write it —
and you write it the way the framework's authors write paradigms: against
the runtime surface, with the governance bar stated in the type system,
the phases structured as prompts, and every claim pinned by a test driven
by the fake model you built in Chapter 2. When this chapter is done,
`cognitionzoo/critic` contains a complete paradigm: contract, agent, and
prompt file. What it will not yet have is a runtime that hands it work —
that is the Chapter 9 wiring, and knowing exactly what is missing is part
of this chapter's lesson.

## Where you are

Twenty-six tests green in the package; the contract registered; the
ensemble runnable. This chapter adds the package's third production file
(`critic.go`), its first workspace artifact (a prompt file under
`relurpify_cfg/`), and six tests. Nothing else in the repository changes.

## `paradigm.Deps`, the curated subset, field by field

`Deps` (`cognitionzoo/paradigm/deps.go`) carries fifteen fields; the
composition root adapts workspace state into this one value, and every
paradigm receives it whole. Here is the curated subset this book teaches
field by field (D12) — the fields a paradigm author actually reasons
about — with the full fifteen carded in Appendix C:

| Field | Type | What it hands you |
|---|---|---|
| `Model` | `model.LanguageModel` | The workspace's model, behind the Chapter 2 contract. Your critique phase is a `Chat` on this. |
| `Registry` | `*registry.CapabilityRegistry` | The governed capability registry — the only door to tools. The Chapter 4 governed invoker forwards here. |
| `PermissionChecker` | `permissions.CapabilityChecker` | The capability gate shared by governed paradigms. One method: `CheckCapability(ctx, agentID, capability) error`. **The critic refuses to run without one.** |
| `CommandRunner`, `CommandPolicy` | `sandbox.CommandRunner`, `sandbox.CommandPolicy` | Sandboxed command execution under its policy — how a paradigm runs anything, never bare. |
| `WorkingMemory` | `*memory.WorkingMemoryStore` | Per-task working memory beyond the envelope's own store. |
| `IndexManager`, `SearchEngine` | `*ast.IndexManager`, `*search.SearchEngine` | The AST index and search — the retrieval seams `RetrievalNode` and your own delegate's tools sit on. |
| `StreamTrigger` | `*contextstream.Trigger` | The compiler trigger `StreamTriggerNode` resolves from context. |
| `Grounder` | `*knowledge.GroundingService` | The capture-as-bridge write path the epoch barrier flushes. |
| `PromptRegistry` | `prompt.Registry` | The workspace prompt corpus — the registry your prompt file loads into at boot. |
| `Config` | `*execution.Config` | Run configuration: model ID, iteration caps, the telemetry sink. |

The four fields this table omits — `EpochDrain`, `AgentLifecycle`,
`Telemetry`, and the rest of `Config`'s surface — are runtime-plumbing
concerns you *receive* rather than reason about at this layer; Appendix C
cards them. Curated is not censored: it is the difference between
learning a surface and memorizing one.

Read one field comment again, because it is the governance bar in the
authors' own words (`deps.go:29-31`):

> PermissionChecker is the capability gate shared by governed paradigms
> (ReWOO refuses to execute without one).

The critic joins that sentence. Its `Execute` begins:

```go
if a.permissionChecker == nil {
	return nil, ErrNoPermissionChecker
}
```

Fail closed — an error, never a silent downgrade. The critique phase
itself is model-only (data, not action, per D14), but the critic *drives
a delegate*, and the delegate is the governed action surface. A paradigm
that cannot answer "who says this agent may act" does not act.

## The agent, mirroring reflection

`CriticAgent` (`cognitionzoo/critic/critic.go`) follows reflection's
shape closely enough that the two files read as siblings — deliberately,
because Chapter 7 declared reflection the template:

- **Constructor**: `New(deps *paradigm.Deps, delegate
  graph.WorkflowExecutor, opts …Option)` — and when the delegate is nil,
  it defaults to `reactpkg.New(deps)`. The `Composes: ["react"]`
  declaration from the contract, as a line of code.
- **Options lower authored directives**: `WithDraftCriteria`,
  `WithCritiqueCriteria` (the `critique` directive), `WithReviseCycle`
  (the `revise when:` block). The zero value preserves the library
  critique report — FR-9's restraint, in the critic's own grammar.
- **Execute** branches on `directiveMode()`. The library mode builds a
  three-node graph — delegate run → critique report → done — where the
  critique node carries a typed contract (`SideEffectNone`,
  `replay-safe`). The directive mode runs the loop:

  ```text
  draft (delegate, state-isolated: clone → execute → merge)
    → critique (one Chat, verdict JSON → scratch.critic.*)
    → (predicate → revise body → critique)ⁿ, n ≤ MaxRevisionCycles
  ```

- **The separation is structural.** The draft phase runs through the
  delegate with state isolation (`Clone` → `Execute` → `Merge`); the
  critique phase's model call carries the task, the authored criterion,
  and a *compact rendering of the result* — never the drafter's
  reasoning. `MaxRevisionCycles` is a contract constant (2), and hitting
  the cap ends the loop *normally* with the last verdict standing — a
  cap is a bound, not a malfunction — after emitting
  `critic.revision_capped`.
- **Telemetry follows the shipped vocabulary pattern**: `critic.iteration`,
  `critic.reviewed`, `critic.revision_capped`, `critic.completed`, as
  package-local constants. Upstreaming named constants into the telemetry
  package's vocabulary file is part of the Chapter 13 PR checklist — the
  event *strings* follow the house convention today, and the vocabulary
  owner will adopt the names when the contribution lands.
- **Verdict vocabulary is `{pass, fail, issues}`**, strictly decoded from
  JSON (with react's `ExtractJSON` used to find it in the response) —
  anything else is `ErrInvalidCritique`. Strict decode is decision repair
  applied at the paradigm layer: malformed output fails loudly and the
  profile's repair strategy gets a chance, instead of a garbage verdict
  silently routing the loop.

## The prompt file

Book 1 taught you that prompts are workspace files with frontmatter. The
critic ships one, at the convention's path:

```text
relurpify_cfg/prompts/agents/agent.critic.default.prompt
```

```text
---
schema framework.prompt/v2
id agent.critic.default
tag "system"
tag "agent"
tag "critic"
---

You are an independent critique agent. You did not write the work under
review, and you never edit it: you judge it.
…
```

The naming convention is `agent.<agent-name>.default` — react, chainer,
and pipeline ship siblings (and ReWOO breaks the pattern with
`rewoo.plan.v1`, a reminder that conventions have documented exceptions).
The file's body says the two things that make a critic a critic: *you did
not write this work*, and *you never edit it*. The agent's library prompt
carries the same stance in code; the file is the operator-facing override
point, loaded into the `PromptRegistry` at boot. The lab proves the file
is honest: it loads through the prompt registry from a `fstest.MapFS` —
the loader's own test pattern — and validates clean.

## Hands-on

The overlay adds `critic.go`, the prompt file, and
`critic_agent_test.go` — six tests, driven by the Chapter 2 fake model
standing in for the critique model and a recording delegate standing in
for react (a scripted *delegate* is a test double for another agent, not
a bypass: react's own governed paths are react's tests to pin).

```bash
./verify.sh build ch08
```

```text
== tier 1 (build): ch08 ==
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.061s
[PASS] build: ch08 compiles clean and its tests pass
```

The six, in the order they matter:

1. **`TestCriticFailsClosedWithoutPermissionChecker`** — deps without the
   checker: `ErrNoPermissionChecker`, asserted with `errors.Is`. The
   chapter's first test is the governance bar.
2. **`TestCritiqueWritesScratch`** — delegate ran once, critique ran once,
   the verdict landed in `scratch.critic.verdict` /
   `scratch.critic.issues`, the result fields carry it, and — via the
   fake model's `LastInput` — the prompt carried the authored criteria.
3. **`TestReviseFiresOnPredicateAndRecritiques`** — body runs exactly
   once, re-critique returns `pass`, two critique calls total.
4. **`TestRevisionCapEndsLoopNormally`** — always-issues script: body
   runs exactly `MaxRevisionCycles` times, `critic.revision_capped` is in
   the recorded telemetry, and the run ends successfully.
5. **`TestLibraryModeReportsWithoutRevision`** — zero value runs the
   graph: draft → critique → done.
6. **`TestCriticPromptFileLoadsAndValidates`** — the workspace artifact
   loads and validates through the registry.

```text
$ go test ./cognitionzoo/critic/... -count=1
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.061s
```

Thirty-two tests. One debugging story from this chapter's build is worth
retelling, because you will hit it: the telemetry helper initially took
`*recordingSink` and stored it in `Config.Telemetry` — a non-nil
*interface* holding a nil pointer, so the nil check passed and the first
`Emit` panicked. The fix is the same shape as reflection's helper: take
the interface. Typed nils are Go's most polite trap; the mirror tests
caught it in seconds.

## Gotchas

- **Fail closed is an error at Execute time, not a panic at construction
  time.** `New` cannot return an error (and reflection's doesn't try);
  the bar lives on the first call that would do work. Mirror it exactly —
  a construction-time panic would take the whole process down in
  production for a misconfiguration a warning could have caught.
- **The scratch namespace is never grounded.** Verdicts are ephemeral
  task-scoped state (`MemoryClassTask`), like reflection's — they route
  the loop, they do not become knowledge.
- **A cap is a bound, not an error.** If your revise loop exits
  non-zero when the cap bites, you have turned a design decision into a
  failure mode.
- **Prompt file frontmatter opens with `---`.** The loader refuses files
  without the opening fence, and the error message will not remind you.
  The lab's test pins the file byte-for-byte, so this cannot regress.
- **The registry override path is boot-time.** `agent.critic.default`
  loads when the workspace composes; unit tests feed it via
  `LoadFS(fstest.MapFS{…})`. There is no mid-run prompt swap.

## Summary

- `paradigm.Deps` is the governed runtime surface; the curated subset is
  ten fields you reason about, with the plumbing carded in Appendix C.
  The permission checker is the line between a paradigm and a liability.
- `CriticAgent` mirrors reflection structurally — constructor, options,
  directive mode versus library mode, state-isolated delegation, scratch
  verdicts, bounded revisions, cap-as-bound — and differs where the
  thesis demands: the critique phase never sees the drafter's reasoning,
  and the fail-closed bar is unconditional.
- Prompts are three-layered: the workspace file (operator-facing,
  convention-named), the library prompt in code, and the authored
  criteria through options. Strict verdict decoding keeps the loop
  honest about malformed model output.
- The paradigm is complete: contract (Ch 7), agent (here), prompt file
  (here) — with `paradigm.Register` already done and the runtime
  composition still to come.

## Check yourself

1. Name the four fields of the curated `Deps` subset that are *not* in
   this chapter's table, and say what kind of concern each is.
2. Why does the critic fail closed at `Execute` rather than panicking in
   `New` — and what would you lose if it panicked?
3. The draft phase wraps the delegate in `Clone` → `Execute` → `Merge`.
   What does the isolation buy the critique phase, and what does the
   merge preserve for the final result?
4. Your critique model returns `verdict: "OK"`. What happens, which error
   surfaces, and what would happen without the strict decode?
5. What can the runner binary's process do now that it could not last
   chapter — and what is still Chapter 9's work?

*Answers.* 1. `EpochDrain` (invalidation drain at the epoch barrier),
`AgentLifecycle` (workflow/run records), `Telemetry` (the emit surface —
the critic reaches telemetry through `Config`), and `Config`'s extended
surface; runtime-plumbing concerns received, not reasoned about. 2.
Because `New` cannot return an error, and a panic would crash the whole
process for a misconfiguration; failing at `Execute` turns the bar into
a routable, testable, telemetry-visible refusal. 3. The critique phase
sees only merged, successful delegate state — never the delegate's
in-flight partials — and the merge preserves the delegate's results in
the parent envelope so the final result and provenance stay intact. 4.
`decodeCritiqueVerdict` returns `ErrInvalidCritique`, the run fails with
it, and the model profile's repair strategy can retry; without the strict
decode, "OK" would silently fail the vocabulary switch downstream or —
worse — be treated as a non-`issues` verdict, ending the loop without a
revision. 5. The process now links the complete paradigm — contract,
agent, prompt file — and `paradigm.Registry` carries it; what cannot
happen yet is the relurpish runtime composing `critic` for a recipe,
because the wiring, lint parity, and goldens are Chapter 9's.

## Try it yourself

1. **A second checker.** Write a `deniedChecker` whose
   `CheckCapability` returns an error, and add a test asserting the
   critic's *delegate* work cannot proceed through a governed path — then
   notice what actually happens: the critic's own `Execute` never checks
   capabilities directly, because it invokes none. Write down where in
   the stack the denial *would* land (the registry's pipeline, Chapter 4)
   and why the checker's presence is still the right bar for the
   paradigm.
2. **Cap semantics, strengthened.** Change `TestRevisionCapEndsLoopNormally`
   to also assert `critic.revise_cycle` in the envelope equals
   `MaxRevisionCycles`. Then re-read the contract guarantee
   ("bounded revision iterations") and confirm every word of it is now
   pinned by an assertion.

## Where next

Everything exists; nothing is addressable. Chapter 9 makes the framework
believe: the runtime composition wiring that hands `critic` its `Deps`
(the `[NEEDS CONFIRMATION]` item — the constructor's callers, pinned by
reading), lint parity so `relurplint` knows the new paradigm, the DSL
goldens re-baselined deliberately, and the payoff — `agent fixer uses
critic` loading and running offline against stringkit, with telemetry as
the proof.

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch08` re-runs everything this chapter proved, and
`transcripts/ch08-critic-agent.md` holds the captured run and the build
notes.
