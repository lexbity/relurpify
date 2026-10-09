package contextdata

import (
	"time"
)

// SetWorkingValue stores a value in working memory without a class.
func (e *Envelope) SetWorkingValue(key string, value any) {
	e.SetWorkingValueWithClass(key, value, MemoryClassTask)
}

// SetWorkingValueWithClass stores a value in working memory with a memory
// class and the default llm origin.
func (e *Envelope) SetWorkingValueWithClass(key string, value any, class MemoryClass) {
	e.SetWorkingValueWithOrigin(key, value, class, OriginLLM)
}

// SetWorkingValueWithOrigin stores a value in working memory with a memory
// class and its dataflow origin class.
func (e *Envelope) SetWorkingValueWithOrigin(key string, value any, class MemoryClass, origin OriginClass) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.WorkingData == nil {
		e.WorkingData = make(map[string]any)
	}
	if e.Origins == nil {
		e.Origins = make(map[string]OriginClass)
	}

	now := time.Now().UTC()
	e.WorkingData[key] = value
	e.Origins[key] = origin

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

// NextSequence atomically reads, increments, and rewrites a per-key monotonic
// counter under the envelope lock (D15). The stored value is the next
// sequence (post-increment), so two concurrent callers can never observe the
// same value: the counter's read-increment-write is one critical section.
// A missing key starts at 1. Numeric compatibility: an existing int/int64/uint
// stored value is normalized to uint64, so code that previously maintained the
// counter via an unlocked read-modify-write migrates without interpretation
// drift.
func (e *Envelope) NextSequence(key string) uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	var current uint64
	switch v := e.WorkingData[key].(type) {
	case uint64:
		current = v
	case uint:
		current = uint64(v)
	case int:
		if v > 0 {
			current = uint64(v)
		}
	case int64:
		if v > 0 {
			current = uint64(v)
		}
	}
	current++

	if e.WorkingData == nil {
		e.WorkingData = make(map[string]any)
	}
	if e.Origins == nil {
		e.Origins = make(map[string]OriginClass)
	}
	e.WorkingData[key] = current
	e.Origins[key] = OriginLLM

	now := time.Now().UTC()
	found := false
	for i, ref := range e.References.WorkingMemory {
		if ref.TaskID == e.TaskID && ref.Key == key {
			e.References.WorkingMemory[i].UpdatedAt = now
			found = true
			break
		}
	}
	if !found {
		e.References.WorkingMemory = append(e.References.WorkingMemory, WorkingMemoryReference{
			TaskID:    e.TaskID,
			Key:       key,
			Class:     MemoryClassTask,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return current
}

// OriginOf returns the recorded origin class for a working-memory key,
// defaulting to the least provable (llm) class when none was recorded.
func (e *Envelope) OriginOf(key string) OriginClass {
	if e == nil {
		return OriginLLM
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if origin, ok := e.Origins[key]; ok {
		return origin
	}
	return OriginLLM
}

// OriginsSnapshot returns a copy of the working-memory origin map.
func (e *Envelope) OriginsSnapshot() map[string]OriginClass {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[string]OriginClass, len(e.Origins))
	for key, origin := range e.Origins {
		out[key] = origin
	}
	return out
}

// DeleteWorkingValue removes a value from working memory.
func (e *Envelope) DeleteWorkingValue(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.WorkingData == nil {
		return
	}
	delete(e.WorkingData, key)
	delete(e.Origins, key)

	newRefs := make([]WorkingMemoryReference, 0, len(e.References.WorkingMemory))
	for _, ref := range e.References.WorkingMemory {
		if ref.TaskID != e.TaskID || ref.Key != key {
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
		delete(e.Origins, key)
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
	val, _ := e.getWorkingValue(key)
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
