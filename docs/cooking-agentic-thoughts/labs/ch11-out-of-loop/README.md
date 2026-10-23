# Chapter 11 lab — out of the loop

The final exam: a queued repair, run out-of-loop, captured complete with
its wobble.

## The captured arc (verification machine, gemma4:e4b on CPU)

| Time | Event |
|---|---|
| +0:00 | Boot; Postures banner (governed / deterministic) |
| +0:18 | `/queue <slugify repair>` typed; **mode picker** surfaced — answered `2` (`local-edit-on`) |
| +0:27 | Approval banner; `s` granted |
| +0:27 → +9:53 | The loop runs: reads, a governed write, test runs |
| Result | Diff landed: correct slugify fix (`[^\w\s]` strip + whitespace→hyphen) **and** the module docstring mangled again — `pytest`: `1 error` (SyntaxError) |
| Closing receipt | `euclo.execution.complete` → `outcome: "success"` |

The book's first lesson at out-of-loop scale: complete is a claim; your
tests are the fact. The supervisor's burden is real.

## The final diff (as captured)

```diff
-"""stringkit — small string utilities, kept tiny on purpose.
+stringkit — small string utilities, kept tiny on purpose.
 ...
-    slug = re.sub(r"\s+", "-", slug)
+    # Remove punctuation (keep only alphanumeric and spaces)
+    slug = re.sub(r'[^\w\s]', '', slug)
+    # Replace whitespace with a hyphen
+    slug = re.sub(r'\s+', '-', slug)
```

## Supervision command card

```text
/queue <instruction>       # queue work for sequential execution
/workflows                 # persisted workflow runs
/workflow <id>             # inspect one
/resume <id|latest>        # re-enter a workflow
/rerun <workflow> <step>   # re-execute one step
/cancelwf <workflow>       # stop one
/hitl                      # pending approvals
/export [md|json] [path]   # session transcript
```

## The governance answer table

| Out-of-loop risk (community critique) | Counter operated in this book |
|---|---|
| Same-model review | Governed approvals + audit chain + independent tests (Ch 2, 9) |
| Ticket as attack surface | Deterministic classification, sandbox, schema-blocked calls (Ch 3, 7) |
| Self-improvement poisoning | Machine-drafted, human-installed (Ch 10) |
| Ambiguous tickets | Clarification frames (Ch 2) |
| Local fixes, global mess | `relurplint`, fences, receipts (Ch 3) |

## State at book close

- stringkit: **red by capture** (the queued wobble) — the reader's
  graduation exercise is to take it green with the Chapter 2 pattern.
- Workspace: 20-record audit chain, queued-run telemetry archived, 19
  recipes, 2 edited live prompts, the drafting recipe, and the
  regression suite — an agentic layer that fails loudly.
