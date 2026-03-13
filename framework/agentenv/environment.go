package agentenv

import (
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/search"
)

// AgentEnvironment bundles the shared runtime dependencies required by agent
// implementations. The container is shallow-copyable so callers can scope
// registry or memory access for child executions without rebuilding the world.
type AgentEnvironment struct {
	Model        core.LanguageModel
	Registry     *capability.Registry
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine
	Memory       memory.MemoryStore
	Config       *core.Config
}

// WithRegistry returns a shallow copy with Registry replaced.
func (e AgentEnvironment) WithRegistry(r *capability.Registry) AgentEnvironment {
	e.Registry = r
	return e
}

// WithMemory returns a shallow copy with Memory replaced.
func (e AgentEnvironment) WithMemory(m memory.MemoryStore) AgentEnvironment {
	e.Memory = m
	return e
}
