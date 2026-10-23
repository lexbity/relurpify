# Chapter 5 — Nodes That Govern Themselves

Last chapter you built a machine that runs. This chapter you build one
that can be *trusted* — because its nodes carry their own government with
them. In most frameworks, safety is a wrapper: a middleware that inspects
your component from the outside and hopes it understands what it sees.
Relurpify types governance into the node itself. A node does not merely
*do* something; it *declares* what it will do, how often it may be done,
where it prefers to run, and what recovery owes it — and the graph
validates those declarations before a single node executes.

Then you use it. The chapter's build is the reason you came: **the critic
pipeline as a hand-composed graph** — drafter → critic → gate → human
approval → conditional revise → re-review — with a required checkpoint
policy on the revision and a hard wall between the drafter's model and the
critic's.

## Where you are

Sixteen tests green in `cognitionzoo/critic/`; the graph API — build,
seal, execute, preflight — is yours. This chapter adds one overlay file,
`critic_graph_test.go`, and takes you into
`execution/agentgraph/contracts.go`, the file where the framework says
what a node is allowed to be.

## The NodeContract, field by field

`NodeContract` is a plain struct (`execution/agentgraph/contracts.go:69`),
and every node that implements one extra method carries it:

```go
// ContractNode extends Node with an explicit execution contract.
type ContractNode interface {
	Node
	Contract() NodeContract
}
```

The struct has **nine fields** — five class dimensions and four
capability/risk fields. Here is the whole card, with the exact vocabulary
each field accepts:

| Field | Values (closed set) | What it declares |
|---|---|---|
| `SideEffectClass` | `none`, `context`, `local`, `external`, `human` | The replay sensitivity of execution: touches nothing; writes context; mutates local state; reaches outside; or needs a human |
| `Idempotency` | `unknown`, `replay-safe`, `single-shot` | Whether re-running the node is expected to be safe |
| `PreferredPlacement` | `any`, `local`, `remote`, `sticky-session` | Preflight's tie-breaker when choosing among matching capabilities |
| `CheckpointPolicy` | `none`, `preferred`, `required` | Whether persisted recovery is optional or required |
| `Recoverability` | `none`, `in-process`, `persisted` | The recovery guarantee the node expects |
| `RequiredCapabilities` | capability selectors | What the node refuses to run without — preflight blocks on a miss |
| `MaxRiskClass` | risk classes (read-only … destructive) | Preflight refuses capabilities that classify above the ceiling |
| `RequiredTrustClass` | trust classes | Preflight refuses capabilities below the floor |
| `ContextPolicy` | a `StateBoundaryPolicy` | The node's declared read/write boundary over the envelope: key patterns, memory classes, entry-size and collection caps |

One struct comment deserves quoting in full, because it is the honesty
policy for the last three rows:

> Placement, recoverability, and checkpoint policy are contract metadata,
> not runtime persistence ownership.

The classes are declarations the framework validates and preflight
consumes — they are not magic. `CheckpointPolicyRequired` does not make
persistence happen; it makes the node *say* that persistence is required,
and the composition wiring that can honor it is the runtime composition
you will meet in Part III. Saying so is not a weakness. A framework that
pretends declarations are enforcement is how wrappers drift from truth.

### Defaults are typed too

If a node does not implement `ContractNode`, the graph resolves a
**type-driven default** — governance is not optional for the unwashed:

| Node type | Default contract |
|---|---|
| `HumanNode` | `SideEffectHuman`, `single-shot` |
| Terminal, Conditional, Observation, System | `SideEffectNone`, `replay-safe` |
| Stream | `SideEffectContext`, `replay-safe` |
| Tool | `SideEffectExternal`, `unknown` |

And for tool nodes, the contract is not just defaulted — it is
**classified from what the tool declares**. `toolNodeContract` reads the
tool's capability descriptor and maps its effect classes onto the
side-effect and idempotency dimensions: a tool that declares filesystem
mutation or network egress classifies `external`/`single-shot`; a tool
that declares nothing classifies `none`/`replay-safe` — trusted to have
no effects *because it declared none*. Your Chapter 4 read-only stringkit
tool resolves exactly that way, and the lab asserts it, capability pinned
and all.

### Validation refuses garbage

`Graph.Validate` runs every node's contract through
`validateNodeContract`, which checks each closed set (a typo in a class
spelling is a build-time refusal with the node's ID attached), validates
the `ContextPolicy`, and enforces one rule with teeth: **a tool node that
declares no required capabilities is rejected** — because a tool node
without a declared need is a tool node nobody can preflight. The lab
plants both sins and captures the refusals.

## Control flow: decisions and pauses

Two shipped node shapes cover the control flow the critic pipeline needs.

**Decisions.** `ConditionalNode` computes the next branch dynamically:
its `Execute` evaluates a predicate over the envelope and stores the
decision in its result (`{"next": …}`). Routing still belongs to the
graph — the edges out of a decision node carry `ConditionFunc`s that test
the decision. Decisions route; edges choose; nothing executes on a
predicate's word alone.

**Pauses.** `HumanNode` is a HITL pause *inside* a graph — and the text
owes you the plain truth about its mechanism: it is a **synchronous
callback**. `Execute` invokes the node's callback with the envelope and
blocks until the callback returns; there is no suspend-the-graph and
resume-tomorrow machinery behind it. Its contract is `SideEffectHuman` /
`single-shot` — a human decision may not be replayed on the graph's
authority. One more honest fact, discovered while building this chapter's
lab: as of this tree, `ConditionalNode` and `HumanNode` carry unexported
`id` fields and ship no constructors, so they cannot be validly
constructed outside the `agentgraph` package.
`[NEEDS CONFIRMATION: constructors for ConditionalNode/HumanNode at
BETA-RC1-FALL2026]` The lab therefore does what a builder extending the
framework would do: it mirrors `HumanNode`'s contract on a
hand-implemented approval node — same classes, same synchronous
decision — and reads HumanNode's default contract through its zero value
to prove the mirror is faithful.

**Checkpoints.** A node may *request* a checkpoint on the envelope —
`env.RequestCheckpoint(reason, priority, evictMemory)` — and the shipped
ReWOO checkpoint node does exactly that (`"rewoo:<phase>"`). The request
is the node's; materialization belongs to the persistence wiring. The
revise node in this chapter's build follows the same shipped pattern.

## Hands-on: the critic pipeline as a graph

The overlay adds `critic_graph_test.go`. The graph:

```text
drafter → critic → gate ──(revise)──→ approve ──(approved)──→ revise ──→ critic
                     └─(finish)──→ done         └─(denied)────→ done
```

Read it as the thesis, drawn. Three model nodes, and they are **not the
same model** — each holds its own `FakeModel` with its own script and its
own prompt, so the critique is structurally incapable of seeing the
drafter's reasoning, and the revision is structurally incapable of
grading itself:

- **`drafter`** — Chapter 4's node, unchanged: streams a draft from its
  model, writes `critic.draft` with `OriginLLM`.
- **`critic`** — the independent critique phase. Reads the draft, streams
  a verdict (`issues` or `clean`) from *its* model, writes
  `critic.verdict`. Contract: `SideEffectNone`, `replay-safe` — a critic
  that mutates nothing and can be replayed.
- **`gate`** — a system node with decision semantics: verdict is
  `issues` and revisions are under the bound → `revise`; otherwise
  `finish`. The edges out of the gate route on `next`.
- **`approve`** — the human pause. Mirrors `HumanNode`'s contract
  (`SideEffectHuman`, `single-shot`); its synchronous decision callback
  records the decision in the envelope with `OriginUser` — in a unit test
  the *decision* is scripted, because there is no human in a unit test;
  the *pause* is real control flow.
- **`revise`** — the revision, under a required checkpoint policy:
  contract `SideEffectContext` / `replay-safe` /
  `CheckpointPolicyRequired` / `persisted`; increments the revision
  counter, requests a checkpoint on the envelope, and streams a revised
  draft from *its* model. Then the edge back to `critic` closes the loop.

The loop is bounded twice: the scripted critique eventually says `clean`,
and the graph's visit cap stands behind that as backstop. Two scenarios
run:

**Approved.** First critique says `issues`, the human approves, one
revision runs, the re-review says `clean`, the run ends at the terminal —
with every claim asserted against the envelope and the call counters:

```text
final verdict:   clean          (critic model calls: 2)
revisions:       1              (revise model calls: 1)
final draft:     a better draft
approval:        approved       (OriginUser)
checkpoint:      requested by the revise node
```

**Denied.** Same graph, the pause says no: the revise model is never
called, the revision counter never moves, and the run ends honestly at
the terminal. A denied revision leaves no trace of having almost
happened.

Two more tests round out the chapter:

- **`TestContractDefaultsAreTyped`** — `ResolveNodeContract` on shipped
  values: terminal is `none`/`replay-safe`; `HumanNode` is
  `human`/`single-shot`; your stringkit tool classifies `none`/
  `replay-safe` *because it declared no effect classes*, with
  `tool:read_stringkit` pinned as its required capability.
- **`TestContractValidationRefusesGarbage`** — a node declaring
  `SideEffectClass("wing-it")` is refused with
  `invalid side effect class "wing-it"`; a tool-typed node that declares
  no capabilities is refused with `declares no required capabilities`.
  The contract's vocabulary is closed, and validation holds the door.

```bash
./verify.sh build ch05
```

```text
== tier 1 (build): ch05 ==
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.059s
[PASS] build: ch05 compiles clean and its tests pass
```

Twenty tests green in the package — and `Part II`'s promise is one
chapter from kept: the critic pipeline exists as a working, governed,
hand-composed graph. What it does not yet have is a command to run it or
a grammar to address it. That is the gap between here and
`agent fixer uses critic`, and it is exactly Part III's subject.

## Gotchas

- **A contract is a declaration, not a force field.** `CheckpointPolicy`
  and `Recoverability` are metadata the framework validates and consumes —
  persistence itself is wired at composition. Claim less than you enforce
  and more than you mean; the struct comment says which is which.
- **Contract classes are closed sets.** A misspelled class is not a
  fallback to default — it is a validation error at graph build. This is
  the AST-gate taste applied to your own types.
- **The human pause is synchronous.** Do not build long-running approval
  flows inside a `HumanNode` callback in production; the pause blocks the
  graph's run loop, and the honest long-form answer (persisted
  interaction frames) lives in Euclo's interaction layer, not in this
  node.
- **`ConditionalNode`/`HumanNode` cannot be constructed outside
  `agentgraph` at this tree** (unexported `id`, no constructors). Mirror
  their contracts on your own nodes, as the lab does — and read their
  defaults through `ResolveNodeContract` to keep the mirror honest.
- **The revise loop needs two bounds.** A scripted-or-real critique that
  never says `clean` plus a missing visit cap is an infinite loop with
  good manners. Bound the revisions in the envelope *and* keep the
  graph's cap as the backstop.

## Summary

- `NodeContract` is nine fields: five closed-set class dimensions
  (side effect, idempotency, placement, checkpoint policy,
  recoverability) plus required capabilities, a risk ceiling, a trust
  floor, and a declared context boundary. Governance is typed into the
  node, validated at graph build, and consumed by preflight — not wrapped
  around it.
- Defaults are typed too: every node resolves a contract, and tool nodes'
  contracts are *classified from what the tool declares* — a tool that
  declares no effects is trusted to have none, and its required
  capability is pinned regardless.
- `ConditionalNode` decides (and edges route on the decision);
  `HumanNode` pauses via a synchronous callback with a
  `human`/`single-shot` contract; checkpoints are requested by nodes on
  the envelope and materialized by persistence wiring.
- The critic pipeline now exists as a governed, hand-composed graph:
  three model phases that cannot see each other's reasoning, a bounded
  conditional revise loop, a human pause whose denial leaves no trace,
  and a required checkpoint policy on the revision.

## Check yourself

1. Name the five class dimensions of `NodeContract` and give the exact
   value your revise node declares for each — and the two extra fields it
   sets.
2. Your tool's descriptor declares filesystem mutation. What does
   `toolNodeContract` classify it as, and what would preflight do if your
   node contract set `MaxRiskClass` below that classification?
3. Why does `validateNodeContract` refuse a tool node that declares no
   required capabilities?
4. The human pause denied the revision. What in the envelope proves the
   revision never ran — and what does the pipeline's structure prove
   about the critic never seeing the drafter's reasoning?
5. `CheckpointPolicyRequired` is on the revise node's contract, yet
   nothing persisted in the lab. Which sentence from `contracts.go`
   explains that, and what arrives later to honor it?

*Answers.* 1. SideEffect (`context`), Idempotency (`replay-safe`),
PreferredPlacement (unset — `any`), CheckpointPolicy (`required`),
Recoverability (`persisted`); plus the revise node sets nothing else —
the critic and drafter declare `none`/`replay-safe`. (If you said
`external`, reread: a revision writes context, not the world.) 2.
`external`/`single-shot` — filesystem mutation is an external effect and
a single-shot risk; preflight would refuse every matching capability as
`capability_constraints_unsatisfied`, blocking the graph. 3. Because a
tool node without declared needs cannot be preflighted — the gate exists
to check declared needs, and an undeclared need is an uncheckable one.
4. The revision counter is absent and the revise model's call count is
zero; structurally, the critic holds a separate model instance with its
own script, so the drafter's reasoning never entered the critic's
context. 5. "Placement, recoverability, and checkpoint policy are
contract metadata, not runtime persistence ownership." The runtime
composition (Part III) wires the persistence that materializes the
node's checkpoint request.

## Try it yourself

1. **Cap the gate, feel the backstop.** Change the gate's revision bound
   from 2 to 99 and script a critic that always says `issues`. Predict
   what stops the run (the visit cap), then run it and read the error.
   You have just met the second bound you will always keep.
2. **A stricter critic.** Give `criticNode` a contract with
   `RequiredTrustClass` set to the workspace-trust class and re-run the
   preflight against an empty registry. Read the
   `capability_constraints_unsatisfied` issue. Trust floors are for
   capability-backed nodes — decide for yourself whether a model-only
   critic should carry one, and write down your argument; it returns in
   Chapter 7 as a contract-design question.

## Where next

Part II closes next chapter with the graph at full strength: parallel
branch subgraphs and their envelope merge — immutable snapshots, deltas
merged in declaration order — plus `RetrievalNode`, `StreamTriggerNode`,
and the grounder. The critic pipeline grows an ensemble flavor: N
independent critiques, merged. Then a small runner command executes it
offline end to end — the first production importer your package has ever
had, and the moment the consumer gate has been waiting for since
Chapter 2.

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch05` re-runs everything this chapter proved, and
`transcripts/ch05-critic-graph.md` holds the captured run and the honest
notes on `ConditionalNode`/`HumanNode` constructors.
