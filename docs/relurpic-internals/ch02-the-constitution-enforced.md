# Chapter 2 — The Constitution, Enforced

Every sufficiently old codebase has a constitution: the set of rules
everyone claims to follow and no tool enforces. It lives in a CONTRIBUTING
file, it is enforced by code review and good intentions, and it loses one
clause per quarter to "just this once." Chapter 1 showed you that
Relurpify's map cannot drift silently. This chapter shows the same is true
of its law: seven architecture decision records, each with a mechanism
that makes it non-optional — and, because the maintainers know fences can
rot, a gate that watches the fences themselves.

The chapter's lab is simple and slightly transgressive: you break one of
the seven rules with a single line of Go, watch two independent layers
catch you, and revert. Read the outputs carefully — they are the closest
thing this tree has to a voice.

> **A note on citations.** As in Chapter 1: tracked files are cited
> `path:line` and verifiable in the clone; the authoring corpus under
> `devdocs/` (handoff docs, whitepaper, plans) is cited by document and
> line but is not part of the public clone (`.gitignore:41`), so every
> claim it supports is anchored to a tracked file too. Captured at commit
> `1bc57c62`; see the pin note in the front matter.

## Where you are

From Chapter 1 you have the map: fifteen domains in an enforced DAG
(`tooling/arch/domaindag.go:37-117`), the two-directory config/state
split, and the knowledge that gates — not goodwill — hold the shape. This
chapter reads the law those gates enforce. You need nothing new: same
toolchain, no model backend, and one clean checkout you are about to
dirtied on purpose.

By the end you will be able to:

1. State each of the seven ADRs and name the mechanism that enforces it.
2. Explain why the fences come in two layers (AST and grep) and what each
   catches that the other cannot.
3. Read an exemption table — and explain why this tree treats every
   exemption as debt with a scoreboard.
4. Break a rule on purpose and read the two failure outputs you get.

## Technical requirements

| Requirement | Version / notes |
|---|---|
| OS / toolchain | As Chapter 1 (Linux primary; Go toolchain pin `go1.27.2`, `go.mod:5`) |
| Model backend | **Not needed** |
| Extra | Nothing. The lab dirties one tracked file and reverts it |

Captured on the Chapter 1 machine, commit `1bc57c62`.

## 2.1 The written law and the enforced law

The constitution is written in three places, and the division of labor
between them matters.

**The ADRs** — `docs/ARCHITECTURE.md:235-296` holds ADR-001 through
ADR-007, each in the same three-part shape: *context* (what went wrong
before the rule), *decision* (the rule), *consequences* (what the rule
costs you). Two further decisions are normative but live in code, not in
the ADR file: **vocabulary-with-owner** and **consumer-defined ports**
(ADR-008 and ADR-009 "in the domain DAG",
`devdocs/HANDOFF-docs/DECISIONS.md:121-147`) — you met both in Chapter 1
as the no-bucket rule and the `context/ports` seam.

**The invariants** — `devdocs/HANDOFF-docs/INVARIANTS.md` is the derived
law: what must always *hold* if the ADRs are true. Its section 1 restates
the eight architectural rules with an enforcement column, and marks every
row **high** for one reason only: each has a named gate in
`tooling/arch/` or the Makefile (`devdocs/HANDOFF-docs/INVARIANTS.md:17-36`).
The document is explicit about its own epistemology — "high = enforced by
a named gate, test, or explicit code path that I read"
(`devdocs/HANDOFF-docs/INVARIANTS.md:9`).

**The gates** — `tooling/arch/` and the Makefile targets that drive it.
This is the law's executable form, and it is the form the project trusts:

> *Why it won*: the docs in this repo are unusually honest about what is
> unfinished, and the failure they could not defend against was
> documentation drifting from reality. A gate cannot drift silently.
> — `devdocs/relurpify-technical-whitepaper.md:91`

The daily driver is `make lint-arch` (`Makefile:15-24`): class-string
normalization, no permissive HITL defaults, the three AST gates
(`envcheck`, `shimcheck`, `symcheck`), then `archcheck` and
`domaincheck` in enforce mode across three checks — direction,
governance-orchestration, context-ports. In CI it runs as its own job
(`.github/workflows/ci.yml:55`), alongside `check-gates-honest`
(`:57`) and `check-gates-slice10` (`:140`).

## 2.2 The seven ADRs, each with its teeth

A rule you cannot break by accident is a preference. The table below is
the constitution's map of itself: each ADR's decision, and the mechanism
that would fail if you violated it. "Teeth" varies by rule — some are
AST gates, some are grep fences, some are structural facts of the DAG,
one is a *deleted* runner guarded by a test that keeps it deleted.

| ADR | The decision (cite) | The teeth |
|---|---|---|
| 001 — one env boundary | Only `userconfig/config` reads the process environment; it snapshots once at boot (`docs/ARCHITECTURE.md:235-243`) | `envcheck` AST gate (`Makefile:122`, `tooling/arch/envcheck.go:23-59`) **plus** a grep fence with the same pattern (`Makefile:265-270`) |
| 002 — generated config | The embedded template bundle is the single source of truth; `relurpify_cfg/` is its materialization (`docs/ARCHITECTURE.md:244-252`) | `check-config-tree-drift` (`Makefile:26`; CI `.github/workflows/ci.yml:137`) |
| 003 — fail closed | Unset scopes deny; nil policies deny; the default posture is `ask`; private/loopback egress denied unconditionally (`docs/ARCHITECTURE.md:253-262`) | A *shape*, not one gate: defaults in the decision paths (e.g. `governance/permissions/decision.go`, `governance/netpolicy/resolve.go`), pinned by the fail-closed inventory (`devdocs/HANDOFF-docs/INVARIANTS.md:40-60`) and by grep fences such as INV-1 — no network I/O in `governance/authorization` (`Makefile:243-250`) |
| 004 — all execution sandboxed | Every command goes through the sandbox chain; the local host-exec runner was *removed* and a guard test keeps it removed (`docs/ARCHITECTURE.md:263-271`) | The guard test; the sandbox backend contract (Chapter 6) |
| 005 — no shims, compat layers, aliases, stubs | The charter forbids all four; bridged types hid real behavior and bred stale docs (`docs/ARCHITECTURE.md:273-278`) | `shimcheck` (forbidden language in string literals, `tooling/arch/shimcheck.go:21-33`) + `symcheck` (deleted symbols cannot resurrect, `tooling/arch/symcheck.go:14-49`) + a grep fence (`Makefile:272`) + `no-dead` |
| 006 — deterministic routing | Default selection posture `deterministic`; ambiguity asks, never coin-flips; `model_assisted` is opt-in (`docs/ARCHITECTURE.md:280-287`) | Behavioral: the posture default and route provenance (`decided_by`) in telemetry; the DSL is kept statically analyzable by the `euclo-*` gates in `lint-all` (`Makefile:146`) |
| 007 — durable jobs run in a sidecar | `relurpify-runner` owns the jobs store, receives work through an atomic file spool, and nothing needing governance runs there (`docs/ARCHITECTURE.md:289-296`) | The DAG itself: `ayenitd`'s allowed imports exclude capability, governance, and model — the sidecar rule *is* a DAG row (`tooling/arch/domaindag.go:57-59`) |

Three of these deserve a second look.

**ADR-005's vocabulary law has an escape hatch, and the escape hatch has
a rule.** `shimcheck` forbids "shim", "compatibility", "stub", and
"backward compatibility" as string literals — but the short spelling
"compat" is deliberately *not* on the list, because as a substring it is
indistinguishable from the project's own canonical vocabulary:
`platform/llm/openaicompat`, the config kind `openai_compatible`, the
capability ID `euclo:cap.api_compat`. It is matched on word boundaries
instead, so "compat layer" is a violation while "api_compat" is not
(`tooling/arch/shimcheck.go:15-33`). Hyphens are boundaries, so
"back-compat" violates too. The gate's comment says why in one line:
a word boundary would let "Shim-" and "shimmy" through.

**ADR-004 is enforced by absence.** The host-exec runner was not
deprecated; it was deleted, and a guard test exists to keep it deleted
(`docs/ARCHITECTURE.md:263-271`). `symcheck` extends the same idea into
the everyday: its `RemovedSymbols` table lists thirty-one identifiers
deleted in earlier remediation slices, and any occurrence in production
code fails the gate — "their return would resurrect the design the
slices removed" (`tooling/arch/symcheck.go:10-49`). In this tree, *the
past is fenced too*.

**ADR-007's teeth are a row you have already read.** Chapter 1 quoted
the DAG comment: the runner supervises services and drains the spool,
and "it must never link app composition" — its allowed imports exclude
capability, governance, and model (`tooling/arch/domaindag.go:57-59`).
"Nothing needing governance runs in the runner" is not a policy someone
must remember; it is an import direction the domain checker refuses to
reverse. And Chapter 1's other finding stands: the binary has never
shipped (D10 in the book plan). The boundary is real and enforced; its
tenant is absent — the framework degrades gracefully and `doctor`
reports the missing runner as an informational warning.

## 2.3 Two fence layers, one violation, two verdicts

The gates come in two layers, and CONTRIBUTING states the division
cleanly (`CONTRIBUTING.md:60-65`):

- **AST gates** (`envcheck`, `shimcheck`, `symcheck`) parse the syntax
  tree, "so they cannot be bypassed by aliasing an import, concatenating
  a forbidden string, or renaming around a pattern"
  (`Makefile:114-119`).
- **Grep gates** remain "for what a syntax tree cannot express, such as
  a forbidden string assembled by concatenation" (`CONTRIBUTING.md:64-65`).

The layers are not redundant; each covers the other's blind spot.
`envcheck` resolves imports on the tree, so `import o "os"; o.Getenv(…)`
is caught — the gate's own doc comment says this is exactly what the
grep version could not do (`tooling/arch/envcheck.go:16-22`). But a
forbidden *string* assembled at runtime (`"sh" + "im"`) is a binary
expression, not a literal, so the AST gate cannot see it — and the shim
gate says so in its comment, deferring to the grep layer
(`tooling/arch/shimcheck.go:43-45`).

Do not take the doc's word for it. Here is the chapter's experiment:
one line of Go, breaking ADR-001 in the leaf domain `model/`. The diff
(the lab ships it as `violation.diff`):

```diff
--- a/model/runtime.go
+++ b/model/runtime.go
@@ -1,6 +1,9 @@
 package model
 
 import "context"
 
+import "os"
+
+// lab violation: one deliberate environment read
+var labViolation = os.Getenv("HOME")
```

Layer one, the AST gate:

```
$ make envcheck
  [FAIL] env: model/runtime.go:8 reads the process environment via os.Getenv
[FAIL] envcheck: 1 env access violation(s) outside userconfig
exit status 1
make: *** [Makefile:123: envcheck] Error 1
```

Layer two, the grep fence, against the *same* violation:

```
$ make grep-architecture-gates
[PASS] check-contract-dissolution: manifest spine removed
[FAIL] grep-architecture-gates: direct env access remains outside userconfig
./model/runtime.go:8:var labViolation = os.Getenv("HOME")
make: *** [Makefile:266: grep-architecture-gates] Error 1
```

Read the two failures as fingerprints. The AST gate names the *semantic
fact* — a selector that resolves to the `os` package, at a position —
and would have caught the same read behind an import alias. The grep
fence quotes the *raw line* — and would catch the alias evasion only if
the literal text still matched, which is precisely the case it exists
for when the AST layer cannot parse the shape. Two independent
verdicts, one violation, and removing either layer leaves the other
still watching.

One wrinkle, stated by the tree itself: `tooling/arch` is exempt from
the env *grep* fence — but from that fence only. The AST gate's own
source must name `os.Getenv` to forbid it, and grep cannot tell a
comment from a call, so the Makefile's comment reads: "`make envcheck`
polices that tree on the tree" (`Makefile:263-268`). An exemption with
its reason attached is the theme of the next section.

## 2.4 Exemption tables are load-bearing

Older fence systems hide their exceptions in `grep -v` filters, and the
project's exemption doc explains exactly why that fails:
"basename filters that silently exempted every file so named anywhere in
the tree, with no record of why the exception existed" — so a newly
added file that happens to share a name is silently un-policed
(`tooling/arch/exemption.go:7-11`). The `Exemption` type replaces that
with explicit path *prefixes* matched at component boundaries and exact
*files*, "never a basename: the point is to exempt one known site, not a
name" (`tooling/arch/exemption.go:15-23`). And above the type, the
philosophy, in the shortest sentence in this book:

> Prefer no exemption. A gate that accepts nothing cannot be argued
> with.
> — `tooling/arch/exemption.go:13`

The tree currently practices what that sentence preaches, and the
practice is *gated*:

- The direction-violation exceptions file is empty by policy, its header
  saying so outright: "All direction violations are resolved
  structurally — no suppressions" (`tooling/arch/exceptions.yaml:1-3`).
- The `exception-count` gate holds the file to a baseline of zero: a
  net-new exception fails CI unless the spec is extended
  (`Makefile:105-119`).
- The gates' own tests police the exemptions from the other side:
  `TestExemptionCoversNothingByDefault`
  (`tooling/arch/exemption_test.go:32`) and
  `TestNoStaleDirectionEdges` (`tooling/arch/stale_edges_test.go:13`) —
  CONTRIBUTING's summary: "Every entry is load-bearing: the gates' own
  tests fail if an exemption stops being needed, so a hole cannot
  quietly grow a comment" (`CONTRIBUTING.md:67-70`).
- The archcheck allowlist lives at `devdocs/ref/arch-allowlist.yaml` —
  outside the public clone — and this is survivable *by design*: a
  missing allowlist loads as empty (`tooling/arch/allowlist.go:18-21`),
  the local file currently exempts nothing, and its header records the
  rule that the list "can only shrink" because the runner fails a
  stale entry that no longer matches a live violation. Delete the file
  and the gate judges the tree identically.

That last point is worth pausing on, because it is the constitution's
design in one mechanism: the tree is so close to violation-free that
the exception machinery can be absent without changing any verdict. The
fences are not holding back a flood of suppressions; there is no flood.

## 2.5 The gate that watches the gates

A fence pattern is itself text in a Makefile, and text can be edited.
`check-gates-honest` exists because the maintainers know the weakest
link in any gate system is the gate. Its mechanism (`Makefile:401-435`):

> Each check expands the gate's own recipe with `make -n` and greps the
> command that would actually run, so a pattern must live in the gate
> itself: the patterns named below cannot satisfy their own test, and
> deleting a pattern, an AST gate, or its wiring all fail here.
> — `Makefile:402-404`

Concretely, it fails if: the env fence loses its
`Getenv|LookupEnv|Environ` pattern; the compatibility fence loses its
pattern; `no-dead` loses its removed-symbol patterns; `lint-arch` loses
the domaincheck direction-enforce run; or any of the three AST gate
binaries goes missing from the tree or from `lint-arch`'s recipe. It
runs in CI (`.github/workflows/ci.yml:57`), and on the pristine tree:

```
$ make check-gates-honest
[PASS] check-gates-honest: all gate patterns and AST gates present
```

The same cross-checking instinct appears inside the gates: `symcheck`'s
removed-symbol table and the `no-dead` grep gate "keep the two in sync"
(`tooling/arch/symcheck.go:14-16`) — yet another place where two layers
watch each other. This is the chapter's thesis in miniature: in this
tree, *the enforcement is itself enforced*, and the regress stops at
the meta-gate.

## 2.6 Hands-on: break the law, get caught, revert

The lab (`docs/relurpic-internals/labs/ch02-constitution/`) ships the
command card, the violation diff, and the captured transcript. Run it
in a clean checkout; every step is reversible.

**1. Baseline: the constitution holds.**

```bash
make lint-arch
make check-gates-honest
```

Captured: `lint-arch` ends with `[PASS] All architecture gates passed`
and three `[PASS] … All domain checks passed` lines (direction,
governance-no-orchestration, context-ports); `check-gates-honest` ends
`[PASS] check-gates-honest: all gate patterns and AST gates present`.

**2. Apply the one-line violation.** The diff in §2.3 — add `import
"os"` and one `os.Getenv` variable to `model/runtime.go`
(`violation.diff` in the lab has the exact hunks).

**3. Watch layer one catch you.**

```bash
make envcheck; echo "exit=$?"
```

Captured: `[FAIL] env: model/runtime.go:8 reads the process environment
via os.Getenv`, exit 1. Note the verdict names the file, line, and the
semantic reason — not a text match.

**4. Watch layer two catch you too.**

```bash
make grep-architecture-gates
```

Captured: `[FAIL] grep-architecture-gates: direct env access remains
outside userconfig`, quoting the raw line `./model/runtime.go:8:var
labViolation = os.Getenv("HOME")`.

**5. Revert, verify nothing happened.**

```bash
git checkout -- model/runtime.go
git status --porcelain model/   # empty
make envcheck                    # PASS again
```

### Verification block

| Chapter claim | Where captured | Lab file |
|---|---|---|
| Pristine tree passes `lint-arch` (all four AST results + three domain checks) | §2.6 step 1 | `transcript.txt` §1 |
| Pristine tree passes `check-gates-honest` | §2.6 step 1 | `transcript.txt` §1 |
| One env line fails `envcheck` with a semantic verdict | §2.6 step 3 | `transcript.txt` §3 |
| The same line fails the grep fence with the raw line | §2.6 step 4 | `transcript.txt` §4 |
| Revert restores the PASS baseline | §2.6 step 5 | `transcript.txt` §5 |
| Exemption machinery is at zero and can only shrink | §2.4 | manual: `tooling/arch/exceptions.yaml:1-3`, `Makefile:105-119`, `devdocs/ref/arch-allowlist.yaml` |

## Summary

- Seven ADRs, each with teeth — but "teeth" is a spectrum: AST gates
  (ADR-001, ADR-005), a drift gate (ADR-002), a guarded deletion
  (ADR-004), behavioral defaults plus DSL gates (ADR-006), and a DAG row
  (ADR-007). ADR-003 is a shape across decision paths, inventoried in
  INVARIANTS.md, not a single gate.
- Two fence layers by design: the AST layer cannot be evaded by aliasing
  or renaming; the grep layer covers strings the tree cannot express.
  One violation produced two structurally different verdicts.
- Exemptions are explicit, path-precise, held to a zero baseline by
  `exception-count`, and failed by the gates' own tests when they go
  stale. "Prefer no exemption. A gate that accepts nothing cannot be
  argued with."
- `check-gates-honest` verifies the fences themselves have not been
  weakened — the enforcement is enforced.
- The sidecar rule of ADR-007 is a row in the domain DAG; the runner it
  names has never shipped, and the framework degrades by design.

## Check yourself

1. `envcheck` and the env grep fence enforce the same ADR. Name one
   evasion the AST gate catches that the grep fence would miss, and one
   the grep fence catches that the AST gate cannot.
2. You need a genuine, permanent exception to the env fence in a new
   package. Walk the path: what must you change, and which gate fails
   first if you just add an entry?
3. ADR-003 has no dedicated AST gate. What enforces fail-closed, and how
   does INVARIANTS.md justify marking its rows **high** anyway?
4. A teammate proposes deleting `tooling/arch/cmd/symcheck/main.go`
   because "the grep gates cover it." Which gate fails, and what does
   its failure output look like?
5. Why can the archcheck allowlist file be absent from a fresh clone
   without changing any gate verdict?

*Answers.* 1. Aliased import (`import o "os"; o.Getenv`) — semantic
resolution the grep fence misses if the text is aliased; a string
assembled by concatenation (`"sh" + "im"`) — a binary expression, not a
literal, invisible to the AST layer (`tooling/arch/shimcheck.go:43-45` states the
delegation for the shim pair). 2. Extend the spec first — `exception-count`
holds `exceptions.yaml` to a baseline of zero and fails a net-new entry
(`Makefile:105-119`); the entry must also name an exact path, never a
basename (`tooling/arch/exemption.go:15-23`), and the gates' own tests fail it when it
goes stale. 3. The decision paths' *defaults* (deny on unset, ask on
ambiguity, deny on unresolvable egress), pinned by the fail-closed
inventory and grep fences such as INV-1 (`Makefile:243-250`);
INVARIANTS marks rows high only where a named gate, test, or read code
path backs them (`devdocs/HANDOFF-docs/INVARIANTS.md:9`). 4. `check-gates-honest`: "AST gate
symcheck/main.go is missing from the tree" (`Makefile:426-429`) — and
separately, `lint-arch` would fail because symcheck is missing from its
recipe (`Makefile:430-433`). The grep gates do not cover it: the AST
layer exists precisely for what grep cannot express. 5. A missing
allowlist loads as empty (`tooling/arch/allowlist.go:18-21`), the local file exempts
nothing, and a stale entry — not a missing file — is what the gate
fails on (`tooling/arch/stale_edges_test.go:13`).

## Try it yourself

1. **Break ADR-005 instead.** Add a Go string literal containing
   "compatibility layer" to any file under `execution/session`, run
   `make shimcheck`, read the verdict, revert. Then try the word-boundary
   nuance: a literal containing `api_compat` should *pass*. If it does
   not, you have found a gate bug — capture it.
2. **Read the third domain check.** `lint-arch` enforces
   `-check=context-ports` (`Makefile:19`). Find its implementation
   (`tooling/arch/context_ports_check.go`), state the rule it enforces in
   one sentence, and check it against the consumer-defined-ports decision
   (`devdocs/HANDOFF-docs/DECISIONS.md:135`).

## Where next

The constitution tells you what may *happen* in this tree; it says
nothing about what the tree *offers*. Chapter 3 inventories the surfaces
— CLIs, config schemas, contracts, the telemetry event vocabulary — and
grades them by confidence and stability, including the one surface the
project explicitly refuses to promise: the Go package API. You will
verify five "high"-confidence claims against the tree yourself, which by
now you know is the only kind of claim this book lets stand.

The lab for this chapter lives in
`docs/relurpic-internals/labs/ch02-constitution/` — the command card,
the violation diff, the captured transcript from the pin, and
`verify.sh ch02` to check your machine against the deterministic
outputs.
