package context

import (
	frameworkcore "codeburg.org/lexbit/relurpify/framework/core"
	runtimepkg "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclorestore "codeburg.org/lexbit/relurpify/named/euclo/runtime/restore"
)

type ContextPolicySummary = runtimepkg.ContextPolicySummary
type ContextRuntimeState = runtimepkg.ContextRuntimeState
type ContextLifecycleStage = runtimepkg.ContextLifecycleStage
type ContextLifecycleState = runtimepkg.ContextLifecycleState
type RuntimeSurfaces = runtimepkg.RuntimeSurfaces
type ActionLogEntry = runtimepkg.ActionLogEntry
type ProofSurface = runtimepkg.ProofSurface

var ResolveRuntimeSurfaces = euclorestore.ResolveRuntimeSurfaces
var BuildContextLifecycleState = runtimepkg.BuildContextLifecycleState

func ApplyEditIntentArtifacts(ctx *frameworkcore.Context, state *frameworkcore.Context) {
	if ctx == nil || state == nil {
		return
	}
	for _, key := range []string{
		"pipeline.code",
		"euclo.edit_execution",
	} {
		if raw, ok := ctx.Get(key); ok && raw != nil {
			state.Set(key, raw)
		}
	}
}
