package contracts

// ToolRegistry describes the minimal lookup surface for platform tool
// resolution. Implementations are expected to be deterministic and backed by
// loaded configuration data rather than ambient process state.
type ToolRegistry interface {
	LookupTool(name string) (ToolManifest, bool)
	ListTools() []ToolManifest
}
