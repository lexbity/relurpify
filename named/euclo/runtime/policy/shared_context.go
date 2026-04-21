package policy

import (
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	runtimepkg "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

func BuildSharedContextRuntimeState(shared *core.SharedContext, work runtimepkg.UnitOfWork) runtimepkg.SharedContextRuntimeState {
	rt := runtimepkg.SharedContextRuntimeState{
		Enabled:        shared != nil,
		ExecutorFamily: work.ExecutorDescriptor.Family,
		BehaviorFamily: work.BehaviorFamily,
		UpdatedAt:      time.Now().UTC(),
	}
	participants := []string{
		"executor:" + string(work.ExecutorDescriptor.Family),
		"behavior:" + strings.TrimSpace(work.BehaviorFamily),
	}
	for _, routine := range work.RoutineBindings {
		if id := strings.TrimSpace(routine.Family); id != "" {
			participants = append(participants, "routine:"+id)
		}
	}
	for _, skill := range work.SkillBindings {
		if id := strings.TrimSpace(skill.SkillID); id != "" {
			participants = append(participants, "skill:"+id)
		}
	}
	rt.Participants = uniqueStrings(participants)
	if shared == nil {
		return rt
	}
	for _, ref := range shared.WorkingSetReferences() {
		key := strings.TrimSpace(ref.ID)
		if key == "" {
			key = strings.TrimSpace(ref.URI)
		}
		if key != "" {
			rt.WorkingSetRefs = append(rt.WorkingSetRefs, key)
		}
	}
	mutations := shared.RecentMutations(12)
	rt.RecentMutationCount = len(mutations)
	for _, mutation := range mutations {
		if key := strings.TrimSpace(mutation.Key); key != "" {
			rt.RecentMutationKeys = append(rt.RecentMutationKeys, key)
		}
	}
	rt.WorkingSetRefs = uniqueStrings(rt.WorkingSetRefs)
	rt.RecentMutationKeys = uniqueStrings(rt.RecentMutationKeys)
	return rt
}
