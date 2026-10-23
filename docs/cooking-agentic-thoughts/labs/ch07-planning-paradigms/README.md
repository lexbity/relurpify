# Chapter 7 lab — the planning paradigms

Three recipes covering planner, rewoo, and blackboard, plus the captured
governance catch.

## Files

- `plan_repair.erpe` — planner: plan → verify → summarize, no steps (the
  plan document is the product; modeled on the shipped `plan.erpe`).
- `rewoo_outline.erpe` — rewoo: required `plan` + `synthesize`, no steps.
- `board_survey.erpe` — blackboard: two authored sources (`read [state.x]`
  lanes, mandatory `write`), no `do` legs — the data-flow skeleton form.

## Reproduce

```bash
relurplint --check recipes --workspace .   # 0 diagnostics (first pass)
# restart relurpish, pin, dispatch:
#   /recipe euclo.thoughtrecipe.plan_repair
```

## Captured: governance catches a malformed tool call

The pinned plan_repair dispatch on the verification machine:

```json
{"decided_by": "pinned", "node.id": "euclo.dispatch"}
{"message": "node planner_execute execution failed: tool file_read blocked: input schema invalid: $.path required"}
{"outcome": "failure", "llm_calls": "0",
 "outcome_kind": "execution failed: node planner_execute execution failed: tool file_read blocked: input schema invalid: $.path required"}
```

The planning model omitted `file_read`'s required `path` argument; the
tool-manifest schema validation blocked the call at execution, named the
JSON path, and the default step-error policy aborted the route. Fix:
`/retry`, or a model that emits complete arguments.

## Flagged, not explained

`llm_calls` reads `0` for planner and chainer runs while react turns
counted (Chapter 2: five). Pattern suggests the counter is react-bound as
of the pin.
`[NEEDS CONFIRMATION: llm_calls counting for non-react paradigms at your pin]`

## Contract pointers

- planner/rewoo/htn/blackboard: `cognitionzoo/<paradigm>/contract.go`
- htn composes react; blackboard authored sources:
  `cognitionzoo/blackboard/authored_source.go` ("authored structure is
  authoritative"; no model calls beyond a source's own execution)
- `do` literal-input limitation: recipe DSL manual §8.8
