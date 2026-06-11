package ports

// InvocationState is a governance-owned pass-through interface for delegation
// capability invocation. governance never reads state directly — it simply
// passes the value from the caller through to the capability registry adapter,
// which type-asserts back to capability/ports.State.
type InvocationState any
