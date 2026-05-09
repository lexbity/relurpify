package thoughtrecipe

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// LowerCaptureBindings normalizes capture bindings for runtime execution.
func LowerCaptureBindings(block *CaptureBlock) []CaptureBinding {
	if block == nil || len(block.Bindings) == 0 {
		return nil
	}
	out := make([]CaptureBinding, 0, len(block.Bindings))
	for _, binding := range block.Bindings {
		out = append(out, binding)
	}
	return out
}

// CaptureDestinationKey returns the explicit working-memory key for a capture destination.
func CaptureDestinationKey(binding CaptureBinding) string {
	return strings.TrimSpace(binding.Destination.Raw)
}

// CaptureSourceValue resolves the source value for a capture binding from the
// current runtime data.
func CaptureSourceValue(binding CaptureBinding, resultData map[string]any, envData map[string]any) (any, bool) {
	if binding.Source == nil {
		return nil, false
	}

	if binding.Forwarding {
		if value, ok := lookupCaptureSourceValue(binding.Source, resultData, envData); ok {
			return value, true
		}
		return nil, false
	}

	if value, ok := lookupCaptureSourceValue(binding.Source, resultData, envData); ok {
		return value, true
	}
	return nil, false
}

// ApplyCaptureBindings writes the capture bindings to the envelope and returns
// the destinations that were updated.
func ApplyCaptureBindings(env *contextdata.Envelope, bindings []CaptureBinding, resultData map[string]any) ([]string, error) {
	if env == nil {
		return nil, fmt.Errorf("envelope is nil")
	}
	return ApplyCaptureBindingsFromSnapshot(env, env.Snapshot(), bindings, resultData)
}

// ApplyCaptureBindingsFromSnapshot writes capture bindings using the supplied
// source snapshot for lookups and the destination envelope for writes.
func ApplyCaptureBindingsFromSnapshot(env *contextdata.Envelope, sourceData map[string]any, bindings []CaptureBinding, resultData map[string]any) ([]string, error) {
	if env == nil {
		return nil, fmt.Errorf("envelope is nil")
	}
	if len(bindings) == 0 {
		return nil, nil
	}

	writes := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		dest := CaptureDestinationKey(binding)
		if dest == "" {
			return nil, fmt.Errorf("%s:%d:%d: capture destination is required", binding.GetSpan().Start.File, binding.GetSpan().Start.Line, binding.GetSpan().Start.Column)
		}
		if _, ok := validateCaptureDestinationPath(binding.Destination); !ok {
			return nil, fmt.Errorf("%s:%d:%d: capture destination must be a namespace reference", binding.GetSpan().Start.File, binding.GetSpan().Start.Line, binding.GetSpan().Start.Column)
		}
		value, ok := CaptureSourceValue(binding, resultData, sourceData)
		if !ok {
			value = resultData
		}
		if binding.Forwarding {
			if _, ok := valueExprPath(binding.Source); !ok {
				return nil, fmt.Errorf("%s:%d:%d: direct forwarding requires a reference source", binding.GetSpan().Start.File, binding.GetSpan().Start.Line, binding.GetSpan().Start.Column)
			}
		}
		env.SetWorkingValue(dest, value, contextdata.MemoryClassTask)
		writes = append(writes, dest)
	}
	return writes, nil
}

func lookupCaptureSourceValue(expr ValueExpr, resultData map[string]any, envData map[string]any) (any, bool) {
	if value, ok := lookupNamedValue(expr, resultData); ok {
		return value, true
	}
	if value, ok := lookupNamedValue(expr, envData); ok {
		return value, true
	}
	return nil, false
}

func lookupNamedValue(expr ValueExpr, data map[string]any) (any, bool) {
	if data == nil {
		return nil, false
	}
	switch v := expr.(type) {
	case *Identifier:
		if value, ok := data[v.Value]; ok {
			return value, true
		}
	case Identifier:
		if value, ok := data[v.Value]; ok {
			return value, true
		}
	case *PathExpr:
		if value, ok := data[v.Raw]; ok {
			return value, true
		}
		if len(v.Parts) > 0 {
			if value, ok := data[v.Parts[len(v.Parts)-1].Value]; ok {
				return value, true
			}
		}
	case PathExpr:
		if value, ok := data[v.Raw]; ok {
			return value, true
		}
		if len(v.Parts) > 0 {
			if value, ok := data[v.Parts[len(v.Parts)-1].Value]; ok {
				return value, true
			}
		}
	case *StringLiteral:
		return v.Value, true
	case StringLiteral:
		return v.Value, true
	case *NumberLiteral:
		return v.Value, true
	case NumberLiteral:
		return v.Value, true
	}
	return nil, false
}

func validateCaptureDestinationPath(dest PathExpr) (PathExpr, bool) {
	if len(dest.Parts) < 2 {
		return PathExpr{}, false
	}
	switch strings.TrimSpace(dest.Parts[0].Value) {
	case "state", "scratch", "user", "output":
		return dest, true
	default:
		return PathExpr{}, false
	}
}
