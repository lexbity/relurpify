package ports

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
)

type nativeRegistry struct {
	mu   sync.RWMutex
	ctor map[string]NativeToolConstructor
}

var nreg = &nativeRegistry{
	ctor: make(map[string]NativeToolConstructor),
}

// RegisterNative registers a go_native tool constructor under the given key.
// The key is normalized via NormalizeToolName. It is intended for init()-time
// use. For explicit construction at package level, use DefaultNativeConstructors
// or NativeConstructorSet instead. Panics on duplicate or empty key.
func RegisterNative(key string, ctor NativeToolConstructor) {
	if err := registerNative(key, ctor); err != nil {
		panic(fmt.Sprintf("ports.RegisterNative: %v", err))
	}
}

// RegisterNativeNoPanic registers a native tool constructor and returns an error
// instead of panicking on duplicates. This is the safer alternative for use
// outside init().
func RegisterNativeNoPanic(key string, ctor NativeToolConstructor) error {
	return registerNative(key, ctor)
}

func registerNative(key string, ctor NativeToolConstructor) error {
	norm := normalizeToolName(key)
	if norm == "" {
		return fmt.Errorf("empty key %q", key)
	}
	if ctor == nil {
		return fmt.Errorf("nil constructor for key %q", key)
	}
	nreg.mu.Lock()
	defer nreg.mu.Unlock()
	if _, dup := nreg.ctor[norm]; dup {
		return fmt.Errorf("duplicate key %q", norm)
	}
	nreg.ctor[norm] = ctor
	return nil
}

// DefaultNativeConstructors returns a snapshot of all currently registered
// native tool constructors as an explicit map. Callers can copy or filter the
// returned map without holding the registry lock.
func DefaultNativeConstructors() map[string]NativeToolConstructor {
	nreg.mu.RLock()
	defer nreg.mu.RUnlock()
	out := make(map[string]NativeToolConstructor, len(nreg.ctor))
	for k, v := range nreg.ctor {
		out[k] = v
	}
	return out
}

// LookupNative retrieves a previously registered constructor by normalized key.
func LookupNative(key string) (NativeToolConstructor, bool) {
	norm := normalizeToolName(key)
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

// normalizeToolName is a local helper for key normalization.
func normalizeToolName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || r == '-' || r == '.' || r == '/' || unicode.IsSpace(r):
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	return strings.ReplaceAll(out, "__", "_")
}
