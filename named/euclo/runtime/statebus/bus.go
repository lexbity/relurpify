package statebus

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// Getter is the minimal read-only state interface used by helper packages.
type Getter interface {
	Get(string) (any, bool)
}

// Get retrieves a strongly typed value from the shared state.
func Get[T any](ctx *core.Context, key string) (T, bool) {
	var zero T
	if ctx == nil {
		return zero, false
	}
	raw, ok := ctx.Get(key)
	if !ok || raw == nil {
		return zero, false
	}
	value, ok := raw.(T)
	if !ok {
		return zero, false
	}
	return value, true
}

// GetFrom retrieves a raw value from any read-only state getter.
func GetFrom(ctx Getter, key string) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	return ctx.Get(key)
}

// GetAny retrieves a raw value from the shared state.
func GetAny(ctx *core.Context, key string) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	return ctx.Get(key)
}

// Set stores a strongly typed value in the shared state.
func Set[T any](ctx *core.Context, key string, value T) {
	if ctx == nil {
		return
	}
	ctx.Set(key, value)
}

// SetAny stores a raw value in the shared state.
func SetAny(ctx *core.Context, key string, value any) {
	if ctx == nil {
		return
	}
	ctx.Set(key, value)
}

// GetString retrieves a string value from the shared state.
func GetString(ctx *core.Context, key string) string {
	if ctx == nil {
		return ""
	}
	if raw, ok := ctx.Get(key); ok && raw != nil {
		if s, ok := raw.(string); ok {
			return s
		}
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
}

// GetBool retrieves a bool value from the shared state.
func GetBool(ctx *core.Context, key string) (bool, bool) {
	if ctx == nil {
		return false, false
	}
	if raw, ok := ctx.Get(key); ok && raw != nil {
		if b, ok := raw.(bool); ok {
			return b, true
		}
	}
	return false, false
}

// HasFrom reports whether the key exists in any read-only state getter.
func HasFrom(ctx Getter, key string) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Get(key)
	return ok
}

// Has reports whether the key exists in the shared state.
func Has(ctx *core.Context, key string) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Get(key)
	return ok
}
