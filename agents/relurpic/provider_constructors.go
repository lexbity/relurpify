package relurpic

import (
	"database/sql"

	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func NewPatternSurfacingProvider(model core.LanguageModel, cfg *core.Config, registry *capability.Registry, indexManager *ast.IndexManager, graphDB *graphdb.Engine, patternStore patterns.PatternStore, retrievalDB *sql.DB) PatternSurfacingProvider {
	provider := PatternSurfacingProvider{
		Model:        model,
		Config:       cfg,
		Registry:     registry,
		IndexManager: indexManager,
		GraphDB:      graphDB,
		PatternStore: patternStore,
		RetrievalDB:  retrievalDB,
	}
	provider.Service = newPatternSurfacingService(provider)
	return provider
}

func NewTensionAnalysisProvider(model core.LanguageModel, cfg *core.Config, registry *capability.Registry, indexManager *ast.IndexManager, graphDB *graphdb.Engine, retrievalDB *sql.DB, planStore frameworkplan.PlanStore, broker *guidance.GuidanceBroker, workflowStore memory.WorkflowStateStore) TensionAnalysisProvider {
	provider := TensionAnalysisProvider{
		Model:         model,
		Config:        cfg,
		Registry:      registry,
		IndexManager:  indexManager,
		GraphDB:       graphDB,
		RetrievalDB:   retrievalDB,
		PlanStore:     planStore,
		Guidance:      broker,
		WorkflowStore: workflowStore,
	}
	provider.Service = newTensionAnalysisService(provider)
	return provider
}

func NewProspectiveAnalysisProvider(model core.LanguageModel, cfg *core.Config, patternStore patterns.PatternStore, retrievalDB *sql.DB) ProspectiveAnalysisProvider {
	provider := ProspectiveAnalysisProvider{
		Model:        model,
		Config:       cfg,
		PatternStore: patternStore,
		RetrievalDB:  retrievalDB,
	}
	provider.Service = newProspectiveAnalysisService(provider)
	return provider
}

func NewConvergenceReviewProvider(patternStore patterns.PatternStore, tensionStore memory.WorkflowStateStore) ConvergenceReviewProvider {
	provider := ConvergenceReviewProvider{
		PatternStore: patternStore,
		TensionStore: tensionStore,
	}
	provider.Service = newConvergenceReviewService(provider)
	return provider
}
