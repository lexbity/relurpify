package contextmgr

import "fmt"

// DetailBand maps a minimum relevance threshold to a detail level.
// Bands are evaluated high-to-low; the first matching band wins.
type DetailBand struct {
	MinRelevance float64
	Level        DetailLevel
}

// PrioritizationMode controls how PrioritizeContext ranks items.
type PrioritizationMode int

const (
	PrioritizationRelevance PrioritizationMode = iota // sort by RelevanceScore descending
	PrioritizationRecency                             // sort by Age ascending
	PrioritizationWeighted                            // weighted mix of relevance and recency
)

// ExpandTriggerKind controls which runtime signals cause ShouldExpandContext
// to return true.
type ExpandTriggerKind int

const (
	ExpandOnErrorType            ExpandTriggerKind = iota // expand on specific error_type values
	ExpandOnToolUse                                       // expand after specific tool_used values
	ExpandOnFailureOrUncertainty                          // expand on failure or uncertainty markers
)

// StrategyProfile holds all parameterisable decisions for a context strategy
// personality. A zero-value profile is valid (no compression, no expansion,
// no detail banding).
type StrategyProfile struct {
	Name string

	// ShouldCompress threshold: compress when history length exceeds this value.
	// Zero means never compress via this strategy.
	CompressThreshold int

	// DetermineDetailLevel: bands evaluated high-to-low by MinRelevance.
	// If no band matches, DetailSignatureOnly is returned.
	DetailBands []DetailBand

	// PrioritizeContext ordering.
	PrioritizationMode PrioritizationMode
	RelevanceWeight    float64 // used when PrioritizationWeighted
	RecencyWeight      float64 // used when PrioritizationWeighted

	// SelectContext: fraction of budget.AvailableForContext assigned to MaxTokens.
	TokenBudgetFraction float64

	// AST listing filter for the initial symbol query.
	ASTExportedOnly bool

	// File loading behaviour for explicitly referenced files.
	FileDetailLevel DetailLevel
	FilePinned      bool // pin all referenced files
	PinFirstN       int  // pin only the first N files (overrides FilePinned when > 0)

	// Load AST dependency queries for each referenced file.
	LoadDependencies bool

	// Keyword/full-instruction search. Zero SearchMaxResults disables search.
	SearchMaxResults         int
	SearchUseFullInstruction bool // true = full instruction text; false = keyword extract

	// Memory loading. Zero MemoryMaxResults disables memory queries.
	LoadMemory       bool
	MemoryMaxResults int

	// ShouldExpandContext trigger.
	ExpandTrigger    ExpandTriggerKind
	ExpandErrorTypes []string // matched against result.Data["error_type"]
	ExpandToolTypes  []string // matched against result.Data["tool_used"]
}

var AggressiveProfile = StrategyProfile{
	Name:              "aggressive",
	CompressThreshold: 5,
	DetailBands: []DetailBand{
		{MinRelevance: 0.9, Level: DetailDetailed},
		{MinRelevance: 0.7, Level: DetailConcise},
		{MinRelevance: 0.5, Level: DetailMinimal},
		{MinRelevance: 0.0, Level: DetailSignatureOnly},
	},
	PrioritizationMode:  PrioritizationRecency,
	TokenBudgetFraction: 0.25,
	ASTExportedOnly:     true,
	FileDetailLevel:     DetailSignatureOnly,
	FilePinned:          false,
	SearchMaxResults:    0,
	LoadMemory:          false,
	ExpandTrigger:       ExpandOnErrorType,
	ExpandErrorTypes:    []string{"insufficient_context", "file_not_found"},
}

var ConservativeProfile = StrategyProfile{
	Name:              "conservative",
	CompressThreshold: 15,
	DetailBands: []DetailBand{
		{MinRelevance: 0.8, Level: DetailFull},
		{MinRelevance: 0.5, Level: DetailDetailed},
		{MinRelevance: 0.0, Level: DetailConcise},
	},
	PrioritizationMode:       PrioritizationRelevance,
	TokenBudgetFraction:      0.75,
	ASTExportedOnly:          false,
	FileDetailLevel:          DetailDetailed,
	FilePinned:               true,
	LoadDependencies:         true,
	SearchMaxResults:         20,
	SearchUseFullInstruction: true,
	LoadMemory:               true,
	MemoryMaxResults:         10,
	ExpandTrigger:            ExpandOnToolUse,
	ExpandToolTypes:          []string{"search", "query_ast"},
}

var BalancedProfile = StrategyProfile{
	Name:              "balanced",
	CompressThreshold: 10,
	DetailBands: []DetailBand{
		{MinRelevance: 0.85, Level: DetailFull},
		{MinRelevance: 0.6, Level: DetailDetailed},
		{MinRelevance: 0.0, Level: DetailConcise},
	},
	PrioritizationMode:  PrioritizationWeighted,
	RelevanceWeight:     0.6,
	RecencyWeight:       0.4,
	TokenBudgetFraction: 0.5,
	ASTExportedOnly:     true,
	FileDetailLevel:     DetailConcise,
	PinFirstN:           2,
	SearchMaxResults:    10,
	LoadMemory:          false,
	ExpandTrigger:       ExpandOnFailureOrUncertainty,
}

// profileRegistry maps stable strategy IDs to their canonical profiles.
var profileRegistry = map[string]StrategyProfile{
	"aggressive":           AggressiveProfile,
	"balanced":             BalancedProfile,
	"conservative":         ConservativeProfile,
	"narrow_to_wide":       ConservativeProfile,
	"localize_then_expand": BalancedProfile,
	"targeted":             AggressiveProfile,
	"read_heavy":           ConservativeProfile,
	"expand_carefully":     BalancedProfile,
}

// LookupProfile returns the profile registered under name, or false if unknown.
func LookupProfile(name string) (StrategyProfile, bool) {
	p, ok := profileRegistry[name]
	return p, ok
}

// ValidateProfileBands checks that bands are ordered from highest to lowest
// relevance threshold.
func ValidateProfileBands(profile StrategyProfile) error {
	if len(profile.DetailBands) < 2 {
		return nil
	}
	for i := 1; i < len(profile.DetailBands); i++ {
		if profile.DetailBands[i-1].MinRelevance < profile.DetailBands[i].MinRelevance {
			return fmt.Errorf("detail bands out of order at index %d: %.3f < %.3f", i, profile.DetailBands[i-1].MinRelevance, profile.DetailBands[i].MinRelevance)
		}
	}
	return nil
}
