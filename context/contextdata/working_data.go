package contextdata

import (
	"fmt"
	"time"
)

// SetWorkingValue stores a value in working memory.
//
// Deprecated: use SetTyped or TypedOverlay instead.
func (e *Envelope) SetWorkingValue(key string, value any, class MemoryClass) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.WorkingData == nil {
		e.WorkingData = make(map[string]any)
	}

	now := time.Now().UTC()
	e.WorkingData[key] = value

	found := false
	for i, ref := range e.References.WorkingMemory {
		if ref.TaskID == e.TaskID && ref.Key == key {
			e.References.WorkingMemory[i].UpdatedAt = now
			e.References.WorkingMemory[i].Class = class
			found = true
			break
		}
	}

	if !found {
		e.References.WorkingMemory = append(e.References.WorkingMemory, WorkingMemoryReference{
			TaskID:    e.TaskID,
			Key:       key,
			Class:     class,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
}

// GetWorkingValue retrieves a value from working memory.
//
// Deprecated: use GetTyped or TypedOverlay instead.
func (e *Envelope) GetWorkingValue(key string) (any, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.WorkingData == nil {
		return nil, false
	}
	val, ok := e.WorkingData[key]
	return val, ok
}

// DeleteWorkingValue removes a value from working memory.
func (e *Envelope) DeleteWorkingValue(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.WorkingData == nil {
		return
	}
	delete(e.WorkingData, key)

	newRefs := make([]WorkingMemoryReference, 0, len(e.References.WorkingMemory))
	for _, ref := range e.References.WorkingMemory {
		if !(ref.TaskID == e.TaskID && ref.Key == key) {
			newRefs = append(newRefs, ref)
		}
	}
	e.References.WorkingMemory = newRefs
}

// ClearWorkingData removes all working memory entries for this envelope's task.
func (e *Envelope) ClearWorkingData() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.WorkingData == nil {
		return
	}
	keysToDelete := make([]string, 0)
	for _, ref := range e.References.WorkingMemory {
		if ref.TaskID == e.TaskID {
			keysToDelete = append(keysToDelete, ref.Key)
		}
	}
	for _, key := range keysToDelete {
		delete(e.WorkingData, key)
	}
	newRefs := make([]WorkingMemoryReference, 0, len(e.References.WorkingMemory))
	for _, ref := range e.References.WorkingMemory {
		if ref.TaskID != e.TaskID {
			newRefs = append(newRefs, ref)
		}
	}
	e.References.WorkingMemory = newRefs
}

// WorkingMemoryKeys returns all keys in the working memory for this envelope's task.
func (e *Envelope) WorkingMemoryKeys() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	keys := make([]string, 0, len(e.References.WorkingMemory))
	for _, ref := range e.References.WorkingMemory {
		if ref.TaskID == e.TaskID {
			keys = append(keys, ref.Key)
		}
	}
	return keys
}

// WorkingDataSnapshot returns a point-in-time copy of working memory data.
func (e *Envelope) WorkingDataSnapshot() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.WorkingData == nil {
		return nil
	}
	out := make(map[string]any, len(e.WorkingData))
	for k, v := range e.WorkingData {
		out[k] = v
	}
	return out
}

// Snapshot returns a point-in-time copy of working memory data.
func (e *Envelope) Snapshot() map[string]any {
	return e.WorkingDataSnapshot()
}

// StringSliceFromContext extracts a string slice from working memory.
func (e *Envelope) StringSliceFromContext(key string) []string {
	val, _ := e.GetWorkingValue(key)
	if arr, ok := val.([]string); ok {
		return arr
	}
	if arr, ok := val.([]any); ok {
		result := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// SetHandleScoped stores a value with a scope identifier.
//
// Deprecated: use TypedOverlay with an explicit scoped key instead.
func (e *Envelope) SetHandleScoped(key string, value any, scope string) {
	scopedKey := fmt.Sprintf("%s:%s", scope, key)
	e.SetWorkingValue(scopedKey, value, MemoryClassTask)
}

// GetHandle retrieves a scoped value.
//
// Deprecated: use TypedOverlay with an explicit scoped key instead.
func (e *Envelope) GetHandle(key string) (any, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.WorkingData == nil {
		return nil, false
	}
	if val, ok := e.WorkingData[key]; ok {
		return val, ok
	}
	for i := len(e.References.WorkingMemory) - 1; i >= 0; i-- {
		ref := e.References.WorkingMemory[i]
		if ref.Key == key && ref.TaskID == e.TaskID {
			if val, ok := e.WorkingData[key]; ok {
				return val, ok
			}
		}
	}
	return nil, false
}
