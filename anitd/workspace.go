package anitd

import (
	"io"

	"github.com/lexcodex/relurpify/framework/core"
)

// Workspace is a live, initialized workspace session. It holds all open
// resources. Close() must be called when the session ends.
type Workspace struct {
	Environment WorkspaceEnvironment
	Registration interface{} // *authorization.AgentRegistration (placeholder)

	// Internals held for Close()
	logFile     io.Closer
	eventLog    io.Closer
	patternDB   io.Closer

	// Derived fields for callers that need them
	AgentSpec         *core.AgentRuntimeSpec
	AgentDefinitions  map[string]*core.AgentDefinition
	CompiledPolicy    interface{} // *policybundle.CompiledPolicyBundle (placeholder)
	EffectiveContract interface{} // *contractpkg.EffectiveAgentContract (placeholder)
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
