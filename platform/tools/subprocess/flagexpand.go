// Package subprocess provides manifest-driven subprocess tool execution.
// It is the relocated home of the generic subprocess executor (from
// framework/cfgload) and the typed flag-expansion engine (C3).
package subprocess

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ExpandCommand builds the full argv for a subprocess tool from its manifest
// and invocation args. Every returned token is a discrete, indivisible argv
// element — never split on =/space/quotes, never shell-interpreted.
//
// Expansion order:
//  1. command.base
//  2. typed/boolean flags (deterministic flag-key sort)
//  3. command.args (placeholder tokens)
//  4. default_args
//  5. raw args["args"] (flagged only if sandbox.allow_flags)
func ExpandCommand(manifest contracts.ToolManifest, args map[string]interface{}) ([]string, error) {
	execSpec := manifest.Execution
	commandSpec := execSpec.Command

	if variant, ok := execSpec.PlatformVariants[runtime.GOOS]; ok {
		commandSpec = &variant
	}
	if commandSpec == nil {
		return nil, errors.New("execution.command required for subprocess backend")
	}

	var cmd []string

	// 1. Base tokens
	for _, token := range commandSpec.Base {
		expanded, err := expandToken(token, args)
		if err != nil {
			return nil, err
		}
		cmd = append(cmd, expanded...)
	}

	// 2. Flags (deterministic order)
	flags, err := expandFlags(commandSpec.Flags, args)
	if err != nil {
		return nil, err
	}
	cmd = append(cmd, flags...)

	// 3. Command.Args (positional placeholders)
	for _, token := range commandSpec.Args {
		expanded, err := expandToken(token, args)
		if err != nil {
			return nil, err
		}
		cmd = append(cmd, expanded...)
	}

	// 4. DefaultArgs
	for _, token := range execSpec.DefaultArgs {
		expanded, err := expandToken(token, args)
		if err != nil {
			return nil, err
		}
		cmd = append(cmd, expanded...)
	}

	// 5. Raw args with flag-injection guard
	allowFlags := execSpec.Sandbox != nil && execSpec.Sandbox.AllowFlags
	if raw, ok := args["args"]; ok {
		extra, err := contracts.NormalizeStringSlice(raw)
		if err != nil {
			return nil, fmt.Errorf("args: %w", err)
		}
		if !allowFlags {
			for _, arg := range extra {
				if strings.HasPrefix(arg, "-") {
					return nil, fmt.Errorf("flag injection: arg %q must not begin with '-'; "+
						"set allow_flags: true in the tool manifest to permit flags", arg)
				}
			}
		}
		cmd = append(cmd, extra...)
	}

	if len(cmd) == 0 {
		return nil, errors.New("execution.command.base required for subprocess backend")
	}
	return cmd, nil
}

// expandFlags emits argv tokens for every declared flag in deterministic
// key order. Boolean flags are emitted when the matching parameter is true;
// typed flags bind to the declared Param and are formatted per Style/Repeat.
func expandFlags(flags map[string]contracts.ToolManifestFlag, args map[string]interface{}) ([]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(flags))
	for k := range flags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result []string
	for _, key := range keys {
		flag := flags[key]
		hasBool := len(flag.WhenTrue) > 0 || len(flag.WhenFalse) > 0
		hasTyped := flag.Param != ""

		switch {
		case hasBool && !hasTyped:
			tokens, err := expandBooleanFlag(key, flag, args)
			if err != nil {
				return nil, err
			}
			result = append(result, tokens...)

		case hasTyped && !hasBool:
			tokens, err := expandTypedFlag(key, flag, args)
			if err != nil {
				return nil, err
			}
			result = append(result, tokens...)

		default:
			// Should not happen — validation catches mixed/empty forms.
			// Skip silently if both are empty (no form specified).
			continue
		}
	}
	return result, nil
}

func expandBooleanFlag(key string, flag contracts.ToolManifestFlag, args map[string]interface{}) ([]string, error) {
	val, exists := lookupArg(args, key)
	if !exists || val == nil {
		return nil, nil // not provided — skip
	}
	b, err := toBool(val)
	if err != nil {
		return nil, fmt.Errorf("flag %q expects a boolean value for parameter %q", key, key)
	}
	if b {
		return copyStrings(flag.WhenTrue), nil
	}
	return copyStrings(flag.WhenFalse), nil
}

func expandTypedFlag(key string, flag contracts.ToolManifestFlag, args map[string]interface{}) ([]string, error) {
	val, exists := lookupArg(args, flag.Param)
	if !exists || val == nil {
		return nil, nil // not provided — skip
	}

	flagName := "--" + key
	style := flag.Style
	if style == "" {
		style = contracts.FlagStyleEquals
	}

	switch style {
	case contracts.FlagStyleEquals:
		if flag.Repeat {
			values, err := toStringSlice(val)
			if err != nil {
				return nil, fmt.Errorf("flag %q param %q: %w", key, flag.Param, err)
			}
			out := make([]string, 0, len(values))
			for _, v := range values {
				out = append(out, flagName+"="+v)
			}
			return out, nil
		}
		return []string{flagName + "=" + fmt.Sprint(val)}, nil

	case contracts.FlagStyleSeparate:
		if flag.Repeat {
			values, err := toStringSlice(val)
			if err != nil {
				return nil, fmt.Errorf("flag %q param %q: %w", key, flag.Param, err)
			}
			out := make([]string, 0, len(values)*2)
			for _, v := range values {
				out = append(out, flagName, v)
			}
			return out, nil
		}
		return []string{flagName, fmt.Sprint(val)}, nil

	default:
		return nil, fmt.Errorf("flag %q: unsupported style %q", key, style)
	}
}

// --- token expansion helpers (moved from framework/cfgload) ---

func expandToken(token string, args map[string]interface{}) ([]string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	if name, ok := placeholderName(token); ok {
		value, exists := lookupArg(args, name)
		if !exists {
			return nil, fmt.Errorf("missing parameter %q", name)
		}
		switch typed := value.(type) {
		case []string:
			return copyStrings(typed), nil
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				out = append(out, fmt.Sprint(item))
			}
			return out, nil
		default:
			values, err := contracts.NormalizeStringSlice(value)
			if err == nil && len(values) > 1 {
				return values, nil
			}
			return []string{fmt.Sprint(value)}, nil
		}
	}
	if strings.Contains(token, "${") || strings.Contains(token, "{{") {
		return nil, fmt.Errorf("token %q must be a single placeholder token", token)
	}
	return []string{token}, nil
}

func placeholderName(token string) (string, bool) {
	switch {
	case strings.HasPrefix(token, "${") && strings.HasSuffix(token, "}"):
		return strings.TrimSpace(token[2 : len(token)-1]), true
	case strings.HasPrefix(token, "{{") && strings.HasSuffix(token, "}}"):
		return strings.TrimSpace(token[2 : len(token)-2]), true
	default:
		return "", false
	}
}

func lookupArg(args map[string]interface{}, name string) (interface{}, bool) {
	if len(args) == 0 {
		return nil, false
	}
	want := contracts.NormalizeToolName(name)
	for key, value := range args {
		if contracts.NormalizeToolName(key) == want {
			return value, true
		}
	}
	return nil, false
}

func toStringSlice(val interface{}) ([]string, error) {
	switch v := val.(type) {
	case []string:
		return copyStrings(v), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out, nil
	default:
		s, err := contracts.NormalizeStringSlice(val)
		if err != nil {
			return nil, fmt.Errorf("expected array, got %T", val)
		}
		if len(s) == 0 {
			return nil, fmt.Errorf("expected non-empty array")
		}
		return s, nil
	}
}

func toBool(val interface{}) (bool, error) {
	switch v := val.(type) {
	case bool:
		return v, nil
	case string:
		return strconv.ParseBool(v)
	case int64, float64:
		s := fmt.Sprint(v)
		return strconv.ParseBool(s)
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			return false, fmt.Errorf("cannot convert empty value to bool")
		}
		return strconv.ParseBool(s)
	}
}

func copyStrings(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}
