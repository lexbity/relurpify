package ports

// InvocationState is a governance-owned pass-through interface for delegation
// capability invocation. governance never reads state directly — it simply
// passes the value from the caller through to the capability registry adapter,
// which type-asserts back to capability/ports.State.
type InvocationState interface {
	// governance-owned pass-through — no methods needed.
	// The concrete capability registry type-asserts to ports.State.
}
