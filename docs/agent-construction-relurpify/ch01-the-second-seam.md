# Chapter 1 — The Second Seam

Book 1 taught you to hire a line cook with a work permit. This book hands
you the forge. By the last page you will have added a new kind of worker to
the framework itself — a paradigm, declared in the same language the shipped
ones speak, proved at every test layer the framework runs, and shaped into a
contribution you could merge. You will never look at an agent framework the
same way again. That is not a promise about your feelings; it is a promise
about what you will be able to read.

The worker you will build is a **critic**. The thesis it embodies is the
reason this book exists at all, so we start there, because every design
decision in the next twelve chapters falls out of it.

## Where you are

This book assumes you finished *Cooking Agentic Thoughts with Relurpify*.
You have driven relurpish, read a governed turn's receipts, authored Euclo
thoughtrecipes in the Euclo DSL, and you know the two-directory rule —
`relurpify_cfg/` is yours to edit, `.relurpify_state/` is the runtime's to
write. None of that is re-taught here. The DSL is not re-taught either; when
a recipe appears in this book, you are expected to read it fluently.

New in this book, assumed from page one: working Go. Interfaces, `context`,
table-driven tests, and the patience to read other people's Go — because
from Chapter 2 on, that is most of what we do. You do not need generics
gymnastics, Go module internals, prior agent-framework experience, or a
formal CS background. If you can read `cognitionzoo/reflection/` after this
chapter without flinching, you are ready for Chapter 2.

Everything in this chapter was captured on a real machine: Linux 6.18, Go
1.27.3, Ollama 0.40.8 with `gemma4:e4b` served, Docker with `runsc`
installed, Python 3.14.8, pytest 9.1.1. Where your output will differ, the
text says so.

| Requirement | Version / notes |
|---|---|
| OS | Linux or macOS (book verified on Linux) |
| Go | go directive 1.26.0, toolchain go1.27.2 — the repository's own toolchain |
| Git | Any recent version |
| Python | 3.10+ with pytest, for the stringkit demo |
| Model | Ollama serving `gemma4:e4b` (your book-1 setup) |
| Sandbox | Docker, plus gVisor `runsc` on Linux for the default backend |
| Book 1 | Assumed fluency: relurpish, governed turns, the Euclo recipe DSL |

## Why extend the framework at all: the critic thesis

Book 1's agent could fix a bug because you watched it work. But watch what
happens when you ask an agent whether its own work is good. It grades the
work with the same weights it wrote the work with. The blind spot that chose
the wrong variable name is the same blind spot now asked to notice the wrong
variable name. Self-review is not review. A model grading its own output
shares its own blind spots — the errors are correlated with the checker.

Every serious agentic system runs into this, and the standard response is a
second prompt: "now review your answer." That is better than nothing, and it
is still not review, because nothing separates the two roles. The reviewer
is a costume the same actor puts on.

Relurpify already ships a paradigm for the costume: **reflection**, a review
loop over a react delegate with bounded revisions. We will read it in
Chapter 7 as our template, and it is honest about what it is — one agent
loop that pauses to grade itself before revising. The critic is a different
claim: a **genuinely separate phase**, with its own prompt, its own
criteria, and its own verdict, handed to an independent drafter that must
respond to it. Draft, independent critique, conditional revise. The
separation is the mechanism: the drafter cannot grade the critique's leniency,
and the critic cannot reach in and edit the draft.

Here is the thing that makes this book worth your weeks: Relurpify already
agrees with the thesis, mechanically. In the turn you will run in this
chapter, a completed run gets flagged by the framework itself — *the model
asserting success is not evidence* — because a verification claim needs an
observation behind it. The framework refuses to take the model's word. The
critic paradigm you will build is that principle, promoted from a guard
inside one node to a first-class agent shape, addressable from the recipe
DSL as:

```text
agent fixer uses critic
```

By the end of Part III, that line will be real, and by the end of the book
it will be merge-ready. First you need to see the machine you are extending
— from the builder's seat, not the operator's.

## The forge, from the doorway

Book 1 showed you the framework as a product. Now it is raw material. Three
names, and a rule about how they relate.

**`cognitionzoo` is where agents live.** The package doc states its question
verbatim: *"which agent paradigms, planning models, and execution patterns
does Relurpify use?"* It ships eight paradigms, each in its own package,
each declaring what it is before any code says how it runs:

| Paradigm | Package | Execution shape | Directives it accepts |
|---|---|---|---|
| ReAct | `cognitionzoo/react` | iterative | `until` |
| ReWOO | `cognitionzoo/rewoo` | staged plan | `plan` (required), `step`, `synthesize` |
| Planner | `cognitionzoo/planner` | plan-execute | `plan`, `step`, `verify`, `summarize` |
| HTN | `cognitionzoo/htn` | decomposition | `method`, `task` |
| Blackboard | `cognitionzoo/blackboard` | blackboard | `source` |
| Pipeline | `cognitionzoo/pipeline` | linear | (none — stages are grammar) |
| Chainer | `cognitionzoo/chainer` | chain | `link` |
| Reflection | `cognitionzoo/reflection` | review loop | `review`, `revise` |

Read that middle column slowly. **Execution shape** is not a base class and
not a strategy object. Each paradigm declares its shape as data — a label in
a closed set — along with the directives a recipe may address it with, the
order those directives may appear, what it composes with, what it guarantees,
and executable conformance cases that prove the guarantees. All of that lives
in a struct. In Chapter 7 you will fill in a struct of your own.

**`paradigm.Deps` is what the runtime hands an agent.** Every paradigm's
constructor takes `*paradigm.Deps` (`cognitionzoo/paradigm/deps.go`), and
the composition root fills it at boot. Surveyed, the struct carries
fifteen fields — the model port, the capability registry, the permission
checker, a sandbox command runner and its policy, working memory, the AST
index, search, the stream trigger, the grounder, the prompt registry, and a
handful of runtime services you will meet properly in Chapter 8. Two fields
deserve a look right now, because they set the governance bar you are
joining:

```go
// PermissionChecker is the capability gate shared by governed paradigms
// (ReWOO refuses to execute without one). Sourced from the same
// authorization bundle that backs the registry's permission manager.
PermissionChecker permissions.CapabilityChecker
```

*ReWOO refuses to execute without one.* That sentence is the whole culture
of this codebase in one field comment: fail closed, and make the failing
refusal part of the type. When your critic runs, it inherits exactly this
bar. No paradigm in this framework executes a tool because a model asked
nicely.

**`execution/agentgraph` is the machinery underneath.** Its package doc
opens: *"a deterministic state-machine workflow runtime for Relurpify
agents."* A graph is a directed structure of typed nodes — `ToolNode`,
`ConditionalNode`, `HumanNode`, `SystemNode`, `ObservationNode`,
`TerminalNode`, `StreamTriggerNode`, `RetrievalNode` — executed in order
while a typed context structure threads through each step. Graphs seal on
first execution and refuse mutation afterward. The shipped paradigms are
themselves graphs: ReAct is a think → act → observe loop you will see in
your telemetry as `react_think`, `react_act`, `react_observe` nodes.

The rule that relates the three names is the **domain DAG** — the enforced
layering you will make mechanical in Chapter 3. In one sentence:
`cognitionzoo` sits above `execution`, imports it, and may not be imported
by it. Paradigms consume the graph runtime; the graph runtime does not know
paradigms exist.

## The two seams

So you want to add an agent to this framework. There are exactly two doors,
and the framework's own docs name them as extension points:

| Want to add… | Door | Where you will live |
|---|---|---|
| A one-off agent for your workspace | **Compose graphs** | your recipe or your Go test, out of `agentgraph` nodes |
| A new kind of agent *the DSL can address* | **Declare a paradigm** | `cognitionzoo/`, registered like the shipped eight |

**The first seam — compose graphs — is power without vocabulary.** You
build a `Graph` by hand: `NewGraph`, `AddNode`, `AddEdge`, `SetStart`. Full
control over every node, edge, and condition. What you do not get is a
grammar. Nobody can write a recipe that says *use that thing you built*;
your agent is code only its author can invoke. Part II of this book lives
here, and by the end of Chapter 6 you will have a working critic pipeline
as a hand-composed graph — and you will feel, concretely, why that is not
enough.

**The second seam — declare a paradigm — is vocabulary, and vocabulary has
a price: a contract.** Look at the entire contract for the shipped
reflection paradigm, the specimen you will model your own on
(`cognitionzoo/reflection/contract.go` — this is the whole file minus the
`init` that registers it):

```go
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "reflection",
		Summary:  "Review loop over a react delegate with bounded revision iterations.",
		Shape:    paradigm.ShapeReviewLoop,
		Composes: []string{"react"},
		Directives: []paradigm.DirectiveSpec{
			{Name: "review", Form: paradigm.FormLine, Text: paradigm.ArgOne},
			{Name: "revise", Form: paradigm.FormBlock, Text: paradigm.ArgNone,
				Predicate: true,
				Body:      []paradigm.BodyItem{paradigm.BodyItemRun, paradigm.BodyItemDelegate},
				Requires:  []string{"review"}},
		},
		Order: &paradigm.OrderRule{Sequence: []string{"review", "revise"}},
		Guarantees: []string{
			"bounded revision iterations",
			"authored review verdicts live in the ephemeral scratch namespace",
			"composes the react delegate through the scoped-registry path",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "reflection/review_writes_scratch",
				Directive:    "review",
				Assertion:    "the review verdict is written to scratch.review / scratch.review_issues and the result fields",
				FixtureShape: "agent uses reflection; run: review \"…\"",
			},
			// … three more cases: revise_fires_on_predicate,
			// revision_cap_bounded, review_pass_ends_loop
		},
	}
}
```

Thirty lines, and it is a load-bearing document: the recipe loader reads it
to decide which directives are legal, the linter replays it to reject bad
recipes at lint time, and the conformance harness runs its cases to prove
the paradigm keeps its word. Notice `Requires: []string{"review"}` — the
grammar itself forbids a `revise` block with no `review` before it. The
contract is not documentation that drifts. It is data the loader reads, and
data that gets tested.

That is the door this book walks through. The critic will not merely *run*;
it will be addressable, in the same sentence as `react` and `reflection`:
`agent fixer uses critic`.

Which door, when? Compose graphs when the agent is yours alone and the
vocabulary would be premature — prototype, ensemble experiment, one recipe's
private step. Declare a paradigm when the *pattern* is the contribution:
when other workspaces should be able to address it, constrain it, and hold
it to conformance. The book makes you walk both in order, graph first,
because you cannot declare a good contract for a pattern you have not built.

## Hands-on

Four steps, one arc: build everything, run the framework's law over it,
reset the demo repo, and drive one turn with your eyes on the receipts.

### 1. Get the code, pin it, build it

```bash
git clone https://github.com/lexbity/relurpify.git
cd relurpify
git checkout BETA-RC1-FALL2026
```

This book, like book 1, is pinned to an edition tag; every internal the
chapters show is read at that tag, and the labs re-verify on re-pin. (One
honesty note: as of this writing the tag is not yet pushed to the public
remote — the chapter transcripts were captured on the tree that became it,
whose own telemetry reports the commit it was built from. If `checkout`
fails, build `main` and expect small diffs; the labs README records the
exact tree.)

Build everything:

```bash
go build ./...
go vet ./...
```

```text
$ go build ./...        # no output; exit 0
$ go vet ./...          # no output; exit 0
```

One Go module, one command each, nothing to configure. The build is
offline-friendly; a first build takes a minute or two.

### 2. Run the law: `make lint-arch`

This is the baseline you will restore after every chapter of this book, so
learn to read it now:

```bash
make lint-arch
```

```text
[PASS] lint-class-normalization: all class casts normalize
[PASS] lint-no-permissive-hitl: no permissive HITL broker
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/envcheck
[PASS] envcheck: no env access outside userconfig (1087 files scanned)
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/shimcheck
[PASS] shimcheck: no forbidden language (1087 files scanned)
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/symcheck
[PASS] symcheck: no removed symbols (1087 files scanned)
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/archcheck; EXIT_CODE=$?; \
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=direction; \
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=governance-orch; \
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/domaincheck -mode=enforce -check=context-ports; \
exit $EXIT_CODE
[PASS] forbidden: no violations
[PASS] converter: no violations
[PASS] sqlite: no violations
[PASS] glob: no violations
[PASS] stub: no violations
[PASS] cycle: no violations
[PASS] bucket: no violations
[PASS] consumer: no violations
[PASS] All architecture gates passed
[PASS] domain-direction: no violations
[PASS] All domain checks passed
[PASS] governance-no-orchestration: no violations
[PASS] All domain checks passed
[PASS] context-ports: no violations
[PASS] All domain checks passed
```

Three seconds, and here is what just happened — because this output *is*
Chapter 3, compressed:

- **`envcheck`** parses every one of 1,087 Go files and fails if anything
  outside the configuration boundary reads a process environment variable.
  Not a grep — an AST walk, so aliasing the import does not slip it.
- **`shimcheck`** enforces the charter's four bans, stated in the
  repository's `AGENTS.md` as four lines: *no shims / no compat / no
  aliases / no stubs.* The `[PASS] forbidden` and `[PASS] converter` rows
  are its report: neither the vocabulary nor the telltale type-converter
  signature may appear.
- **`symcheck`** fails if a symbol the project removed ever reappears.
- **`archcheck` and `domaincheck`** enforce the domain DAG: import
  direction, no governance→orchestration imports, no context-port
  violations, no bucket packages, no cycles.

Notice the shape of the deal. The layering is not a convention diagram in a
wiki; it is a program, and it runs before your PR does. When your critic
lands in `cognitionzoo/`, this same gate will check its imports in three
seconds — and when you get one wrong, it will tell you exactly where. You
will come to love a program that says no.

### 3. Reset stringkit

The demo repo carries over from book 1, reset: no seeded failure, tests
green. Copy it from the labs and start clean:

```bash
mkdir -p ~/code/stringkit && cd ~/code/stringkit
cp ~/relurpify/devdocs/agent-construction-relurpify/labs/stringkit/*.py .
git init && git add -A && git commit -m "seed stringkit (book 2 reset)"
python3 -m pytest test_stringkit.py -q
```

```text
..                                                                       [100%]
2 passed in 0.02s
```

Both tests green. Book 1's deliberately broken `slugify` is history; this
is the repaired state, and it is where the critic's story starts. You do
not fix anything this chapter. You make the agent *look* at this code —
which, this book will argue, is a different job than writing it, and needs
a different agent.

### 4. One turn, receipts and all

Run doctor, boot the TUI, and drive one turn — any read-only instruction
about stringkit will do:

```bash
~/relurpify/relurpish doctor --workspace ~/code/stringkit   # verdict: ready
~/relurpify/relurpish chat --workspace ~/code/stringkit
```

Book-1 readers know the doctor's report; `Result: ready` with the two
informational warnings is a healthy machine. The turn is where it gets
interesting, and on the verification machine the *first* turn taught a
lesson worth its own section. Here is what happened, exactly.

**Turn one.** With the materialized profile as shipped
(`tool_calling.intent: auto` in
`relurpify_cfg/model/profiles/default.llm.yaml`), asked to explain
`slugify`, the panel showed the model's raw decision *as text*:

```text
│ 🤖 [10:05] Agent                                                   │
│ 🤔 Thinking [−]                                                    │
│ └─ 🔍 Analyzing request (4s)                                       │
│                                                                    │
│ {"thought":"I need to find and read the content of 'stringkit.py'  │
│ to explain the 'slugify' function. I will start by listing the     │
│ files to locate the file.","action":"file_list","tool":"file_list", │
│ "arguments":{"directory":"."}}<tool_call|><|tool_response>          │
│                                                                    │
│ 🧾 Result                                                          │
│ node=euclo.done | status=ok                                        │
│ ⏱  4s | 29 tok                                                     │
```

No tool ran. (Panels reflowed from the captured 120-column transcript; the
full-width originals are in
`labs/transcripts/ch01-baseline.md`.) The model's *decision* arrived as
prose — the template leak
book 1 flagged in its own first chapter — and the run closed `ok` having
done nothing at all. Keep this failure in your pocket: it is Chapter 2's
door. What the runtime consumes is not "what the model said" but a
structured decision, and the *shape* a model's output must have for that
parse to work is a contract like any other in this framework. Chapter 2
puts you on the implementing side of it, and your fake model will never
leak.

**The fix is the book-1 drill, now with a builder's explanation:** set the
intent explicitly, restart the TUI so the profile reloads.

```bash
sed -i 's/  intent: auto/  intent: prompt_based/' \
  relurpify_cfg/model/profiles/default.llm.yaml
```

**Turn two.** Same instruction. This time the model decided cleanly, the
runtime parsed the decision, executed the read through the governed
pipeline, and the model produced a real explanation of `slugify` —
lowercase, strip punctuation, collapse whitespace — with a correct account
of what a bug in it would break. The run closed, and then the framework
said this:

```text
│ 🧾 Result                                                          │
│ node=euclo.done | status=ok                                        │
│ ⏱  2s | 29 tok                                                     │
╰────────────────────────────────────────────────────────────────────╯
│ ⚙ [10:08] System                                                   │
│ Agent error: task edits code but the run executed no file write or │
│ edit observation                                                   │
```

Read it again, because this is the critic thesis living in the code today.
The task looked edit-shaped; the run executed no write; the framework
declined to certify the result. The rule is written in the observe node of
the shipped react paradigm
(`cognitionzoo/react/react_observe_node.go`), as a comment on the check
that produced that flag:

> Loop-close honesty: an edit-shaped task that closed without an executed
> edit observation is unverified no matter who claimed completion — the
> model asserting success is not evidence.

*The model asserting success is not evidence.* You just watched the
framework refuse to take its own agent's word for it — a correct,
complete-looking answer, downgraded because the claim had no observation
behind it. Every chapter of this book builds toward making that instinct
into an agent. (`[NEEDS CONFIRMATION: at BETA-RC1-FALL2026, chat-routed
react runs appear edit-shaped regardless of phrasing — "explain…",
"analyze…", "without modifying anything", "investigate…" all flagged — so
read-only chat turns close unverified. The exact chat→react task-type
wiring gets pinned during Chapter 2 research.]`)

**The receipts.** You have seen them before as an operator; look once as a
builder, because your agent will write these too. The audit chain
(`.relurpify_state/audit/agent-stringkit-euclo/chain-*.jsonl`) recorded each
tool the run used, with the decision that let it through:

```json
{"action": "tool:file_list", "type": "hitl", "result": "tool_allowed",
 "timestamp": "2026-10-23T17:05:06.99Z"}
{"action": "tool:file_read", "type": "hitl", "result": "tool_allowed",
 "timestamp": "2026-10-23T17:06:11.79Z"}
```

And the telemetry export
(`.relurpify_state/telemetry/otlp/relurpify-*.jsonl`) carries the turn's
spans — the node names are the graph you read about above, made visible:

```text
node euclo.intake  node euclo.family_select  node euclo.ingest
node euclo.checkpoint  node euclo.policy_gate  node euclo.dispatch
node euclo.route_fork  node euclo.execute_thoughtrecipe
node euclo.merge  node euclo.report
node react_think  node react_act  node react_observe  node react_done
```

Twelve Euclo nodes for one instruction, then four react nodes for the run
it dispatched to. The Euclo root graph classifies, gates, and routes; the
react paradigm *is* the graph it hands the work to. When your critic ships,
its nodes will appear in this same list, and operators will read them in
their TUI the way you just read these.

## Gotchas

- **The profile edit needs a TUI restart.** `relurpify_cfg/model/profiles/`
  is read at boot; a mid-session edit does nothing to a running TUI. Quit,
  restart, retry — and check the transcript in
  `labs/transcripts/ch01-baseline.md` if your turn looks different.
- **The unverified-run flag is normal for read-only chat turns at this
  edition.** If your explain-style turn closes with the edit-observation
  flag, you are not broken; you are watching the loop-close honesty guard.
  Turns that actually edit and verify close clean.
- **`git checkout BETA-RC1-FALL2026` may fail until the tag is pushed.**
  Build `main` and note the commit; the labs README carries the tree the
  transcripts came from.
- **`make lint-arch` needs no sandbox, no model, no workspace.** It is pure
  source analysis — three seconds, offline, every time. If it goes red, fix
  the cause before anything else; never add an exemption to make it green.
  Exemption tables are load-bearing, and Chapter 11 explains what that
  costs.
- **stringkit ships green.** Do not break it "to give the agent something
  to do" — chapters that need a broken state seed their own, and the labs
  copy is the reference.

## Summary

- The critic thesis: self-review is not review; the same model grading its
  own work shares its own blind spots. The buildable answer is a separate
  phase — own prompt, own criteria, own verdict — and the framework's
  loop-close honesty rule (*the model asserting success is not evidence*)
  is that principle already enforced in miniature.
- `cognitionzoo` is the paradigm collection: eight shipped paradigms, each
  declaring its contract — shape, directives, order, guarantees,
  conformance cases — as data. `paradigm.Deps` is the governed surface the
  runtime hands every paradigm; the permission checker is mandatory, and
  ReWOO refuses to run without one.
- Two extension seams: compose graphs (power without vocabulary) and
  declare paradigms (vocabulary with a price — the contract). This book
  walks both, in that order.
- `make lint-arch` is the baseline: three AST gates, the charter's four
  bans, and the domain DAG, enforced in three seconds over 1,087 files.
  You will restore this baseline at the top of every chapter.
- One real turn showed the book's two themes in the wild: a model decision
  arriving as unparseable text (Chapter 2's subject), and the framework
  downgrading an uncertified success (the critic thesis, met in person).

## Check yourself

1. Why is a self-review loop not the same thing as independent critique,
   and what does the reflection paradigm's contract have that the critic's
   will also need?
2. Your recipe needs an agent that only you will ever invoke, once, for one
   experiment. Which seam do you use — and what do you give up by choosing
   it?
3. What does `envcheck` enforce, why is it an AST walk rather than a grep,
   and what happens if your new `cognitionzoo/critic` package reads an
   environment variable?
4. In the turn you ran, the framework flagged a completed, correct-looking
   run. Quote the rule it enforced and say why the rule exists.
5. The audit chain showed `tool:file_read … tool_allowed`. Which field of
   `paradigm.Deps` is the gate that made that decision, and which shipped
   paradigm refuses to run without it?

*Answers.* 1. Self-review is one agent grading itself with the same weights
that produced the work — errors correlate with the checker; reflection
already gives the vocabulary (review/revise, bounded revisions, conformance
cases), which is why it is Chapter 7's template. 2. The graph seam: full
control, but no grammar — no recipe can address your agent, and nothing
holds it to conformance. 3. No code outside the configuration boundary may
read process environment variables; as an AST walk it survives import
aliasing and string tricks; your package fails the gate, in three seconds,
by exact file. 4. "An edit-shaped task that closed without an executed edit
observation is unverified no matter who claimed completion — the model
asserting success is not evidence" — because a completion claim from the
actor that produced the work is exactly the correlated-errors problem. 5.
`PermissionChecker permissions.CapabilityChecker`; ReWOO fails closed
without one.

## Try it yourself

1. **Read the neighbor contracts.** Open `cognitionzoo/rewoo/contract.go`
   and `cognitionzoo/htn/contract.go` next to the reflection contract you
   saw here. Find, in each: the shape label, the required directive, and
   one guarantee you could imagine writing a conformance case for. You are
   practicing Chapter 7's core skill on shipped material.
2. **Break a gate on purpose.** In a scratch copy of the repo, add
   `os.Getenv("HOME")` to a file in `cognitionzoo/react/` and run
   `make lint-arch`. Read the failure. Then revert it and confirm the
   baseline goes green. (Do this in a throwaway clone, not your working
   tree — and notice how fast the gate tells you exactly where you sinned.
   That speed is why the dev loop in Chapter 3 starts here.)

## Where next

You have a green baseline, a clean stringkit, and one turn's worth of
receipts — plus a leak, which is the best kind of homework. Chapter 2 goes
straight at it: the `model.LanguageModel` contract, read the way its authors
wrote it, and then implemented — by you, as a scripted fake model you
control. Every later chapter drives the critic through that fake, because
determinism is the only honest way to test a loop. Bring the leak; we turn
it into an interface.

The lab for this chapter is the baseline itself:
`devdocs/agent-construction-relurpify/labs/` holds the reset stringkit,
`verify.sh` (`./verify.sh gates ch01` re-runs this chapter's baseline in
your checkout), and the captured transcripts from the verification machine.
