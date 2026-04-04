package ayenitd

import (
	"io"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/policybundle"
)

// Workspace is a live, initialized workspace session. It holds all open
// resources. Close() must be called when the session ends.
type Workspace struct {
	Environment  WorkspaceEnvironment
	Registration *fauthorization.AgentRegistration

	// Internals held for Close()
	logFile   io.Closer
	eventLog  io.Closer
	patternDB io.Closer

	// Derived fields for callers that need them
	AgentSpec         *core.AgentRuntimeSpec
	AgentDefinitions  map[string]*core.AgentDefinition
	CompiledPolicy    *policybundle.CompiledPolicyBundle
	EffectiveContract *contractpkg.EffectiveAgentContract
}

// Close releases all resources held by the Workspace.
func (w *Workspace) Close() error {
	// TODO: Implement proper cleanup
	if w.logFile != nil {
		w.logFile.Close()
	}
	if w.eventLog != nil {
		w.eventLog.Close()
	}
	if w.patternDB != nil {
		w.patternDB.Close()
	}
	return nil
}
