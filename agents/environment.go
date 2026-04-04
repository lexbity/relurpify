package agents

import (
	"github.com/lexcodex/relurpify/anitd"
	"github.com/lexcodex/relurpify/framework/agentenv"
)

// WorkspaceEnvironment is the new composition-root-supplied environment.
// Use this in new code. AgentEnvironment is kept for compatibility.
type WorkspaceEnvironment = anitd.WorkspaceEnvironment

// AgentEnvironment re-exports the shared agent dependency container.
// Deprecated: Use WorkspaceEnvironment instead.
type AgentEnvironment = agentenv.AgentEnvironment
