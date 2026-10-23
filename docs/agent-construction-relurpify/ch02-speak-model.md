# Chapter 2 — Speak Model

Last chapter ended with a leak. Asked to explain a function, the model
answered in prose that *looked* like a tool call —
`{"thought":…,"action":"file_list","tool":"file_list",…}` — and the runtime
closed the run having done nothing. Four seconds, twenty-nine tokens, no
work.

Here is the builder's translation of that failure: the runtime never reads
the model's mind. It consumes a *structured decision*, and the decision was
not structured. The seam between "what the model said" and "what the agent
can act on" is a Go interface, it has six normative rules about streaming,
and this chapter puts you on the implementing side of it. By the end you
will have written a model — a scripted, deterministic, table-driven fake —
that satisfies the same contract the Ollama transport satisfies, and every
remaining chapter of this book will drive the critic through it.

Why a fake? Because a loop you cannot control is a loop you cannot test.
The book's thesis is that behavior must be pinned before it can be trusted;
a model with a temperature knob is the opposite of pinned. Your fake will
do exactly what its script says: emit these chunks, return this tool call,
fail on command, cancel when told. When a test passes, it will be a fact
about the agent.

## Where you are

Chapter 1 left you with a green baseline (`go build`, `go vet`,
`make lint-arch`), a reset stringkit, and one governed turn's receipts. All
of that stays standing; this chapter adds one new directory to the tree —
and even that only inside a scratch clone, by the end. Nothing here needs a
running model, a sandbox, or stringkit. It is the one chapter you could
write on a train.

## Technical requirements

Nothing new. Go (go directive 1.26.0, toolchain go1.27.2), the Chapter 1
checkout, and the `devdocs/agent-construction-relurpify/labs/` folder from
the repository. No model, no Docker, no Python.

## The contract, read closely

Open `model/ports.go`. The whole provider seam starts at one interface:

```go
// LanguageModel is the primary contract every LLM backend satisfies.
type LanguageModel interface {
	Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error)
	GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error)
	Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error)
	ChatWithTools(ctx context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error)
}
```

Four methods, and the shape of the whole framework falls out of them. The
single-prompt methods (`Generate`, `GenerateStream`) exist for thinking
models driven by a compiled context. The conversation methods (`Chat`,
`ChatWithTools`) exist for the paradigm loop, where a turn history is the
actual interface. The `tools []LLMToolSpec` parameter is what Chapter 1's
leak was about: when the runtime asks a model to decide among tools, it
sends the schemas and expects a structured `ToolCall` back — not prose.

Three types carry the vocabulary, and each has one line worth memorizing:

- **`LLMResponse`** is the decision envelope. `Text` is the content;
  `Thinking` is the reasoning channel — the doc comment is emphatic that a
  turn may spend its whole response there and that the reasoning is
  *"first-class material for the consumer, never waste"*; `FinishReason`
  says why generation stopped; `ToolCalls` carries the structured
  decisions, each an `ID`, a `Name`, and `Args`.
- **`Message`** is one conversation turn: `Role`, `Content`, and — for
  feeding tool results back — `ToolCallID`.
- **`LLMOptions`** is per-invocation configuration: `Model`,
  `Temperature`, `MaxTokens`, `Stop` sequences, `ContextWindow` (the doc
  comment explains that zero means the transport derives it from the
  configured budget, *never* the server's silent default), and
  `StreamCallback` for consumers that want tokens pushed as well as pulled.

### The six rules of streaming

`GenerateStream` returns `<-chan string` — a channel, not a callback — and
the interface's doc comment spells out six normative rules. Normative means
every implementation must satisfy them and every consumer may rely on them;
when your fake is written, your critic will be one of those consumers.
Condensed to their spine:

1. **R1 — The call-time context is the only stop signal.** An
   implementation must not need the consumer's cooperation to stop;
   abandoning a stream means cancelling a context.
2. **R2 — On cancellation, internal goroutines exit and the channel
   closes within 250 ms.**
3. **R3 — The channel is closed exactly once, by its producer.**
4. **R4 — Resources the pump owns (HTTP bodies, files) are closed by that
   pump, exactly once, including on cancellation.**
5. **R5 — Producer-side encode/write failures degrade** — drop, count,
   emit telemetry — **and must not panic.**
6. **R6 — Send contention resolves by selecting on `ctx.Done()`**; the
   token is dropped and counted when the context is done.

Read R1 and R6 together and you can see the design taste of this
codebase: no consumer-side stopping protocol, no blocking producers, no
silent deadlock. Every failure path degrades into something countable. You
will enforce these rules in tests before this chapter ends.

### The seam around the port

`model/runtime.go` adds the vocabulary the app uses to *hold* a model:

- **`ModelBackend`** wraps a `LanguageModel` with lifecycle (`Close`) and
  debug controls. Execution and workspace-session code see the backend,
  not the provider.
- **`EventSink`** is the emit port for model-level events, and its doc
  comment is a layering lesson in itself: *"Deliberately not
  telemetry.Telemetry: model must not gain a DAG edge to the telemetry
  domain — providers emit plain events and the composition root adapts."*
  The model domain is a leaf; it does not know the telemetry package
  exists. Chapter 3 makes that kind of statement mechanically enforced.
- **`ModelFactory`** and **`ModelProduct`** are how the composition root
  hands a workspace a model wrapped in app-level instrumentation.

And one more interface earns its place on the wall,
`ProfiledModel` (`ports.go:80`):

```go
type ProfiledModel interface {
	LanguageModel
	ToolRepairStrategy() string
	MaxToolsPerCall() int
	UsesNativeToolCalling() bool
}
```

There is Chapter 1's leak, named in code. A model *profile* declares how
tool decisions arrive — `UsesNativeToolCalling` is the `intent: native`
path where the model's template speaks tool calls natively; `prompt_based`
asks for a structured decision in prose and parses it. `ToolRepairStrategy`
returns `"heuristic-only"` or `"llm"` — whether a malformed decision gets
repaired locally or by re-asking the model.

### Decision repair, from the implementer's seat

That repair loop is real code, and you read part of it in Chapter 1's
telemetry. When the react paradigm's think node gets a response, it
normalizes the payload (`cognitionzoo/react/react_think_node.go`,
`normalizeDecision`): thinking-channel or content, structured tool calls
or prose, `finish_reason` consulted. If the decision is unparseable,
`repairDecision` re-prompts the model — with the JSON schema and the
original raw response appended, so the model can see its own mistake —
bounded by the profile's repair strategy. The lesson for you is not the
regexes; it is that **the parseable decision is a contract**. Your critic
will produce decisions too, and it will be driven by a model that always
produces parseable ones — because you will write that model yourself, in
about a hundred lines.

## Hands-on

### 1. The scratch-clone convention

From here on, the book's Go code lives in one self-contained sub-package,
`cognitionzoo/critic/` — the critic and everything that supports it. You
never edit the repository's existing files to make it work (one exception,
Chapter 9's registration, which is the point of that chapter), and your
working tree stays clean: `verify.sh` applies the chapter overlay to a
scratch clone and runs there.

The overlay for this chapter is
`devdocs/agent-construction-relurpify/labs/testdata/overlay/ch02-fake-model/`,
containing two files at their true repository paths:

```text
cognitionzoo/critic/fake_model_impl_test.go       # the fake (package critic)
cognitionzoo/critic/fake_model_contract_test.go   # the contract tests (package critic_test)
```

Notice the file names: both are `_test.go` files. Hold that thought — the
gate will explain it before this chapter ends.

Run the tier-1 verifier; it clones, applies the overlay, builds, vets, and
tests:

```bash
./verify.sh build ch02
```

```text
== tier 1 (build): ch02 ==
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.057s
[PASS] build: ch02 compiles clean and its tests pass
```

(The labs directory lives inside the repository checkout; `verify.sh` is
at `devdocs/agent-construction-relurpify/labs/verify.sh`.)

### 2. Read the fake before you trust it

The fake is small enough to read whole, and you should — every later
chapter stands on it. The center of the design is one scripted turn:

```go
type FakeTurn struct {
	Chunks []string // streamed in order by GenerateStream
	Text   string   // non-streaming text; empty means "join Chunks"
	Thinking string
	FinishReason string // defaults to "stop"
	ToolCalls []model.ToolCall
}
```

and the model that replays turns in order, cycling back to the first when
it runs out — so a bounded loop can be driven turn by turn:

```go
type FakeModel struct { /* turns, next, err, pace, counters */ }
var _ model.LanguageModel = (*FakeModel)(nil) // compile-time proof
```

That one line — `var _ model.LanguageModel = (*FakeModel)(nil)` — is the
contract test before any test runs: if the fake drifts from the interface,
the package does not compile.

The streaming half is where the six rules live. The pump is the only code
that touches the channel, and it closes it exactly once (R3), on every
path:

```go
func (f *FakeModel) pump(ctx context.Context, turn FakeTurn, out chan string) {
	defer close(out)
	for _, chunk := range turn.Chunks {
		if f.paceOf() > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(f.paceOf()):
			}
		}
		select {
		case out <- chunk:
		case <-ctx.Done():
			f.noteDrop() // R6: dropped and counted
			return
		}
	}
}
```

Two controls make tests deterministic: `SetPace(d)` inserts a pause before
each chunk (so you can cancel mid-flight), and `SetError(err)` makes every
method fail on command (so you can test degradation). Counters — `Calls`,
`LastInput`, `DroppedTokens` — are what later chapters assert against.

### 3. The contract tests enforce the rules that matter

The external test file (`package critic_test`) drives the fake through its
public surface — deliberately, because that is what every consumer gets.
Nine tests, and two of them *are* the streaming contract:

The R2 test cancels mid-stream and holds the implementation to the 250 ms
budget:

```go
<-ch // first chunk arrived; abandon the stream
cancel()

// R2: the channel closes within 250 ms of cancellation.
closed := make(chan struct{})
go func() {
	for range ch {
	}
	close(closed)
}()
select {
case <-closed:
case <-time.After(250 * time.Millisecond):
	t.Fatal("channel did not close within 250 ms of cancellation")
}
```

The R6 test starts a stream nobody reads; the producer blocks on its first
send until the context ends, then drops and counts:

```go
time.Sleep(30 * time.Millisecond)
cancel()
for range ch {
}

if got := fm.DroppedTokens(); got != 1 {
	t.Fatalf("DroppedTokens() = %d, want 1", got)
}
```

The rest: scripted chunks arrive in order; `Generate` joins them into
`Text` and honors `FinishReason`; `ChatWithTools` returns the scripted
`ToolCall` untouched (the fake never invents decisions); turns cycle in
order so a loop can be driven; error injection fails all four methods with
the same error; an empty script fails cleanly rather than panicking.

Run it:

```bash
go test ./cognitionzoo/critic/... -count=1 -v
```

```text
=== RUN   TestFakeModelIsALanguageModel
--- PASS: TestFakeModelIsALanguageModel (0.00s)
=== RUN   TestGenerateStreamEmitsScriptedChunksInOrder
--- PASS: TestGenerateStreamEmitsScriptedChunksInOrder (0.00s)
=== RUN   TestGenerateReturnsJoinedText
--- PASS: TestGenerateReturnsJoinedText (0.00s)
=== RUN   TestChatWithToolsReturnsScriptedToolCalls
--- PASS: TestChatWithToolsReturnsScriptedToolCalls (0.00s)
=== RUN   TestTurnsCycleInOrder
--- PASS: TestTurnsCycleInOrder (0.00s)
=== RUN   TestGenerateStreamClosesOnCancellation
--- PASS: TestGenerateStreamClosesOnCancellation (0.03s)
=== RUN   TestGenerateStreamDropsAndCountsOnSendContention
--- PASS: TestGenerateStreamDropsAndCountsOnSendContention (0.03s)
=== RUN   TestErrorInjectionFailsEveryMethod
--- PASS: TestErrorInjectionFailsEveryMethod (0.00s)
=== RUN   TestEmptyScriptFailsCleanly
--- PASS: TestEmptyScriptFailsCleanly (0.00s)
PASS
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.057s
```

Fifty-seven milliseconds, no network, no model, no sandbox — and the two
hardest rules of the streaming contract proven, not assumed.

### 4. The gate has opinions about your package

The first draft of this chapter's package did not survive the gates, and
the autopsy is worth ten pages of theory. The draft put the fake in
`fake_model.go` — a production file — in a package nothing imported yet.
`make lint-arch`:

```text
[FAIL] consumer: 2 violation(s)
  consumer: codeburg.org/lexbit/relurpify/cognitionzoo/critic has no importers
```

(The second violation came from leaving `.go` files inside the labs folder
itself; the overlay now lives under `testdata/`, which the toolchain —
and the gates — ignore.)

Read the gate's own comment
(`tooling/arch/consumer_check.go`):

> a package consumed only by tests is still load-bearing infrastructure
> here. Packages with zero importers of any kind are dead code.

That is not bureaucracy; that is the same no-dead-code taste that makes
the charter ban stubs. An unimported package is a stub the linker would
skip. The fix is not an exemption — you never add one of those — it is
putting the code where it belongs: until something consumes the critic
package in production (Chapter 6's runner command becomes its first
importer), every file it contains is a `_test.go` file. The gate skips
test-only packages by design, and nothing honest is lost: test helpers
*are* test code in Go.

Chapter 3 turns this from one instructive failure into the full law of the
tree — including why the AST gates did not even *look* at your test file
when they scanned 1,087 others.

## Gotchas

- **`var _ model.LanguageModel = (*FakeModel)(nil)` is load-bearing.**
  Delete it and the fake can drift from the contract silently; the tests
  would keep passing against a narrower interface than consumers use.
- **`SetPace` exists for cancellation tests only.** With pace zero the
  pump finishes before your test can cancel anything; if your cancel test
  flakes, you forgot the pace, not the contract.
- **The fake cycles its script.** Three calls against a two-turn script
  replay turn one. Assert `Calls()` when order matters; do not assume the
  script is consumed once.
- **`Thinking` is not `Text`.** A scripted turn that puts everything in
  `Thinking` and nothing in `Chunks` is a *legal* model response — the
  contract says so — and your consumer code must decide what to do with
  it. Chapter 5's nodes will have opinions.
- **Test files never reach production.** If you later find yourself
  wanting `FakeModel` in non-test code, that is the Chapter 6 conversation
  about the runner — not a reason to rename the file.

## Summary

- `model.LanguageModel` is four methods and six normative streaming rules;
  every provider, wrapper, and your fake satisfy it, and every consumer —
  including the critic you are building — may rely on the rules without
  checking.
- The decision is data: `LLMResponse.ToolCalls` for structured tool
  decisions, `Thinking` as first-class reasoning-channel output,
  `FinishReason` for the why. Chapter 1's leak was a decision that never
  became data; `ProfiledModel.ToolRepairStrategy` names how the framework
  repairs one that arrives malformed.
- `EventSink` is deliberately *not* the telemetry interface: the model
  domain is a DAG leaf, and even its event vocabulary is chosen to keep
  that edge from existing.
- The fake model is a hundred lines, deterministic, and testable in 57 ms;
  from Chapter 4 on, every graph execution in this book runs on it.
- The consumer gate rejected the first package layout — production code
  with no importer — and the fix was to write test code in test files.
  The gates teach; your job is to read what they say.

## Check yourself

1. Why does `GenerateStream` return a channel rather than taking a
   callback, and which of the six rules would be hardest to keep if it
   took one?
2. Your scripted turn has `Thinking: "…"` and no `Chunks`. What does
   `Generate` return as `Text`, and what does that tell you about the
   difference between the reasoning channel and content?
3. What does `ProfiledModel.ToolRepairStrategy()` control, and where did
   you already watch a decision fail to parse?
4. The consumer gate failed the first package layout. Quote the rule it
   enforces and explain why the fix was moving code into `_test.go` files
   rather than adding an exemption.
5. `EventSink` is not `telemetry.Telemetry`. What edge is that choice
   protecting, and which chapter's subject makes that protection
   mechanical?

*Answers.* 1. Ownership: the producer owns the channel and its closing;
with a callback, cancellation semantics would need a consumer protocol,
violating R1 — R3's exactly-once close would be the hardest to keep. 2.
Empty string — `Text` is empty when `Chunks` is empty; the reasoning went
to the thinking channel, which is first-class material a consumer may
render, log, or spend, never waste. 3. Whether a malformed decision is
repaired heuristically or by re-prompting the model with the schema and
the raw response; the Chapter 1 leak, where the decision arrived as prose
and the run closed without executing anything. 4. A package with zero
importers is dead code, and test-only importers count as consumption; test
helpers belong in `_test.go` files, so the package was legitimately
test-only until a production importer exists — no exemption needed, and
exemptions are load-bearing anyway. 5. The `model` → `telemetry` import
edge in the domain DAG; Chapter 3, where `domaincheck` enforces import
direction mechanically.

## Try it yourself

1. **Slow first token.** Write a test (in `fake_model_contract_test.go`)
   with a two-chunk turn at `SetPace(20ms)` whose consumer renders a
   "typing…" marker before the first chunk and asserts it appears exactly
   once. You are practicing the consumer side of R1: cooperate with
   cancellation, never with stopping.
2. **Degradation, witnessed.** `SetError` on the fake, then write a
   consumer that drains `GenerateStream` and counts *nothing*: assert that
   the error surfaces from the call, the channel is nil-or-closed, and no
   panic escapes. You have just written the shape of R5's consumer.

## Where next

You can now speak model — and you own the only model in the book that
always says what it means. Next chapter the lens pulls back: the whole
tree as law. The domain DAG, the AST gates that read 1,087 files in three
seconds, the charter's four bans, and the envelope that threads every
graph — ending with a deliberate violation, planted in production code,
caught by exact file and line, and reverted while the gate watches. Bring
your new favorite failure: the consumer gate is only the first gate with
opinions.

The lab lives at
`devdocs/agent-construction-relurpify/labs/` — `verify.sh build ch02`
re-runs this chapter's checks from nothing, and
`transcripts/ch02-critic-package.md` holds the captured output, including
the consumer-gate failure this chapter walked through.
