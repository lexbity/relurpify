# Chapter 2 — critic package transcripts (captured)

Real output from the verification machine (same environment as
`ch01-baseline.md`). The package under test is `cognitionzoo/critic/`,
applied into a scratch clone by `verify.sh build ch02`.

## 1. First layout, first lesson: the consumer gate

The chapter's first draft of the package was two production files
(`fake_model.go`, `fake_model_test.go`) with no importer anywhere in the
tree. `make lint-arch` in the scratch clone:

```text
[FAIL] consumer: 2 violation(s)
  consumer: codeburg.org/lexbit/relurpify/cognitionzoo/critic has no importers
  consumer: codeburg.org/lexbit/relurpify/devdocs/agent-construction-relurpify/labs/overlay/ch02-fake-model/cognitionzoo/critic has no importers
```

(Two violations because the second draft also kept `.go` files inside the
labs overlay itself — the fix that stuck: the overlay lives under
`labs/testdata/`, which the Go toolchain and the gates ignore. The gate's
own comment states the rule it enforced:
"a package consumed only by tests is still load-bearing infrastructure
here. Packages with zero importers of any kind are dead code."
(`tooling/arch/consumer_check.go`).)

## 2. The layout that holds: a test-only package

With the fake model and its contract tests all in `_test.go` files —
implementation in `fake_model_impl_test.go` (package critic), tests in
`fake_model_contract_test.go` (package critic_test) — the consumer gate
skips the package by design (`OnlyTestGoFiles`), and everything the reader
needs still runs:

```text
$ go build ./...        # no output; exit 0
$ go vet ./cognitionzoo/critic/...   # no output; exit 0
$ go test ./cognitionzoo/critic/... -count=1 -v
=== RUN   TestEnvelopeTour            (ch03 overlay; see ch03 transcript)
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
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.058s
```

## 3. The verifier

```text
$ ./verify.sh build ch02
== tier 1 (build): ch02 ==
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.057s
[PASS] build: ch02 compiles clean and its tests pass
$ ./verify.sh gates ch02
== tier 2 (gates): ch02 ==
... (build, vet, tests, make lint-arch — all PASS)
[PASS] gates: ch02 holds the framework's law
```
