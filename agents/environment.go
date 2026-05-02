package agents

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// AgentEnvironment is the shared agent dependency container.
// It contains the common fields that all agent paradigms need.
// This is a subset of WorkspaceEnvironment for use by generic agents.
type AgentEnvironment struct {
	Config         *core.Config
	Model          contracts.LanguageModel
	Registry       *capability.Registry
	Memory         *memory.WorkingMemoryStore
	IndexManager   *ast.IndexManager
	SearchEngine   *search.SearchEngine
	OutputIngester *knowledge.OutputIngester
	IngestOutputs  bool
}

// ToWorkspace converts an AgentEnvironment to a WorkspaceEnvironment.
// Fields not present in AgentEnvironment are left as zero/nil values.
func ToWorkspace(env AgentEnvironment) agentenv.WorkspaceEnvironment {
	return agentenv.WorkspaceEnvironment{
		Config:         env.Config,
		Model:          env.Model,
		Registry:       env.Registry,
		WorkingMemory:  env.Memory,
		IndexManager:   env.IndexManager,
		SearchEngine:   env.SearchEngine,
		OutputIngester: env.OutputIngester,
		IngestOutputs:  env.IngestOutputs,
	}
}
