package cfgload

import (
	"fmt"
	"sort"
	"sync"
)

var defaultSchemaKinds = []string{
	"workspace",
	"policy/sandbox",
	"policy/shell",
	"policy/localtool",
	"policy/ingestion",
	"model/provider",
	"model/profile",
	"tool",
	"agent",
	"skill",
}

// SchemaRegistry tracks supported schema kinds and versions.
type SchemaRegistry struct {
	mu      sync.RWMutex
	entries map[string]map[int]struct{}
}

// NewSchemaRegistry returns the phase-2 registry with the current v1 schema set.
func NewSchemaRegistry() *SchemaRegistry {
	reg := &SchemaRegistry{
		entries: make(map[string]map[int]struct{}, len(defaultSchemaKinds)),
	}
	for _, kind := range defaultSchemaKinds {
		_ = reg.Register(kind, 1)
	}
	return reg
}

// Register marks a schema kind/version pair as supported.
func (r *SchemaRegistry) Register(kind string, version int) error {
	if r == nil {
		return fmt.Errorf("schema registry required")
	}
	if kind == "" {
		return fmt.Errorf("schema kind required")
	}
	if version <= 0 {
		return fmt.Errorf("schema version must be positive")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string]map[int]struct{})
	}
	versions, ok := r.entries[kind]
	if !ok {
		versions = make(map[int]struct{})
		r.entries[kind] = versions
	}
	versions[version] = struct{}{}
	return nil
}

// Require validates a declaration against the registry.
func (r *SchemaRegistry) Require(decl SchemaDeclaration) error {
	if r == nil {
		r = NewSchemaRegistry()
	}
	r.mu.RLock()
	versions, ok := r.entries[decl.Kind]
	r.mu.RUnlock()
	if !ok {
		return &SchemaError{
			Line:   decl.Line,
			Schema: decl.String(),
			Err:    ErrUnknownSchema,
		}
	}
	if _, ok := versions[decl.Version]; !ok {
		return &SchemaError{
			Line:   decl.Line,
			Schema: decl.String(),
			Err:    ErrUnsupportedSchemaVersion,
		}
	}
	return nil
}

// KnownKinds returns the sorted set of schema kinds currently registered.
func (r *SchemaRegistry) KnownKinds() []string {
	if r == nil {
		r = NewSchemaRegistry()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.entries))
	for kind := range r.entries {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}
