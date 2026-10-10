package thoughtrecipe

import (
	"context"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// ExecuteDelegateCore is the shared, node-free delegate execution core: it runs
// one delegate step exactly as a DelegateNode would — child envelope isolation,
// agent build, governed execution, captures — minus the graph node plumbing. It
// is what NewDelegateNode.Execute does today, exposed so the reflection revise
// body can execute its lowered steps sequentially in-process (D5). Scoped
// registries, permission checks, and Wave-1 grounding apply to body products
// automatically because the step carries its own resolved scope and captures.
func ExecuteDelegateCore(ctx context.Context, deps *paradigm.Deps, step ExecutionStep, env *contextdata.Envelope) (*execution.Result, error) {
	node := NewDelegateNode(step.ID, deps, step)
	return node.Execute(ctx, env)
}
