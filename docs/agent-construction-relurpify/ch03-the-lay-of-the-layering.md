# Chapter 3 — The Lay of the Layering

Every mature codebase has architecture rules. Most keep them in a diagram,
and the diagram rots the first time someone merges against it. Relurpify
keeps its rules in programs that run in CI and fail loudly — and in this
chapter you learn to read them, break one on purpose, and feel how fast
the law answers. By the end, `make lint-arch` stops being a ritual and
becomes what it is: the fastest code review you will ever get.

This is also the chapter where the book's own code earns the right to
exist. Last chapter ended with a gate telling us where our package
belongs. Now you get the whole law, and the vocabulary it protects —
including the type that threads through every graph the framework will
ever run.

## Where you are

You have the fake model in `cognitionzoo/critic/` (test-only, gate-clean)
and a green baseline. This chapter adds one more test file to the critic
package — an envelope walkthrough — and then deliberately breaks the law
twice, in a scratch clone, to watch the gates catch it. Nothing in this
chapter touches the network, the model, or your working tree.

## The shape: a domain DAG, not a diagram

Relurpify is a modular monolith: one Go module, a handful of local
binaries, no broker, nothing listening on a socket. The interesting
structure is *inside* the module, and it is expressed as a **directed
acyclic graph of domains** — top-level directories that may import the
domains they point at and no others. The normative table lives in the
repository's own handoff docs and is *encoded twice*: in
`tooling/arch/domaindag.go`, and asserted by a layout test. The repo's
rule when doc and encoder disagree: the encoder wins, and both are fixed
in the same commit.

The part of the DAG you will live in, simplified:

```text
app ──▶ named/euclo, cognitionzoo ──▶ execution ──▶ context, capability,
                                     governance, model …
cognitionzoo ──▶ execution, context, capability, governance, model, …
execution  ──▶ context, capability, governance, model, …
context    ──▶ capability, governance, model, …
governance ──▶ model, …
model      ──▶ (leaf vocabulary; platform is its implementation home)
```

Arrows point in the *depends-on* direction. Your critic package sits in
`cognitionzoo`, which may import `execution`, `context`, `governance`,
`model` — and is imported by `app` and `named/euclo`, never the reverse.
Two structural rules give the DAG its shape, and both have names you will
use:

- **Vocabulary-with-owner.** Every shared type lives in exactly one domain.
  The language-model port is owned by `model`; backends live in
  `platform/llm`. Cross-domain re-exports via type aliases are forbidden —
  this is one of the reasons the charter's "no aliases" ban is not a style
  quirk.
- **Consumer-defined ports.** When a domain needs behavior from a domain
  above it, the *consumer* declares a narrow interface in its own `ports`
  package; the upper domain implements it; the composition root injects
  the concrete. You saw the shape in Chapter 2: `model.EventSink` is what
  it looks like when the leaf defines the narrow thing it is willing to
  know about.

## The gates: three programs and a fence

`make lint-arch` runs in about three seconds, and everything it prints is
one of five programs. From the Chapter 1 transcript, annotated:

| Program | What it reads | What it enforces |
|---|---|---|
| `envcheck` | AST of every production Go file | Only `userconfig` reads process environment variables |
| `shimcheck` | AST, string literals, identifiers | The charter's forbidden vocabulary and the telltale type-converter signature |
| `symcheck` | AST | Symbols the project removed stay removed |
| `archcheck` | `go list` import graph | `forbidden`, `converter`, `sqlite`, `glob`, `stub`, `cycle`, `bucket`, `consumer` checks |
| `domaincheck` | import graph, domain table | `direction`, `governance-orch`, `context-ports` |

(Makefile detail: `lint-arch` also runs `lint-class-normalization` and
`lint-no-permissive-hitl` before the AST gates — class strings must
normalize at every boundary, and no permissive HITL default may creep in.)

Three properties of this machinery matter more than any single check:

**The gates read production code.** The shared walker (`tooling/arch/
gatescan`) takes an `IncludeTests` flag whose comment states the policy:
*"Gates that must police production code only leave it false."* All three
AST gates leave it false. `_test.go` files are not exemptions, not holes —
they are a different kind of material, and the gates know it. This is why
your test-only critic package scanned clean while 1,087 production files
were checked around it.

**The consumer gate is the anti-dead-code gate.** Every non-main,
production package must have at least one importer — production or test.
A package with zero importers of any kind is dead code, and the gate's
comment says so in exactly those words. Test-only packages are skipped
(consumed by their own tests); `main` packages are skipped (they are
endpoints, not libraries); documented domain anchors are exempt by name;
and everything else goes through an allowlist whose entries are
load-bearing — the gate's own tests fail if an exemption stops being
needed. You met this gate personally in Chapter 2.

**The gates check the gates.** `make grep-architecture-gates` runs the
grep-layer fences (what a syntax tree cannot express — a forbidden string
assembled by concatenation), and `make check-gates-honest` verifies that
the fences themselves still bite: that the env fence would still catch an
`os.Getenv`, that every AST gate exists and is wired into `lint-arch`. A
weakened gate is treated as a broken gate.

### The charter's four bans

The repository's `AGENTS.md` states the whole policy in four lines:

```text
no shims
no compat
no aliases
no stubs
```

`shimcheck` enforces the vocabulary: the words `shim`, `compatibility`,
`backward compatibility` outright, plus `compat` as a whole word — the
short spelling is legitimate inside names the project actually ships
(`api_compat`, `openai_compatible`), so the gate matches word boundaries:
"compat layer" is a violation, `api_compat` is a capability ID. And the
gate reads **string literals** — a comment saying "compatibility shim"
passes, a constant holding the string `"compatibility alias…"` does not.
You will prove both halves yourself in the lab.

Why ban them at all? Because each of the four is a half-finished migration
left in the tree: callers keep calling the old name, the new surface rots,
and the documentation drifts. The charter's answer is that removals are
clean breaks. When the critic paradigm ships, there will be no
`criticCompat` wrapper around `reflection` and no "temporary" alias —
there will be one paradigm, and its name will be the only way to say it.

## The envelope: the type that threads everything

One type flows through every graph, node, and branch in the framework:
`contextdata.Envelope` (`context/contextdata/`). Its package doc states
the three-tier model the whole runtime is built on:

1. **Streamed Context** (read-only) — assembled by the compiler from
   knowledge chunks, summaries, and checkpoint state; represented by
   references, never copies.
2. **Working Memory** (mutable) — per-turn, per-task in-memory state; the
   only tier graph nodes may write.
3. **Retrieval State** (controlled read) — results from retrieval
   operations, stored as references in the envelope.

The invariants are stated just as flatly: streamed context is read-only
outside the compiler; working memory is the only mutable tier for graph
nodes; checkpoints may be requested by nodes but are owned by the
compiler. Your Chapter 1 turn was a sequence of nodes mutating one
envelope under those rules — `react_act` writing observations,
`euclo.checkpoint` requesting a checkpoint — and every later chapter's
graphs will live inside the same contract.

Three API facts are enough to work with an envelope:

- **Everything you write is classified twice.** A working-memory value
  carries a memory class (`MemoryClassEphemeral`, `MemoryClassSession`,
  `MemoryClassTask`) and an origin (`OriginUser`, `OriginTool`,
  `OriginLLM`). Provenance is not metadata you add later; it is part of
  the write.

  ```go
  env.SetWorkingValueWithOrigin("critic.draft", "first draft",
      contextdata.MemoryClassTask, contextdata.OriginLLM)
  ```

- **Typed reads are generic and honest.** `contextdata.GetTyped[string]`
  returns the value and an `ok` — no assertions, no surprises.

- **Branches clone, then merge deltas.** A parallel branch works on
  `contextdata.CloneEnvelope(env)`; when branches converge, the parent
  applies each branch's **delta** — `ComputeBranchDelta` computes what the
  branch added, modified, and deleted; `ApplyBranchMerges` installs it.
  The parent never saw the branch's writes before the merge, and the
  merge, not the branch, is where conflicts become visible.

The other half of the flow is `execution.Result`
(`execution/agent_executor.go`) — what a node returns:

```go
type Result struct {
	Success  bool
	Data     ResultPayload
	Error    string
	Metadata map[string]any
	NodeID   string
}
```

`ResultPayload` is a small interface with two shipped shapes —
`ToolResultPayload` (keyed fields) and `PromptResultPayload` (text plus
fields). Chapter 1's result panel — `node=euclo.done | status=ok` — was
this type rendered.

### The envelope tour, runnable

The chapter overlay adds one test to the critic package that walks an
envelope through exactly this lifecycle — write with class and origin,
typed read, clone a branch, verify the parent is untouched, compute the
delta, merge, verify the merged value:

```go
env.SetWorkingValueWithOrigin("critic.draft", "first draft",
	contextdata.MemoryClassTask, contextdata.OriginLLM)

branch := contextdata.CloneEnvelope(env)
branch.SetWorkingValueWithOrigin("critic.verdict", "issues found",
	contextdata.MemoryClassTask, contextdata.OriginLLM)

// The parent is untouched by the branch's write.
if _, ok := contextdata.GetTyped[string](env, "critic.verdict"); ok {
	t.Fatal("parent envelope saw a branch-local key before merge")
}

delta := contextdata.ComputeBranchDelta(env, branch)
stats, err := env.ApplyBranchMerges([]contextdata.BranchMergeUnit{
	{Index: 0, ID: "critique", Delta: delta, Env: branch},
})
```

(The full test is `envelope_tour_test.go` in the chapter overlay; it
asserts the delta carries exactly the one added key and the merge reports
exactly one unit and one key written.)

```bash
./verify.sh build ch03
```

```text
== tier 1 (build): ch03 ==
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.057s
[PASS] build: ch03 compiles clean and its tests pass
```

Ten tests green now — the nine from Chapter 2 plus `TestEnvelopeTour` —
and the clone was built, vetted, and thrown away by the verifier without
your tree noticing.

## Hands-on: break the law, watch it answer

The lab plants two violations in a scratch clone, captures the failures,
and reverts. The full transcripts live in
`labs/transcripts/ch03-gate-failures.md`; here is each beat with what it
teaches.

**Beat 1 — the same sin in a test file: silence.** Plant
`os.Getenv("CRITIC_FALLBACK")` in the critic package's test file and run
the gate:

```text
[PASS] envcheck: no env access outside userconfig (1087 files scanned)
```

Nothing. The gates do not read test files — by policy, not oversight. If
you want your sin caught, ship it in production code.

**Beat 2 — the same sin in production code: exact coordinates.** The
identical `os.Getenv`, planted in a production file, plus its `"os"`
import:

```text
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/envcheck
  [FAIL] env: cognitionzoo/react/react_think_node.go:38 reads the process environment via os.Getenv
[FAIL] envcheck: 1 env access violation(s) outside userconfig
exit status 1
make: *** [Makefile:123: envcheck] Error 1
```

File, line, mechanism. And `make` stops here: a red gate short-circuits
the run, so the first failure you see is the first one in line.

**Beat 3 — the vocabulary ban, where it actually bites.** A comment
saying "compatibility shim" passes shimcheck; a string literal does not:

```go
const criticLabAliasNote = "compatibility alias for the old flag name"
```

```text
  [FAIL] shim: cognitionzoo/react/react_think_node.go:35 carries forbidden language "compatibility" in a string literal
[FAIL] shimcheck: 1 forbidden-language literal(s)
```

The gate reads what ships, not what you say about what ships. That is the
difference between a linter and a gate.

**Beat 4 — revert, and the law is restored in one command:**

```bash
git checkout cognitionzoo/react/react_think_node.go
make lint-arch
```

```text
[PASS] All architecture gates passed
[PASS] All domain checks passed
```

Every violation in this lab existed only in the scratch clone; your tree
never saw them. That is the dev loop you will run from now on — build,
vet, test, `lint-arch` — and the reason violations here are cheap: the
gates answer in seconds, with coordinates.

## Gotchas

- **A green `envcheck` does not mean your test code may read the
  environment.** The gates police production code; the *policy* ("no env
  access outside userconfig") is not suspended for tests, and AGENTS.md
  states it without a test-file asterisk. The gates are a floor, not the
  spec.
- **`make lint-arch` fails fast.** When it stops at envcheck, later gates
  have not run. Fix in order, or run the gate you care about directly
  (`go run ./tooling/arch/cmd/shimcheck`) — every gate is a `main` in
  `tooling/arch/cmd/`.
- **Gate failures name coordinates.** Read the `[FAIL]` lines before
  opening any file; the gate already knows where the sin is.
- **The consumer gate counts importers at `go list` time.** If you add a
  production file to the critic package before Chapter 6 wires its first
  importer, the gate will (correctly) call it dead code. The chapter
  sequencing in this book exists so you never fight that gate.
- **`testdata` directories are invisible to the toolchain.** That is why
  the labs overlay lives under `labs/testdata/`. It also means: never put
  real, needed Go packages inside a directory named `testdata`.

## Summary

- The architecture is a DAG of domains enforced by programs: import
  direction (`domaincheck`), dead code (`consumer`), forbidden vocabulary
  (`shimcheck`), the env boundary (`envcheck`), removed symbols
  (`symcheck`), plus cycle, bucket, and port checks — all inside
  `make lint-arch`, all in about three seconds.
- The charter bans shims, compat layers, aliases, and stubs — enforced on
  string literals and type-converter signatures, not comments. The gates
  police production code; test files are a different material with its
  own rules.
- `contextdata.Envelope` is the type that threads every run: three tiers
  (streamed context read-only, working memory mutable, retrieval by
  reference), every write carrying a memory class and an origin, branches
  cloning and merging deltas — and `execution.Result` carrying each
  node's verdict.
- You broke the law twice and it answered with file, line, and mechanism,
  both times, in seconds — then reverted cleanly. That reflex — read the
  gate, fix the cause, never add an exemption — is Chapter 11's
  curriculum, met early.

## Check yourself

1. Name the two structural rules that give the domain DAG its shape, and
   say which one decides where the `model.EventSink` interface lives.
2. Why does an `os.Getenv` in a `_test.go` file pass `envcheck`, and does
   that make it acceptable?
3. Your critic package gains a production file in Chapter 5 with no
   importer yet. What does `make lint-arch` print, which check produced
   it, and what is the gate's stated definition of such a package?
4. In the envelope tour, what does the parent see before
   `ApplyBranchMerges`, and what exactly does `ComputeBranchDelta` return?
5. shimcheck passed a comment containing "compatibility shim" but failed
   a string constant. What does that tell you about what the gate reads —
   and about where the charter's bans actually bite?

*Answers.* 1. Vocabulary-with-owner (each shared type lives in exactly
one domain) and consumer-defined ports (the consumer declares the narrow
interface it needs); the ports rule — `EventSink` is declared in the leaf
`model` domain so the model never imports telemetry. 2. The gates walk
production code only (`IncludeTests: false` is the stated policy); it is
*detected-exempt*, not acceptable — AGENTS.md's env rule has no test-file
asterisk, and the gates are a floor. 3. `consumer: … has no importers`
from archcheck's consumer check; the gate's own comment defines it: zero
importers of any kind is dead code. 4. Nothing — the branch's writes are
invisible until the merge; the delta is the added/modified/deleted key
sets (here exactly `critic.verdict` added). 5. It reads string literals
and identifiers (what ships), not comments (what you claim); the bans
bite on shipped vocabulary, which is why "temporary" aliases in constants
are exactly what the charter is for.

## Try it yourself

1. **Probe the skip list.** In the scratch clone, plant the Beat 2 sin
   inside a directory named `testdata` and run `envcheck` directly. Read
   the gatescan skip rules in `tooling/arch/gatescan/gatescan.go` and
   reconcile what you observe with `DefaultSkipDirs`. Then revert.
2. **Name the vocabulary.** Find three legitimate uses of `compat` in the
   shipped tree (`grep -rn "compat" --include="*.yaml" relurpify_cfg/` is
   a start) and explain, in shimcheck's terms, why each survives the
   whole-word boundary rule. You are learning to read the law's
   exceptions as load-bearing, not grudging.

## Where next

You can now read the tree as law and the envelope as contract. Time to
build on both. Chapter 4 opens `execution/agentgraph` for real: you will
implement the three-method `Node` interface by hand, build a graph with
`NewGraph` and `AddEdge`, seal it, run it over stringkit driven by your
fake model — and read your first preflight report. The critic pipeline
stops being a plan and starts being a graph.

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch03` re-runs everything this chapter proved, and
`transcripts/ch03-gate-failures.md` holds the violation arc in full.
