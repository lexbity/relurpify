// Package paradigm defines the narrow runtime dependencies shared by
// cognitionzoo agent paradigms.
package paradigm

import (
	"time"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// Deps is the runtime surface used by generic cognitionzoo paradigms.
// App composition adapts broader workspace state into this value.
type Deps struct {
	Config   *execution.Config
	Model    model.LanguageModel
	Registry *registry.CapabilityRegistry
	// PermissionChecker is the capability gate shared by governed paradigms
	// (ReWOO refuses to execute without one). Sourced from the same
	// authorization bundle that backs the registry's permission manager.
	PermissionChecker permissions.CapabilityChecker
	CommandRunner     sandbox.CommandRunner
	CommandPolicy     sandbox.CommandPolicy
	WorkingMemory     *memory.WorkingMemoryStore
	IndexManager      *ast.IndexManager
	SearchEngine      *search.SearchEngine
	StreamTrigger     *contextstream.Trigger
	OutputIngester    *knowledge.OutputIngester
	IngestOutputs     bool
	// Grounder is the capture-as-bridge write path. Graph runs wire it as their
	// epoch grounder so recipe captures land at the epoch barrier.
	Grounder *knowledge.GroundingService
	// EpochDrain is the invalidation subscriber's bounded drain the epoch
	// barrier calls.
	EpochDrain     func(time.Duration)
	PromptRegistry prompt.Registry
	AgentLifecycle agentlifecycle.Repository
	Telemetry      telemetry.Telemetry
}
