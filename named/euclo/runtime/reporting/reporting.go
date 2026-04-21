package reporting

import (
	frameworkcore "codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/core"
	runtimepkg "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

type RuntimeExecutionStatus = runtimepkg.RuntimeExecutionStatus

var BuildRuntimeExecutionStatus = runtimepkg.BuildRuntimeExecutionStatus

func FinalReportFromState(state *frameworkcore.Context) map[string]any {
	artifacts := core.CollectArtifactsFromState(state)
	return core.AssembleFinalReport(artifacts)
}
