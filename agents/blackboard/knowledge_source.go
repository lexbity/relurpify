package blackboard

import (
	"context"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// KnowledgeSource is the interface every specialist must satisfy.
// A KS reads from the blackboard, performs focused work, and writes results
// back. The controller invokes CanActivate each cycle to determine eligibility
// before calling Execute.
type KnowledgeSource interface {
	// Name returns a stable identifier used for logging and priority ties.
	Name() string
	// CanActivate returns true when this KS has something to contribute in the
	// current blackboard state.
	CanActivate(bb *Blackboard) bool
	// Execute reads from bb, does work, and writes results back.
	Execute(ctx context.Context, bb *Blackboard, tools *capability.Registry, model core.LanguageModel) error
	// Priority breaks ties when multiple KS can activate. Higher wins.
	Priority() int
}
