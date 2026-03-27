package goalcon

import (
	"github.com/lexcodex/relurpify/agents/goalcon/analysis"
)

// Re-exports from analysis package for backward compatibility
type AmbiguityIndicator = analysis.AmbiguityIndicator
type AmbiguityScore = analysis.AmbiguityScore
type AmbiguityAnalyzer = analysis.AmbiguityAnalyzer
type ClarificationChoice = analysis.ClarificationChoice
type ClarificationSession = analysis.ClarificationSession
type GoalClarifier = analysis.GoalClarifier
type DecompositionStrategy = analysis.DecompositionStrategy
type SubGoal = analysis.SubGoal
type GoalDecomposition = analysis.GoalDecomposition
type GoalDecomposer = analysis.GoalDecomposer
type ClassificationResponse = analysis.ClassificationResponse
type ClassifierConfig = analysis.ClassifierConfig
type GoalCache = analysis.GoalCache

// Re-exported constructors and functions
var (
	NewAmbiguityAnalyzer    = analysis.NewAmbiguityAnalyzer
	NewGoalClarifier        = analysis.NewGoalClarifier
	NewGoalDecomposer       = analysis.NewGoalDecomposer
	NewGoalCache            = analysis.NewGoalCache
	DefaultClassifierConfig = analysis.DefaultClassifierConfig
	ClassifyGoal            = analysis.ClassifyGoal
	ClassifyGoalWithLLM     = analysis.ClassifyGoalWithLLM
	ClassifyGoalWithContext = analysis.ClassifyGoalWithContext
)

const (
	DecompositionStrategyPredicates   = analysis.DecompositionStrategyPredicates
	DecompositionStrategySequential   = analysis.DecompositionStrategySequential
	DecompositionStrategyHierarchical = analysis.DecompositionStrategyHierarchical
	DecompositionStrategyDomainBased  = analysis.DecompositionStrategyDomainBased
)
