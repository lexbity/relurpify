package llm

import "codeburg.org/lexbit/relurpify/model"

// BackendClass is re-exported from platform/contracts.
type BackendClass = model.BackendClass

const (
	BackendClassTransport = model.BackendClassTransport
	BackendClassNative    = model.BackendClassNative
)

// Note: BackendCapabilities is declared in backend.go to avoid duplication
