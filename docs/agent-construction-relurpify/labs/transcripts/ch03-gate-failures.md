# Chapter 3 — gate-failure transcripts (captured)

Real output from the verification machine, produced in a scratch clone
with the Chapter 2 overlay applied. Every violation below was planted,
captured, and reverted (`git checkout` restores the file; `make lint-arch`
ends green). The violations live only in this transcript.

## Beat 1 — env access in a `_test.go` file: the gate does not look

An `os.Getenv("CRITIC_FALLBACK")` planted in the critic package's
`fake_model_impl_test.go` — envcheck still passes, and its file count does
not move:

```text
[PASS] envcheck: no env access outside userconfig (1087 files scanned)
[PASS] shimcheck: no forbidden language (1087 files scanned)
```

This is deliberate, not a hole: `gatescan.Walk` takes an `IncludeTests`
flag whose comment states the policy — "Gates that must police production
code only leave it false." Test files are exempt from the AST gates.

## Beat 2 — env access in a production file: exact coordinates

The same sin planted in a production file
(`cognitionzoo/react/react_think_node.go`, plus an `"os"` import):

```text
GOPROXY=off GOSUMDB=off go run ./tooling/arch/cmd/envcheck
  [FAIL] env: cognitionzoo/react/react_think_node.go:38 reads the process environment via os.Getenv
[FAIL] envcheck: 1 env access violation(s) outside userconfig
exit status 1
make: *** [Makefile:123: envcheck] Error 1
```

The gate names file and line, then stops the make before the next gate
runs. Fix the cause — here, `git checkout` the file — and the baseline
goes green again.

## Beat 3 — forbidden language in a string literal

shimcheck reads string literals (and identifiers), not comments — a
"compatibility shim" in a comment passes, a literal does not. With

```go
const criticLabAliasNote = "compatibility alias for the old flag name"
```

in the same production file:

```text
  [FAIL] shim: cognitionzoo/react/react_think_node.go:35 carries forbidden language "compatibility" in a string literal
[FAIL] shimcheck: 1 forbidden-language literal(s)
exit status 1
```

## Beat 4 — reverted, green

```text
$ git checkout cognitionzoo/react/react_think_node.go
$ make lint-arch          # all gates PASS, exit 0
```

## The consumer beat (from Chapter 2's research)

The first draft of the critic package was production files with no
importer:

```text
[FAIL] consumer: 1 violation(s)
  consumer: codeburg.org/lexbit/relurpify/cognitionzoo/critic has no importers
```

The restructure to `_test.go`-only files resolves it without touching any
gate or exemption table — see `ch02-critic-package.md`.
