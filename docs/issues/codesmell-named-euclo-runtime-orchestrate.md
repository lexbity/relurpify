# Codesmells: `named/euclo/runtime/orchestrate`

## Observations

- `controller.go` mixes phase routing, capability selection, artifact merging, recovery dispatch, and observability writes in one execution path. The code is cohesive at the runtime boundary, but the control flow is hard to reason about because the success, recovery, and early-stop branches all mutate shared state.
- `recovery.go` repeats the same stack-recording pattern across several fallback levels. The behavior is consistent, but the duplicated record/return structure makes the branch matrix harder to audit.
- `interactive.go` combines resume handling, machine execution, transition carry-over, and final result shaping. That is practical for a controller layer, but it creates a dense test surface with many state handoffs.

## Tradeoff Note

- The broad controller shape looks intentional because this package sits between `framework/core`, `named/euclo/interaction`, and the capability registry. Splitting the logic more aggressively would likely push coordination complexity into other packages rather than remove it.

---

update:

Euclo Smells

The only explicit smell note in /docs/issues for named/euclo is the orchestrator package: docs/issues/codesmell-named-euclo-runtime-orchestrate.md (line 1).
That note matches the code shape in named/euclo/runtime/orchestrate/controller.go (line 66), where ExecuteProfile combines phase routing, capability selection, artifact merging, recovery, and observability in one long control flow. This is the main maintainability smell.
The recovery layer also has repeated branch/record patterns in named/euclo/runtime/orchestrate/recovery.go (line 94), especially around recovery attempts and fallback handling.
The interactive path in named/euclo/runtime/orchestrate/interactive.go (line 24) is another dense handoff point: resume handling, machine execution, transition carry-over, and result shaping are all co-located.
The package-level tradeoff is intentional, not accidental. The issue note explicitly says the broad controller shape sits between core runtime, interaction, and capability registry concerns, so splitting it too aggressively would likely move complexity instead of removing it.
The Euclo README also says the remaining work includes reducing duplicated helper logic and continuing to sort shared helpers into subpackages: named/euclo/README.md (line 165).
My take:

named/euclo/runtime/orchestrate is the clearest documented smell.
named/euclo/runtime/context, named/euclo/runtime/policy, and named/euclo/relurpicabilities/local look more like maintenance hotspots and under-tested adapter layers than confirmed smells.
I did not find matching explicit /docs/issues notes for those other euclo packages.