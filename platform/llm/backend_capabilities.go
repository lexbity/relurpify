package llm

import "codeburg.org/lexbit/relurpify/platform/contracts"

// BackendClass is re-exported from platform/contracts.
type BackendClass = contracts.BackendClass

const (
	BackendClassTransport = contracts.BackendClassTransport
	BackendClassNative    = contracts.BackendClassNative
)

// Note: BackendCapabilities is declared in backend.go to avoid duplication
