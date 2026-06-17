package thoughtrecipe

import (
	"fmt"
	"math"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
)

// lookupConditionValue reads a value from the envelope by key.
func lookupConditionValue(env *contextdata.Envelope, key string) any {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	v, ok := contextdata.GetTyped[any](env, key)
	if !ok {
		return nil
	}
	return v
}

// compilePredicate compiles a typed Predicate into a pure ConditionFunc closure.
// Unknown PredicateOp produces a build error (panic at compile time).
func compilePredicate(p Predicate) agentgraph.ConditionFunc {
	switch p.Op {
	case PredOpIs:
		return func(_ *execution.Result, env *contextdata.Envelope) bool {
			return strings.TrimSpace(fmt.Sprint(lookupConditionValue(env, p.Subject))) == p.Value.StringVal
		}
	case PredOpContains:
		want := p.Value.StringVal
		return func(_ *execution.Result, env *contextdata.Envelope) bool {
			have := lookupConditionValue(env, p.Subject)
			switch v := have.(type) {
			case string:
				return strings.Contains(v, want)
			case []string:
				for _, entry := range v {
					if entry == want {
						return true
					}
				}
			case []any:
				for _, entry := range v {
					if strings.TrimSpace(fmt.Sprint(entry)) == want {
						return true
					}
				}
			default:
				return strings.Contains(strings.TrimSpace(fmt.Sprint(have)), want)
			}
			return false
		}
	case PredOpMissing:
		return func(_ *execution.Result, env *contextdata.Envelope) bool {
			return !truthy(lookupConditionValue(env, p.Subject))
		}
	case PredOpPresent:
		return func(_ *execution.Result, env *contextdata.Envelope) bool {
			return truthy(lookupConditionValue(env, p.Subject))
		}
	case PredOpConfidenceLT:
		threshold := p.Value.Percent
		return func(_ *execution.Result, env *contextdata.Envelope) bool {
			confidence := confidenceValue(env, p.Subject)
			return confidence >= 0 && confidence < threshold
		}
	default:
		// Exhaustive switch — every valid op is handled above.
		// Unknown ops fail at compile time (never a silent false).
		panic(fmt.Sprintf("compilePredicate: unknown predicate op %v", p.Op))
	}
}

func confidenceValue(env *contextdata.Envelope, subject string) int {
	// Try the subject directly first (it might be a number or an AmbiguityCharacterization).
	raw := lookupConditionValue(env, subject)
	if c := extractConfidence(raw); c >= 0 {
		return c
	}
	// Try subject with _confidence suffix (common intent pattern).
	if c := extractConfidence(lookupConditionValue(env, subject+"_confidence")); c >= 0 {
		return c
	}
	// Try subject with .confidence suffix.
	return extractConfidence(lookupConditionValue(env, subject+".confidence"))
}

func extractConfidence(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int8:
		return int(x)
	case int16:
		return int(x)
	case int32:
		return int(x)
	case int64:
		return int(x)
	case uint:
		return int(x)
	case uint8:
		return int(x)
	case uint16:
		return int(x)
	case uint32:
		return int(x)
	case uint64:
		if x > uint64(math.MaxInt) {
			return -1
		}
		return int(x)
	case float32:
		// Treat float as percentage (0.0-1.0 → 0-100)
		if x >= 0 && x <= 1 {
			return int(x * 100)
		}
		return int(x)
	case float64:
		if x >= 0 && x <= 1 {
			return int(x * 100)
		}
		return int(x)
	case string:
		var parsed int
		cleaned := strings.TrimSuffix(strings.TrimSpace(x), "%")
		if _, err := fmt.Sscanf(cleaned, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return -1
}
