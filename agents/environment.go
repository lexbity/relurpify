package agents

import (
	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/agentenv"
)

// WorkspaceEnvironment is the new composition-root-supplied environment.
// Use this in new code. AgentEnvironment is kept for compatibility.
type WorkspaceEnvironment = ayenitd.WorkspaceEnvironment

// AgentEnvironment re-exports the shared agent dependency container.
// Deprecated: Use WorkspaceEnvironment instead.
type AgentEnvironment = agentenv.AgentEnvironment
