package archaeographqlserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
)

// Map is a passthrough GraphQL scalar for map-shaped payloads and metadata.
type Map map[string]any

func (*Map) ImplementsGraphQLType(name string) bool {
	return name == "Map"
}

func (m *Map) UnmarshalGraphQL(input any) error {
	if input == nil {
		*m = nil
		return nil
	}
	value, ok := input.(map[string]any)
	if !ok {
		return fmt.Errorf("expected map input")
	}
	*m = Map(value)
	return nil
}

type PrepareLivingPlanInput struct {
	WorkflowID          string
	WorkspaceID         string
	Instruction         string
	CorpusScope         string
	SymbolScope         string
	BasedOnRevision     string
	SemanticSnapshotRef string
}

type PrepareLivingPlanPayload struct {
	WorkflowID                  string
	ActiveExplorationID         string
	ActiveExplorationSnapshotID string
	Plan                        *archaeodomain.VersionedLivingPlan
	StepID                      string
	Success                     bool
	Error                       string
	Result                      Map
}

type RefreshExplorationSnapshotInput struct {
	WorkflowID           string
	SnapshotID           string
	BasedOnRevision      string
	SemanticSnapshotRef  string
	CandidatePatternRefs []string
	CandidateAnchorRefs  []string
	TensionIDs           []string
	OpenLearningIDs      []string
	Summary              string
}

type WorkspaceSummary struct {
	WorkspaceID              string
	DeferredDraftOpenCount   int
	DeferredDraftFormedCount int
	ConvergenceOpenCount     int
	ConvergenceResolvedCount int
	ConvergenceDeferredCount int
	DecisionOpenCount        int
	DecisionResolvedCount    int
	CurrentConvergence       *archaeodomain.ConvergenceRecord
}

type ApplyRequestFulfillmentPayload struct {
	Request  *archaeodomain.RequestRecord
	Validity archaeodomain.RequestValidity
}

func toMap(value any) (*Map, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	mapped := Map(normalizeGraphQLValue(out).(map[string]any))
	return &mapped, nil
}

func toMaps(value any) ([]Map, error) {
	if value == nil {
		return []Map{}, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	maps := make([]Map, 0, len(out))
	for _, item := range out {
		maps = append(maps, Map(normalizeGraphQLValue(item).(map[string]any)))
	}
	return maps, nil
}

func normalizeGraphQLValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[graphqlKey(key)] = normalizeGraphQLValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeGraphQLValue(item))
		}
		return out
	default:
		return value
	}
}

func graphqlKey(key string) string {
	if key == "" {
		return key
	}
	parts := splitGraphQLKey(key)
	if len(parts) == 0 {
		return strings.ToLower(key)
	}
	for i, part := range parts {
		part = strings.ToLower(part)
		if i == 0 {
			parts[i] = part
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

func splitGraphQLKey(key string) []string {
	var parts []string
	start := 0
	for i := 1; i < len(key); i++ {
		prev := rune(key[i-1])
		curr := rune(key[i])
		nextLower := i+1 < len(key) && unicode.IsLower(rune(key[i+1]))
		if unicode.IsLower(prev) && unicode.IsUpper(curr) {
			parts = append(parts, key[start:i])
			start = i
			continue
		}
		if unicode.IsUpper(prev) && unicode.IsUpper(curr) && nextLower {
			parts = append(parts, key[start:i])
			start = i
		}
	}
	parts = append(parts, key[start:])
	return parts
}
