package contextmgr

import (
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/search"
	"sort"
)

// ProfiledStrategy implements ContextStrategy from a StrategyProfile.
type ProfiledStrategy struct {
	Profile StrategyProfile
}

func NewStrategyFromProfile(p StrategyProfile) *ProfiledStrategy {
	return &ProfiledStrategy{Profile: p}
}

func NewAggressiveStrategy() *ProfiledStrategy {
	return NewStrategyFromProfile(AggressiveProfile)
}

func NewConservativeStrategy() *ProfiledStrategy {
	return NewStrategyFromProfile(ConservativeProfile)
}

func (ps *ProfiledStrategy) SelectContext(task *core.Task, budget *core.ContextBudget) (*ContextRequest, error) {
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	if budget == nil {
		return nil, fmt.Errorf("budget required")
	}

	request := &ContextRequest{
		Files:      make([]FileRequest, 0),
		ASTQueries: make([]ASTQuery, 0, 1),
		MaxTokens:  int(float64(budget.AvailableForContext) * ps.Profile.TokenBudgetFraction),
	}
	request.ASTQueries = append(request.ASTQueries, ASTQuery{
		Type: ASTQueryListSymbols,
		Filter: ASTFilter{
			ExportedOnly: ps.Profile.ASTExportedOnly,
		},
	})

	files := ExtractFileReferences(task.Instruction)
	for i, file := range files {
		pinned := ps.Profile.FilePinned
		if ps.Profile.PinFirstN > 0 {
			pinned = i < ps.Profile.PinFirstN
		}
		request.Files = append(request.Files, FileRequest{
			Path:        file,
			DetailLevel: ps.Profile.FileDetailLevel,
			Priority: func() int {
				if pinned {
					return 0
				}
				return 1
			}(),
			Pinned: pinned,
		})
	}

	if ps.Profile.LoadDependencies && len(files) > 0 {
		for _, file := range files {
			request.ASTQueries = append(request.ASTQueries, ASTQuery{
				Type:   ASTQueryGetDependencies,
				Symbol: file,
			})
		}
	}

	if (ps.Profile.LoadDependencies && len(files) == 0) || ps.Profile.SearchMaxResults > 0 {
		if ps.Profile.SearchMaxResults > 0 {
			queryText := task.Instruction
			if !ps.Profile.SearchUseFullInstruction {
				queryText = ExtractKeywords(task.Instruction)
			}
			request.SearchQueries = append(request.SearchQueries, SearchQuery{
				Text:       queryText,
				Mode:       search.SearchHybrid,
				MaxResults: ps.Profile.SearchMaxResults,
			})
		}
	}

	if ps.Profile.LoadMemory {
		request.MemoryQueries = append(request.MemoryQueries, MemoryQuery{
			Scope:      memory.MemoryScopeProject,
			Query:      task.Instruction,
			MaxResults: ps.Profile.MemoryMaxResults,
		})
	}

	AppendContextFiles(request, task, DetailFull)
	return request, nil
}

func (ps *ProfiledStrategy) ShouldCompress(ctx *core.SharedContext) bool {
	if ctx == nil || ps.Profile.CompressThreshold == 0 {
		return false
	}
	return len(ctx.History()) > ps.Profile.CompressThreshold
}

func (ps *ProfiledStrategy) DetermineDetailLevel(file string, relevance float64) DetailLevel {
	for _, band := range ps.Profile.DetailBands {
		if relevance >= band.MinRelevance {
			return band.Level
		}
	}
	return DetailSignatureOnly
}

func (ps *ProfiledStrategy) ShouldExpandContext(ctx *core.SharedContext, lastResult *core.Result) bool {
	if lastResult == nil {
		return false
	}

	switch ps.Profile.ExpandTrigger {
	case ExpandOnErrorType:
		if lastResult.Success || lastResult.Data == nil {
			return false
		}
		errorType, _ := lastResult.Data["error_type"].(string)
		return containsString(ps.Profile.ExpandErrorTypes, errorType)
	case ExpandOnToolUse:
		if lastResult.Data == nil {
			return false
		}
		tool, _ := lastResult.Data["tool_used"].(string)
		return containsString(ps.Profile.ExpandToolTypes, tool)
	case ExpandOnFailureOrUncertainty:
		if !lastResult.Success {
			return true
		}
		if lastResult.Data == nil {
			return false
		}
		output, _ := lastResult.Data["llm_output"].(string)
		if output == "" {
			return false
		}
		for _, marker := range uncertaintyMarkers {
			if ContainsInsensitive(output, marker) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (ps *ProfiledStrategy) PrioritizeContext(items []core.ContextItem) []core.ContextItem {
	sorted := append([]core.ContextItem(nil), items...)
	switch ps.Profile.PrioritizationMode {
	case PrioritizationRelevance:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].RelevanceScore() > sorted[j].RelevanceScore()
		})
	case PrioritizationRecency:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Age() < sorted[j].Age()
		})
	case PrioritizationWeighted:
		relevanceWeight := ps.Profile.RelevanceWeight
		recencyWeight := ps.Profile.RecencyWeight
		sort.Slice(sorted, func(i, j int) bool {
			scoreI := sorted[i].RelevanceScore()*relevanceWeight + (1.0/(1.0+sorted[i].Age().Hours()))*recencyWeight
			scoreJ := sorted[j].RelevanceScore()*relevanceWeight + (1.0/(1.0+sorted[j].Age().Hours()))*recencyWeight
			return scoreI > scoreJ
		})
	default:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].RelevanceScore() > sorted[j].RelevanceScore()
		})
	}
	return sorted
}

var uncertaintyMarkers = []string{
	"not sure",
	"unclear",
	"need more information",
	"cannot determine",
	"insufficient",
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
