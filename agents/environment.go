package agents

import (
	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/model"
)

// AgentEnvironment is the shared agent dependency container.
// It contains the common fields that all agent paradigms need.
// This is a subset of WorkspaceEnvironment for use by generic agents.
type AgentEnvironment struct {
	Config         *execution.Config
	Model          model.LanguageModel
	Registry       *capability.CapabilityRegistry
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
