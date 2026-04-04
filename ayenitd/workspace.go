package ayenitd

import (
	"io"
	"log"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capabilityplan"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/policybundle"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
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
	AgentSpec           *core.AgentRuntimeSpec
	AgentDefinitions    map[string]*core.AgentDefinition
	CompiledPolicy      *policybundle.CompiledPolicyBundle
	EffectiveContract   *contractpkg.EffectiveAgentContract
	CapabilityAdmissions []capabilityplan.AdmissionResult
	SkillResults        []frameworkskills.SkillResolution

	// Observability
	Telemetry core.Telemetry
	Logger    *log.Logger
}

// StealClosers transfers ownership of the raw io.Closer handles from this
// Workspace to the caller. After calling, Close() will no longer close those
// resources — the caller is responsible. This is used by app/relurpish/runtime
// so that Runtime.Close() manages the lifecycle directly without double-close.
func (w *Workspace) StealClosers() (logFile, patternDB, eventLog io.Closer) {
	logFile = w.logFile
	patternDB = w.patternDB
	eventLog = w.eventLog
	w.logFile = nil
	w.patternDB = nil
	w.eventLog = nil
	return
}

// Close releases all resources held by the Workspace.
func (w *Workspace) Close() error {
	if w.Environment.Scheduler != nil {
		w.Environment.Scheduler.Stop()
	}
	if c, ok := w.Environment.WorkflowStore.(io.Closer); ok {
		c.Close()
	}
	if w.patternDB != nil {
		w.patternDB.Close()
	}
	if w.eventLog != nil {
		w.eventLog.Close()
	}
	if w.logFile != nil {
		w.logFile.Close()
	}
	return nil
}
