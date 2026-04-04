package agents

import (
	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/agentenv"
)

// WorkspaceEnvironment is the composition-root-supplied environment produced by
// ayenitd.Open(). Use this in new code; it is the superset of AgentEnvironment.
type WorkspaceEnvironment = ayenitd.WorkspaceEnvironment

// AgentEnvironment re-exports the shared agent dependency container.
// Deprecated: Use WorkspaceEnvironment in new code.
type AgentEnvironment = agentenv.AgentEnvironment
