package contracts

import (
	"fmt"
	"sort"
	"sync"
)

// NativeToolConstructor builds a go_native tool instance for a given workspace
// base path. The returned Tool must be fully initialized and ready to execute.
type NativeToolConstructor func(basePath string) Tool

type nativeRegistry struct {
	mu   sync.RWMutex
	ctor map[string]NativeToolConstructor
}

var nreg = &nativeRegistry{
	ctor: make(map[string]NativeToolConstructor),
}

// RegisterNative registers a go_native tool constructor under the given key.
// The key is normalized via NormalizeToolName. Panics on duplicate or empty key.
func RegisterNative(key string, ctor NativeToolConstructor) {
	norm := NormalizeToolName(key)
	if norm == "" {
		panic(fmt.Sprintf("contracts.RegisterNative: empty key %q", key))
	}
	if ctor == nil {
		panic(fmt.Sprintf("contracts.RegisterNative: nil constructor for key %q", key))
	}

	nreg.mu.Lock()
	defer nreg.mu.Unlock()

	if _, dup := nreg.ctor[norm]; dup {
		panic(fmt.Sprintf("contracts.RegisterNative: duplicate key %q", norm))
	}
	nreg.ctor[norm] = ctor
}

// LookupNative retrieves a previously registered constructor by normalized key.
// Returns nil, false when the key is unknown.
func LookupNative(key string) (NativeToolConstructor, bool) {
	norm := NormalizeToolName(key)
	if norm == "" {
		return nil, false
	}

	nreg.mu.RLock()
	defer nreg.mu.RUnlock()

	ctor, ok := nreg.ctor[norm]
	return ctor, ok
}

// NativeKeys returns all registered native tool keys in sorted order.
func NativeKeys() []string {
	nreg.mu.RLock()
	defer nreg.mu.RUnlock()

	out := make([]string, 0, len(nreg.ctor))
	for k := range nreg.ctor {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ResetNativeRegistry clears all registered native constructors. Intended for
// use in tests only.
func ResetNativeRegistry() {
	nreg.mu.Lock()
	defer nreg.mu.Unlock()

	clear(nreg.ctor)
}
