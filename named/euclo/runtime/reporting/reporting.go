package reporting

import (
	frameworkcore "github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/core"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type RuntimeExecutionStatus = runtimepkg.RuntimeExecutionStatus

var BuildRuntimeExecutionStatus = runtimepkg.BuildRuntimeExecutionStatus

func FinalReportFromState(state *frameworkcore.Context) map[string]any {
	artifacts := core.CollectArtifactsFromState(state)
	return core.AssembleFinalReport(artifacts)
}
