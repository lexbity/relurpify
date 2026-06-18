package ports

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/tools/manifest"
)

// Re-exported types from userconfig/tools/manifest.
type (
	ToolParameterType              = manifest.ToolParameterType
	ToolParameter                  = manifest.ToolParameter
	ToolBackend                    = manifest.ToolBackend
	ToolRateLimit                  = manifest.ToolRateLimit
	ToolManifest                   = manifest.ToolManifest
	ToolManifestSandbox            = manifest.ToolManifestSandbox
	ToolManifestExecution          = manifest.ToolManifestExecution
	ToolManifestCommand            = manifest.ToolManifestCommand
	ToolManifestTelemetry          = manifest.ToolManifestTelemetry
	ToolManifestGuidance           = manifest.ToolManifestGuidance
	ToolManifestFlag               = manifest.ToolManifestFlag
	ToolManifestComposition        = manifest.ToolManifestComposition
	ToolManifestCompositionStep    = manifest.ToolManifestCompositionStep
	ToolManifestReturns            = manifest.ToolManifestReturns
	ToolManifestReturnsChunking    = manifest.ToolManifestReturnsChunking
	ToolManifestCapability         = manifest.ToolManifestCapability
)

const (
	ToolParamString        = manifest.ToolParamString
	ToolParamInteger       = manifest.ToolParamInteger
	ToolParamNumber        = manifest.ToolParamNumber
	ToolParamBoolean       = manifest.ToolParamBoolean
	ToolParamArray         = manifest.ToolParamArray
	ToolParamObject        = manifest.ToolParamObject
	ToolBackendSubprocess  = manifest.ToolBackendSubprocess
	ToolBackendGoNative    = manifest.ToolBackendGoNative
	ToolBackendComposite   = manifest.ToolBackendComposite
	TagReadOnly            = manifest.TagReadOnly
	TagExecute             = manifest.TagExecute
	TagDestructive         = manifest.TagDestructive
	TagNetwork             = manifest.TagNetwork
	FlagStyleEquals        = manifest.FlagStyleEquals
	FlagStyleSeparate      = manifest.FlagStyleSeparate
	ChunkingModeWhole      = manifest.ChunkingModeWhole
	ChunkingModePerItem    = manifest.ChunkingModePerItem
	ChunkingModePerField   = manifest.ChunkingModePerField
)

// NormalizeToolName canonicalizes tool identifiers for lookups.
func NormalizeToolName(name string) string {
	return manifest.NormalizeToolName(name)
}

// CoerceParameterValue attempts to coerce a runtime value to the type declared
// by a ToolParameter.
func CoerceParameterValue(param ToolParameter, v any) (any, error) {
	if v == nil {
		return nil, errors.New("parameter value is nil")
	}
	switch param.Type {
	case ToolParamString:
		switch val := v.(type) {
		case string:
			return val, nil
		default:
			return fmt.Sprint(val), nil
		}
	case ToolParamInteger:
		switch val := v.(type) {
		case int64:
			return val, nil
		case int:
			return int64(val), nil
		case float64:
			if val != float64(int64(val)) {
				return nil, fmt.Errorf("cannot coerce float64 %v to integer: lossy conversion", val)
			}
			return int64(val), nil
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot coerce string %q to integer: %w", val, err)
			}
			return n, nil
		default:
			return nil, fmt.Errorf("cannot coerce %T to integer", v)
		}
	case ToolParamNumber:
		switch val := v.(type) {
		case float64:
			return val, nil
		case int64:
			return float64(val), nil
		case int:
			return float64(val), nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
			if err != nil {
				return nil, fmt.Errorf("cannot coerce string %q to number: %w", val, err)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("cannot coerce %T to number", v)
		}
	case ToolParamBoolean:
		switch val := v.(type) {
		case bool:
			return val, nil
		case string:
			switch strings.ToLower(strings.TrimSpace(val)) {
			case "true", "1", "yes":
				return true, nil
			case "false", "0", "no":
				return false, nil
			default:
				return nil, fmt.Errorf("cannot coerce string %q to boolean", val)
			}
		default:
			return nil, fmt.Errorf("cannot coerce %T to boolean", v)
		}
	default:
		return v, nil
	}
}
