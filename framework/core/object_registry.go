package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ObjectRegistry stores non-serializable runtime objects behind string handles.
type ObjectRegistry struct {
	mu     sync.RWMutex
	items  map[string]interface{}
	scopes map[string]map[string]struct{}
}

// NewObjectRegistry builds an empty registry.
func NewObjectRegistry() *ObjectRegistry {
	return &ObjectRegistry{
		items:  make(map[string]interface{}),
		scopes: make(map[string]map[string]struct{}),
	}
}

// Register stores an object and returns its handle.
func (r *ObjectRegistry) Register(value interface{}) string {
	if r == nil {
		return ""
	}
	handle := newRegistryHandle()
	r.mu.Lock()
	r.items[handle] = value
	r.mu.Unlock()
	return handle
}

// RegisterScoped stores an object and associates it with a scope for cleanup.
func (r *ObjectRegistry) RegisterScoped(scope string, value interface{}) string {
	if r == nil {
		return ""
	}
	handle := newRegistryHandle()
	r.mu.Lock()
	r.items[handle] = value
	if scope != "" {
		if _, ok := r.scopes[scope]; !ok {
			r.scopes[scope] = make(map[string]struct{})
		}
		r.scopes[scope][handle] = struct{}{}
	}
	r.mu.Unlock()
	return handle
}

// Lookup resolves a handle to the stored object.
func (r *ObjectRegistry) Lookup(handle string) (interface{}, bool) {
	if r == nil || handle == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.items[handle]
	return value, ok
}

// Remove deletes a stored object.
func (r *ObjectRegistry) Remove(handle string) {
	if r == nil || handle == "" {
		return
	}
	r.mu.Lock()
	delete(r.items, handle)
	for scope, handles := range r.scopes {
		if _, ok := handles[handle]; ok {
			delete(handles, handle)
			if len(handles) == 0 {
				delete(r.scopes, scope)
			}
			break
		}
	}
	r.mu.Unlock()
}

// ClearScope removes every object associated with the scope.
func (r *ObjectRegistry) ClearScope(scope string) {
	if r == nil || scope == "" {
		return
	}
	r.mu.Lock()
	handles := r.scopes[scope]
	delete(r.scopes, scope)
	for handle := range handles {
		delete(r.items, handle)
	}
	r.mu.Unlock()
}

func newRegistryHandle() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
