package core

// BackendClass identifies the broad backend family so framework code can make
// transport-vs-native decisions without importing platform-specific packages.
type BackendClass string

const (
	BackendClassTransport BackendClass = "transport"
	BackendClassNative    BackendClass = "native"
)

// BackendCapabilities describes the high-level features a backend exposes.
type BackendCapabilities struct {
	NativeToolCalling bool
	Streaming         bool
	Embeddings        bool
	ModelListing      bool
	BackendClass      BackendClass
}
