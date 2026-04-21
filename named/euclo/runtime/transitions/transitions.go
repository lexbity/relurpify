package transitions

import (
	"time"

	runtimepkg "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

type UnitOfWorkTransitionState = runtimepkg.UnitOfWorkTransitionState
type UnitOfWorkHistoryEntry = runtimepkg.UnitOfWorkHistoryEntry
type UnitOfWork = runtimepkg.UnitOfWork
type ExecutionDescriptor = runtimepkg.ExecutionDescriptor

var ApplyUnitOfWorkTransition = runtimepkg.ApplyUnitOfWorkTransition

func Apply(existing UnitOfWork, next *UnitOfWork, now time.Time) UnitOfWorkTransitionState {
	return runtimepkg.ApplyUnitOfWorkTransition(existing, next, now)
}
