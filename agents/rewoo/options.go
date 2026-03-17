package rewoo

import (
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	frameworksearch "github.com/lexcodex/relurpify/framework/search"
)

// WithContextPolicy injects a pre-configured ContextPolicy.
// If not set, Initialize will create a default adaptive strategy policy.
func WithContextPolicy(policy *contextmgr.ContextPolicy) Option {
	return func(a *RewooAgent) {
		if policy != nil {
			a.ContextPolicy = policy
		}
	}
}

// WithPermissionManager injects a pre-configured PermissionManager.
// If not set, Initialize will create a default manager with workspace-read + all-tools.
func WithPermissionManager(pm *authorization.PermissionManager) Option {
	return func(a *RewooAgent) {
		if pm != nil {
			a.PermissionManager = pm
		}
	}
}

// WithIndexManager injects an AST index manager for code-aware context.
func WithIndexManager(im *ast.IndexManager) Option {
	return func(a *RewooAgent) {
		if im != nil {
			a.IndexManager = im
		}
	}
}

// WithSearchEngine injects a search engine for semantic file ranking.
func WithSearchEngine(se *frameworksearch.SearchEngine) Option {
	return func(a *RewooAgent) {
		if se != nil {
			a.SearchEngine = se
		}
	}
}

// WithTelemetry injects a telemetry sink for event recording.
func WithTelemetry(tel core.Telemetry) Option {
	return func(a *RewooAgent) {
		if tel != nil {
			a.Telemetry = tel
		}
	}
}

// WithContextConfig sets the context management configuration.
func WithContextConfig(cfg RewooContextConfig) Option {
	return func(a *RewooAgent) {
		a.Options.ContextConfig = cfg
	}
}

// WithPermissionConfig sets the authorization configuration.
func WithPermissionConfig(cfg RewooPermissionConfig) Option {
	return func(a *RewooAgent) {
		a.Options.PermConfig = cfg
	}
}

// WithGraphConfig sets the graph execution configuration.
func WithGraphConfig(cfg RewooGraphConfig) Option {
	return func(a *RewooAgent) {
		a.Options.GraphConfig = cfg
	}
}

// WithMaxReplanAttempts sets the maximum number of replan attempts.
func WithMaxReplanAttempts(max int) Option {
	return func(a *RewooAgent) {
		a.Options.MaxReplanAttempts = max
	}
}

// WithMaxSteps sets the maximum number of steps per plan.
func WithMaxSteps(max int) Option {
	return func(a *RewooAgent) {
		a.Options.MaxSteps = max
	}
}

// WithOnFailure sets the default failure handling mode.
func WithOnFailure(mode StepOnFailure) Option {
	return func(a *RewooAgent) {
		a.Options.OnFailure = mode
	}
}

// WithMaxParallelSteps limits concurrent step execution.
func WithMaxParallelSteps(max int) Option {
	return func(a *RewooAgent) {
		if max > 0 {
			a.Options.GraphConfig.MaxParallelSteps = max
		}
	}
}

// WithCheckpointInterval sets how often to save execution state.
func WithCheckpointInterval(interval int) Option {
	return func(a *RewooAgent) {
		a.Options.GraphConfig.CheckpointInterval = interval
	}
}

// WithParallelExecutionEnabled controls whether steps can run in parallel.
func WithParallelExecutionEnabled(enabled bool) Option {
	return func(a *RewooAgent) {
		a.Options.GraphConfig.EnableParallelExecution = enabled
	}
}
