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
// The key is normalized via NormalizeToolName. Panics on duplicate or empty key.
func RegisterNative(key string, ctor NativeToolConstructor) {
	norm := normalizeToolName(key)
	if norm == "" {
		panic(fmt.Sprintf("ports.RegisterNative: empty key %q", key))
	}
	if ctor == nil {
		panic(fmt.Sprintf("ports.RegisterNative: nil constructor for key %q", key))
	}

	nreg.mu.Lock()
	defer nreg.mu.Unlock()

	if _, dup := nreg.ctor[norm]; dup {
		panic(fmt.Sprintf("ports.RegisterNative: duplicate key %q", norm))
	}
	nreg.ctor[norm] = ctor
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
