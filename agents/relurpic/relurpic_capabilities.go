package relurpic

import (
	"database/sql"

	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type relurpicOptions struct {
	IndexManager *ast.IndexManager
	GraphDB      *graphdb.Engine
	PatternStore patterns.PatternStore
	CommentStore patterns.CommentStore
	RetrievalDB  *sql.DB
	PlanStore    frameworkplan.PlanStore
	Guidance     *guidance.GuidanceBroker
}

type RelurpicOption func(*relurpicOptions)

func WithPatternStore(store patterns.PatternStore) RelurpicOption {
	return func(opts *relurpicOptions) {
		opts.PatternStore = store
	}
}

func WithCommentStore(store patterns.CommentStore) RelurpicOption {
	return func(opts *relurpicOptions) {
		opts.CommentStore = store
	}
}

func WithIndexManager(manager *ast.IndexManager) RelurpicOption {
	return func(opts *relurpicOptions) {
		opts.IndexManager = manager
	}
}

func WithGraphDB(engine *graphdb.Engine) RelurpicOption {
	return func(opts *relurpicOptions) {
		opts.GraphDB = engine
	}
}

func WithPlanStore(store frameworkplan.PlanStore) RelurpicOption {
	return func(opts *relurpicOptions) {
		opts.PlanStore = store
	}
}

func WithRetrievalDB(db *sql.DB) RelurpicOption {
	return func(opts *relurpicOptions) {
		opts.RetrievalDB = db
	}
}

func WithGuidanceBroker(broker *guidance.GuidanceBroker) RelurpicOption {
	return func(opts *relurpicOptions) {
		opts.Guidance = broker
	}
}

func applyRelurpicOptions(base relurpicOptions, opts []RelurpicOption) relurpicOptions {
	resolved := base
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&resolved)
	}
	return resolved
}

// RegisterBuiltinRelurpicCapabilities installs framework-native orchestrated
// capabilities that are reusable across agents without treating them as local tools.
func RegisterBuiltinRelurpicCapabilities(registry *capability.Registry, model core.LanguageModel, cfg *core.Config, opts ...RelurpicOption) error {
	if registry == nil || model == nil {
		return nil
	}
	resolved := applyRelurpicOptions(relurpicOptions{}, opts)

	handlers := []core.InvocableCapabilityHandler{
		plannerPlanCapabilityHandler{model: model, registry: registry, config: cfg},
		architectExecuteCapabilityHandler{model: model, registry: registry, config: cfg},
		reviewerReviewCapabilityHandler{model: model, config: cfg},
		verifierVerifyCapabilityHandler{model: model, config: cfg},
		executorInvokeCapabilityHandler{registry: registry},
		patternDetectorDetectCapabilityHandler{
			model:        model,
			config:       cfg,
			indexManager: resolved.IndexManager,
			graphDB:      resolved.GraphDB,
			patternStore: resolved.PatternStore,
			retrievalDB:  resolved.RetrievalDB,
		},
		gapDetectorDetectCapabilityHandler{
			model:        model,
			config:       cfg,
			indexManager: resolved.IndexManager,
			graphDB:      resolved.GraphDB,
			retrievalDB:  resolved.RetrievalDB,
			planStore:    resolved.PlanStore,
			guidance:     resolved.Guidance,
		},
		prospectiveMatcherMatchCapabilityHandler{
			model:        model,
			config:       cfg,
			patternStore: resolved.PatternStore,
			retrievalDB:  resolved.RetrievalDB,
		},
		commenterAnnotateCapabilityHandler{
			commentStore: resolved.CommentStore,
			patternStore: resolved.PatternStore,
			retrievalDB:  resolved.RetrievalDB,
		},
	}
	for _, handler := range handlers {
		if err := registry.RegisterInvocableCapability(handler); err != nil {
			return err
		}
	}
	return nil
}
