package contextdata

// TypedOverlay provides compile-time type safety for a single working-memory key.
//
// Create one as a package-level variable and reuse it across callers:
//
//	var routeSelectionOverlay = contextdata.NewTypedOverlay[*orchestrate.RouteSelection]("euclo.route_selection")
//	routeSelectionOverlay.Set(env, selection)
//	sel, ok := routeSelectionOverlay.Get(env)
//
// It is a value type; copying it is safe and free.
type TypedOverlay[T any] struct {
	key   string
	class MemoryClass
}

// NewTypedOverlay returns a TypedOverlay that stores task-scoped values under key.
func NewTypedOverlay[T any](key string) TypedOverlay[T] {
	return TypedOverlay[T]{key: key, class: MemoryClassTask}
}

// NewTypedOverlayWithClass returns a TypedOverlay that stores values with the given memory class.
func NewTypedOverlayWithClass[T any](key string, class MemoryClass) TypedOverlay[T] {
	return TypedOverlay[T]{key: key, class: class}
}

// Get retrieves the typed value from env.
// Returns (zero, false) if env is nil, the key is absent, or the stored value
// cannot be asserted to T.
func (o TypedOverlay[T]) Get(env *Envelope) (T, bool) {
	var zero T
	if env == nil {
		return zero, false
	}
	v, ok := env.GetWorkingValue(o.key)
	if !ok {
		return zero, false
	}
	typed, ok := v.(T)
	return typed, ok
}

// Set stores val into env under this overlay's key and memory class.
// A nil env is a no-op.
func (o TypedOverlay[T]) Set(env *Envelope, val T) {
	if env == nil {
		return
	}
	env.SetWorkingValue(o.key, val, o.class)
}

// Key returns the string key this overlay manages.
func (o TypedOverlay[T]) Key() string { return o.key }

// Class returns the memory class this overlay uses when writing.
func (o TypedOverlay[T]) Class() MemoryClass { return o.class }

// SetTyped stores a task-scoped value under key with a compile-time type constraint.
//
// Prefer TypedOverlay for repeated use of the same key. Use SetTyped for
// one-off writes where creating a named overlay variable is not warranted.
func SetTyped[T any](env *Envelope, key string, val T) {
	if env == nil {
		return
	}
	env.SetWorkingValue(key, val, MemoryClassTask)
}

// GetTyped retrieves a value from env and asserts it to type T.
// Returns (zero, false) if env is nil, the key is absent, or the stored type
// does not match T.
//
// Prefer TypedOverlay for repeated use of the same key. Use GetTyped for
// one-off reads.
func GetTyped[T any](env *Envelope, key string) (T, bool) {
	var zero T
	if env == nil {
		return zero, false
	}
	v, ok := env.GetWorkingValue(key)
	if !ok {
		return zero, false
	}
	typed, ok := v.(T)
	return typed, ok
}
