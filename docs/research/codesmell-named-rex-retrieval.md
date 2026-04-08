# Codesmell: `named/rex/retrieval`

Observed while expanding test coverage in the retrieval path.

## Notes

- `hydrateWorkflowRetrieval` mixes query construction, retrieval execution, fallback knowledge loading, and response serialization in one branch-heavy function.
- `taskPaths` duplicates path discovery across metadata keys and task context keys, which makes the effective input surface harder to reason about.
- `contentBlockResults` and `parseCitations` are small helpers, but they sit inside a larger orchestration file that combines policy selection, workflow expansion, and payload shaping.

## Risk

- Maintainability risk: the retrieval path is doing several jobs at once, so future changes to workflow widening or fallback behavior may be hard to isolate.
- Testability risk: fallback behavior depends on both the retrieval service and the knowledge-lister interface, which creates multiple execution paths that need coordinated fixtures.

## Tradeoff

- This shape appears intentional because rex needs a single workflow-aware retrieval adapter that can widen from local paths into workflow history when the route policy demands it.

