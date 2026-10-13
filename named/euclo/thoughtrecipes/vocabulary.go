package thoughtrecipe

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
)

// ToolVocabularyLookup is the single declared-vocabulary catalog (D1): the
// composed capability registry satisfies it, and GetCapability applies the
// registry's full resolution order — exact capability ID, registry name
// index (manifest names and bare invocable-capability names), then
// model-variant normalization. CallableCapabilityNames backs the
// nearest-candidate suggestion in the load error.
type ToolVocabularyLookup interface {
	GetCapability(idOrName string) (descriptor.CapabilityDescriptor, bool)
	CallableCapabilityNames() []string
}

// ErrUnresolvedToolName is the load-time failure for a declared `may invoke`
// name the capability registry cannot resolve (D2). A recipe whose declared
// vocabulary does not resolve cannot be enforced, so it fails the load —
// naming recipe, step, and tool — instead of running disarmed.
type ErrUnresolvedToolName struct {
	Recipe  string
	Step    string
	Tool    string
	Nearest []string
}

func (e *ErrUnresolvedToolName) Error() string {
	msg := fmt.Sprintf("recipe %q step %q: may invoke %q does not resolve in the capability registry", e.Recipe, e.Step, e.Tool)
	if len(e.Nearest) > 0 {
		msg += fmt.Sprintf(" (nearest: %s)", strings.Join(e.Nearest, ", "))
	}
	return msg
}

// ResolveDeclaredToolNames resolves every declared tool name through the
// vocabulary lookup and returns an *ErrUnresolvedToolName for the first name
// that does not resolve. A nil lookup resolves nothing: callers that have no
// registry (constructed symbol tables in tests) skip validation here and are
// caught by the programmatic registration arm (FR-14).
func ResolveDeclaredToolNames(recipe, step string, names []string, lookup ToolVocabularyLookup) error {
	if lookup == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || name == "__euclo__.deny_all__" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if _, ok := lookup.GetCapability(name); ok {
			continue
		}
		return &ErrUnresolvedToolName{
			Recipe:  recipe,
			Step:    step,
			Tool:    name,
			Nearest: nearestVocabularyNames(name, lookup.CallableCapabilityNames()),
		}
	}
	return nil
}

// nearestVocabularyNames returns up to 5 known vocabulary names within
// Levenshtein distance 3 of the unresolved name.
func nearestVocabularyNames(name string, known []string) []string {
	const (
		maxDistance    = 3
		maxSuggestions = 5
	)
	var nearest []string
	for _, candidate := range known {
		if levenshtein(name, candidate) <= maxDistance {
			nearest = append(nearest, candidate)
			if len(nearest) == maxSuggestions {
				break
			}
		}
	}
	return nearest
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// ModelCallableToolClassifier optionally refines a ToolVocabularyLookup with
// the registry's model-facing view. A declared `may invoke` name can resolve
// in the registry (legal for `do` steps and scope enforcement) yet not be
// exposed as a model-callable tool — the model never sees it in its tool list
// and cannot invoke it. Production lookups implement this; constructed symbol
// tables in tests may not, and the classification is then skipped.
type ModelCallableToolClassifier interface {
	ModelCallableToolNames() []string
}

// FindNonCallableDeclaredTools returns the declared names that resolve in the
// registry but are not exposed as model-callable tools. Unresolved names are
// not reported here — ResolveDeclaredToolNames fails the load for those. A
// lookup without the classifier reports nothing.
func FindNonCallableDeclaredTools(names []string, lookup ToolVocabularyLookup) []string {
	classifier, ok := lookup.(ModelCallableToolClassifier)
	if !ok || lookup == nil {
		return nil
	}
	callable := make(map[string]struct{})
	for _, name := range classifier.ModelCallableToolNames() {
		callable[strings.TrimSpace(name)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(names))
	var out []string
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || name == "__euclo__.deny_all__" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		if _, resolved := lookup.GetCapability(name); !resolved {
			continue
		}
		if _, callableName := callable[name]; callableName {
			continue
		}
		out = append(out, name)
	}
	return out
}
