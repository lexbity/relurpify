# Codesmells: `named/euclo/runtime/orchestrate`

## Observations

- `controller.go` mixes phase routing, capability selection, artifact merging, recovery dispatch, and observability writes in one execution path. The code is cohesive at the runtime boundary, but the control flow is hard to reason about because the success, recovery, and early-stop branches all mutate shared state.
- `recovery.go` repeats the same stack-recording pattern across several fallback levels. The behavior is consistent, but the duplicated record/return structure makes the branch matrix harder to audit.
- `interactive.go` combines resume handling, machine execution, transition carry-over, and final result shaping. That is practical for a controller layer, but it creates a dense test surface with many state handoffs.

## Tradeoff Note

- The broad controller shape looks intentional because this package sits between `framework/core`, `named/euclo/interaction`, and the capability registry. Splitting the logic more aggressively would likely push coordination complexity into other packages rather than remove it.
